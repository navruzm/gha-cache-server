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

func TestDownload_MergedSetsContentLengthAndAcceptsRange(t *testing.T) {
	srv, svc, _ := newTestServer(t)
	ctx := context.Background()
	u, _ := svc.CreateUpload(ctx, "k", "v", "main", "42")
	_ = svc.UploadPart(ctx, u.ID, 0, strings.NewReader("hello "))
	_ = svc.UploadPart(ctx, u.ID, 1, strings.NewReader("world"))
	_, _ = svc.CompleteUpload(ctx, "k", "v", "main", "42")

	rows, _ := svc.Match(ctx, "k", "v", "main", "42")

	resp, err := http.Get(srv.URL + "/download/" + rows.Entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "hello world" {
		t.Fatalf("first download body=%q", body)
	}
	svc.WaitForOngoingMerges(ctx)

	resp, err = http.Get(srv.URL + "/download/" + rows.Entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if cl := resp.Header.Get("Content-Length"); cl != "11" {
		t.Errorf("expected Content-Length=11, got %q", cl)
	}
	if ar := resp.Header.Get("Accept-Ranges"); ar != "bytes" {
		t.Errorf("expected Accept-Ranges=bytes, got %q", ar)
	}

	req, _ := http.NewRequest("GET", srv.URL+"/download/"+rows.Entry.ID, nil)
	req.Header.Set("Range", "bytes=6-10")
	rangeResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer rangeResp.Body.Close()
	if rangeResp.StatusCode != http.StatusPartialContent {
		t.Fatalf("expected 206, got %d", rangeResp.StatusCode)
	}
	rangeBody, _ := io.ReadAll(rangeResp.Body)
	if string(rangeBody) != "world" {
		t.Errorf("range body=%q, want %q", rangeBody, "world")
	}
}
