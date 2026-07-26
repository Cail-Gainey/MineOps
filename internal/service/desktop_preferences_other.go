//go:build !windows

package service

import "context"

func syncWindowsApplicationVersion(_ context.Context, _ string) error {
	return nil
}

func applyWindowsStartupPreference(_ context.Context, _ string, _ bool) error {
	return nil
}
