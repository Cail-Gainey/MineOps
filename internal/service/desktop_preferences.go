package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/constants"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

// DesktopPreferences applies platform desktop integration controlled by General Settings.
type DesktopPreferences struct {
	executable string
}

// SyncApplicationVersion updates platform installation metadata after a self-update.
func (m *DesktopPreferences) SyncApplicationVersion(ctx context.Context) error {
	return syncWindowsApplicationVersion(ctx, constants.ApplicationVersion)
}

// NewDesktopPreferences resolves the current executable used for platform integration.
func NewDesktopPreferences() (*DesktopPreferences, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeIOReadFailed, "读取 MineOps 可执行文件路径失败", err)
	}
	resolved, err := filepath.EvalSymlinks(executable)
	if err == nil {
		executable = resolved
	}
	return &DesktopPreferences{executable: filepath.Clean(executable)}, nil
}

// ApplyGeneral applies the committed launch-at-startup preference for the current platform.
func (m *DesktopPreferences) ApplyGeneral(ctx context.Context, settings model.GeneralSettings) error {
	switch runtime.GOOS {
	case "darwin":
		return m.applyDarwinStartup(settings.LaunchAtStartup)
	case "windows":
		return applyWindowsStartupPreference(ctx, m.executable, settings.LaunchAtStartup)
	default:
		return m.applyLinuxStartup(settings.LaunchAtStartup)
	}
}

func (m *DesktopPreferences) applyDarwinStartup(enabled bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return apperror.Wrap(apperror.CodeIOReadFailed, "读取用户目录失败", err)
	}
	path := filepath.Join(home, "Library", "LaunchAgents", "com.gainey.mineops.plist")
	if !enabled {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return apperror.Wrap(apperror.CodeIOWriteFailed, "关闭开机启动失败", err)
		}
		return nil
	}
	payload := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>Label</key><string>com.gainey.mineops</string><key>ProgramArguments</key><array><string>%s</string></array><key>RunAtLoad</key><true/></dict></plist>
`, xmlEscape(m.executable))
	return writeDesktopPreference(path, []byte(payload))
}

func (m *DesktopPreferences) applyLinuxStartup(enabled bool) error {
	configDirectory, err := os.UserConfigDir()
	if err != nil {
		return apperror.Wrap(apperror.CodeIOReadFailed, "读取用户配置目录失败", err)
	}
	path := filepath.Join(configDirectory, "autostart", "mineops.desktop")
	if !enabled {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return apperror.Wrap(apperror.CodeIOWriteFailed, "关闭开机启动失败", err)
		}
		return nil
	}
	payload := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=MineOps\nExec=%s\nTerminal=false\nX-GNOME-Autostart-enabled=true\n", strconv.Quote(m.executable))
	return writeDesktopPreference(path, []byte(payload))
}

func writeDesktopPreference(path string, payload []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建开机启动目录失败", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".mineops-startup-")
	if err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建开机启动临时文件失败", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return apperror.Wrap(apperror.CodeIOPermissionDenied, "设置开机启动文件权限失败", err)
	}
	if _, err := temporary.Write(payload); err != nil {
		_ = temporary.Close()
		return apperror.Wrap(apperror.CodeIOWriteFailed, "写入开机启动文件失败", err)
	}
	if err := temporary.Close(); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "关闭开机启动文件失败", err)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "替换开机启动文件失败", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "提交开机启动文件失败", err)
	}
	return nil
}

func xmlEscape(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;").Replace(value)
}
