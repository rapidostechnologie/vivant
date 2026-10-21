// Package mrz lit et génère la zone de lecture automatique (MRZ) des cartes
// d'identité au format TD1 (norme OACI 9303) : 3 lignes de 30 caractères.
// C'est le format utilisé par la carte d'identité biométrique CEDEAO.
package mrz

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var (
	ErrFormat     = errors.New("format MRZ invalide")
	ErrCheckDigit = errors.New("chiffre de contrôle MRZ invalide")
)

const lineLen = 30

// TD1 contient les données lues dans la MRZ d'une carte d'identité.
type TD1 struct {
	DocumentCode   string
	IssuingState   string
	DocumentNumber string
	OptionalData1  string
	BirthDate      time.Time
	Sex            string // "M", "F" ou "X"
	ExpiryDate     time.Time
	Nationality    string
	OptionalData2  string
	Surname        string
	GivenNames     string
}

func charValue(c byte) (int, bool) {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0'), true
	case c >= 'A' && c <= 'Z':
		return int(c-'A') + 10, true
	case c == '<':
		return 0, true
	}
	return 0, false
}

// CheckDigit calcule le chiffre de contrôle OACI (pondération 7-3-1, modulo 10).
func CheckDigit(s string) byte {
	weights := [3]int{7, 3, 1}
	sum := 0
	for i := 0; i < len(s); i++ {
		v, _ := charValue(s[i])
		sum += v * weights[i%3]
	}
	return byte('0' + sum%10)
}

// ParseTD1 analyse une MRZ TD1 (3 lignes, ou une seule chaîne de 90 caractères)
// et vérifie tous ses chiffres de contrôle. now sert à déterminer le siècle
// de la date de naissance.
func ParseTD1(raw string, now time.Time) (*TD1, error) {
	lines, err := splitLines(raw)
	if err != nil {
		return nil, err
	}
	l1, l2, l3 := lines[0], lines[1], lines[2]

	docNumber, docCheck, opt1 := l1[5:14], l1[14], l1[15:30]
	if docCheck == '<' {
		// Numéro de document de plus de 9 caractères : la suite et le
		// chiffre de contrôle se trouvent dans les données facultatives.
		end := strings.IndexByte(opt1, '<')
		if end < 0 {
			end = len(opt1)
		}
		if end < 1 {
			return nil, fmt.Errorf("%w : numéro de document étendu", ErrFormat)
		}
		docNumber += opt1[:end-1]
		docCheck = opt1[end-1]
		opt1 = opt1[end:]
	}
	if CheckDigit(docNumber) != docCheck {
		return nil, fmt.Errorf("%w : numéro de document", ErrCheckDigit)
	}
	if CheckDigit(l2[0:6]) != l2[6] {
		return nil, fmt.Errorf("%w : date de naissance", ErrCheckDigit)
	}
	if CheckDigit(l2[8:14]) != l2[14] {
		return nil, fmt.Errorf("%w : date d'expiration", ErrCheckDigit)
	}
	if CheckDigit(l1[5:30]+l2[0:7]+l2[8:15]+l2[18:29]) != l2[29] {
		return nil, fmt.Errorf("%w : contrôle global", ErrCheckDigit)
	}

	birth, err := parseDate(l2[0:6], func(yy int) int {
		if yy > now.Year()%100 {
			return 1900 + yy
		}
		return 2000 + yy
	})
	if err != nil {
		return nil, err
	}
	expiry, err := parseDate(l2[8:14], func(yy int) int { return 2000 + yy })
	if err != nil {
		return nil, err
	}

	var sex string
	switch l2[7] {
	case 'M':
		sex = "M"
	case 'F':
		sex = "F"
	case '<', 'X':
		sex = "X"
	default:
		return nil, fmt.Errorf("%w : sexe", ErrFormat)
	}

	surname, given, _ := strings.Cut(l3, "<<")
	t := &TD1{
		DocumentCode:   unfill(l1[0:2]),
		IssuingState:   unfill(l1[2:5]),
		DocumentNumber: unfill(docNumber),
		OptionalData1:  unfill(opt1),
		BirthDate:      birth,
		Sex:            sex,
		ExpiryDate:     expiry,
		Nationality:    unfill(l2[15:18]),
		OptionalData2:  unfill(l2[18:29]),
		Surname:        unfill(surname),
		GivenNames:     unfill(given),
	}
	if t.Surname == "" {
		return nil, fmt.Errorf("%w : nom absent", ErrFormat)
	}
	return t, nil
}

