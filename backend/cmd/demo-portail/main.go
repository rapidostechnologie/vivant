// Commande demo-portail : portail administratif FICTIF qui montre comment un
// service (comme un portail de démarches en ligne) intègre « Se connecter
// avec Vivant ».
//
// Variables d'environnement :
//
//	PORTAIL_ADDR          adresse d'écoute (défaut :8081)
//	PORTAIL_URL           URL publique du portail (défaut http://localhost:8081)
//	VIVANT_ISSUER         URL de Vivant (défaut http://localhost:8080)
//	PORTAIL_CLIENT_ID     défaut portail-demo
//	PORTAIL_CLIENT_SECRET défaut demo-secret-a-changer
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"vivant/internal/jwt"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

const sessionCookie = "portail_session"

type session struct {
	State     string
	Nonce     string
	Verifier  string
	Claims    map[string]any
	CreatedAt time.Time
}

type discovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
}

type portal struct {
	issuer       string
	baseURL      string
	clientID     string
	clientSecret string
	tmpl         *template.Template
	http         *http.Client
	logger       *slog.Logger

	mu       sync.Mutex
	sessions map[string]*session
	disc     *discovery
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		logger.Error("gabarits", "err", err)
		os.Exit(1)
	}
	p := &portal{
		issuer:       strings.TrimRight(env("VIVANT_ISSUER", "http://localhost:8080"), "/"),
		baseURL:      strings.TrimRight(env("PORTAIL_URL", "http://localhost:8081"), "/"),
		clientID:     env("PORTAIL_CLIENT_ID", "portail-demo"),
		clientSecret: env("PORTAIL_CLIENT_SECRET", "demo-secret-a-changer"),
		tmpl:         tmpl,
		http:         &http.Client{Timeout: 10 * time.Second},
		logger:       logger,
		sessions:     map[string]*session{},
	}

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.FileServerFS(staticFS))
	mux.HandleFunc("GET /{$}", p.handleHome)
	mux.HandleFunc("GET /login", p.handleLogin)
	mux.HandleFunc("GET /callback", p.handleCallback)
	mux.HandleFunc("GET /espace", p.handleEspace)
	mux.HandleFunc("POST /demande", p.handleDemande)
	mux.HandleFunc("POST /logout", p.handleLogout)

	addr := env("PORTAIL_ADDR", ":8081")
	logger.Info("Portail de démonstration démarré", "adresse", addr, "vivant", p.issuer)
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	if err := srv.ListenAndServe(); err != nil {
		logger.Error("arrêt", "err", err)
		os.Exit(1)
	}
}

func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// --- Sessions ---

func (p *portal) session(r *http.Request) (string, *session) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return "", nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.sessions[c.Value]
	if s == nil || time.Since(s.CreatedAt) > 2*time.Hour {
		return "", nil
	}
	return c.Value, s
}

func (p *portal) newSession(w http.ResponseWriter, s *session) {
	id := randomToken(32)
	s.CreatedAt = time.Now()
	p.mu.Lock()
	p.sessions[id] = s
	p.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: id, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: strings.HasPrefix(p.baseURL, "https://"),
	})
}

func (p *portal) dropSession(id string) {
	p.mu.Lock()
	delete(p.sessions, id)
	p.mu.Unlock()
}

// --- OpenID Connect ---

func (p *portal) discovery(ctx context.Context) (*discovery, error) {
	p.mu.Lock()
	d := p.disc
	p.mu.Unlock()
	if d != nil {
		return d, nil
	}
	var fresh discovery
	if err := p.getJSON(ctx, p.issuer+"/.well-known/openid-configuration", &fresh); err != nil {
		return nil, err
	}
	if fresh.Issuer != p.issuer {
		return nil, fmt.Errorf("issuer inattendu : %s", fresh.Issuer)
	}
	p.mu.Lock()
	p.disc = &fresh
	p.mu.Unlock()
	return &fresh, nil
}

func (p *portal) getJSON(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return err
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s : statut %d", u, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (p *portal) handleLogin(w http.ResponseWriter, r *http.Request) {
	d, err := p.discovery(r.Context())
	if err != nil {
		p.logger.Error("découverte OIDC", "err", err)
		p.renderError(w, "Vivant est injoignable pour le moment. Réessayez plus tard.")
		return
	}
	s := &session{State: randomToken(16), Nonce: randomToken(16), Verifier: randomToken(48)}
	p.newSession(w, s)
	sum := sha256.Sum256([]byte(s.Verifier))
	q := url.Values{
		"client_id":             {p.clientID},
		"redirect_uri":          {p.baseURL + "/callback"},
		"response_type":         {"code"},
		"scope":                 {"openid profile birth nin phone"},
		"state":                 {s.State},
		"nonce":                 {s.Nonce},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(sum[:])},
		"code_challenge_method": {"S256"},
	}
	http.Redirect(w, r, d.AuthorizationEndpoint+"?"+q.Encode(), http.StatusFound)
}

