package service

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

// LogStatus describes the actual runtime mode, active log file, and bounded directory usage.
type LogStatus struct {
	RuntimeMode string `json:"runtimeMode"`
	Directory   string `json:"directory"`
	CurrentPath string `json:"currentPath"`
	Files       int    `json:"files"`
	Bytes       int64  `json:"bytes"`
}

// LogManager applies committed logging policy and exposes safe directory maintenance.
type LogManager struct {
	writer *applog.RotatingWriter
	mode   applog.RuntimeMode
}

// NewLogManager creates the runtime logging settings boundary.
func NewLogManager(writer *applog.RotatingWriter) (*LogManager, error) {
	if writer == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "LogManager Writer 不能为空")
	}
	return &LogManager{writer: writer, mode: applog.DetectRuntimeMode()}, nil
}

// Apply updates file rotation and retention policy after Settings transaction commit.
func (m *LogManager) Apply(settings model.LoggingSettings) error {
	return m.writer.UpdatePolicy(int64(settings.MaxFileMiB)*1024*1024, settings.RetentionDays, int64(settings.TotalCapacityMiB)*1024*1024)
}

// Status returns the current runtime log location and aggregate usage.
func (m *LogManager) Status() (LogStatus, error) {
	status, err := m.writer.Status()
	if err != nil {
		return LogStatus{}, apperror.Wrap(apperror.CodeIOReadFailed, "读取日志目录状态失败", err)
	}
	return LogStatus{RuntimeMode: string(m.mode), Directory: status.Directory, CurrentPath: status.CurrentPath, Files: status.Files, Bytes: status.Bytes}, nil
}

// ClearArchived removes rotated logs immediately while preserving the active process log.
func (m *LogManager) ClearArchived() error {
	if err := m.writer.ClearArchived(); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "清理历史日志失败", err)
	}
	return nil
}

// OpenDirectory opens the actual runtime log directory with the platform file manager.
func (m *LogManager) OpenDirectory(ctx context.Context) error {
	status, err := m.writer.Status()
	if err != nil {
		return apperror.Wrap(apperror.CodeIOReadFailed, "读取日志目录失败", err)
	}
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.CommandContext(ctx, "open", status.Directory)
	case "windows":
		command = exec.CommandContext(ctx, "explorer", filepath.Clean(status.Directory))
	default:
		command = exec.CommandContext(ctx, "xdg-open", status.Directory)
	}
	if err := command.Start(); err != nil {
		return apperror.Wrap(apperror.CodeIOReadFailed, "打开日志目录失败", err)
	}
	_ = command.Process.Release()
	return nil
}
