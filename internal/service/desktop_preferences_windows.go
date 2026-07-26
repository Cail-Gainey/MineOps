//go:build windows

package service

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"golang.org/x/sys/windows/registry"
)

const (
	windowsMineOpsUninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\MineOpsMineOps`
	windowsStartupKey          = `Software\Microsoft\Windows\CurrentVersion\Run`
)

func syncWindowsApplicationVersion(_ context.Context, version string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, windowsMineOpsUninstallKey, registry.SET_VALUE)
	if err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "打开 Windows MineOps 安装注册表失败", err)
	}
	defer func() { _ = key.Close() }()
	if err := key.SetStringValue("DisplayVersion", version); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "同步 Windows MineOps 安装版本失败", err)
	}
	return nil
}

func applyWindowsStartupPreference(_ context.Context, executable string, enabled bool) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, windowsStartupKey, registry.SET_VALUE)
	if err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "打开 Windows 开机启动注册表失败", err)
	}
	defer func() { _ = key.Close() }()
	if enabled {
		if err := key.SetStringValue("MineOps", `"`+executable+`"`); err != nil {
			return apperror.Wrap(apperror.CodeIOWriteFailed, "启用 Windows 开机启动失败", err)
		}
		return nil
	}
	if err := key.DeleteValue("MineOps"); err != nil && err != registry.ErrNotExist {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "关闭 Windows 开机启动失败", err)
	}
	return nil
}