func (p *portal) handleCallback(w http.ResponseWriter, r *http.Request) {
	id, s := p.session(r)
	if s == nil || s.State == "" {
		p.renderError(w, "Votre session a expiré. Recommencez la connexion.")
		return
	}
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		p.dropSession(id)
		p.renderError(w, "Connexion non aboutie : "+q.Get("error_description"))
		return
	}
	if q.Get("state") != s.State {
		p.renderError(w, "Paramètre state invalide. Recommencez la connexion.")
		return
	}
	claims, err := p.exchange(r.Context(), q.Get("code"), s)
	if err != nil {
		p.logger.Error("échange du code", "err", err)
		p.renderError(w, "Impossible de vérifier votre identité. Recommencez la connexion.")
		return
	}
	// Nouvelle session après connexion (protection contre la fixation de session).
	p.dropSession(id)
	p.newSession(w, &session{Claims: claims})
	http.Redirect(w, r, "/espace", http.StatusSeeOther)
}

// exchange échange le code contre un id_token et vérifie ce dernier.
func (p *portal) exchange(ctx context.Context, code string, s *session) (map[string]any, error) {
	d, err := p.discovery(ctx)
	if err != nil {
		return nil, err
	}
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {p.baseURL + "/callback"},
		"code_verifier": {s.Verifier},
	}
	req, err := http.NewRequestWithContext(ctx, "POST", d.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(url.QueryEscape(p.clientID), url.QueryEscape(p.clientSecret))
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var tok struct {
		IDToken string `json:"id_token"`
		Error   string `json:"error"`
		Desc    string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jeton refusé : %s (%s)", tok.Error, tok.Desc)
	}

	var keys jwt.JWKSet
	if err := p.getJSON(ctx, d.JWKSURI, &keys); err != nil {
		return nil, err
	}
	claims, err := jwt.Verify(tok.IDToken, keys)
	if err != nil {
		return nil, err
	}
	exp, _ := claims["exp"].(float64)
	switch {
	case claims["iss"] != p.issuer:
		return nil, errors.New("iss invalide")
	case claims["aud"] != p.clientID:
		return nil, errors.New("aud invalide")
	case time.Now().Unix() > int64(exp):
		return nil, errors.New("jeton expiré")
	case claims["nonce"] != s.Nonce:
		return nil, errors.New("nonce invalide")
	}
	return claims, nil
}

// --- Pages ---

type identityView struct {
	FullName   string
	FamilyName string
	GivenName  string
	BirthDate  string
	BirthPlace string
	Gender     string
	NIN        string
	Phone      string
	Level      string
	AuthTime   string
	Subject    string
}

func viewOf(c map[string]any) identityView {
	str := func(k string) string { v, _ := c[k].(string); return v }
	v := identityView{
		FamilyName: str("family_name"),
		GivenName:  str("given_name"),
		NIN:        str("nin"),
		Phone:      str("phone_number"),
		Subject:    str("sub"),
		Gender:     map[string]string{"female": "Féminin", "male": "Masculin"}[str("gender")],
	}
	v.FullName = v.GivenName + " " + strings.ToUpper(v.FamilyName)
	if t, err := time.Parse(time.DateOnly, str("birthdate")); err == nil {
		v.BirthDate = t.Format("02/01/2006")
	}
	if pob, ok := c["place_of_birth"].(map[string]any); ok {
		v.BirthPlace, _ = pob["locality"].(string)
	}
	if str("acr") == "urn:vivant:loa:substantial" {
		v.Level = "Substantiel"
	}
	if at, ok := c["auth_time"].(float64); ok {
		v.AuthTime = time.Unix(int64(at), 0).Format("02/01/2006 à 15:04")
	}
	return v
}

type pageData struct {
	Title    string
	LoggedIn bool
	Message  string
	ID       identityView
	Ref      string
	Document string
}

func (p *portal) render(w http.ResponseWriter, status int, name string, data pageData) {
	var buf bytes.Buffer
	if err := p.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		p.logger.Error("gabarit", "nom", name, "err", err)
		http.Error(w, "Erreur interne", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

func (p *portal) renderError(w http.ResponseWriter, msg string) {
	p.render(w, http.StatusBadRequest, "erreur.html", pageData{Title: "Erreur", Message: msg})
}

func (p *portal) handleHome(w http.ResponseWriter, r *http.Request) {
	_, s := p.session(r)
	p.render(w, http.StatusOK, "accueil.html", pageData{Title: "Accueil", LoggedIn: s != nil && s.Claims != nil})
}

func (p *portal) handleEspace(w http.ResponseWriter, r *http.Request) {
	_, s := p.session(r)
	if s == nil || s.Claims == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	p.render(w, http.StatusOK, "espace.html", pageData{Title: "Mon espace", LoggedIn: true, ID: viewOf(s.Claims)})
}

var documents = map[string]string{
	"extrait_naissance":    "Extrait d'acte de naissance",
	"casier_judiciaire":    "Bulletin n°3 du casier judiciaire",
	"certificat_residence": "Certificat de résidence",
}

func (p *portal) handleDemande(w http.ResponseWriter, r *http.Request) {
	_, s := p.session(r)
	if s == nil || s.Claims == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	doc, ok := documents[r.FormValue("document")]
	if !ok {
		p.renderError(w, "Type de document inconnu.")
		return
	}
	n, _ := rand.Int(rand.Reader, big.NewInt(1_000_000))
	ref := fmt.Sprintf("DEM-%d-%06d", time.Now().Year(), n.Int64())
	p.render(w, http.StatusOK, "demande.html", pageData{
		Title: "Demande enregistrée", LoggedIn: true, ID: viewOf(s.Claims), Ref: ref, Document: doc,
	})
}

func (p *portal) handleLogout(w http.ResponseWriter, r *http.Request) {
	if id, _ := p.session(r); id != "" {
		p.dropSession(id)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
