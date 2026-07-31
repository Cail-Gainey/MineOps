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

// LogStatus 描述真实运行模式、当前日志文件与有界的目录占用。
type LogStatus struct {
	RuntimeMode string `json:"runtimeMode"`
	Directory   string `json:"directory"`
	CurrentPath string `json:"currentPath"`
	Files       int    `json:"files"`
	Bytes       int64  `json:"bytes"`
}

// LogManager 应用已提交的日志策略,并暴露安全的目录维护操作。
type LogManager struct {
	writer *applog.RotatingWriter
	mode   applog.RuntimeMode
}

// NewLogManager 创建运行期日志设置边界。
func NewLogManager(writer *applog.RotatingWriter) (*LogManager, error) {
	if writer == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "LogManager Writer 不能为空")
	}
	return &LogManager{writer: writer, mode: applog.DetectRuntimeMode()}, nil
}

// Apply 在设置事务提交后更新文件轮转与保留策略。
func (m *LogManager) Apply(settings model.LoggingSettings) error {
	return m.writer.UpdatePolicy(int64(settings.MaxFileMiB)*1024*1024, settings.RetentionDays, int64(settings.TotalCapacityMiB)*1024*1024)
}

// Status 返回当前运行期日志位置与总体占用。
func (m *LogManager) Status() (LogStatus, error) {
	status, err := m.writer.Status()
	if err != nil {
		return LogStatus{}, apperror.Wrap(apperror.CodeIOReadFailed, "读取日志目录状态失败", err)
	}
	return LogStatus{RuntimeMode: string(m.mode), Directory: status.Directory, CurrentPath: status.CurrentPath, Files: status.Files, Bytes: status.Bytes}, nil
}

// ClearArchived 立即删除已轮转日志,保留当前进程正在写的日志。
func (m *LogManager) ClearArchived() error {
	if err := m.writer.ClearArchived(); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "清理历史日志失败", err)
	}
	return nil
}

// OpenDirectory 用系统文件管理器打开真实的运行期日志目录。
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
