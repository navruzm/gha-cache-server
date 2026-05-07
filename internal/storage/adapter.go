package storage

import (
	"context"
	"errors"
	"io"
)

var ErrObjectNotFound = errors.New("object not found in storage")

type Adapter interface {
	CreateDownloadStream(ctx context.Context, objectName string) (io.ReadCloser, error)
	UploadStream(ctx context.Context, objectName string, body io.Reader) error
	DeleteFolder(ctx context.Context, folderName string) error
	CountFilesInFolder(ctx context.Context, folderName string) (int, error)
	CreateDownloadURL(ctx context.Context, objectName string) (string, bool, error)
	Clear(ctx context.Context) error
}
