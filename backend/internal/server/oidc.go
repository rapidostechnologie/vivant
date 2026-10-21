package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"vivant/internal/phone"
	"vivant/internal/store"
)

// Parcours de connexion OpenID Connect (flux « authorization code » + PKCE) :
//
//  1. Le service redirige le navigateur vers /oauth/authorize.
//  2. L'utilisateur saisit son numéro de téléphone ou son NIN.
//  3. Le navigateur affiche un code à 2 chiffres ; le téléphone reçoit la
//     demande, l'utilisateur choisit ce code, déverrouille sa clé (PIN ou
//     biométrie) et signe son accord.
//  4. Le navigateur est renvoyé vers le service avec un code d'autorisation,
//     que le service échange contre un id_token signé.
const (
	authRequestTTL      = 10 * time.Minute
	deviceApprovalTTL   = 3 * time.Minute
	codeTTL             = time.Minute
	accessTokenTTL      = 10 * time.Minute
	idTokenTTL          = 10 * time.Minute
	browserCookiePrefix = "vivant_ar_"
)

func (s *Server) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	iss := s.cfg.Issuer
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                iss,
		"authorization_endpoint":                iss + "/oauth/authorize",
		"token_endpoint":                        iss + "/oauth/token",
		"userinfo_endpoint":                     iss + "/oauth/userinfo",
		"jwks_uri":                              iss + "/oauth/jwks",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code"},
		"subject_types_supported":               []string{"pairwise"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post"},
		"code_challenge_methods_supported":      []string{"S256"},
		"scopes_supported":                      scopeOrder,
		"acr_values_supported":                  []string{loaSubstantial},
		"claims_supported": []string{"sub", "iss", "aud", "exp", "iat", "auth_time", "nonce", "acr", "amr",
			"family_name", "given_name", "name", "birthdate", "gender", "nationalities", "place_of_birth",
			"nin", "phone_number", "phone_number_verified", "address"},
		"ui_locales_supported": []string{"fr"},
	})
}

func (s *Server) handleJWKS(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Signer.JWKS())
}

func redirectWith(w http.ResponseWriter, r *http.Request, redirectURI string, params map[string]string) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		http.Error(w, "redirect_uri invalide", http.StatusBadRequest)
		return
	}
	q := u.Query()
	for k, v := range params {
		if v != "" {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

func (s *Server) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	client, ok := s.client(q.Get("client_id"))
	if !ok {
		s.renderError(w, http.StatusBadRequest, "Service inconnu", "Ce service n'est pas autorisé à utiliser "+s.cfg.AppName+".")
		return
	}
	redirectURI := q.Get("redirect_uri")
	if !slices.Contains(client.RedirectURIs, redirectURI) {
		s.renderError(w, http.StatusBadRequest, "Adresse de retour invalide", "L'adresse de retour de ce service n'est pas enregistrée.")
		return
	}
	state := q.Get("state")
	fail := func(code, desc string) {
		redirectWith(w, r, redirectURI, map[string]string{"error": code, "error_description": desc, "state": state})
	}
	if q.Get("response_type") != "code" {
		fail("unsupported_response_type", "seul response_type=code est pris en charge")
		return
	}
	scopes := strings.Fields(q.Get("scope"))
	if !slices.Contains(scopes, "openid") {
		fail("invalid_scope", "le scope openid est obligatoire")
		return
	}
	for _, sc := range scopes {
		if _, known := scopeDescriptions[sc]; !known || !slices.Contains(client.Scopes, sc) {
			fail("invalid_scope", "scope non autorisé : "+sc)
			return
		}
	}
	if q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" {
		fail("invalid_request", "PKCE avec code_challenge_method=S256 est obligatoire")
		return
	}

	now := s.Now()
	secret := randomToken(24)
	ar := store.AuthRequest{
		ID:                randomToken(16),
		ClientID:          client.ID,
		RedirectURI:       redirectURI,
		Scopes:            scopes,
		State:             state,
		Nonce:             q.Get("nonce"),
		CodeChallenge:     q.Get("code_challenge"),
		BrowserSecretHash: hashToken(secret),
		Status:            store.AuthPendingUser,
		CreatedAt:         now,
		ExpiresAt:         now.Add(authRequestTTL),
	}
	s.Store.CreateAuthRequest(ar)
	s.countPartner(client.ID, func(st *store.PartnerStats) { st.Requests++ })
	s.setBrowserCookie(w, ar.ID, secret, authRequestTTL)
	s.renderLogin(w, http.StatusOK, ar, "")
}

