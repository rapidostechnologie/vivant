package server

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"vivant/internal/devicekey"
	"vivant/internal/jwt"
	"vivant/internal/notify"
	"vivant/internal/registry"
	"vivant/internal/store"
	"vivant/internal/verification"
)

const (
	testClientID     = "portail-test"
	testClientSecret = "secret-test"
	testRedirect     = "http://client.example/callback"
	testVerifier     = "verificateur-pkce-de-test-suffisamment-long-0123456789"
)

type testEnv struct {
	t       *testing.T
	ts      *httptest.Server
	srv     *Server
	browser *http.Client
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	st, err := store.NewMemory("")
	if err != nil {
		t.Fatal(err)
	}
	reg, err := registry.NewMock()
	if err != nil {
		t.Fatal(err)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewUnstartedServer(nil)
	srv, err := New(Config{
		Issuer:  "http://" + ts.Listener.Addr().String(),
		AppName: "Vivant",
		DevMode: true,
		Clients: []Client{{
			ID: testClientID, Name: "Portail de test", Secret: testClientSecret,
			RedirectURIs: []string{testRedirect},
			Scopes:       []string{"openid", "profile", "birth", "nin", "phone"},
		}},
		PairwiseSecret: []byte("secret-pairwise-de-test"),
	}, Deps{
		Store:    st,
		Registry: reg,
		Faces:    verification.MockFaceVerifier{},
		SMS:      notify.Discard{},
		Signer:   jwt.NewSigner(key),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	ts.Config.Handler = srv.Handler()
	ts.Start()
	t.Cleanup(ts.Close)
	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	return &testEnv{t: t, ts: ts, srv: srv, browser: browser}
}

func (e *testEnv) postJSON(path string, body any) (int, map[string]any) {
	e.t.Helper()
	raw, _ := json.Marshal(body)
	resp, err := http.Post(e.ts.URL+path, "application/json", bytes.NewReader(raw))
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (e *testEnv) mustPostJSON(path string, body any, want int) map[string]any {
	e.t.Helper()
	status, out := e.postJSON(path, body)
	if status != want {
		e.t.Fatalf("POST %s : statut %d, attendu %d (%v)", path, status, want, out)
	}
	return out
}

type testDevice struct {
	id  string
	key *ecdsa.PrivateKey
}

func (e *testEnv) signed(d testDevice, method, path string, body any) (int, []byte) {
	e.t.Helper()
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig, err := devicekey.Sign(d.key, devicekey.RequestMessage(method, path, ts, raw))
	if err != nil {
		e.t.Fatal(err)
	}
	req, _ := http.NewRequest(method, e.ts.URL+path, bytes.NewReader(raw))
	req.Header.Set("X-Vivant-Device", d.id)
	req.Header.Set("X-Vivant-Timestamp", ts)
	req.Header.Set("X-Vivant-Signature", sig)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

func citizen(t *testing.T, nin string) registry.Citizen {
	t.Helper()
	cs, _ := registry.Fixtures()
	for _, c := range cs {
		if c.NIN == nin {
			return c
		}
	}
	t.Fatalf("citoyen %s absent des données de test", nin)
	return registry.Citizen{}
}

// enroll déroule l'inscription complète et renvoie l'appareil activé.
func (e *testEnv) enroll(nin string) testDevice {
	e.t.Helper()
	c := citizen(e.t, nin)
	start := e.mustPostJSON("/api/v1/enrollments", map[string]string{"nin": nin, "phone": c.SIMPhone}, http.StatusCreated)
	id := start["enrollment_id"].(string)
	e.mustPostJSON("/api/v1/enrollments/"+id+"/otp", map[string]string{"code": start["dev_otp"].(string)}, http.StatusOK)
	card, err := registry.SimulatedCardMRZ(c)
	if err != nil {
		e.t.Fatal(err)
	}
	e.mustPostJSON("/api/v1/enrollments/"+id+"/document", map[string]string{"mrz": card}, http.StatusOK)
	selfie := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0xff}, 512))
	e.mustPostJSON("/api/v1/enrollments/"+id+"/selfie", map[string]string{"selfie": selfie}, http.StatusOK)

	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	pub, _ := devicekey.EncodePublicKey(&key.PublicKey)
	sig, _ := devicekey.Sign(key, devicekey.EnrollmentMessage(id))
	done := e.mustPostJSON("/api/v1/enrollments/"+id+"/complete",
		map[string]string{"public_key": pub, "device_name": "Téléphone de test", "signature": sig}, http.StatusCreated)
	return testDevice{id: done["device_id"].(string), key: key}
}

var (
	reqIDPattern   = regexp.MustCompile(`name="req" value="([^"]+)"`)
	bindingPattern = regexp.MustCompile(`id="binding-code">(\d+)<`)
)

func (e *testEnv) browserGet(rawURL string) (*http.Response, string) {
	e.t.Helper()
	resp, err := e.browser.Get(rawURL)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, string(body)
}

// startLogin ouvre /oauth/authorize, saisit l'identifiant et renvoie
// l'identifiant de la demande et le code affiché dans le navigateur.
func (e *testEnv) startLogin(identifier string) (reqID, bindingCode string) {
	e.t.Helper()
	sum := sha256.Sum256([]byte(testVerifier))
	q := url.Values{
		"client_id":             {testClientID},
		"redirect_uri":          {testRedirect},
		"response_type":         {"code"},
		"scope":                 {"openid profile birth nin"},
		"state":                 {"etat-123"},
		"nonce":                 {"nonce-456"},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(sum[:])},
		"code_challenge_method": {"S256"},
	}
	resp, body := e.browserGet(e.ts.URL + "/oauth/authorize?" + q.Encode())
	m := reqIDPattern.FindStringSubmatch(body)
	if resp.StatusCode != http.StatusOK || m == nil {
		e.t.Fatalf("page de connexion inattendue (%d) : %s", resp.StatusCode, body)
	}
	reqID = m[1]

	resp, err := e.browser.PostForm(e.ts.URL+"/oauth/authorize", url.Values{"req": {reqID}, "identifiant": {identifier}})
	if err != nil {
		e.t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		e.t.Fatalf("identification : statut %d", resp.StatusCode)
	}
	_, body = e.browserGet(e.ts.URL + "/oauth/authorize/wait?req=" + url.QueryEscape(reqID))
	m = bindingPattern.FindStringSubmatch(body)
	if m == nil {
		e.t.Fatalf("code de vérification absent : %s", body)
	}
	return reqID, m[1]
}

func (e *testEnv) pendingRequests(d testDevice) []deviceRequestView {
	e.t.Helper()
	status, raw := e.signed(d, "GET", "/api/v1/device/requests", nil)
	if status != http.StatusOK {
		e.t.Fatalf("demandes en attente : statut %d (%s)", status, raw)
	}
	var reqs []deviceRequestView
	if err := json.Unmarshal(raw, &reqs); err != nil {
		e.t.Fatal(err)
	}
	return reqs
}

func (e *testEnv) approve(d testDevice, rq deviceRequestView, code string) (int, []byte) {
	sig, _ := devicekey.Sign(d.key, devicekey.ApprovalMessage(rq.ID, rq.Challenge, code))
	return e.signed(d, "POST", "/api/v1/device/requests/"+rq.ID+"/approve",
		map[string]string{"binding_code": code, "signature": sig})
}

func (e *testEnv) exchangeCode(code string) (int, map[string]any) {
	e.t.Helper()
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {testRedirect},
		"code_verifier": {testVerifier},
	}
	req, _ := http.NewRequest("POST", e.ts.URL+"/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(testClientID, testClientSecret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestFullLoginFlow(t *testing.T) {
	e := newTestEnv(t)
	dev := e.enroll("2751199000123")

	reqID, code := e.startLogin("77 123 45 67")
	reqs := e.pendingRequests(dev)
	if len(reqs) != 1 || reqs[0].ID != reqID {
		t.Fatalf("demande attendue sur le téléphone : %+v", reqs)
	}
	if reqs[0].ClientName != "Portail de test" || len(reqs[0].BindingChoices) != 3 {
		t.Errorf("vue de la demande inattendue : %+v", reqs[0])
	}
	if status, raw := e.approve(dev, reqs[0], code); status != http.StatusOK {
		t.Fatalf("approbation : statut %d (%s)", status, raw)
	}

	resp, _ := e.browserGet(e.ts.URL + "/oauth/authorize/complete?req=" + url.QueryEscape(reqID))
	loc, err := url.Parse(resp.Header.Get("Location"))
	if resp.StatusCode != http.StatusFound || err != nil {
		t.Fatalf("redirection attendue, obtenu %d", resp.StatusCode)
	}
	if loc.Query().Get("state") != "etat-123" || loc.Query().Get("code") == "" {
		t.Fatalf("redirection inattendue : %s", loc)
	}
	authCode := loc.Query().Get("code")

	status, tok := e.exchangeCode(authCode)
	if status != http.StatusOK {
		t.Fatalf("échange du code : %d %v", status, tok)
	}
	claims, err := jwt.Verify(tok["id_token"].(string), e.srv.Signer.JWKS())
	if err != nil {
		t.Fatal(err)
	}
	checks := map[string]any{
		"iss":         e.srv.cfg.Issuer,
		"aud":         testClientID,
		"nonce":       "nonce-456",
		"family_name": "Diop",
		"given_name":  "Fatou",
		"birthdate":   "1990-03-14",
		"nin":         "2751199000123",
		"acr":         loaSubstantial,
	}
	for k, want := range checks {
		if claims[k] != want {
			t.Errorf("claim %s = %v, attendu %v", k, claims[k], want)
		}
	}
	if pob, _ := claims["place_of_birth"].(map[string]any); pob["locality"] != "Dakar" {
		t.Errorf("place_of_birth = %v", claims["place_of_birth"])
	}
	if _, ok := claims["phone_number"]; ok {
		t.Error("phone_number transmis alors que le scope phone n'a pas été demandé")
	}

	// Un code d'autorisation ne s'utilise qu'une fois.
	if status, _ := e.exchangeCode(authCode); status != http.StatusBadRequest {
		t.Errorf("réutilisation du code : statut %d, attendu 400", status)
	}

	req, _ := http.NewRequest("GET", e.ts.URL+"/oauth/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+tok["access_token"].(string))
	uresp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer uresp.Body.Close()
	var info map[string]any
	_ = json.NewDecoder(uresp.Body).Decode(&info)
	if info["sub"] != claims["sub"] || info["family_name"] != "Diop" {
		t.Errorf("userinfo inattendu : %v", info)
	}
}

func TestWrongBindingCodeCancelsRequest(t *testing.T) {
	e := newTestEnv(t)
	dev := e.enroll("1752198500456")
	reqID, code := e.startLogin("1752198500456")
	rq := e.pendingRequests(dev)[0]

	var wrong string
	for _, c := range rq.BindingChoices {
		if c != code {
			wrong = c
			break
		}
	}
	if status, _ := e.approve(dev, rq, wrong); status != http.StatusForbidden {
		t.Fatalf("mauvais code : statut %d, attendu 403", status)
	}
	// La demande est annulée : le bon code ne fonctionne plus.
	if status, _ := e.approve(dev, rq, code); status != http.StatusConflict {
		t.Fatalf("après annulation : statut %d, attendu 409", status)
	}
	resp, _ := e.browserGet(e.ts.URL + "/oauth/authorize/complete?req=" + url.QueryEscape(reqID))
	loc, _ := url.Parse(resp.Header.Get("Location"))
	if loc.Query().Get("error") != "access_denied" {
		t.Fatalf("erreur access_denied attendue : %s", loc)
	}
}

func TestOtherDeviceCannotApprove(t *testing.T) {
	e := newTestEnv(t)
	victim := e.enroll("2751199000123")
	attacker := e.enroll("2753200100789")
	_, code := e.startLogin("771234567")
	rq := e.pendingRequests(victim)[0]

	if len(e.pendingRequests(attacker)) != 0 {
		t.Fatal("la demande ne doit apparaître que sur le téléphone du titulaire")
	}
	if status, _ := e.approve(attacker, rq, code); status != http.StatusNotFound {
		t.Fatalf("approbation par un autre appareil : statut %d, attendu 404", status)
	}
}

func TestReenrollmentRevokesOldDevice(t *testing.T) {
	e := newTestEnv(t)
	old := e.enroll("2751199000123")
	e.enroll("2751199000123")
	if status, _ := e.signed(old, "GET", "/api/v1/device/me", nil); status != http.StatusUnauthorized {
		t.Fatalf("ancien appareil : statut %d, attendu 401", status)
	}
}

func TestEnrollmentRejections(t *testing.T) {
	e := newTestEnv(t)

	// Numéro qui n'appartient pas au titulaire du NIN.
	status, out := e.postJSON("/api/v1/enrollments", map[string]string{"nin": "2751199000123", "phone": "781112233"})
	if status != http.StatusForbidden || out["error"] != "phone_mismatch" {
		t.Errorf("mauvais numéro : %d %v", status, out)
	}

	// Carte expirée.
	c := citizen(t, "1754197800321")
	start := e.mustPostJSON("/api/v1/enrollments", map[string]string{"nin": c.NIN, "phone": c.SIMPhone}, http.StatusCreated)
	id := start["enrollment_id"].(string)
	e.mustPostJSON("/api/v1/enrollments/"+id+"/otp", map[string]string{"code": start["dev_otp"].(string)}, http.StatusOK)
	card, _ := registry.SimulatedCardMRZ(c)
	status, out = e.postJSON("/api/v1/enrollments/"+id+"/document", map[string]string{"mrz": card})
	if status != http.StatusUnprocessableEntity || out["error"] != "document_expired" {
		t.Errorf("carte expirée : %d %v", status, out)
	}

	// Carte d'une autre personne.
	c = citizen(t, "2751199000123")
	start = e.mustPostJSON("/api/v1/enrollments", map[string]string{"nin": c.NIN, "phone": c.SIMPhone}, http.StatusCreated)
	id = start["enrollment_id"].(string)
	e.mustPostJSON("/api/v1/enrollments/"+id+"/otp", map[string]string{"code": start["dev_otp"].(string)}, http.StatusOK)
	other, _ := registry.SimulatedCardMRZ(citizen(t, "2753200100789"))
	status, out = e.postJSON("/api/v1/enrollments/"+id+"/document", map[string]string{"mrz": other})
	if status != http.StatusUnprocessableEntity || out["error"] != "document_mismatch" {
		t.Errorf("carte d'un tiers : %d %v", status, out)
	}

	// Étape suivante impossible sans document valide.
	sel := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 64))
	if status, _ := e.postJSON("/api/v1/enrollments/"+id+"/selfie", map[string]string{"selfie": sel}); status != http.StatusConflict {
		t.Errorf("selfie avant document : statut %d, attendu 409", status)
	}
}

