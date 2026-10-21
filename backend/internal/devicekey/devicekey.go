// Package devicekey gère les clés ECDSA P-256 liées au téléphone.
//
// La clé privée est créée dans la puce sécurisée du téléphone (Secure
// Enclave sur iOS, StrongBox/Keystore sur Android), n'en sort jamais et
// n'est utilisable qu'après déverrouillage par PIN ou biométrie. Le serveur
// ne connaît que la clé publique.
package devicekey

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"math/big"
	"strings"
)

var (
	ErrInvalidKey       = errors.New("clé publique invalide")
	ErrInvalidSignature = errors.New("signature invalide")
)

// En-tête DER SubjectPublicKeyInfo d'une clé P-256, à placer devant un point
// non compressé (format X9.63 renvoyé par la Secure Enclave d'iOS).
var p256SPKIPrefix, _ = hex.DecodeString("3059301306072a8648ce3d020106082a8648ce3d030107034200")

func decodeB64(s string) ([]byte, error) {
	s = strings.TrimRight(strings.TrimSpace(s), "=")
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.RawStdEncoding.DecodeString(s)
}

// ParsePublicKey accepte une clé P-256 encodée en base64 (standard ou URL),
// au format SubjectPublicKeyInfo DER (Android) ou point non compressé de
// 65 octets (iOS).
func ParsePublicKey(encoded string) (*ecdsa.PublicKey, error) {
	der, err := decodeB64(encoded)
	if err != nil {
		return nil, ErrInvalidKey
	}
	if len(der) == 65 && der[0] == 0x04 {
		der = append(append([]byte{}, p256SPKIPrefix...), der...)
	}
	pub, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, ErrInvalidKey
	}
	ec, ok := pub.(*ecdsa.PublicKey)
	if !ok || ec.Curve != elliptic.P256() {
		return nil, ErrInvalidKey
	}
	return ec, nil
}

// EncodePublicKey renvoie la clé au format SubjectPublicKeyInfo DER en base64url.
func EncodePublicKey(pub *ecdsa.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(der), nil
}

// Verify vérifie une signature ECDSA-SHA256 au format DER (Android, iOS
// SecKey) ou brut r||s de 64 octets (WebCrypto).
func Verify(pub *ecdsa.PublicKey, message []byte, signature string) error {
	sig, err := decodeB64(signature)
	if err != nil {
		return ErrInvalidSignature
	}
	digest := sha256.Sum256(message)
	if len(sig) == 64 {
		r := new(big.Int).SetBytes(sig[:32])
		s := new(big.Int).SetBytes(sig[32:])
		if ecdsa.Verify(pub, digest[:], r, s) {
			return nil
		}
	}
	if ecdsa.VerifyASN1(pub, digest[:], sig) {
		return nil
	}
	return ErrInvalidSignature
}

// Sign signe un message (DER, base64url). Sur un vrai téléphone, cette
// opération est faite par la puce sécurisée ; cette fonction sert aux tests
// et au simulateur.
func Sign(priv *ecdsa.PrivateKey, message []byte) (string, error) {
	digest := sha256.Sum256(message)
	sig, err := ecdsa.SignASN1(rand.Reader, priv, digest[:])
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(sig), nil
}

// EnrollmentMessage est signé à la fin de l'inscription pour prouver la
// possession de la clé privée.
func EnrollmentMessage(enrollmentID string) []byte {
	return []byte("vivant-enroll:" + enrollmentID)
}

// ApprovalMessage est signé lorsque l'utilisateur accepte une demande de
// connexion. La signature est conservée comme preuve du consentement.
func ApprovalMessage(requestID, challenge, bindingCode string) []byte {
	return []byte("vivant-approve:" + requestID + ":" + challenge + ":" + bindingCode)
}

// RequestMessage est signé pour chaque appel à l'API appareil.
func RequestMessage(method, path, timestamp string, body []byte) []byte {
	h := sha256.Sum256(body)
	return []byte(method + "\n" + path + "\n" + timestamp + "\n" + hex.EncodeToString(h[:]))
}
