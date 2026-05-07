package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilesystem_UploadDownloadDelete(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	a, err := NewFilesystemAdapter(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := a.UploadStream(ctx, "1234/parts/0", strings.NewReader("hello")); err != nil {
		t.Fatalf("UploadStream: %v", err)
	}
	r, err := a.CreateDownloadStream(ctx, "1234/parts/0")
	if err != nil {
		t.Fatalf("CreateDownloadStream: %v", err)
	}
	defer r.Close()
	got, _ := io.ReadAll(r)
	if string(got) != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}

	n, err := a.CountFilesInFolder(ctx, "1234/parts")
	if err != nil || n != 1 {
		t.Errorf("CountFiles=%d err=%v", n, err)
	}

	if err := a.DeleteFolder(ctx, "1234"); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}
	if _, err := a.CreateDownloadStream(ctx, "1234/parts/0"); !errors.Is(err, ErrObjectNotFound) {
		t.Errorf("expected ErrObjectNotFound, got %v", err)
	}
}

func TestFilesystem_PathTraversalRejected(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	a, _ := NewFilesystemAdapter(dir)
	err := a.UploadStream(ctx, "../escape", bytes.NewReader([]byte("x")))
	if err == nil {
		t.Fatal("expected path traversal to be rejected")
	}
}

func TestFilesystem_Clear(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	a, _ := NewFilesystemAdapter(dir)
	_ = a.UploadStream(ctx, "a/b", strings.NewReader("x"))
	_ = a.Clear(ctx)
	entries, _ := os.ReadDir(filepath.Clean(dir))
	if len(entries) != 0 {
		t.Errorf("expected empty after Clear, got %d entries", len(entries))
	}
}
