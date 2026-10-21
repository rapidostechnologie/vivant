package server

import (
	"bytes"
	"crypto/pbkdf2"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"net/mail"
	"net/url"
	"slices"
	"strings"
	"time"

	"vivant/internal/store"
)

// Espace partenaires : inscription, clés API, adresses de retour, données
// autorisées et statistiques. La mise en production est validée par
// l'équipe Vivant après vérification de l'organisation.
const (
	partnerCookie     = "vivant_partenaire"
	partnerSessionTTL = 12 * time.Hour
	pbkdf2Iterations  = 210_000
	maxRedirectURIs   = 10
)

var partnerSectors = []string{"Banque", "Assurance", "Administration", "Santé", "Télécoms", "Commerce", "Autre"}

// Données qu'un partenaire peut demander (openid est toujours inclus).
var partnerScopes = []string{"profile", "birth", "nin", "phone", "address"}

type scopeOption struct {
	Name    string
	Label   string
	Checked bool
}

type partnerPage struct {
	Title     string
	Issuer    string
	Partner   *store.Partner
	NewSecret string
	Error     string
	Notice    string
	Form      map[string]string
	Sectors   []string
	Scopes    []scopeOption
}

func (s *Server) renderPartner(w http.ResponseWriter, status int, name string, data partnerPage) {
	data.Issuer = s.cfg.Issuer
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

func hashPassword(password string, salt []byte) (string, error) {
	key, err := pbkdf2.Key(sha256.New, password, salt, pbkdf2Iterations, 32)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(key), nil
}

func checkPassword(p store.Partner, password string) bool {
	salt, err := hex.DecodeString(p.PasswordSalt)
	if err != nil {
		return false
	}
	h, err := hashPassword(password, salt)
	return err == nil && subtle.ConstantTimeCompare([]byte(h), []byte(p.PasswordHash)) == 1
}

// sameOrigin bloque les formulaires envoyés depuis un autre site. Les
// navigateurs récents envoient Sec-Fetch-Site ; avec Referrer-Policy
// no-referrer, l'en-tête Origin vaut « null » et ne suffit pas.
func (s *Server) sameOrigin(r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
		return site == "same-origin" || site == "none"
	}
	o := r.Header.Get("Origin")
	return o == "" || o == s.cfg.Issuer
}

