// Package jwt signe et vérifie des jetons JWT RS256 et publie les clés
// publiques au format JWKS, sans dépendance externe.
package jwt

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
)

var ErrInvalidToken = errors.New("jeton invalide")

var b64 = base64.RawURLEncoding

// LoadOrGenerateKey charge la clé de signature RSA depuis path (PEM PKCS#8),
// ou en génère une nouvelle et l'enregistre. En production, la clé doit
// résider dans un HSM.
func LoadOrGenerateKey(path string) (*rsa.PrivateKey, error) {
	if path != "" {
		data, err := os.ReadFile(path)
		if err == nil {
			block, _ := pem.Decode(data)
			if block == nil {
				return nil, fmt.Errorf("%s : PEM invalide", path)
			}
			k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
			if err != nil {
				return nil, err
			}
			rk, ok := k.(*rsa.PrivateKey)
			if !ok {
				return nil, fmt.Errorf("%s : clé RSA attendue", path)
			}
			return rk, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	if path != "" {
		der, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600); err != nil {
			return nil, err
		}
	}
	return key, nil
}

type Signer struct {
	key *rsa.PrivateKey
	kid string
}

func NewSigner(key *rsa.PrivateKey) *Signer {
	sum := sha256.Sum256(key.PublicKey.N.Bytes())
	return &Signer{key: key, kid: b64.EncodeToString(sum[:8])}
}

// Sign sérialise claims en JSON et renvoie le JWT signé.
func (s *Signer) Sign(claims any) (string, error) {
	header, err := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT", "kid": s.kid})
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	input := b64.EncodeToString(header) + "." + b64.EncodeToString(payload)
	digest := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return input + "." + b64.EncodeToString(sig), nil
}

type JWK struct {
	Kty string `json:"kty"`
	Use string `json:"use,omitempty"`
	Alg string `json:"alg,omitempty"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type JWKSet struct {
	Keys []JWK `json:"keys"`
}

func (s *Signer) JWKS() JWKSet {
	pub := s.key.PublicKey
	return JWKSet{Keys: []JWK{{
		Kty: "RSA",
		Use: "sig",
		Alg: "RS256",
		Kid: s.kid,
		N:   b64.EncodeToString(pub.N.Bytes()),
		E:   b64.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}}}
}

func (k JWK) RSAPublicKey() (*rsa.PublicKey, error) {
	if k.Kty != "RSA" {
		return nil, fmt.Errorf("type de clé %q non pris en charge", k.Kty)
	}
	n, err := b64.DecodeString(k.N)
	if err != nil {
		return nil, err
	}
	e, err := b64.DecodeString(k.E)
	if err != nil {
		return nil, err
	}
	exp := new(big.Int).SetBytes(e)
	if !exp.IsInt64() || exp.Int64() < 3 || exp.Int64() > 1<<31-1 {
		return nil, errors.New("exposant RSA invalide")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(exp.Int64())}, nil
}

// Verify vérifie la signature RS256 d'un jeton avec les clés fournies et
// renvoie ses claims. La validation de iss, aud, exp et nonce reste à la
// charge de l'appelant.
func Verify(token string, keys JWKSet) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}
	rawHeader, err := b64.DecodeString(parts[0])
	if err != nil {
		return nil, ErrInvalidToken
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(rawHeader, &header); err != nil || header.Alg != "RS256" {
		return nil, ErrInvalidToken
	}
	sig, err := b64.DecodeString(parts[2])
	if err != nil {
		return nil, ErrInvalidToken
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	verified := false
	for _, k := range keys.Keys {
		if k.Kid != header.Kid {
			continue
		}
		pub, err := k.RSAPublicKey()
		if err != nil {
			continue
		}
		if rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig) == nil {
			verified = true
			break
		}
	}
	if !verified {
		return nil, ErrInvalidToken
	}
	payload, err := b64.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