// setBrowserCookie lie la demande à ce navigateur : un tiers qui
// connaîtrait l'identifiant de la demande ne pourrait pas récupérer le code.
func (s *Server) setBrowserCookie(w http.ResponseWriter, reqID, secret string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     browserCookiePrefix + reqID,
		Value:    secret,
		Path:     "/oauth/authorize",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   strings.HasPrefix(s.cfg.Issuer, "https://"),
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) renderLogin(w http.ResponseWriter, status int, ar store.AuthRequest, errMsg string) {
	s.render(w, status, "login.html", pageData{
		Title:      "Connexion",
		ClientName: s.clientName(ar.ClientID),
		ReqID:      ar.ID,
		Claims:     describeScopes(ar.Scopes),
		Error:      errMsg,
	})
}

// browserRequest charge la demande et vérifie qu'elle appartient à ce navigateur.
func (s *Server) browserRequest(r *http.Request, id string) (store.AuthRequest, bool) {
	ar, ok := s.Store.GetAuthRequest(id)
	if !ok {
		return store.AuthRequest{}, false
	}
	c, err := r.Cookie(browserCookiePrefix + ar.ID)
	if err != nil || subtle.ConstantTimeCompare([]byte(hashToken(c.Value)), []byte(ar.BrowserSecretHash)) != 1 {
		return store.AuthRequest{}, false
	}
	return ar, true
}

// effectiveStatus tient compte de l'expiration.
func (s *Server) effectiveStatus(ar store.AuthRequest) store.AuthStatus {
	if (ar.Status == store.AuthPendingUser || ar.Status == store.AuthPendingDevice) && s.Now().After(ar.ExpiresAt) {
		return store.AuthExpired
	}
	return ar.Status
}

func bindingChoices() (code string, choices []string) {
	seen := map[string]bool{}
	for len(choices) < 3 {
		c := randomDigits(2)
		if c[0] == '0' || seen[c] {
			continue
		}
		seen[c] = true
		choices = append(choices, c)
	}
	return choices[randInt(len(choices))], choices
}

func (s *Server) handleIdentify(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.renderError(w, http.StatusBadRequest, "Requête invalide", "Le formulaire est invalide.")
		return
	}
	ar, ok := s.browserRequest(r, r.PostForm.Get("req"))
	if !ok {
		s.renderError(w, http.StatusBadRequest, "Demande introuvable", "Cette demande de connexion est introuvable ou a expiré. Retournez sur le service et recommencez.")
		return
	}
	if s.effectiveStatus(ar) != store.AuthPendingUser {
		http.Redirect(w, r, "/oauth/authorize/wait?req="+url.QueryEscape(ar.ID), http.StatusSeeOther)
		return
	}

	identifier := strings.TrimSpace(r.PostForm.Get("identifiant"))
	var user store.User
	var found bool
	if digits := phone.Digits(identifier); len(digits) == 13 {
		user, found = s.Store.UserByNIN(digits)
	} else if tel, err := phone.Normalize(identifier); err == nil {
		user, found = s.Store.UserByPhone(tel)
	} else {
		s.renderLogin(w, http.StatusBadRequest, ar, "Saisissez votre numéro de téléphone ou votre NIN (13 chiffres).")
		return
	}

	// Même si aucun compte n'existe, la page d'attente s'affiche : on ne
	// révèle pas qui possède un compte. La demande expirera simplement.
	code, choices := bindingChoices()
	now := s.Now()
	err := s.Store.UpdateAuthRequest(ar.ID, func(a *store.AuthRequest) error {
		if found {
			a.UserID = user.ID
		}
		a.Status = store.AuthPendingDevice
		a.Challenge = randomToken(32)
		a.BindingCode = code
		a.BindingChoices = choices
		a.ExpiresAt = now.Add(deviceApprovalTTL)
		return nil
	})
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, "Erreur", "Une erreur est survenue.")
		return
	}
	if found {
		s.audit(store.AuditEvent{Type: "auth_requested", UserID: user.ID, ClientID: ar.ClientID,
			Detail: "Demande de connexion de " + s.clientName(ar.ClientID)})
	}
	http.Redirect(w, r, "/oauth/authorize/wait?req="+url.QueryEscape(ar.ID), http.StatusSeeOther)
}

