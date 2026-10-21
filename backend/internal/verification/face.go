// Package verification compare le selfie de l'utilisateur à la photo de
// référence et détecte les tentatives de fraude (photo, écran, masque).
package verification

import "context"

// FaceResult est le résultat d'une vérification faciale.
type FaceResult struct {
	Live  bool    // la détection du vivant a réussi
	Score float64 // similarité avec la photo de référence, entre 0 et 1
}

// FaceVerifier compare un selfie à la photo de référence du citoyen (photo
// de la carte ou photo du registre national).
//
// Implémentations réelles possibles : prestataire KYC (Smile ID, très
// présent en Afrique, Onfido, IDnow…) ou modèle hébergé localement pour
// garder les données biométriques au Sénégal.
type FaceVerifier interface {
	Verify(ctx context.Context, nin string, selfie []byte) (FaceResult, error)
}

// MockFaceVerifier accepte tout selfie non vide. Réservé au développement.
type MockFaceVerifier struct{}

func (MockFaceVerifier) Verify(_ context.Context, _ string, selfie []byte) (FaceResult, error) {
	if len(selfie) < 16 {
		return FaceResult{Live: false}, nil
	}
	return FaceResult{Live: true, Score: 0.93}, nil
}
