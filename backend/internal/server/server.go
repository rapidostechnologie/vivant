// Package server expose l'API HTTP de Vivant : inscription vérifiée, API
// du téléphone et fournisseur d'identité OpenID Connect.
package server

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"time"

	"vivant/internal/jwt"
	"vivant/internal/notify"
	"vivant/internal/registry"
	"vivant/internal/store"
	"vivant/internal/verification"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

const maxBodyBytes = 8 << 20 // les selfies sont envoyés en base64

// Client est un service autorisé à utiliser Vivant (fournisseur de service).
type Client struct {
	ID           string   `json:"client_id"`
	Name         string   `json:"name"`
	Secret       string   `json:"client_secret"`
	RedirectURIs []string `json:"redirect_uris"`
	Scopes       []string `json:"scopes"`
	secretHash   [32]byte
}

func (c *Client) checkSecret(secret string) bool {
	h := sha256.Sum256([]byte(secret))
	return subtle.ConstantTimeCompare(h[:], c.secretHash[:]) == 1
}

type Config struct {
	Issuer  string
	AppName string
	// DevMode renvoie les codes SMS dans l'API. Jamais en production.
	DevMode bool
	Clients []Client
	// PairwiseSecret sert à calculer un identifiant différent par service,
	// pour que les services ne puissent pas croiser leurs données.
	PairwiseSecret []byte
}

type Deps struct {
	Store    *store.Memory
	Registry registry.Registry
	Faces    verification.FaceVerifier
	SMS      notify.SMSSender
	Signer   *jwt.Signer
	Logger   *slog.Logger
	Now      func() time.Time
}

type Server struct {
	cfg Config
	Deps
	clients map[string]*Client
	tmpl    *template.Template
	handler http.Handler
}

func New(cfg Config, deps Deps) (*Server, error) {
	if cfg.Issuer == "" || len(cfg.PairwiseSecret) == 0 {
		return nil, errors.New("Issuer et PairwiseSecret sont obligatoires")
	}
	if deps.Store == nil || deps.Registry == nil || deps.Faces == nil || deps.SMS == nil || deps.Signer == nil {
		return nil, errors.New("dépendance manquante")
	}
	if cfg.AppName == "" {
		cfg.AppName = "Vivant"
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	s := &Server{cfg: cfg, Deps: deps, clients: map[string]*Client{}}
	for i := range cfg.Clients {
		c := cfg.Clients[i]
		if c.ID == "" || c.Secret == "" || len(c.RedirectURIs) == 0 {
			return nil, errors.New("client incomplet : client_id, client_secret et redirect_uris sont obligatoires")
		}
		c.secretHash = sha256.Sum256([]byte(c.Secret))
		c.Secret = ""
		s.clients[c.ID] = &c
	}
	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	s.tmpl = tmpl
	s.handler = s.middleware(s.routes())
	return s, nil
}

func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.FileServerFS(staticFS))
	mux.HandleFunc("GET /{$}", s.handleHome)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// OpenID Connect (navigateur et fournisseurs de service)
	mux.HandleFunc("GET /.well-known/openid-configuration", s.handleDiscovery)
	mux.HandleFunc("GET /oauth/jwks", s.handleJWKS)
	mux.HandleFunc("GET /oauth/authorize", s.handleAuthorize)
	mux.HandleFunc("POST /oauth/authorize", s.handleIdentify)
	mux.HandleFunc("GET /oauth/authorize/wait", s.handleWait)
	mux.HandleFunc("GET /oauth/authorize/status", s.handleAuthStatus)
	mux.HandleFunc("GET /oauth/authorize/complete", s.handleAuthComplete)
	mux.HandleFunc("POST /oauth/token", s.handleToken)
	mux.HandleFunc("GET /oauth/userinfo", s.handleUserInfo)
	mux.HandleFunc("POST /oauth/userinfo", s.handleUserInfo)

	mux.HandleFunc("GET /oauth/authorize/start", s.handleStart)

	// API simple des partenaires (clé secrète)
	mux.HandleFunc("POST /v1/verifications", s.handleCreateVerification)
	mux.HandleFunc("GET /v1/verifications/{id}", s.handleGetVerification)

	// Espace partenaires et documentation
	mux.HandleFunc("GET /docs", s.handleDocs)
	mux.HandleFunc("GET /partenaires", s.handlePartnerHome)
	mux.HandleFunc("GET /partenaires/inscription", s.handleSignupForm)
	mux.HandleFunc("POST /partenaires/inscription", s.handleSignup)
	mux.HandleFunc("GET /partenaires/connexion", s.handleLoginForm)
	mux.HandleFunc("POST /partenaires/connexion", s.handleLogin)
	mux.HandleFunc("POST /partenaires/deconnexion", s.handleLogout)
	mux.HandleFunc("GET /partenaires/tableau", s.partnerAuth(s.handleDashboard))
	mux.HandleFunc("POST /partenaires/cle", s.partnerAuth(s.handleRotateKey))
	mux.HandleFunc("POST /partenaires/retours", s.partnerAuth(s.handleAddRedirect))
	mux.HandleFunc("POST /partenaires/retours/supprimer", s.partnerAuth(s.handleRemoveRedirect))
	mux.HandleFunc("POST /partenaires/donnees", s.partnerAuth(s.handleScopes))
	mux.HandleFunc("POST /partenaires/production", s.partnerAuth(s.handleRequestLive))

	// Inscription (application mobile, avant que l'appareil ait une clé)
	mux.HandleFunc("POST /api/v1/enrollments", s.handleEnrollStart)
	mux.HandleFunc("POST /api/v1/enrollments/{id}/otp", s.handleEnrollOTP)
	mux.HandleFunc("POST /api/v1/enrollments/{id}/document", s.handleEnrollDocument)
	mux.HandleFunc("POST /api/v1/enrollments/{id}/selfie", s.handleEnrollSelfie)
	mux.HandleFunc("POST /api/v1/enrollments/{id}/complete", s.handleEnrollComplete)
	mux.HandleFunc("GET /api/v1/dev/enrollments/{id}/card", s.handleDevCard)

	// API de l'appareil (requêtes signées par la clé du téléphone)
	mux.HandleFunc("GET /api/v1/device/me", s.deviceAuth(s.handleDeviceMe))
	mux.HandleFunc("GET /api/v1/device/requests", s.deviceAuth(s.handleDeviceRequests))
	mux.HandleFunc("POST /api/v1/device/requests/{id}/approve", s.deviceAuth(s.handleApprove))
	mux.HandleFunc("POST /api/v1/device/requests/{id}/deny", s.deviceAuth(s.handleDeny))
	mux.HandleFunc("POST /api/v1/device/revoke", s.deviceAuth(s.handleRevoke))
	mux.HandleFunc("GET /api/v1/services", s.handleServices)
	return mux
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'")
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		if s.cfg.DevMode && strings.HasPrefix(r.URL.Path, "/api/") {
			// Développement uniquement : l'application Flutter tourne alors
			// dans un navigateur, sur une autre origine.
			h.Set("Access-Control-Allow-Origin", "*")
			h.Set("Access-Control-Allow-Headers", "Content-Type, X-Vivant-Device, X-Vivant-Timestamp, X-Vivant-Signature")
			h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.Logger.Info("http", "methode", r.Method, "chemin", r.URL.Path, "statut", rec.status,
			"duree", time.Since(start).Round(time.Millisecond))
	})
}