// BuildTD1 génère une MRZ TD1 valide. Utilisé par les tests et le simulateur
// de téléphone pour imiter la lecture d'une vraie carte.
func BuildTD1(t TD1) (string, error) {
	if len(t.DocumentNumber) > 9 {
		return "", errors.New("mrz : numéro de document de plus de 9 caractères non pris en charge")
	}
	sex := t.Sex
	if sex == "" || sex == "X" {
		sex = "<"
	}
	l1 := fill(t.DocumentCode, 2) + fill(t.IssuingState, 3) + fill(t.DocumentNumber, 9)
	l1 += string(CheckDigit(fill(t.DocumentNumber, 9))) + fill(t.OptionalData1, 15)

	birth, exp := t.BirthDate.Format("060102"), t.ExpiryDate.Format("060102")
	l2 := birth + string(CheckDigit(birth)) + sex + exp + string(CheckDigit(exp)) +
		fill(t.Nationality, 3) + fill(t.OptionalData2, 11)
	l2 += string(CheckDigit(l1[5:30] + l2[0:7] + l2[8:15] + l2[18:29]))

	l3 := fill(nameField(t.Surname)+"<<"+nameField(t.GivenNames), lineLen)
	return l1 + "\n" + l2 + "\n" + l3, nil
}

var accents = strings.NewReplacer(
	"À", "A", "Á", "A", "Â", "A", "Ã", "A", "Ä", "A", "Å", "A", "Æ", "AE",
	"Ç", "C", "È", "E", "É", "E", "Ê", "E", "Ë", "E",
	"Ì", "I", "Í", "I", "Î", "I", "Ï", "I", "Ñ", "N",
	"Ò", "O", "Ó", "O", "Ô", "O", "Õ", "O", "Ö", "O", "Œ", "OE",
	"Ù", "U", "Ú", "U", "Û", "U", "Ü", "U", "Ý", "Y", "Ÿ", "Y",
)

// NormalizeName met un nom en majuscules sans accents, les séparateurs
// (espaces, tirets, apostrophes) étant réduits à un seul espace.
func NormalizeName(s string) string {
	s = accents.Replace(strings.ToUpper(s))
	var b strings.Builder
	sep := false
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			if sep && b.Len() > 0 {
				b.WriteByte(' ')
			}
			sep = false
			b.WriteRune(r)
		} else {
			sep = true
		}
	}
	return b.String()
}

// NamesMatch compare un nom de l'état civil avec celui lu dans la MRZ,
// qui peut être tronqué lorsque la ligne est pleine.
func NamesMatch(reference, fromMRZ string) bool {
	a, b := NormalizeName(reference), NormalizeName(fromMRZ)
	if a == "" || b == "" {
		return false
	}
	return a == b || (len(b) >= 3 && strings.HasPrefix(a, b))
}

func splitLines(raw string) ([]string, error) {
	var lines []string
	for _, line := range strings.Split(strings.ToUpper(raw), "\n") {
		line = strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return -1
			}
			return r
		}, line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 1 && len(lines[0]) == 3*lineLen {
		l := lines[0]
		lines = []string{l[:lineLen], l[lineLen : 2*lineLen], l[2*lineLen:]}
	}
	if len(lines) != 3 {
		return nil, fmt.Errorf("%w : 3 lignes attendues, %d reçues", ErrFormat, len(lines))
	}
	for i, l := range lines {
		for j := 0; j < len(l); j++ {
			if _, ok := charValue(l[j]); !ok {
				return nil, fmt.Errorf("%w : caractère interdit ligne %d", ErrFormat, i+1)
			}
		}
		if len(l) != lineLen {
			return nil, fmt.Errorf("%w : la ligne %d contient %d caractères au lieu de 30", ErrFormat, i+1, len(l))
		}
	}
	return lines, nil
}

func parseDate(s string, year func(yy int) int) (time.Time, error) {
	yy, err1 := strconv.Atoi(s[0:2])
	mm, err2 := strconv.Atoi(s[2:4])
	dd, err3 := strconv.Atoi(s[4:6])
	if err1 != nil || err2 != nil || err3 != nil {
		return time.Time{}, fmt.Errorf("%w : date %q", ErrFormat, s)
	}
	t := time.Date(year(yy), time.Month(mm), dd, 0, 0, 0, 0, time.UTC)
	if t.Month() != time.Month(mm) || t.Day() != dd {
		return time.Time{}, fmt.Errorf("%w : date %q", ErrFormat, s)
	}
	return t, nil
}

func unfill(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "<", " ")), " ")
}

func fill(s string, n int) string {
	s = strings.ReplaceAll(strings.ToUpper(s), " ", "<")
	if len(s) >= n {
		return s[:n]
	}
	return s + strings.Repeat("<", n-len(s))
}

func nameField(s string) string {
	return strings.ReplaceAll(NormalizeName(s), " ", "<")
}
