package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

var secretPattern = regexp.MustCompile(`class="secret">(sk_test_[A-Za-z0-9_-]+)<`)

// signupPartner crée un compte partenaire et renvoie sa clé secrète.
func (e *testEnv) signupPartner(email string) string {
	e.t.Helper()
	resp, err := e.browser.PostForm(e.ts.URL+"/partenaires/inscription", url.Values{
		"org": {"Banque Démo"}, "sector": {"Banque"}, "email": {email}, "password": {"motdepasse-solide"},
	})
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	m := secretPattern.FindStringSubmatch(string(body))
	if resp.StatusCode != http.StatusOK || m == nil {
		e.t.Fatalf("inscription : statut %d, clé absente", resp.StatusCode)
	}
	return m[1]
}

func (e *testEnv) api(method, path, key string, body any) (int, map[string]any) {
	e.t.Helper()
	var r io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		r = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, e.ts.URL+path, r)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestPartnerSimpleAPIFlow(t *testing.T) {
	e := newTestEnv(t)
	dev := e.enroll("2751199000123")
	key := e.signupPartner("contact@banque-demo.com")

	// Adresse non enregistrée : refusée.
	status, out := e.api("POST", "/v1/verifications", key, map[string]any{"redirect_url": "https://pirate.example/retour"})
	if status != http.StatusBadRequest || out["error"] != "invalid_redirect_url" {
		t.Fatalf("adresse non enregistrée : %d %v", status, out)
	}
	// Donnée non activée : refusée.
	status, out = e.api("POST", "/v1/verifications", key, map[string]any{
		"redirect_url": "http://localhost:3000/retour", "data": []string{"nin"}})
	if status != http.StatusForbidden || out["error"] != "data_not_allowed" {
		t.Fatalf("donnée non activée : %d %v", status, out)
	}

	status, out = e.api("POST", "/v1/verifications", key, map[string]any{
		"redirect_url": "http://localhost:3000/retour", "reference": "dossier-42"})
	if status != http.StatusCreated {
		t.Fatalf("création : %d %v", status, out)
	}
	id := out["id"].(string)
	startURL := out["url"].(string)

	// Le client ouvre le lien, s'identifie et confirme sur son téléphone.
	resp, body := e.browserGet(startURL)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Banque Démo") {
		t.Fatalf("lien de départ : %d", resp.StatusCode)
	}
	if again, _ := e.browserGet(startURL); again.StatusCode != http.StatusBadRequest {
		t.Errorf("le lien de départ doit être à usage unique, statut %d", again.StatusCode)
	}
	resp, err := e.browser.PostForm(e.ts.URL+"/oauth/authorize", url.Values{"req": {id}, "identifiant": {"771234567"}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	_, body = e.browserGet(e.ts.URL + "/oauth/authorize/wait?req=" + url.QueryEscape(id))
	code := bindingPattern.FindStringSubmatch(body)[1]

	if status, out := e.api("GET", "/v1/verifications/"+id, key, nil); status != http.StatusOK || out["status"] != "pending" {
		t.Fatalf("avant confirmation : %d %v", status, out)
	}
	rq := e.pendingRequests(dev)[0]
	if rq.ClientName != "Banque Démo" {
		t.Errorf("nom du service sur le téléphone : %q", rq.ClientName)
	}
	if st, raw := e.approve(dev, rq, code); st != http.StatusOK {
		t.Fatalf("approbation : %d %s", st, raw)
	}

	resp, _ = e.browserGet(e.ts.URL + "/oauth/authorize/complete?req=" + url.QueryEscape(id))
	loc, _ := url.Parse(resp.Header.Get("Location"))
	if loc.Host != "localhost:3000" || loc.Query().Get("verification_id") != id || loc.Query().Get("status") != "approved" {
		t.Fatalf("retour vers le partenaire inattendu : %s", loc)
	}

	status, out = e.api("GET", "/v1/verifications/"+id, key, nil)
	if status != http.StatusOK || out["status"] != "approved" || out["reference"] != "dossier-42" {
		t.Fatalf("résultat : %d %v", status, out)
	}
	identity := out["identity"].(map[string]any)
	if identity["family_name"] != "Diop" || identity["sub"] == nil {
		t.Errorf("identité : %v", identity)
	}
	if _, ok := identity["nin"]; ok {
		t.Error("le NIN ne doit pas être transmis sans la donnée « nin »")
	}

	// Un autre partenaire ne peut pas lire ce résultat.
	other := e.signupPartner("contact@assurance-demo.com")
	if status, _ := e.api("GET", "/v1/verifications/"+id, other, nil); status != http.StatusNotFound {
		t.Errorf("lecture par un autre partenaire : statut %d, attendu 404", status)
	}
	if status, out := e.api("GET", "/v1/verifications/"+id, "", nil); status != http.StatusUnauthorized || out["error"] != "missing_api_key" {
		t.Errorf("sans clé : %d %v", status, out)
	}
}

func TestPartnerDashboard(t *testing.T) {
	e := newTestEnv(t)
	key := e.signupPartner("dsi@impots-demo.gov")

	resp, body := e.browserGet(e.ts.URL + "/partenaires/tableau")
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Mode test") {
		t.Fatalf("tableau de bord : %d", resp.StatusCode)
	}
	if strings.Contains(body, key) {
		t.Error("la clé secrète ne doit plus être affichée après la création")
	}

	// Activer le NIN puis ajouter une adresse de retour.
	resp, err := e.browser.PostForm(e.ts.URL+"/partenaires/donnees", url.Values{"profile": {"on"}, "nin": {"on"}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	resp, err = e.browser.PostForm(e.ts.URL+"/partenaires/retours", url.Values{"url": {"https://impots-demo.gov/retour"}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	status, out := e.api("POST", "/v1/verifications", key, map[string]any{
		"redirect_url": "https://impots-demo.gov/retour", "data": []string{"profile", "nin"}})
	if status != http.StatusCreated {
		t.Fatalf("création après configuration : %d %v", status, out)
	}

	// Une adresse http non locale est refusée.
	resp, err = e.browser.PostForm(e.ts.URL+"/partenaires/retours", url.Values{"url": {"http://impots-demo.gov/retour"}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("adresse http : statut %d, attendu 400", resp.StatusCode)
	}

	// Régénérer la clé invalide l'ancienne.
	resp, err = e.browser.PostForm(e.ts.URL+"/partenaires/cle", url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if status, _ := e.api("POST", "/v1/verifications", key, map[string]any{"redirect_url": "https://impots-demo.gov/retour"}); status != http.StatusUnauthorized {
		t.Errorf("ancienne clé : statut %d, attendu 401", status)
	}

	// Formulaire envoyé depuis un autre site : refusé.
	req, _ := http.NewRequest("POST", e.ts.URL+"/partenaires/cle", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	resp, err = e.browser.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("origine étrangère : statut %d, attendu 403", resp.StatusCode)
	}
}

func TestPartnerLoginAndPublicPages(t *testing.T) {
	e := newTestEnv(t)
	e.signupPartner("contact@banque-demo.com")
	for _, p := range []string{"/partenaires", "/docs", "/partenaires/connexion", "/partenaires/inscription"} {
		if resp, _ := e.browserGet(e.ts.URL + p); resp.StatusCode != http.StatusOK {
			t.Errorf("%s : statut %d", p, resp.StatusCode)
		}
	}
	resp, err := http.PostForm(e.ts.URL+"/partenaires/connexion", url.Values{"email": {"contact@banque-demo.com"}, "password": {"mauvais"}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("mauvais mot de passe : statut %d", resp.StatusCode)
	}
	// Un doublon d'e-mail est refusé.
	resp, err = http.PostForm(e.ts.URL+"/partenaires/inscription", url.Values{
		"org": {"Copie"}, "sector": {"Banque"}, "email": {"contact@banque-demo.com"}, "password": {"motdepasse-solide"}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("e-mail en double : statut %d", resp.StatusCode)
	}
}