func (s *Server) handleWait(w http.ResponseWriter, r *http.Request) {
	ar, ok := s.browserRequest(r, r.URL.Query().Get("req"))
	if !ok {
		s.renderError(w, http.StatusBadRequest, "Demande introuvable", "Cette demande de connexion est introuvable ou a expiré.")
		return
	}
	switch s.effectiveStatus(ar) {
	case store.AuthPendingUser:
		s.renderLogin(w, http.StatusOK, ar, "")
	case store.AuthPendingDevice:
		s.render(w, http.StatusOK, "wait.html", pageData{
			Title:       "Confirmation",
			ClientName:  s.clientName(ar.ClientID),
			ReqID:       ar.ID,
			BindingCode: ar.BindingCode,
		})
	default:
		http.Redirect(w, r, "/oauth/authorize/complete?req="+url.QueryEscape(ar.ID), http.StatusSeeOther)
	}
}

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	ar, ok := s.browserRequest(r, r.URL.Query().Get("req"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	status := s.effectiveStatus(ar)
	if status == store.AuthExpired && ar.Status != store.AuthExpired {
		_ = s.Store.UpdateAuthRequest(ar.ID, func(a *store.AuthRequest) error {
			a.Status = store.AuthExpired
			return nil
		})
	}
	remaining := int(ar.ExpiresAt.Sub(s.Now()).Seconds())
	if remaining < 0 {
		remaining = 0
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": status, "expires_in": remaining})
}

func (s *Server) handleAuthComplete(w http.ResponseWriter, r *http.Request) {
	ar, ok := s.browserRequest(r, r.URL.Query().Get("req"))
	if !ok {
		s.renderError(w, http.StatusBadRequest, "Demande introuvable", "Cette demande de connexion est introuvable ou a expiré.")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: browserCookiePrefix + ar.ID, Path: "/oauth/authorize", MaxAge: -1})
	if ar.Verification {
		// API simple : le partenaire lit le résultat avec GET /v1/verifications/{id}.
		if status := s.publicStatus(ar); status != "pending" {
			redirectWith(w, r, ar.RedirectURI, map[string]string{"verification_id": ar.ID, "status": status})
			return
		}
	}
	switch s.effectiveStatus(ar) {
	case store.AuthApproved:
		redirectWith(w, r, ar.RedirectURI, map[string]string{"code": ar.Code, "state": ar.State})
	case store.AuthDenied:
		redirectWith(w, r, ar.RedirectURI, map[string]string{
			"error": "access_denied", "error_description": "La demande a été refusée sur le téléphone.", "state": ar.State})
	case store.AuthExpired:
		redirectWith(w, r, ar.RedirectURI, map[string]string{
			"error": "access_denied", "error_description": "Délai de confirmation dépassé.", "state": ar.State})
	default:
		http.Redirect(w, r, "/oauth/authorize/wait?req="+url.QueryEscape(ar.ID), http.StatusSeeOther)
	}
}

func oauthError(w http.ResponseWriter, status int, code, desc string) {
	if status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `Basic realm="vivant"`)
	}
	writeJSON(w, status, map[string]string{"error": code, "error_description": desc})
}

func (s *Server) authenticateClient(r *http.Request) (*Client, bool) {
	id, secret, basic := r.BasicAuth()
	if basic {
		var err1, err2 error
		id, err1 = url.QueryUnescape(id)
		secret, err2 = url.QueryUnescape(secret)
		if err1 != nil || err2 != nil {
			return nil, false
		}
	} else {
		id, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	}
	c, ok := s.client(id)
	if !ok || !c.checkSecret(secret) {
		return nil, false
	}
	return c, true
}

