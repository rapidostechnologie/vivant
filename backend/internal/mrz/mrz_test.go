package mrz

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// Exemple officiel de la norme OACI 9303, partie 5.
const icaoSample = "I<UTOD231458907<<<<<<<<<<<<<<<\n7408122F1204159UTO<<<<<<<<<<<6\nERIKSSON<<ANNA<MARIA<<<<<<<<<<"

func TestParseICAOSample(t *testing.T) {
	td, err := ParseTD1(icaoSample, time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if td.DocumentNumber != "D23145890" || td.IssuingState != "UTO" || td.Sex != "F" {
		t.Errorf("champs inattendus : %+v", td)
	}
	if td.Surname != "ERIKSSON" || td.GivenNames != "ANNA MARIA" {
		t.Errorf("nom inattendu : %q / %q", td.Surname, td.GivenNames)
	}
	if !td.BirthDate.Equal(time.Date(1974, 8, 12, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("date de naissance : %v", td.BirthDate)
	}
	if !td.ExpiryDate.Equal(time.Date(2012, 4, 15, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("date d'expiration : %v", td.ExpiryDate)
	}
}

func TestParseRejectsAlteredMRZ(t *testing.T) {
	// Date de naissance modifiée (1974 -> 1975) sans recalcul des contrôles.
	altered := strings.Replace(icaoSample, "7408122F", "7508122F", 1)
	_, err := ParseTD1(altered, time.Now())
	if !errors.Is(err, ErrCheckDigit) {
		t.Fatalf("erreur attendue ErrCheckDigit, obtenu %v", err)
	}
}

func TestParseAcceptsSingleLineAndSpaces(t *testing.T) {
	single := strings.ReplaceAll(icaoSample, "\n", "")
	if _, err := ParseTD1(single, time.Now()); err != nil {
		t.Fatal(err)
	}
	spaced := strings.ReplaceAll(icaoSample, "<<<", "< < <")
	if _, err := ParseTD1(spaced, time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestBuildRoundTrip(t *testing.T) {
	in := TD1{
		DocumentCode:   "ID",
		IssuingState:   "SEN",
		DocumentNumber: "A01234567",
		OptionalData1:  "2751199000123",
		BirthDate:      time.Date(1990, 3, 14, 0, 0, 0, 0, time.UTC),
		Sex:            "F",
		ExpiryDate:     time.Date(2031, 5, 20, 0, 0, 0, 0, time.UTC),
		Nationality:    "SEN",
		Surname:        "Bâ",
		GivenNames:     "Aïssatou Ndèye",
	}
	raw, err := BuildTD1(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := ParseTD1(raw, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("%v\n%s", err, raw)
	}
	if out.DocumentNumber != in.DocumentNumber || out.OptionalData1 != in.OptionalData1 {
		t.Errorf("numéros : %+v", out)
	}
	if !out.BirthDate.Equal(in.BirthDate) || !out.ExpiryDate.Equal(in.ExpiryDate) {
		t.Errorf("dates : %v %v", out.BirthDate, out.ExpiryDate)
	}
	if out.Surname != "BA" || out.GivenNames != "AISSATOU NDEYE" {
		t.Errorf("noms : %q %q", out.Surname, out.GivenNames)
	}
}

func TestNamesMatch(t *testing.T) {
	cases := []struct {
		ref, mrz string
		want     bool
	}{
		{"Bâ", "BA", true},
		{"N'Diaye", "N DIAYE", true},
		{"Sow-Fall", "SOW FALL", true},
		{"Diop", "DIOUF", false},
		{"Abdoulaye Mbacké", "ABDOULAYE MBA", true}, // MRZ tronquée
		{"Diop", "", false},
	}
	for _, c := range cases {
		if got := NamesMatch(c.ref, c.mrz); got != c.want {
			t.Errorf("NamesMatch(%q, %q) = %v, attendu %v", c.ref, c.mrz, got, c.want)
		}
	}
}
