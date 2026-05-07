package server

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestDownload_StreamsCacheEntry(t *testing.T) {
	srv, svc, _ := newTestServer(t)
	ctx := context.Background()
	u, _ := svc.CreateUpload(ctx, "k", "v", "main", "42")
	_ = svc.UploadPart(ctx, u.ID, 0, strings.NewReader("hello "))
	_ = svc.UploadPart(ctx, u.ID, 1, strings.NewReader("world"))
	_, _ = svc.CompleteUpload(ctx, "k", "v", "main", "42")

	rows, err := svc.Match(ctx, "k", "v", "main", "42")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(srv.URL + "/download/" + rows.Entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || string(body) != "hello world" {
		t.Errorf("status=%d body=%q", resp.StatusCode, body)
	}
}

func TestDownload_NotFound(t *testing.T) {
	srv, _, _ := newTestServer(t)
	resp, err := http.Get(srv.URL + "/download/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}
