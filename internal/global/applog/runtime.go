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

// ParseLevel converts one persisted log-level name to a slog level.
func ParseLevel(value string) (slog.Level, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.TrimSpace(value))); err != nil {
		return 0, err
	}
	return level, nil
}

// RuntimeMode distinguishes source development from a packaged desktop executable.
type RuntimeMode string

const (
	RuntimeDevelopment RuntimeMode = "development"
	RuntimePackaged    RuntimeMode = "packaged"
)

// RuntimeLogOptions controls the process log file and cleanup policy.
type RuntimeLogOptions struct {
	Level              slog.Level
	MaxFileBytes       int64
	RetentionDays      int
	TotalCapacityBytes int64
}

// DetectRuntimeMode uses Go BuildInfo tags instead of guessing from the current working directory.
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

// ResolveLogDirectory returns the deterministic logs directory for the current runtime mode.
func ResolveLogDirectory(mode RuntimeMode) (string, error) {
	if mode == RuntimeDevelopment {
		_, sourceFile, _, ok := runtime.Caller(0)
		if !ok || sourceFile == "" {
			return "", errors.New("cannot resolve applog source path")
		}
		projectRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", ".."))
		return filepath.Join(projectRoot, constants.LogsDirectoryName), nil
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

// ProbeLogDirectory creates the directory and verifies that the current process can write and remove a probe file.
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

// NewRuntimeLogger creates the process logger in the required writable runtime directory.
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

// DefaultRuntimeLogOptions returns the bootstrap policy before encrypted Settings are loaded.
func DefaultRuntimeLogOptions() RuntimeLogOptions {
	return RuntimeLogOptions{
		Level: slog.LevelInfo, MaxFileBytes: 20 * 1024 * 1024,
		RetentionDays: 14, TotalCapacityBytes: 500 * 1024 * 1024,
	}
}

func logDate(now time.Time) string {
	return now.Local().Format("2006-01-02")
}
