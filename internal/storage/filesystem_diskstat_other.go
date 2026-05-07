//go:build !unix

package storage

import "errors"

func (a *FilesystemAdapter) UsableFreeBytes() (int64, error) {
	return 0, errors.New("disk-usage reporting unavailable on this platform")
}
