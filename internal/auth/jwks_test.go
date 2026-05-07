package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func jwksWithRSA(t *testing.T, kid string, k *rsa.PublicKey) string {
	t.Helper()
	body := map[string]any{
		"keys": []map[string]any{{
			"kty": "RSA",
			"kid": kid,
			"alg": "RS256",
			"use": "sig",
			"n":   base64.RawURLEncoding.EncodeToString(k.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString([]byte{0x01, 0x00, 0x01}),
		}},
	}
	b, _ := json.Marshal(body)
	return string(b)
}

func TestFetcher_LoadsAndCaches(t *testing.T) {
	k, _ := rsa.GenerateKey(rand.Reader, 2048)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(jwksWithRSA(t, "k1", &k.PublicKey)))
	}))
	defer srv.Close()

	f := NewJWKSFetcher(srv.URL)
	ks, err := f.Fetch(t.Context())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if ks.Get("k1") == nil {
		t.Error("expected k1")
	}
	_, _ = f.Fetch(t.Context())
	if calls != 1 {
		t.Errorf("expected 1 HTTP call (cache hit), got %d", calls)
	}
}
