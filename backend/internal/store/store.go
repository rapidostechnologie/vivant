// Package store conserve les données de Vivant.
//
// Memory garde tout en mémoire et enregistre les utilisateurs, les appareils
// et le journal d'audit dans un fichier JSON. Il convient au prototype ; la
// production utilisera PostgreSQL derrière les mêmes méthodes.
package store

import (
	"encoding/json"
	"errors"
	"os"
	"slices"
	"sort"
	"sync"
	"time"
)

var (
	ErrNotFound    = errors.New("introuvable")
	ErrCodeInvalid = errors.New("code d'autorisation invalide ou déjà utilisé")
)

// Identity est l'identité vérifiée, copiée depuis le registre national.
type Identity struct {
	NIN            string `json:"nin"`
	FamilyName     string `json:"family_name"`
	GivenNames     string `json:"given_names"`
	BirthDate      string `json:"birth_date"`
	BirthPlace     string `json:"birth_place"`
	Sex            string `json:"sex"`
	Nationality    string `json:"nationality"`
	Address        string `json:"address"`
	DocumentNumber string `json:"document_number"`
}

type User struct {
	ID         string    `json:"id"`
	Identity   Identity  `json:"identity"`
	Phone      string    `json:"phone"`
	LoA        string    `json:"loa"`
	FaceScore  float64   `json:"face_score"`
	CreatedAt  time.Time `json:"created_at"`
	VerifiedAt time.Time `json:"verified_at"`
}

