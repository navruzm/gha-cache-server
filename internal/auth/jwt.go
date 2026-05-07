package auth

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

type Claims map[string]any

type Keyset struct {
	keys map[string]any
}

func (k *Keyset) Get(kid string) any { return k.keys[kid] }

var (
	ErrMalformedToken   = errors.New("malformed token")
	ErrUnsupportedAlg   = errors.New("unsupported algorithm")
	ErrInvalidSignature = errors.New("invalid signature")
	ErrIssuerMismatch   = errors.New("issuer mismatch")
	ErrTokenExpired     = errors.New("token expired")
	ErrUnknownKey       = errors.New("unknown signing key")
)

func DecodeUnverified(token string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrMalformedToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	var c Claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, fmt.Errorf("unmarshal payload: %w", err)
	}
	return c, nil
}

func Verify(token string, ks *Keyset, expectedIssuer string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrMalformedToken
	}
	hdrBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode header: %w", err)
	}
	var hdr struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(hdrBytes, &hdr); err != nil {
		return nil, fmt.Errorf("unmarshal header: %w", err)
	}
	key := ks.Get(hdr.Kid)
	if key == nil {
		return nil, fmt.Errorf("%w: kid=%q", ErrUnknownKey, hdr.Kid)
	}
	signed := []byte(parts[0] + "." + parts[1])
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decode signature: %w", err)
	}

	switch hdr.Alg {
	case "RS256":
		pk, ok := key.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("%w: kid %q is not RSA", ErrUnsupportedAlg, hdr.Kid)
		}
		h := sha256.Sum256(signed)
		if err := rsa.VerifyPKCS1v15(pk, crypto.SHA256, h[:], sig); err != nil {
			return nil, ErrInvalidSignature
		}
	case "ES256":
		pk, ok := key.(*ecdsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("%w: kid %q is not ECDSA", ErrUnsupportedAlg, hdr.Kid)
		}
		if len(sig) != 64 {
			return nil, ErrInvalidSignature
		}
		r := new(big.Int).SetBytes(sig[:32])
		s := new(big.Int).SetBytes(sig[32:])
		h := sha256.Sum256(signed)
		if !ecdsa.Verify(pk, h[:], r, s) {
			return nil, ErrInvalidSignature
		}
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedAlg, hdr.Alg)
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var c Claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, err
	}

	if iss, _ := c["iss"].(string); iss != expectedIssuer {
		return nil, fmt.Errorf("%w: got %q want %q", ErrIssuerMismatch, iss, expectedIssuer)
	}
	if expF, ok := c["exp"].(float64); ok {
		if int64(expF) < time.Now().Unix() {
			return nil, ErrTokenExpired
		}
	}
	return c, nil
}

func signRS256(input string, k *rsa.PrivateKey) ([]byte, error) {
	h := sha256.Sum256([]byte(input))
	return rsa.SignPKCS1v15(nil, k, crypto.SHA256, h[:])
}

func signES256(input string, k *ecdsa.PrivateKey) ([]byte, error) {
	h := sha256.Sum256([]byte(input))
	r, s, err := ecdsa.Sign(nilReader{}, k, h[:])
	if err != nil {
		return nil, err
	}
	out := make([]byte, 64)
	r.FillBytes(out[:32])
	s.FillBytes(out[32:])
	return out, nil
}

type nilReader struct{}

func (nilReader) Read(b []byte) (int, error) {
	return cryptoRand(b)
}
