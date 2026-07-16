//go:build !windows

package service

import "os"

func replaceLocalFile(source, destination string) error {
	return os.Rename(source, destination)
}
