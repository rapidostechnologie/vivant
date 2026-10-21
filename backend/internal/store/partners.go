package store

import (
	"errors"
	"strings"
	"time"
)

var ErrEmailTaken = errors.New("cette adresse e-mail est déjà utilisée")

// Modes d'un partenaire.
const (
	PartnerTest   = "test"   // clés de test, identités fictives
	PartnerReview = "review" // mise en production demandée, vérification en cours
	PartnerLive   = "live"   // production
)

// PartnerStats compte les vérifications d'un partenaire.
type PartnerStats struct {
	Requests int `json:"requests"`
	Approved int `json:"approved"`
	Denied   int `json:"denied"`
}

// Partner est une organisation (banque, assurance, administration…) qui
// utilise Vivant pour vérifier l'identité de ses clients.
type Partner struct {
	ID           string       `json:"id"`
	OrgName      string       `json:"org_name"`
	Sector       string       `json:"sector"`
	Website      string       `json:"website"`
	Email        string       `json:"email"`
	PasswordHash string       `json:"password_hash"`
	PasswordSalt string       `json:"password_salt"`
	ClientID     string       `json:"client_id"`
	SecretHash   string       `json:"secret_hash"` // SHA-256 hexadécimal de la clé secrète
	SecretHint   string       `json:"secret_hint"` // 4 derniers caractères
	RedirectURIs []string     `json:"redirect_uris"`
	Scopes       []string     `json:"scopes"`
	Mode         string       `json:"mode"`
	Stats        PartnerStats `json:"stats"`
	CreatedAt    time.Time    `json:"created_at"`
}

type partnerSession struct {
	PartnerID string
	ExpiresAt time.Time
}

func (m *Memory) CreatePartner(p Partner) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, other := range m.partners {
		if strings.EqualFold(other.Email, p.Email) {
			return ErrEmailTaken
		}
	}
	m.partners[p.ID] = p
	return m.persistLocked()
}

func (m *Memory) GetPartner(id string) (Partner, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.partners[id]
	return p, ok
}

func (m *Memory) PartnerByEmail(email string) (Partner, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.partners {
		if strings.EqualFold(p.Email, email) {
			return p, true
		}
	}
	return Partner{}, false
}

func (m *Memory) PartnerByClientID(clientID string) (Partner, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.partners {
		if p.ClientID == clientID {
			return p, true
		}
	}
	return Partner{}, false
}

// PartnerBySecretHash retrouve le partenaire propriétaire d'une clé secrète.
func (m *Memory) PartnerBySecretHash(hash string) (Partner, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.partners {
		if p.SecretHash == hash {
			return p, true
		}
	}
	return Partner{}, false
}

// UpdatePartner applique fn au partenaire puis l'enregistre si fn réussit.
func (m *Memory) UpdatePartner(id string, fn func(*Partner) error) (Partner, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.partners[id]
	if !ok {
		return Partner{}, ErrNotFound
	}
	if err := fn(&p); err != nil {
		return Partner{}, err
	}
	m.partners[id] = p
	return p, m.persistLocked()
}

// --- Sessions de l'espace partenaires (non persistées) ---

func (m *Memory) CreatePartnerSession(tokenHash, partnerID string, expiresAt time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.partnerSess[tokenHash] = partnerSession{PartnerID: partnerID, ExpiresAt: expiresAt}
}

func (m *Memory) PartnerSession(tokenHash string, now time.Time) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.partnerSess[tokenHash]
	if !ok || now.After(s.ExpiresAt) {
		return "", false
	}
	return s.PartnerID, true
}

func (m *Memory) DeletePartnerSession(tokenHash string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.partnerSess, tokenHash)
}
