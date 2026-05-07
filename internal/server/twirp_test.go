package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/navruzm/github-actions-cache-server-go/internal/auth"
	"github.com/navruzm/github-actions-cache-server-go/internal/config"
	dbpkg "github.com/navruzm/github-actions-cache-server-go/internal/db"
	"github.com/navruzm/github-actions-cache-server-go/internal/logging"
	"github.com/navruzm/github-actions-cache-server-go/internal/storage"

	_ "modernc.org/sqlite"
)

func newTestServer(t *testing.T) (*httptest.Server, *storage.Service, string) {
	t.Helper()
	raw, _ := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)")
	t.Cleanup(func() { raw.Close() })
	raw.SetMaxOpenConns(1)
	d := &dbpkg.DB{DB: raw, Driver: dbpkg.SQLite}
	if err := dbpkg.Migrate(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	q := dbpkg.New(d)
	a, _ := storage.NewFilesystemAdapter(t.TempDir())
	cfg := &config.Config{
		APIBaseURL:          "http://localhost:3000",
		SkipTokenValidation: true,
	}
	svc := storage.NewService(q, a, storage.ServiceConfig{APIBaseURL: cfg.APIBaseURL})
	v := auth.NewVerifier(nil, "issuer", true)
	h := NewHandler(Deps{Cfg: cfg, Logger: logging.New(false), Storage: svc, Verifier: v})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	tok := makeUnsignedToken(t, []auth.Scope{{Scope: "main", Permission: 3}}, "42")
	return srv, svc, tok
}

func makeUnsignedToken(t *testing.T, scopes []auth.Scope, repoID string) string {
	t.Helper()
	_, _ = rsa.GenerateKey(rand.Reader, 2048)
	ac, _ := json.Marshal(scopes)
	header := `{"alg":"none","typ":"JWT"}`
	payload, _ := json.Marshal(map[string]any{
		"ac": string(ac), "repository_id": repoID,
	})
	enc := func(b []byte) string {
		return strings.TrimRight(strings.NewReplacer("+", "-", "/", "_").Replace(b64(b)), "=")
	}
	return enc([]byte(header)) + "." + enc(payload) + "."
}

func b64(b []byte) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	out := make([]byte, 0, ((len(b)+2)/3)*4)
	for i := 0; i < len(b); i += 3 {
		var n uint32
		switch len(b) - i {
		case 1:
			n = uint32(b[i]) << 16
			out = append(out, alphabet[(n>>18)&0x3f], alphabet[(n>>12)&0x3f], '=', '=')
		case 2:
			n = uint32(b[i])<<16 | uint32(b[i+1])<<8
			out = append(out, alphabet[(n>>18)&0x3f], alphabet[(n>>12)&0x3f], alphabet[(n>>6)&0x3f], '=')
		default:
			n = uint32(b[i])<<16 | uint32(b[i+1])<<8 | uint32(b[i+2])
			out = append(out, alphabet[(n>>18)&0x3f], alphabet[(n>>12)&0x3f], alphabet[(n>>6)&0x3f], alphabet[n&0x3f])
		}
	}
	return string(out)
}

func TestTwirp_CreateCacheEntry(t *testing.T) {
	srv, _, tok := newTestServer(t)
	body, _ := json.Marshal(map[string]any{"key": "k", "version": "v"})
	req, _ := http.NewRequest("POST",
		srv.URL+"/twirp/github.actions.results.api.v1.CacheService/CreateCacheEntry",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var got map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got["ok"] != true {
		t.Errorf("expected ok=true, got %+v", got)
	}
	url, _ := got["signed_upload_url"].(string)
	if !strings.HasPrefix(url, "http://localhost:3000/devstoreaccount1/upload/") {
		t.Errorf("signed_upload_url = %q", url)
	}
}

func TestTwirp_FinalizeAndGet(t *testing.T) {
	srv, svc, tok := newTestServer(t)
	u, _ := svc.CreateUpload(context.Background(), "k", "v", "main", "42")
	_ = svc.UploadPart(context.Background(), u.ID, 0, strings.NewReader("hi"))

	body, _ := json.Marshal(map[string]any{"key": "k", "version": "v"})
	req, _ := http.NewRequest("POST",
		srv.URL+"/twirp/github.actions.results.api.v1.CacheService/FinalizeCacheEntryUpload",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("finalize status=%d", resp.StatusCode)
	}

	body, _ = json.Marshal(map[string]any{"key": "k", "version": "v"})
	req, _ = http.NewRequest("POST",
		srv.URL+"/twirp/github.actions.results.api.v1.CacheService/GetCacheEntryDownloadURL",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("get status=%d", resp.StatusCode)
	}
	var got map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got["ok"] != true || got["matched_key"] != "k" {
		t.Errorf("got %+v", got)
	}
}

func TestTwirp_GetMissReturnsMessage(t *testing.T) {
	srv, _, tok := newTestServer(t)
	body, _ := json.Marshal(map[string]any{"key": "missing", "version": "v"})
	req, _ := http.NewRequest("POST",
		srv.URL+"/twirp/github.actions.results.api.v1.CacheService/GetCacheEntryDownloadURL",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var got map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got["ok"] != false {
		t.Errorf("expected ok=false, got %+v", got)
	}
	if msg, _ := got["message"].(string); msg == "" {
		t.Errorf("expected non-empty message, got %+v", got)
	}
}

func TestTwirp_FinalizeMissingUploadReturns200WithMessage(t *testing.T) {
	srv, _, tok := newTestServer(t)
	body, _ := json.Marshal(map[string]any{"key": "nope", "version": "v", "size_bytes": "1234"})
	req, _ := http.NewRequest("POST",
		srv.URL+"/twirp/github.actions.results.api.v1.CacheService/FinalizeCacheEntryUpload",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d (expected 200 with ok=false body)", resp.StatusCode)
	}
	var got map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got["ok"] != false {
		t.Errorf("expected ok=false, got %+v", got)
	}
	if msg, _ := got["message"].(string); msg == "" {
		t.Errorf("expected non-empty message, got %+v", got)
	}
}

func TestTwirp_CreateDuplicateReturnsMessage(t *testing.T) {
	srv, svc, tok := newTestServer(t)
	_, _ = svc.CreateUpload(context.Background(), "dup", "v", "main", "42")

	body, _ := json.Marshal(map[string]any{"key": "dup", "version": "v"})
	req, _ := http.NewRequest("POST",
		srv.URL+"/twirp/github.actions.results.api.v1.CacheService/CreateCacheEntry",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got["ok"] != false {
		t.Errorf("expected ok=false for duplicate, got %+v", got)
	}
	if msg, _ := got["message"].(string); msg == "" {
		t.Errorf("expected non-empty message, got %+v", got)
	}
}
