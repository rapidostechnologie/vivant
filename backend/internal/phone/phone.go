// Package phone normalise les numéros de téléphone au format E.164.
package phone

import (
	"errors"
	"strings"
)

var ErrInvalid = errors.New("numéro de téléphone invalide")

// DefaultCountryCode est l'indicatif appliqué aux numéros nationaux
// (Sénégal : 9 chiffres, ex. 77 123 45 67).
const DefaultCountryCode = "221"

// Normalize convertit un numéro au format E.164 (+221771234567). Les numéros
// internationaux préfixés par + ou 00 sont acceptés pour les autres pays.
func Normalize(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	international := strings.HasPrefix(s, "+")
	d := Digits(s)
	if !international && strings.HasPrefix(d, "00") {
		d, international = d[2:], true
	}
	if !international {
		switch {
		case len(d) == 9:
			return "+" + DefaultCountryCode + d, nil
		case len(d) == 12 && strings.HasPrefix(d, DefaultCountryCode):
			return "+" + d, nil
		}
		return "", ErrInvalid
	}
	if len(d) < 8 || len(d) > 15 {
		return "", ErrInvalid
	}
	return "+" + d, nil
}

// Digits ne garde que les chiffres de s.
func Digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
