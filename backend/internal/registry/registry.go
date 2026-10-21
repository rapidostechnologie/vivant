// Package registry représente la source d'identité de référence : le
// registre national (au Sénégal, le fichier de la DAF) et le registre des
// abonnés mobiles des opérateurs.
//
// Cette version est une SIMULATION avec des citoyens fictifs. L'accès aux
// vrais registres nécessite une convention avec l'État et les opérateurs.
package registry

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"time"

	"vivant/internal/mrz"
)

var ErrNotFound = errors.New("NIN introuvable dans le registre")

// Citizen est une fiche du registre national.
type Citizen struct {
	NIN            string `json:"nin"`
	FamilyName     string `json:"family_name"`
	GivenNames     string `json:"given_names"`
	BirthDate      string `json:"birth_date"` // AAAA-MM-JJ
	BirthPlace     string `json:"birth_place"`
	Sex            string `json:"sex"`         // M ou F
	Nationality    string `json:"nationality"` // code OACI à 3 lettres
	Address        string `json:"address"`
	DocumentNumber string `json:"document_number"`
	DocumentExpiry string `json:"document_expiry"` // AAAA-MM-JJ
	// SIMPhone est le numéro enregistré au nom du citoyen chez son opérateur
	// (identification obligatoire des cartes SIM).
	SIMPhone string `json:"sim_phone"`
}

// Registry interroge la source d'identité de référence.
type Registry interface {
	LookupByNIN(ctx context.Context, nin string) (Citizen, error)
	// LookupByPhone donne le titulaire d'une puce (registre des abonnés).
	LookupByPhone(ctx context.Context, phone string) (Citizen, error)
}

//go:embed fixtures/citoyens.json
var fixturesJSON []byte

// Fixtures renvoie les citoyens fictifs du registre simulé.
func Fixtures() ([]Citizen, error) {
	var cs []Citizen
	if err := json.Unmarshal(fixturesJSON, &cs); err != nil {
		return nil, err
	}
	return cs, nil
}

// Mock est un registre en mémoire alimenté par les citoyens fictifs.
type Mock struct {
	byNIN map[string]Citizen
}

func NewMock() (*Mock, error) {
	cs, err := Fixtures()
	if err != nil {
		return nil, err
	}
	m := &Mock{byNIN: make(map[string]Citizen, len(cs))}
	for _, c := range cs {
		m.byNIN[c.NIN] = c
	}
	return m, nil
}

func (m *Mock) LookupByNIN(_ context.Context, nin string) (Citizen, error) {
	c, ok := m.byNIN[nin]
	if !ok {
		return Citizen{}, ErrNotFound
	}
	return c, nil
}

func (m *Mock) LookupByPhone(_ context.Context, phone string) (Citizen, error) {
	for _, c := range m.byNIN {
		if c.SIMPhone == phone {
			return c, nil
		}
	}
	return Citizen{}, ErrNotFound
}

// SimulatedCardMRZ produit la MRZ imprimée sur la carte physique du citoyen.
// Elle remplace la lecture par la caméra dans les tests et le simulateur.
func SimulatedCardMRZ(c Citizen) (string, error) {
	birth, err := time.Parse(time.DateOnly, c.BirthDate)
	if err != nil {
		return "", err
	}
	expiry, err := time.Parse(time.DateOnly, c.DocumentExpiry)
	if err != nil {
		return "", err
	}
	return mrz.BuildTD1(mrz.TD1{
		DocumentCode:   "ID",
		IssuingState:   c.Nationality,
		DocumentNumber: c.DocumentNumber,
		OptionalData1:  c.NIN,
		BirthDate:      birth,
		Sex:            c.Sex,
		ExpiryDate:     expiry,
		Nationality:    c.Nationality,
		Surname:        c.FamilyName,
		GivenNames:     c.GivenNames,
	})
}
