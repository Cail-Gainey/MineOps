//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package service

import "syscall"

func availableDiskBytes(path string) (int64, error) {
	var status syscall.Statfs_t
	if err := syscall.Statfs(path, &status); err != nil {
		return 0, err
	}
	return int64(status.Bavail) * int64(status.Bsize), nil
}
