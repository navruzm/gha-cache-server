package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/navruzm/gha-cache-server/internal/auth"
	"github.com/navruzm/gha-cache-server/internal/config"
	dbpkg "github.com/navruzm/gha-cache-server/internal/db"
	"github.com/navruzm/gha-cache-server/internal/logging"
	"github.com/navruzm/gha-cache-server/internal/server"
	"github.com/navruzm/gha-cache-server/internal/storage"
	_ "modernc.org/sqlite"
)

func b64url(b []byte) string { return strings.TrimRight(base64.URLEncoding.EncodeToString(b), "=") }

func unsignedToken(scopes []auth.Scope, repoID string) string {
	header := []byte(`{"alg":"none","typ":"JWT"}`)
	ac, _ := json.Marshal(scopes)
	payload, _ := json.Marshal(map[string]any{"ac": string(ac), "repository_id": repoID})
	return b64url(header) + "." + b64url(payload) + "."
}

func TestE2E_Roundtrip(t *testing.T) {
	ctx := context.Background()
	raw, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	raw.SetMaxOpenConns(1)
	d := &dbpkg.DB{DB: raw, Driver: dbpkg.SQLite}
	if err := dbpkg.Migrate(ctx, d); err != nil {
		t.Fatal(err)
	}
	q := dbpkg.New(d)
	a, err := storage.NewFilesystemAdapter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(nil)
	baseURL := "http://" + srv.Listener.Addr().String()
	cfg := &config.Config{APIBaseURL: baseURL, SkipTokenValidation: true}
	svc := storage.NewService(q, a, storage.ServiceConfig{APIBaseURL: cfg.APIBaseURL})
	v := auth.NewVerifier(nil, "issuer", true)
	h := server.NewHandler(server.Deps{Cfg: cfg, Logger: logging.New(false), Storage: svc, Verifier: v})
	srv.Config.Handler = h
	srv.Start()
	defer srv.Close()
	tok := unsignedToken([]auth.Scope{{Scope: "main", Permission: 3}}, "42")

	bodyJSON, _ := json.Marshal(map[string]any{"key": "k1", "version": "v1"})
	req, err := http.NewRequest("POST",
		srv.URL+"/twirp/github.actions.results.api.v1.CacheService/CreateCacheEntry",
		bytes.NewReader(bodyJSON))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("create status=%d body=%s", resp.StatusCode, body)
	}
	var createOut struct {
		OK              bool   `json:"ok"`
		SignedUploadURL string `json:"signed_upload_url"`
	}
	_ = json.Unmarshal(body, &createOut)
	if !createOut.OK {
		t.Fatalf("create not ok: %s", body)
	}

	payload := bytes.Repeat([]byte("AB"), 512)
	blockBytes := make([]byte, 64)
	binary.BigEndian.PutUint32(blockBytes[16:20], 0)
	blockID := base64.StdEncoding.EncodeToString(blockBytes)
	chunkURL := createOut.SignedUploadURL + "?blockid=" + blockID
	req, err = http.NewRequest("PUT", chunkURL, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("upload chunk status=%d", resp.StatusCode)
	}
	req, err = http.NewRequest("PUT", createOut.SignedUploadURL+"?comp=blocklist", bytes.NewReader(nil))
	if err != nil {
		t.Fatal(err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("commit blocklist status=%d", resp.StatusCode)
	}

	bodyJSON, _ = json.Marshal(map[string]any{"key": "k1", "version": "v1"})
	req, err = http.NewRequest("POST",
		srv.URL+"/twirp/github.actions.results.api.v1.CacheService/FinalizeCacheEntryUpload",
		bytes.NewReader(bodyJSON))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("finalize status=%d body=%s", resp.StatusCode, body)
	}
	var finOut struct {
		OK      bool   `json:"ok"`
		EntryID string `json:"entry_id"`
	}
	_ = json.Unmarshal(body, &finOut)
	if !finOut.OK {
		t.Fatalf("finalize not ok: %s", body)
	}
	if _, err := strconv.ParseInt(finOut.EntryID, 10, 64); err != nil {
		t.Errorf("entry_id not numeric: %s", finOut.EntryID)
	}

	bodyJSON, _ = json.Marshal(map[string]any{"key": "k1", "version": "v1"})
	req, err = http.NewRequest("POST",
		srv.URL+"/twirp/github.actions.results.api.v1.CacheService/GetCacheEntryDownloadURL",
		bytes.NewReader(bodyJSON))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	var getOut struct {
		OK                bool   `json:"ok"`
		SignedDownloadURL string `json:"signed_download_url"`
		MatchedKey        string `json:"matched_key"`
	}
	_ = json.Unmarshal(body, &getOut)
	if !getOut.OK || getOut.MatchedKey != "k1" {
		t.Fatalf("get not ok: %s", body)
	}

	resp, err = http.Get(getOut.SignedDownloadURL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(got, payload) {
		t.Errorf("payload mismatch: %d bytes", len(got))
	}

	_ = rand.Reader
}
