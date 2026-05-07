package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
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

func newMgmtServer(t *testing.T, apiKey string) (*httptest.Server, *storage.Service) {
	t.Helper()
	raw, _ := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)")
	t.Cleanup(func() { raw.Close() })
	raw.SetMaxOpenConns(1)
	d := &dbpkg.DB{DB: raw, Driver: dbpkg.SQLite}
	_ = dbpkg.Migrate(context.Background(), d)
	q := dbpkg.New(d)
	a, _ := storage.NewFilesystemAdapter(t.TempDir())
	cfg := &config.Config{APIBaseURL: "http://localhost:3000", ManagementAPIKey: apiKey}
	svc := storage.NewService(q, a, storage.ServiceConfig{})
	v := auth.NewVerifier(nil, "issuer", true)
	h := NewHandler(Deps{Cfg: cfg, Logger: logging.New(false), Storage: svc, Verifier: v})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, svc
}

func TestManagement_RequiresAPIKey(t *testing.T) {
	srv, _ := newMgmtServer(t, "secret")
	resp, err := http.Get(srv.URL + "/management-api/cache-entries")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("status=%d", resp.StatusCode)
	}
}

func TestManagement_503IfDisabled(t *testing.T) {
	srv, _ := newMgmtServer(t, "")
	resp, err := http.Get(srv.URL + "/management-api/cache-entries")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Errorf("status=%d", resp.StatusCode)
	}
}

func TestManagement_ListAndGet(t *testing.T) {
	srv, svc := newMgmtServer(t, "secret")
	ctx := context.Background()
	u, _ := svc.CreateUpload(ctx, "k", "v", "s", "r")
	_ = svc.UploadPart(ctx, u.ID, 0, strings.NewReader("x"))
	_, _ = svc.CompleteUpload(ctx, "k", "v", "s", "r")

	req, _ := http.NewRequest("GET", srv.URL+"/management-api/cache-entries", nil)
	req.Header.Set("X-Api-Key", "secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	var page struct {
		Total int              `json:"total"`
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal(body, &page)
	if page.Total != 1 || len(page.Items) != 1 {
		t.Errorf("got %+v", page)
	}
}

func TestManagement_DocsSpecJSON(t *testing.T) {
	srv, _ := newMgmtServer(t, "secret")
	resp, err := http.Get(srv.URL + "/management-api/_docs/spec.json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var doc map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&doc)
	if doc["openapi"] != "3.1.0" {
		t.Errorf("openapi field = %v", doc["openapi"])
	}
}

func TestManagement_DocsHTML(t *testing.T) {
	srv, _ := newMgmtServer(t, "secret")
	resp, err := http.Get(srv.URL + "/management-api/_docs")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !bytes.Contains(body, []byte("swagger-ui")) {
		t.Errorf("status=%d body has swagger-ui? %v", resp.StatusCode, bytes.Contains(body, []byte("swagger-ui")))
	}
}