func (s *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request", "formulaire invalide")
		return
	}
	client, ok := s.authenticateClient(r)
	if !ok {
		oauthError(w, http.StatusUnauthorized, "invalid_client", "authentification du client échouée")
		return
	}
	if r.PostForm.Get("grant_type") != "authorization_code" {
		oauthError(w, http.StatusBadRequest, "unsupported_grant_type", "seul authorization_code est pris en charge")
		return
	}
	now := s.Now()
	ar, err := s.Store.ConsumeCode(r.PostForm.Get("code"), now)
	if err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "code invalide, expiré ou déjà utilisé")
		return
	}
	if ar.ClientID != client.ID || ar.RedirectURI != r.PostForm.Get("redirect_uri") {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "client ou redirect_uri différent")
		return
	}
	verifier := r.PostForm.Get("code_verifier")
	sum := sha256.Sum256([]byte(verifier))
	if len(verifier) < 43 || len(verifier) > 128 || base64.RawURLEncoding.EncodeToString(sum[:]) != ar.CodeChallenge {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "code_verifier PKCE invalide")
		return
	}
	user, ok := s.Store.GetUser(ar.UserID)
	if !ok {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "utilisateur introuvable")
		return
	}

	claims := s.userClaims(user, client.ID, ar.Scopes)
	claims["iss"] = s.cfg.Issuer
	claims["aud"] = client.ID
	claims["iat"] = now.Unix()
	claims["exp"] = now.Add(idTokenTTL).Unix()
	claims["auth_time"] = ar.ApprovedAt.Unix()
	claims["acr"] = user.LoA
	// hwk : clé matérielle du téléphone ; user : PIN ou biométrie.
	claims["amr"] = []string{"hwk", "user"}
	if ar.Nonce != "" {
		claims["nonce"] = ar.Nonce
	}
	idToken, err := s.Signer.Sign(claims)
	if err != nil {
		s.writeError(w, err)
		return
	}
	access := randomToken(32)
	s.Store.SaveToken(store.AccessToken{
		Hash:      hashToken(access),
		UserID:    user.ID,
		ClientID:  client.ID,
		Scopes:    ar.Scopes,
		ExpiresAt: now.Add(accessTokenTTL),
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": access,
		"token_type":   "Bearer",
		"expires_in":   int(accessTokenTTL.Seconds()),
		"id_token":     idToken,
		"scope":        strings.Join(ar.Scopes, " "),
	})
}

func (s *Server) handleUserInfo(w http.ResponseWriter, r *http.Request) {
	invalid := func() {
		w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_token"})
	}
	raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		invalid()
		return
	}
	tok, ok := s.Store.GetToken(hashToken(strings.TrimSpace(raw)))
	if !ok || s.Now().After(tok.ExpiresAt) {
		invalid()
		return
	}
	user, ok := s.Store.GetUser(tok.UserID)
	if !ok {
		invalid()
		return
	}
	writeJSON(w, http.StatusOK, s.userClaims(user, tok.ClientID, tok.Scopes))
}

// subjectFor calcule un identifiant propre à chaque service (« pairwise ») :
// deux services ne peuvent pas relier leurs utilisateurs par cet identifiant.
func (s *Server) subjectFor(userID, clientID string) string {
	mac := hmac.New(sha256.New, s.cfg.PairwiseSecret)
	mac.Write([]byte(clientID + "|" + userID))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// userClaims ne renvoie que les données correspondant aux scopes accordés.
func (s *Server) userClaims(u store.User, clientID string, scopes []string) map[string]any {
	c := map[string]any{"sub": s.subjectFor(u.ID, clientID)}
	id := u.Identity
	for _, sc := range scopes {
		switch sc {
		case "profile":
			c["family_name"] = id.FamilyName
			c["given_name"] = id.GivenNames
			c["name"] = id.GivenNames + " " + id.FamilyName
			c["birthdate"] = id.BirthDate
			c["gender"] = map[string]string{"F": "female", "M": "male"}[id.Sex]
			c["nationalities"] = []string{id.Nationality}
		case "birth":
			c["place_of_birth"] = map[string]string{"locality": id.BirthPlace}
		case "nin":
			c["nin"] = id.NIN
		case "phone":
			c["phone_number"] = u.Phone
			c["phone_number_verified"] = true
		case "address":
			c["address"] = map[string]string{"formatted": id.Address}
		}
	}
	return c
}
