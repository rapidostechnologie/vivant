package server

import (
	"encoding/hex"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"vivant/internal/store"
)

// API simple pour les partenaires, en deux appels :
//
//	POST /v1/verifications       crée une vérification et renvoie l'adresse
//	                             où envoyer la personne
//	GET  /v1/verifications/{id}  donne le résultat et l'identité vérifiée
//
// Authentification : en-tête « Authorization: Bearer sk_… » (clé secrète
// de l'espace partenaires). Cette API repose sur le même parcours que
// OpenID Connect ; les partenaires choisissent l'un ou l'autre.

// client renvoie un service autorisé : d'abord ceux de la configuration,
// puis les partenaires inscrits dans l'espace partenaires.
func (s *Server) client(id string) (*Client, bool) {
	if c, ok := s.clients[id]; ok {
		return c, true
	}
	p, ok := s.Store.PartnerByClientID(id)
	if !ok {
		return nil, false
	}
	return partnerClient(p), true
}

func partnerClient(p store.Partner) *Client {
	c := &Client{
		ID:           p.ClientID,
		Name:         p.OrgName,
		RedirectURIs: p.RedirectURIs,
		Scopes:       append([]string{"openid"}, p.Scopes...),
	}
	if b, err := hex.DecodeString(p.SecretHash); err == nil && len(b) == len(c.secretHash) {
		copy(c.secretHash[:], b)
	}
	return c
}

// countPartner met à jour les statistiques du partenaire, s'il en est un.
func (s *Server) countPartner(clientID string, fn func(*store.PartnerStats)) {
	p, ok := s.Store.PartnerByClientID(clientID)
	if !ok {
		return
	}
	if _, err := s.Store.UpdatePartner(p.ID, func(p *store.Partner) error {
		fn(&p.Stats)
		return nil
	}); err != nil {
		s.Logger.Error("statistiques partenaire", "err", err)
	}
}

func (s *Server) partnerFromKey(r *http.Request) (store.Partner, error) {
	key, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || strings.TrimSpace(key) == "" {
		return store.Partner{}, newAPIError(http.StatusUnauthorized, "missing_api_key",
			"Ajoutez l'en-tête « Authorization: Bearer <votre clé secrète> ».")
	}
	p, ok := s.Store.PartnerBySecretHash(hashToken(strings.TrimSpace(key)))
	if !ok {
		return store.Partner{}, newAPIError(http.StatusUnauthorized, "invalid_api_key", "Clé secrète invalide.")
	}
	return p, nil
}

// publicStatus traduit l'état interne pour l'API.
func (s *Server) publicStatus(ar store.AuthRequest) string {
	switch s.effectiveStatus(ar) {
	case store.AuthApproved:
		return "approved"
	case store.AuthDenied:
		return "denied"
	case store.AuthExpired:
		return "expired"
	default:
		return "pending"
	}
}

func (s *Server) handleCreateVerification(w http.ResponseWriter, r *http.Request) {
	p, err := s.partnerFromKey(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	var req struct {
		RedirectURL string   `json:"redirect_url"`
		Data        []string `json:"data"`
		Reference   string   `json:"reference"`
	}
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	if !slices.Contains(p.RedirectURIs, req.RedirectURL) {
		s.writeError(w, newAPIError(http.StatusBadRequest, "invalid_redirect_url",
			"Cette adresse de retour n'est pas enregistrée. Ajoutez-la dans votre espace partenaires."))
		return
	}
	if len(req.Data) == 0 {
		req.Data = []string{"profile"}
	}
	for _, d := range req.Data {
		if _, known := scopeDescriptions[d]; !known || d == "openid" {
			s.writeError(w, newAPIError(http.StatusBadRequest, "unknown_data", "Donnée inconnue : "+d+"."))
			return
		}
		if !slices.Contains(p.Scopes, d) {
			s.writeError(w, newAPIError(http.StatusForbidden, "data_not_allowed",
				"La donnée « "+d+" » n'est pas autorisée pour votre compte. Activez-la dans votre espace partenaires."))
			return
		}
	}
	if len(req.Reference) > 128 {
		s.writeError(w, newAPIError(http.StatusBadRequest, "invalid_reference", "La référence dépasse 128 caractères."))
		return
	}

	now := s.Now()
	token := randomToken(24)
	ar := store.AuthRequest{
		ID:             "ver_" + randomToken(16),
		ClientID:       p.ClientID,
		RedirectURI:    req.RedirectURL,
		Scopes:         append([]string{"openid"}, req.Data...),
		Verification:   true,
		Reference:      req.Reference,
		StartTokenHash: hashToken(token),
		Status:         store.AuthPendingUser,
		CreatedAt:      now,
		ExpiresAt:      now.Add(authRequestTTL),
	}
	s.Store.CreateAuthRequest(ar)
	s.countPartner(p.ClientID, func(st *store.PartnerStats) { st.Requests++ })

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         ar.ID,
		"status":     "pending",
		"url":        s.cfg.Issuer + "/oauth/authorize/start?req=" + url.QueryEscape(ar.ID) + "&t=" + url.QueryEscape(token),
		"expires_at": ar.ExpiresAt,
	})
}

func (s *Server) handleGetVerification(w http.ResponseWriter, r *http.Request) {
	p, err := s.partnerFromKey(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	ar, ok := s.Store.GetAuthRequest(r.PathValue("id"))
	if !ok || !ar.Verification || ar.ClientID != p.ClientID {
		s.writeError(w, newAPIError(http.StatusNotFound, "not_found", "Vérification introuvable ou expirée."))
		return
	}
	status := s.publicStatus(ar)
	resp := map[string]any{
		"id":         ar.ID,
		"status":     status,
		"created_at": ar.CreatedAt,
	}
	if ar.Reference != "" {
		resp["reference"] = ar.Reference
	}
	if !ar.CompletedAt.IsZero() {
		resp["completed_at"] = ar.CompletedAt
	}
	if status == "approved" {
		user, ok := s.Store.GetUser(ar.UserID)
		if !ok {
			s.writeError(w, newAPIError(http.StatusNotFound, "not_found", "Vérification introuvable ou expirée."))
			return
		}
		resp["identity"] = s.userClaims(user, ar.ClientID, ar.Scopes)
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleStart est l'adresse renvoyée par POST /v1/verifications : elle lie
// la vérification au navigateur de la personne (lien à usage unique).
func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ar, ok := s.Store.GetAuthRequest(q.Get("req"))
	if !ok || !ar.Verification || ar.StartTokenHash == "" || ar.StartTokenHash != hashToken(q.Get("t")) {
		s.renderError(w, http.StatusBadRequest, "Lien invalide",
			"Ce lien de vérification n'est plus valable. Retournez sur le service et recommencez.")
		return
	}
	if s.effectiveStatus(ar) != store.AuthPendingUser {
		s.renderError(w, http.StatusBadRequest, "Lien expiré", "Ce lien de vérification a expiré. Retournez sur le service et recommencez.")
		return
	}
	secret := randomToken(24)
	err := s.Store.UpdateAuthRequest(ar.ID, func(a *store.AuthRequest) error {
		a.BrowserSecretHash = hashToken(secret)
		a.StartTokenHash = ""
		return nil
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.setBrowserCookie(w, ar.ID, secret, time.Until(ar.ExpiresAt))
	s.renderLogin(w, http.StatusOK, ar, "")
}