func TestUnknownUserStillSeesWaitingPage(t *testing.T) {
	e := newTestEnv(t)
	// Aucun compte : la page d'attente s'affiche quand même (pas de fuite).
	if _, code := e.startLogin("+221 70 000 00 00"); code == "" {
		t.Fatal("code de vérification attendu")
	}
}

func TestDeviceRequestSignatureRequired(t *testing.T) {
	e := newTestEnv(t)
	dev := e.enroll("2751199000123")
	impostor := testDevice{id: dev.id}
	impostor.key, _ = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if status, _ := e.signed(impostor, "GET", "/api/v1/device/me", nil); status != http.StatusUnauthorized {
		t.Fatalf("signature d'une autre clé : statut %d, attendu 401", status)
	}
}

func TestEnrollmentWithPhoneOnly(t *testing.T) {
	e := newTestEnv(t)
	// Le NIN vient du registre des abonnés : le numéro suffit.
	start := e.mustPostJSON("/api/v1/enrollments", map[string]string{"phone": "78 111 22 33"}, http.StatusCreated)
	id := start["enrollment_id"].(string)
	e.mustPostJSON("/api/v1/enrollments/"+id+"/otp", map[string]string{"code": start["dev_otp"].(string)}, http.StatusOK)

	// En mode développement, la carte de démonstration remplace la caméra.
	resp, err := http.Get(e.ts.URL + "/api/v1/dev/enrollments/" + id + "/card")
	if err != nil {
		t.Fatal(err)
	}
	var card map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&card)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(card["mrz"], "NDIAYE<<MOUSSA") {
		t.Fatalf("carte de démonstration : %d %v", resp.StatusCode, card)
	}
	e.mustPostJSON("/api/v1/enrollments/"+id+"/document", map[string]string{"mrz": card["mrz"]}, http.StatusOK)

	if status, out := e.postJSON("/api/v1/enrollments", map[string]string{"phone": "+221 70 000 00 00"}); status != http.StatusNotFound || out["error"] != "holder_not_found" {
		t.Errorf("numéro inconnu : %d %v", status, out)
	}
}

func TestDeviceRevokeAndServices(t *testing.T) {
	e := newTestEnv(t)
	dev := e.enroll("2751199000123")
	if status, raw := e.signed(dev, "POST", "/api/v1/device/revoke", map[string]string{}); status != http.StatusOK {
		t.Fatalf("révocation : %d %s", status, raw)
	}
	if status, _ := e.signed(dev, "GET", "/api/v1/device/me", nil); status != http.StatusUnauthorized {
		t.Errorf("appareil révoqué : statut %d, attendu 401", status)
	}
	resp, err := http.Get(e.ts.URL + "/api/v1/services")
	if err != nil {
		t.Fatal(err)
	}
	var services []map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&services)
	resp.Body.Close()
	if len(services) != 1 || services[0]["name"] != "Portail de test" {
		t.Errorf("services : %v", services)
	}
}