// --- Réponses JSON ---

type apiError struct {
	Status  int
	Code    string
	Message string
}

func (e *apiError) Error() string { return e.Code + " : " + e.Message }

func newAPIError(status int, code, message string) *apiError {
	return &apiError{Status: status, Code: code, Message: message}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) writeError(w http.ResponseWriter, err error) {
	var ae *apiError
	switch {
	case errors.As(err, &ae):
		writeJSON(w, ae.Status, map[string]string{"error": ae.Code, "message": ae.Message})
	case errors.Is(err, store.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "Ressource introuvable."})
	default:
		s.Logger.Error("erreur interne", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error", "message": "Erreur interne."})
	}
}

func decodeJSON(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return newAPIError(http.StatusBadRequest, "invalid_json", "Corps de requête JSON invalide.")
	}
	return nil
}

// --- Pages HTML ---

type pageData struct {
	AppName     string
	Title       string
	Message     string
	Error       string
	ClientName  string
	ReqID       string
	BindingCode string
	Claims      []string
}

func (s *Server) render(w http.ResponseWriter, status int, name string, data pageData) {
	data.AppName = s.cfg.AppName
	var buf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		s.Logger.Error("gabarit", "nom", name, "err", err)
		http.Error(w, "Erreur interne", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

func (s *Server) renderError(w http.ResponseWriter, status int, title, message string) {
	s.render(w, status, "error.html", pageData{Title: title, Message: message})
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusOK, "home.html", pageData{Title: "Accueil"})
}

// --- Utilitaires ---

func randomToken(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func randInt(max int) int {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		panic(err)
	}
	return int(v.Int64())
}

func randomDigits(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte('0' + randInt(10))
	}
	return string(b)
}

func hashToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

func (s *Server) audit(ev store.AuditEvent) {
	ev.At = s.Now()
	if err := s.Store.Audit(ev); err != nil {
		s.Logger.Error("audit", "err", err)
	}
}

// Données que les services peuvent demander, dans l'ordre d'affichage.
var scopeOrder = []string{"openid", "profile", "birth", "nin", "phone", "address"}

var scopeDescriptions = map[string]string{
	"openid":  "Un identifiant unique pour ce service",
	"profile": "Nom, prénoms, date de naissance et sexe",
	"birth":   "Lieu de naissance",
	"nin":     "Numéro d'identification national (NIN)",
	"phone":   "Numéro de téléphone vérifié",
	"address": "Adresse",
}

func describeScopes(scopes []string) []string {
	var out []string
	for _, sc := range scopeOrder {
		for _, want := range scopes {
			if want == sc {
				out = append(out, scopeDescriptions[sc])
			}
		}
	}
	return out
}
