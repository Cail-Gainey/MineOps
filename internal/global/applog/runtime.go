package applog

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/constants"
)

// ParseLevel 把持久化的日志等级名转换成 slog 等级。
func ParseLevel(value string) (slog.Level, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.TrimSpace(value))); err != nil {
		return 0, err
	}
	return level, nil
}

// RuntimeMode 区分源码开发态与已打包的桌面可执行文件。
type RuntimeMode string

const (
	RuntimeDevelopment RuntimeMode = "development"
	RuntimePackaged    RuntimeMode = "packaged"
)

// RuntimeLogOptions 控制进程日志文件与清理策略。
type RuntimeLogOptions struct {
	Level              slog.Level
	MaxFileBytes       int64
	RetentionDays      int
	TotalCapacityBytes int64
}

// DetectRuntimeMode 依据 Go BuildInfo 标记判断,而不是从当前工作目录猜测。
func DetectRuntimeMode() RuntimeMode {
	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range buildInfo.Settings {
			if setting.Key == "-tags" && strings.Contains(setting.Value, "production") {
				return RuntimePackaged
			}
		}
	}
	return RuntimeDevelopment
}

// ResolveStorageMode 仅对开发构建应用可选的存储位置覆盖。
func ResolveStorageMode(mode RuntimeMode) RuntimeMode {
	if mode != RuntimeDevelopment {
		return mode
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("MINEOPS_STORAGE_PROFILE"))) {
	case string(RuntimePackaged):
		return RuntimePackaged
	default:
		return RuntimeDevelopment
	}
}

// ResolveLogDirectory 返回当前运行模式下确定的日志目录。
func ResolveLogDirectory(mode RuntimeMode) (string, error) {
	mode = ResolveStorageMode(mode)
	if mode == RuntimeDevelopment {
		programDirectory, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(programDirectory, constants.DevelopmentApplicationName, constants.LogsDirectoryName), nil
	}
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" && strings.Contains(executable, ".app"+string(filepath.Separator)+"Contents"+string(filepath.Separator)+"MacOS") {
		programDirectory, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(programDirectory, constants.ApplicationName, constants.LogsDirectoryName), nil
	}
	if runtime.GOOS == "linux" {
		if programDirectory := strings.TrimSpace(os.Getenv("MINEOPS_PROGRAM_DIR")); programDirectory != "" {
			if !filepath.IsAbs(programDirectory) {
				return "", errors.New("MINEOPS_PROGRAM_DIR must be absolute")
			}
			return filepath.Join(filepath.Clean(programDirectory), constants.LogsDirectoryName), nil
		}
		if appImage := strings.TrimSpace(os.Getenv("APPIMAGE")); appImage != "" {
			if !filepath.IsAbs(appImage) {
				return "", errors.New("APPIMAGE must be absolute")
			}
			appImage, err = filepath.EvalSymlinks(appImage)
			if err != nil {
				return "", err
			}
			return filepath.Join(filepath.Dir(appImage), constants.LogsDirectoryName), nil
		}
	}
	return filepath.Join(filepath.Dir(executable), constants.LogsDirectoryName), nil
}

// ProbeLogDirectory 创建目录并验证当前进程能够写入并删除探测文件。
func ProbeLogDirectory(directory string) error {
	if directory == "" {
		return errors.New("log directory is empty")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	probe, err := os.CreateTemp(directory, ".mineops-log-probe-")
	if err != nil {
		return err
	}
	probePath := probe.Name()
	if _, err := probe.WriteString("mineops-log-probe"); err != nil {
		_ = probe.Close()
		_ = os.Remove(probePath)
		return err
	}
	if err := probe.Sync(); err != nil {
		_ = probe.Close()
		_ = os.Remove(probePath)
		return err
	}
	if err := probe.Close(); err != nil {
		_ = os.Remove(probePath)
		return err
	}
	return os.Remove(probePath)
}

// NewRuntimeLogger 在必需的可写运行目录中创建进程日志器。
func NewRuntimeLogger(options RuntimeLogOptions) (*Logger, *RotatingWriter, string, error) {
	directory, err := ResolveLogDirectory(DetectRuntimeMode())
	if err != nil {
		return nil, nil, "", fmt.Errorf("resolve log directory: %w", err)
	}
	if err := ProbeLogDirectory(directory); err != nil {
		return nil, nil, directory, fmt.Errorf("probe log directory: %w", err)
	}
	writer, err := NewRotatingWriter(RotatingWriterOptions{
		Directory: directory, Prefix: "mineops", MaxFileBytes: options.MaxFileBytes,
		RetentionDays: options.RetentionDays, TotalCapacityBytes: options.TotalCapacityBytes,
	})
	if err != nil {
		return nil, nil, directory, err
	}
	return New(writer, options.Level), writer, directory, nil
}

// DefaultRuntimeLogOptions 返回加密设置加载之前的引导期策略。
func DefaultRuntimeLogOptions() RuntimeLogOptions {
	return RuntimeLogOptions{
		Level: slog.LevelInfo, MaxFileBytes: 20 * 1024 * 1024,
		RetentionDays: 14, TotalCapacityBytes: 500 * 1024 * 1024,
	}
}

func logDate(now time.Time) string {
	return now.Local().Format("2006-01-02")
}
