package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func sign(t *testing.T, alg string, kid string, claims map[string]any, key any) string {
	t.Helper()
	hdr := map[string]any{"alg": alg, "typ": "JWT", "kid": kid}
	hb, _ := json.Marshal(hdr)
	cb, _ := json.Marshal(claims)
	enc := base64.RawURLEncoding.EncodeToString
	signing := enc(hb) + "." + enc(cb)
	var sig []byte
	switch k := key.(type) {
	case *rsa.PrivateKey:
		s, err := signRS256(signing, k)
		if err != nil {
			t.Fatal(err)
		}
		sig = s
	case *ecdsa.PrivateKey:
		s, err := signES256(signing, k)
		if err != nil {
			t.Fatal(err)
		}
		sig = s
	}
	return signing + "." + enc(sig)
}

func TestVerify_RS256(t *testing.T) {
	k, _ := rsa.GenerateKey(rand.Reader, 2048)
	jwt := sign(t, "RS256", "k1", map[string]any{"iss": "issuer", "sub": "x"}, k)
	keyset := &Keyset{keys: map[string]any{"k1": &k.PublicKey}}
	claims, err := Verify(jwt, keyset, "issuer")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims["sub"] != "x" {
		t.Errorf("claims=%+v", claims)
	}
}

func TestVerify_ES256(t *testing.T) {
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	jwt := sign(t, "ES256", "e1", map[string]any{"iss": "issuer"}, k)
	keyset := &Keyset{keys: map[string]any{"e1": &k.PublicKey}}
	if _, err := Verify(jwt, keyset, "issuer"); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestVerify_BadIssuerRejected(t *testing.T) {
	k, _ := rsa.GenerateKey(rand.Reader, 2048)
	jwt := sign(t, "RS256", "k1", map[string]any{"iss": "wrong"}, k)
	keyset := &Keyset{keys: map[string]any{"k1": &k.PublicKey}}
	if _, err := Verify(jwt, keyset, "issuer"); err == nil {
		t.Fatal("expected error for bad issuer")
	}
}

func TestDecodeUnverified(t *testing.T) {
	parts := []string{
		base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)),
		base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"x"}`)),
		"",
	}
	c, err := DecodeUnverified(strings.Join(parts, "."))
	if err != nil || c["sub"] != "x" {
		t.Errorf("DecodeUnverified: %v %+v", err, c)
	}
}