func (s *Server) startPartnerSession(w http.ResponseWriter, partnerID string) {
	token := randomToken(32)
	s.Store.CreatePartnerSession(hashToken(token), partnerID, s.Now().Add(partnerSessionTTL))
	http.SetCookie(w, &http.Cookie{
		Name:     partnerCookie,
		Value:    token,
		Path:     "/partenaires",
		MaxAge:   int(partnerSessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   strings.HasPrefix(s.cfg.Issuer, "https://"),
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) currentPartner(r *http.Request) (store.Partner, bool) {
	c, err := r.Cookie(partnerCookie)
	if err != nil {
		return store.Partner{}, false
	}
	id, ok := s.Store.PartnerSession(hashToken(c.Value), s.Now())
	if !ok {
		return store.Partner{}, false
	}
	return s.Store.GetPartner(id)
}

type partnerHandler func(w http.ResponseWriter, r *http.Request, p store.Partner)

func (s *Server) partnerAuth(h partnerHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := s.currentPartner(r)
		if !ok {
			http.Redirect(w, r, "/partenaires/connexion", http.StatusSeeOther)
			return
		}
		if r.Method == http.MethodPost && !s.sameOrigin(r) {
			http.Error(w, "Origine refusée", http.StatusForbidden)
			return
		}
		h(w, r, p)
	}
}

// newSecret génère une clé secrète et son empreinte.
func newSecret(mode string) (secret, hash, hint string) {
	prefix := "sk_test_"
	if mode == store.PartnerLive {
		prefix = "sk_live_"
	}
	secret = prefix + randomToken(24)
	return secret, hashToken(secret), secret[len(secret)-4:]
}

// --- Pages publiques ---

func (s *Server) handlePartnerHome(w http.ResponseWriter, r *http.Request) {
	data := partnerPage{Title: "Partenaires"}
	if p, ok := s.currentPartner(r); ok {
		data.Partner = &p
	}
	s.renderPartner(w, http.StatusOK, "partner_home.html", data)
}

func (s *Server) handleDocs(w http.ResponseWriter, r *http.Request) {
	s.renderPartner(w, http.StatusOK, "docs.html", partnerPage{Title: "Documentation"})
}

func (s *Server) handleSignupForm(w http.ResponseWriter, r *http.Request) {
	s.renderPartner(w, http.StatusOK, "partner_signup.html", partnerPage{Title: "Créer un compte", Sectors: partnerSectors, Form: map[string]string{}})
}

func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	if !s.sameOrigin(r) {
		http.Error(w, "Origine refusée", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulaire invalide", http.StatusBadRequest)
		return
	}
	form := map[string]string{
		"org":     strings.TrimSpace(r.PostForm.Get("org")),
		"sector":  r.PostForm.Get("sector"),
		"website": strings.TrimSpace(r.PostForm.Get("website")),
		"email":   strings.TrimSpace(r.PostForm.Get("email")),
	}
	password := r.PostForm.Get("password")
	fail := func(msg string) {
		s.renderPartner(w, http.StatusBadRequest, "partner_signup.html",
			partnerPage{Title: "Créer un compte", Sectors: partnerSectors, Form: form, Error: msg})
	}
	switch {
	case form["org"] == "" || len(form["org"]) > 120:
		fail("Indiquez le nom de votre organisation.")
		return
	case !slices.Contains(partnerSectors, form["sector"]):
		fail("Choisissez un secteur.")
		return
	case !validEmail(form["email"]):
		fail("Adresse e-mail invalide.")
		return
	case len(password) < 10:
		fail("Le mot de passe doit contenir au moins 10 caractères.")
		return
	}

	salt := []byte(randomToken(16))
	pwHash, err := hashPassword(password, salt)
	if err != nil {
		s.writeError(w, err)
		return
	}
	secret, secretHash, hint := newSecret(store.PartnerTest)
	p := store.Partner{
		ID:           "par_" + randomToken(12),
		OrgName:      form["org"],
		Sector:       form["sector"],
		Website:      form["website"],
		Email:        strings.ToLower(form["email"]),
		PasswordHash: pwHash,
		PasswordSalt: hex.EncodeToString(salt),
		ClientID:     "pk_test_" + randomToken(12),
		SecretHash:   secretHash,
		SecretHint:   hint,
		RedirectURIs: []string{"http://localhost:3000/retour"},
		Scopes:       []string{"profile"},
		Mode:         store.PartnerTest,
		CreatedAt:    s.Now(),
	}
	if err := s.Store.CreatePartner(p); err != nil {
		if errors.Is(err, store.ErrEmailTaken) {
			fail("Un compte existe déjà avec cette adresse e-mail.")
			return
		}
		s.writeError(w, err)
		return
	}
	s.Logger.Info("nouveau partenaire", "organisation", p.OrgName, "secteur", p.Sector)
	s.startPartnerSession(w, p.ID)
	s.renderDashboard(w, p, secret, "Votre compte est prêt. Copiez votre clé secrète maintenant : elle ne sera plus affichée.", "")
}

func validEmail(e string) bool {
	a, err := mail.ParseAddress(e)
	return err == nil && a.Address == e && strings.Contains(e, ".")
}

func (s *Server) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	s.renderPartner(w, http.StatusOK, "partner_login.html", partnerPage{Title: "Connexion", Form: map[string]string{}})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.sameOrigin(r) {
		http.Error(w, "Origine refusée", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulaire invalide", http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(r.PostForm.Get("email"))
	p, ok := s.Store.PartnerByEmail(email)
	if !ok || !checkPassword(p, r.PostForm.Get("password")) {
		s.renderPartner(w, http.StatusUnauthorized, "partner_login.html", partnerPage{
			Title: "Connexion", Form: map[string]string{"email": email}, Error: "E-mail ou mot de passe incorrect."})
		return
	}
	s.startPartnerSession(w, p.ID)
	http.Redirect(w, r, "/partenaires/tableau", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(partnerCookie); err == nil {
		s.Store.DeletePartnerSession(hashToken(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: partnerCookie, Path: "/partenaires", MaxAge: -1})
	http.Redirect(w, r, "/partenaires", http.StatusSeeOther)
}

// --- Tableau de bord ---

func (s *Server) renderDashboard(w http.ResponseWriter, p store.Partner, newSecret, notice, errMsg string) {
	var opts []scopeOption
	for _, sc := range partnerScopes {
		opts = append(opts, scopeOption{Name: sc, Label: scopeDescriptions[sc], Checked: slices.Contains(p.Scopes, sc)})
	}
	status := http.StatusOK
	if errMsg != "" {
		status = http.StatusBadRequest
	}
	s.renderPartner(w, status, "partner_dashboard.html", partnerPage{
		Title: "Tableau de bord", Partner: &p, NewSecret: newSecret, Notice: notice, Error: errMsg, Scopes: opts,
	})
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request, p store.Partner) {
	s.renderDashboard(w, p, "", "", "")
}

func (s *Server) handleRotateKey(w http.ResponseWriter, r *http.Request, p store.Partner) {
	var secret string
	p, err := s.Store.UpdatePartner(p.ID, func(p *store.Partner) error {
		var hash, hint string
		secret, hash, hint = newSecret(p.Mode)
		p.SecretHash, p.SecretHint = hash, hint
		return nil
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.renderDashboard(w, p, secret, "Nouvelle clé créée. L'ancienne ne fonctionne plus.", "")
}

func validRedirectURI(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Fragment != "" || u.User != nil {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	host := u.Hostname()
	return u.Scheme == "http" && (host == "localhost" || host == "127.0.0.1")
}

func (s *Server) handleAddRedirect(w http.ResponseWriter, r *http.Request, p store.Partner) {
	uri := strings.TrimSpace(r.FormValue("url"))
	if !validRedirectURI(uri) {
		s.renderDashboard(w, p, "", "", "Adresse invalide : utilisez https:// (ou http://localhost pour vos tests).")
		return
	}
	p, err := s.Store.UpdatePartner(p.ID, func(p *store.Partner) error {
		if slices.Contains(p.RedirectURIs, uri) {
			return nil
		}
		if len(p.RedirectURIs) >= maxRedirectURIs {
			return newAPIError(http.StatusBadRequest, "too_many", "")
		}
		p.RedirectURIs = append(p.RedirectURIs, uri)
		return nil
	})
	if err != nil {
		cur, _ := s.Store.GetPartner(p.ID)
		s.renderDashboard(w, cur, "", "", "Maximum 10 adresses de retour.")
		return
	}
	http.Redirect(w, r, "/partenaires/tableau#retours", http.StatusSeeOther)
}

func (s *Server) handleRemoveRedirect(w http.ResponseWriter, r *http.Request, p store.Partner) {
	uri := r.FormValue("url")
	if _, err := s.Store.UpdatePartner(p.ID, func(p *store.Partner) error {
		p.RedirectURIs = slices.DeleteFunc(p.RedirectURIs, func(u string) bool { return u == uri })
		return nil
	}); err != nil {
		s.writeError(w, err)
		return
	}
	http.Redirect(w, r, "/partenaires/tableau#retours", http.StatusSeeOther)
}

func (s *Server) handleScopes(w http.ResponseWriter, r *http.Request, p store.Partner) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulaire invalide", http.StatusBadRequest)
		return
	}
	var chosen []string
	for _, sc := range partnerScopes {
		if r.PostForm.Get(sc) == "on" {
			chosen = append(chosen, sc)
		}
	}
	if len(chosen) == 0 {
		s.renderDashboard(w, p, "", "", "Choisissez au moins une donnée.")
		return
	}
	if _, err := s.Store.UpdatePartner(p.ID, func(p *store.Partner) error {
		p.Scopes = chosen
		return nil
	}); err != nil {
		s.writeError(w, err)
		return
	}
	http.Redirect(w, r, "/partenaires/tableau#donnees", http.StatusSeeOther)
}

func (s *Server) handleRequestLive(w http.ResponseWriter, r *http.Request, p store.Partner) {
	p, err := s.Store.UpdatePartner(p.ID, func(p *store.Partner) error {
		if p.Mode == store.PartnerTest {
			p.Mode = store.PartnerReview
		}
		return nil
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.audit(store.AuditEvent{Type: "partner_review_requested", ClientID: p.ClientID, Detail: p.OrgName})
	s.renderDashboard(w, p, "", "Demande envoyée. Notre équipe vous contacte sous 48 h pour vérifier votre organisation.", "")
}
