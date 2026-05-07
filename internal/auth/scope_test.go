package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtractScopes_FromValidToken(t *testing.T) {
	k, _ := rsa.GenerateKey(rand.Reader, 2048)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(jwksWithRSA(t, "k1", &k.PublicKey)))
	}))
	defer srv.Close()
	v := NewVerifier(NewJWKSFetcher(srv.URL), "issuer", false)
	ac, _ := json.Marshal([]Scope{{Scope: "refs/heads/main", Permission: 3}})
	tok := sign(t, "RS256", "k1", map[string]any{
		"iss": "issuer", "ac": string(ac), "repository_id": "42",
	}, k)
	res, err := v.Authorize(context.Background(), "Bearer "+tok)
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if res.RepoID != "42" || len(res.Scopes) != 1 || res.Scopes[0].Permission != 3 {
		t.Errorf("got %+v", res)
	}
}

func TestExtractScopes_SkipsValidation(t *testing.T) {
	v := NewVerifier(nil, "issuer", true)
	ac, _ := json.Marshal([]Scope{{Scope: "x", Permission: 2}})
	parts := []string{
		"e30",
		mustB64Json(map[string]any{"ac": string(ac), "repository_id": "1"}),
		"",
	}
	res, err := v.Authorize(context.Background(), "Bearer "+joinDots(parts))
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if res.RepoID != "1" || len(res.Scopes) != 1 {
		t.Errorf("got %+v", res)
	}
}

func mustB64Json(v any) string {
	b, _ := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(b)
}

func joinDots(parts []string) string { return strings.Join(parts, ".") }
