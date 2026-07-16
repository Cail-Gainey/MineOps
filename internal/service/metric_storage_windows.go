//go:build windows

package service

import "golang.org/x/sys/windows"

func availableDiskBytes(path string) (int64, error) {
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var available uint64
	if err := windows.GetDiskFreeSpaceEx(pointer, &available, nil, nil); err != nil {
		return 0, err
	}
	return int64(available), nil
}
