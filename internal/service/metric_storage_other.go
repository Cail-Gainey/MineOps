//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package service

func availableDiskBytes(string) (int64, error) {
	return -1, nil
}
