package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type FilesystemAdapter struct {
	root string
}

func NewFilesystemAdapter(root string) (*FilesystemAdapter, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	return &FilesystemAdapter{root: abs}, nil
}

func (a *FilesystemAdapter) safe(name string) (string, error) {
	cleaned := filepath.Clean(name)
	if cleaned == "." {
		return a.root, nil
	}
	resolved := filepath.Join(a.root, cleaned)
	rel, err := filepath.Rel(a.root, resolved)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid object name: %q", name)
	}
	return resolved, nil
}

func (a *FilesystemAdapter) CreateDownloadStream(_ context.Context, objectName string) (io.ReadCloser, error) {
	p, err := a.safe(objectName)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrObjectNotFound
	}
	return f, err
}

func (a *FilesystemAdapter) UploadStream(_ context.Context, objectName string, body io.Reader) error {
	p, err := a.safe(objectName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, body); err != nil {
		return err
	}
	return f.Sync()
}

func (a *FilesystemAdapter) DeleteFolder(_ context.Context, folderName string) error {
	p, err := a.safe(folderName)
	if err != nil {
		return err
	}
	return os.RemoveAll(p)
}

func (a *FilesystemAdapter) CountFilesInFolder(_ context.Context, folderName string) (int, error) {
	p, err := a.safe(folderName)
	if err != nil {
		return 0, err
	}
	entries, err := os.ReadDir(p)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			n++
		}
	}
	return n, nil
}

func (a *FilesystemAdapter) CreateDownloadURL(_ context.Context, _ string) (string, bool, error) {
	return "", false, nil
}

func (a *FilesystemAdapter) Clear(_ context.Context) error {
	if err := os.RemoveAll(a.root); err != nil {
		return err
	}
	return os.MkdirAll(a.root, 0o755)
}

func (a *FilesystemAdapter) OpenSeekable(_ context.Context, objectName string) (io.ReadSeekCloser, time.Time, error) {
	p, err := a.safe(objectName)
	if err != nil {
		return nil, time.Time{}, err
	}
	f, err := os.Open(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, time.Time{}, ErrObjectNotFound
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, time.Time{}, err
	}
	return f, fi.ModTime(), nil
}