type Device struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	Name       string     `json:"name"`
	PublicKey  string     `json:"public_key"`
	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt time.Time  `json:"last_seen_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

type EnrollmentStep string

const (
	StepOTP      EnrollmentStep = "otp_pending"
	StepDocument EnrollmentStep = "document_pending"
	StepSelfie   EnrollmentStep = "selfie_pending"
	StepDevice   EnrollmentStep = "device_pending"
	StepDone     EnrollmentStep = "completed"
)

type Enrollment struct {
	ID           string
	NIN          string
	Phone        string
	Step         EnrollmentStep
	OTPHash      string
	OTPExpiresAt time.Time
	OTPAttempts  int
	Failures     int
	Identity     Identity
	FaceScore    float64
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

type AuthStatus string

const (
	AuthPendingUser   AuthStatus = "pending_user"
	AuthPendingDevice AuthStatus = "pending_device"
	AuthApproved      AuthStatus = "approved"
	AuthDenied        AuthStatus = "denied"
	AuthExpired       AuthStatus = "expired"
)

// AuthRequest est une demande de connexion OpenID Connect en cours.
type AuthRequest struct {
	ID                string
	ClientID          string
	RedirectURI       string
	Scopes            []string
	State             string
	Nonce             string
	CodeChallenge     string
	BrowserSecretHash string

	UserID         string
	Challenge      string
	BindingCode    string
	BindingChoices []string
	// Verification : demande créée par l'API simple des partenaires
	// (POST /v1/verifications) plutôt que par OpenID Connect.
	Verification    bool
	Reference       string
	StartTokenHash  string
	CompletedAt     time.Time
	Status          AuthStatus
	CreatedAt       time.Time
	ExpiresAt       time.Time
	DeviceID        string
	DeviceSignature string
	ApprovedAt      time.Time

	Code          string
	CodeExpiresAt time.Time
	CodeUsed      bool
}

type AccessToken struct {
	Hash      string
	UserID    string
	ClientID  string
	Scopes    []string
	ExpiresAt time.Time
}

// AuditEvent est une ligne du journal d'audit, visible aussi par
// l'utilisateur dans l'historique de son application.
type AuditEvent struct {
	At       time.Time `json:"at"`
	Type     string    `json:"type"`
	UserID   string    `json:"user_id,omitempty"`
	DeviceID string    `json:"device_id,omitempty"`
	ClientID string    `json:"client_id,omitempty"`
	Detail   string    `json:"detail,omitempty"`
}

const maxAuditEvents = 10000

type Memory struct {
	mu           sync.Mutex
	path         string
	enrollments  map[string]Enrollment
	users        map[string]User
	devices      map[string]Device
	authRequests map[string]AuthRequest
	codes        map[string]string
	tokens       map[string]AccessToken
	partners     map[string]Partner
	partnerSess  map[string]partnerSession
	audit        []AuditEvent
}

type snapshot struct {
	Users    map[string]User    `json:"users"`
	Devices  map[string]Device  `json:"devices"`
	Partners map[string]Partner `json:"partners"`
	Audit    []AuditEvent       `json:"audit"`
}

// NewMemory crée le stockage. Si path n'est pas vide, les données
// persistantes y sont chargées puis enregistrées à chaque modification.
func NewMemory(path string) (*Memory, error) {
	m := &Memory{
		path:         path,
		enrollments:  map[string]Enrollment{},
		users:        map[string]User{},
		devices:      map[string]Device{},
		authRequests: map[string]AuthRequest{},
		codes:        map[string]string{},
		tokens:       map[string]AccessToken{},
		partners:     map[string]Partner{},
		partnerSess:  map[string]partnerSession{},
	}
	if path == "" {
		return m, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	var snap snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	if snap.Users != nil {
		m.users = snap.Users
	}
	if snap.Devices != nil {
		m.devices = snap.Devices
	}
	if snap.Partners != nil {
		m.partners = snap.Partners
	}
	m.audit = snap.Audit
	return m, nil
}

func (m *Memory) persistLocked() error {
	if m.path == "" {
		return nil
	}
	data, err := json.MarshalIndent(snapshot{Users: m.users, Devices: m.devices, Partners: m.partners, Audit: m.audit}, "", "  ")
	if err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.path)
}

// --- Inscriptions ---

func (m *Memory) CreateEnrollment(e Enrollment) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enrollments[e.ID] = e
}

func (m *Memory) GetEnrollment(id string) (Enrollment, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.enrollments[id]
	return e, ok
}

// UpdateEnrollment applique fn à l'inscription. Les modifications sont
// enregistrées même si fn renvoie une erreur (compteurs de tentatives).
func (m *Memory) UpdateEnrollment(id string, fn func(*Enrollment) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.enrollments[id]
	if !ok {
		return ErrNotFound
	}
	err := fn(&e)
	m.enrollments[id] = e
	return err
}

// --- Utilisateurs ---

func (m *Memory) GetUser(id string) (User, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[id]
	return u, ok
}

func (m *Memory) UserByNIN(nin string) (User, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.Identity.NIN == nin {
			return u, true
		}
	}
	return User{}, false
}

func (m *Memory) UserByPhone(phone string) (User, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.Phone == phone {
			return u, true
		}
	}
	return User{}, false
}

func (m *Memory) SaveUser(u User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[u.ID] = u
	return m.persistLocked()
}

// --- Appareils ---

func (m *Memory) AddDevice(d Device) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.devices[d.ID] = d
	return m.persistLocked()
}

func (m *Memory) GetDevice(id string) (Device, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.devices[id]
	return d, ok
}

// TouchDevice met à jour la date de dernière activité (non persistée).
func (m *Memory) TouchDevice(id string, at time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if d, ok := m.devices[id]; ok {
		d.LastSeenAt = at
		m.devices[id] = d
	}
}

func (m *Memory) UserDevices(userID string) []Device {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Device
	for _, d := range m.devices {
		if d.UserID == userID {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

// RevokeDevice révoque un appareil.
func (m *Memory) RevokeDevice(id string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.devices[id]
	if !ok {
		return ErrNotFound
	}
	if d.RevokedAt == nil {
		t := at
		d.RevokedAt = &t
		m.devices[id] = d
	}
	return m.persistLocked()
}

// ListPartners renvoie tous les partenaires.
func (m *Memory) ListPartners() []Partner {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Partner, 0, len(m.partners))
	for _, p := range m.partners {
		out = append(out, p)
	}
	return out
}

// RevokeUserDevices révoque tous les appareils actifs de l'utilisateur et
// renvoie leur nombre.
func (m *Memory) RevokeUserDevices(userID string, at time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for id, d := range m.devices {
		if d.UserID == userID && d.RevokedAt == nil {
			t := at
			d.RevokedAt = &t
			m.devices[id] = d
			n++
		}
	}
	if n == 0 {
		return 0, nil
	}
	return n, m.persistLocked()
}

// --- Demandes de connexion ---

func (m *Memory) CreateAuthRequest(a AuthRequest) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.authRequests[a.ID] = a
}

func (m *Memory) GetAuthRequest(id string) (AuthRequest, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.authRequests[id]
	return a, ok
}

// UpdateAuthRequest applique fn à la demande. Les modifications sont
// enregistrées même si fn renvoie une erreur.
func (m *Memory) UpdateAuthRequest(id string, fn func(*AuthRequest) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.authRequests[id]
	if !ok {
		return ErrNotFound
	}
	err := fn(&a)
	m.authRequests[id] = a
	if a.Code != "" {
		m.codes[a.Code] = id
	}
	return err
}

// PendingAuthRequests renvoie les demandes en attente de validation sur le
// téléphone de l'utilisateur.
func (m *Memory) PendingAuthRequests(userID string, now time.Time) []AuthRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []AuthRequest
	for _, a := range m.authRequests {
		if a.UserID == userID && a.Status == AuthPendingDevice && now.Before(a.ExpiresAt) {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

// ConsumeCode marque un code d'autorisation comme utilisé et renvoie la
// demande associée. Un code ne peut être échangé qu'une seule fois.
func (m *Memory) ConsumeCode(code string, now time.Time) (AuthRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.codes[code]
	if !ok {
		return AuthRequest{}, ErrCodeInvalid
	}
	a := m.authRequests[id]
	if a.CodeUsed || a.Status != AuthApproved || now.After(a.CodeExpiresAt) {
		return AuthRequest{}, ErrCodeInvalid
	}
	a.CodeUsed = true
	m.authRequests[id] = a
	return a, nil
}

// --- Jetons d'accès ---

func (m *Memory) SaveToken(t AccessToken) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokens[t.Hash] = t
}

func (m *Memory) GetToken(hash string) (AccessToken, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tokens[hash]
	return t, ok
}

// --- Audit ---

func (m *Memory) Audit(ev AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.audit = append(m.audit, ev)
	if len(m.audit) > maxAuditEvents {
		m.audit = slices.Clone(m.audit[len(m.audit)-maxAuditEvents:])
	}
	return m.persistLocked()
}

// AuditForUser renvoie les derniers événements de l'utilisateur, du plus
// récent au plus ancien.
func (m *Memory) AuditForUser(userID string, limit int) []AuditEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []AuditEvent
	for i := len(m.audit) - 1; i >= 0 && len(out) < limit; i-- {
		if m.audit[i].UserID == userID {
			out = append(out, m.audit[i])
		}
	}
	return out
}

// Sweep supprime les inscriptions, demandes et jetons expirés.
func (m *Memory) Sweep(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, e := range m.enrollments {
		if now.After(e.ExpiresAt) {
			delete(m.enrollments, id)
		}
	}
	for id, a := range m.authRequests {
		if now.After(a.ExpiresAt.Add(10 * time.Minute)) {
			delete(m.authRequests, id)
			if a.Code != "" {
				delete(m.codes, a.Code)
			}
		}
	}
	for h, t := range m.tokens {
		if now.After(t.ExpiresAt) {
			delete(m.tokens, h)
		}
	}
	for h, ps := range m.partnerSess {
		if now.After(ps.ExpiresAt) {
			delete(m.partnerSess, h)
		}
	}
}
