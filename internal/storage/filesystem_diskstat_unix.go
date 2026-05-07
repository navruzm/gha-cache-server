//go:build unix

package storage

import "syscall"

func (a *FilesystemAdapter) UsableFreeBytes() (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(a.root, &stat); err != nil {
		return 0, err
	}
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}
