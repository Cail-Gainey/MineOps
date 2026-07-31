package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/appsettings"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

const maximumBackgroundImageBytes int64 = 16 * 1024 * 1024

// BackgroundResource 描述当前受控的应用背景图。
type BackgroundResource struct {
	Configured bool   `json:"configured"`
	Available  bool   `json:"available"`
	Path       string `json:"path,omitempty"`
	DataURL    string `json:"dataURL,omitempty"`
	Bytes      int64  `json:"bytes,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// BackgroundManager 负责受控本地背景资源的导入、解析与重置。
type BackgroundManager struct {
	settings      *appsettings.Manager
	dataDirectory string
}

// NewBackgroundManager 创建受控背景资源边界。
func NewBackgroundManager(settings *appsettings.Manager, dataDirectory string) (*BackgroundManager, error) {
	if settings == nil || strings.TrimSpace(dataDirectory) == "" {
		return nil, apperror.New(apperror.CodeValidationRequired, "BackgroundManager 依赖不能为空")
	}
	return &BackgroundManager{settings: settings, dataDirectory: filepath.Clean(dataDirectory)}, nil
}

// Import 把一张受支持的图片复制进 MineOps 数据目录并提交其相对路径。
func (m *BackgroundManager) Import(ctx context.Context, sourcePath string) (model.SettingsSnapshot, error) {
	resolvedSource, err := filepath.EvalSymlinks(filepath.Clean(strings.TrimSpace(sourcePath)))
	if err != nil {
		return model.SettingsSnapshot{}, apperror.Wrap(apperror.CodeIOReadFailed, "读取背景图片路径失败", err)
	}
	info, err := os.Stat(resolvedSource)
	if err != nil {
		return model.SettingsSnapshot{}, apperror.Wrap(apperror.CodeIOReadFailed, "读取背景图片失败", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maximumBackgroundImageBytes {
		return model.SettingsSnapshot{}, apperror.New(apperror.CodeValidationInvalidArgument, "背景图片必须是 16 MiB 以内的普通文件")
	}
	file, err := os.Open(resolvedSource)
	if err != nil {
		return model.SettingsSnapshot{}, apperror.Wrap(apperror.CodeIOReadFailed, "打开背景图片失败", err)
	}
	payload, readErr := io.ReadAll(io.LimitReader(file, maximumBackgroundImageBytes+1))
	closeErr := file.Close()
	if int64(len(payload)) > maximumBackgroundImageBytes {
		return model.SettingsSnapshot{}, apperror.New(apperror.CodeValidationInvalidArgument, "背景图片超过 16 MiB 限制")
	}
	if readErr != nil || closeErr != nil {
		return model.SettingsSnapshot{}, apperror.Wrap(apperror.CodeIOReadFailed, "读取背景图片失败", errors.Join(readErr, closeErr))
	}
	_, extension := supportedBackgroundType(payload)
	if extension == "" {
		return model.SettingsSnapshot{}, apperror.New(apperror.CodeValidationInvalidArgument, "背景图片仅支持 PNG、JPEG 和 WebP")
	}
	digest := sha256.Sum256(payload)
	backgroundDirectory := filepath.Join(m.dataDirectory, "backgrounds")
	if err := os.MkdirAll(backgroundDirectory, 0o700); err != nil {
		return model.SettingsSnapshot{}, apperror.Wrap(apperror.CodeIOWriteFailed, "创建背景资源目录失败", err)
	}
	filename := hex.EncodeToString(digest[:]) + extension
	destination := filepath.Join(backgroundDirectory, filename)
	if _, err := os.Stat(destination); os.IsNotExist(err) {
		temporary, err := os.CreateTemp(backgroundDirectory, ".mineops-background-")
		if err != nil {
			return model.SettingsSnapshot{}, apperror.Wrap(apperror.CodeIOWriteFailed, "创建背景资源临时文件失败", err)
		}
		temporaryPath := temporary.Name()
		defer func() { _ = os.Remove(temporaryPath) }()
		if err := temporary.Chmod(0o600); err != nil {
			_ = temporary.Close()
			return model.SettingsSnapshot{}, apperror.Wrap(apperror.CodeIOPermissionDenied, "设置背景资源权限失败", err)
		}
		_, writeErr := temporary.Write(payload)
		closeTemporaryErr := temporary.Close()
		if writeErr != nil || closeTemporaryErr != nil {
			return model.SettingsSnapshot{}, apperror.Wrap(apperror.CodeIOWriteFailed, "写入背景资源失败", errors.Join(writeErr, closeTemporaryErr))
		}
		if err := os.Rename(temporaryPath, destination); err != nil {
			return model.SettingsSnapshot{}, apperror.Wrap(apperror.CodeIOWriteFailed, "提交背景资源失败", err)
		}
	} else if err != nil {
		return model.SettingsSnapshot{}, apperror.Wrap(apperror.CodeIOReadFailed, "检查背景资源失败", err)
	}

	snapshot := m.settings.Snapshot()
	previous := snapshot.Paths.BackgroundImage
	relative, err := filepath.Rel(m.dataDirectory, destination)
	if err != nil {
		return model.SettingsSnapshot{}, apperror.Wrap(apperror.CodeInternal, "计算背景资源相对路径失败", err)
	}
	snapshot.Paths.BackgroundImage = filepath.ToSlash(relative)
	snapshot.Theme.BackgroundMode = "image"
	if err := m.settings.Save(ctx, snapshot); err != nil {
		return model.SettingsSnapshot{}, err
	}
	if previous != "" && previous != snapshot.Paths.BackgroundImage {
		m.removeControlled(previous)
	}
	return m.settings.Snapshot(), nil
}

// Resolve 返回当前受控背景图的有界 Data URL。
func (m *BackgroundManager) Resolve() (BackgroundResource, error) {
	configured := strings.TrimSpace(m.settings.Snapshot().Paths.BackgroundImage)
	if configured == "" {
		return BackgroundResource{}, nil
	}
	path, ok := m.controlledPath(configured)
	if !ok {
		return BackgroundResource{Configured: true, Path: configured, Reason: "背景路径不在受控数据目录"}, nil
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return BackgroundResource{Configured: true, Path: configured, Reason: "背景文件不存在"}, nil
	}
	if err != nil {
		return BackgroundResource{}, apperror.Wrap(apperror.CodeIOReadFailed, "读取背景资源状态失败", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maximumBackgroundImageBytes {
		return BackgroundResource{Configured: true, Path: configured, Bytes: info.Size(), Reason: "背景文件类型或大小无效"}, nil
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return BackgroundResource{}, apperror.Wrap(apperror.CodeIOReadFailed, "读取背景资源失败", err)
	}
	mediaType, _ := supportedBackgroundType(payload)
	if mediaType == "" {
		return BackgroundResource{Configured: true, Path: configured, Bytes: info.Size(), Reason: "背景文件格式无效"}, nil
	}
	return BackgroundResource{
		Configured: true, Available: true, Path: configured, Bytes: info.Size(),
		DataURL: "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(payload),
	}, nil
}

// Reset 移除当前受控背景资源引用并恢复主题背景。
func (m *BackgroundManager) Reset(ctx context.Context) (model.SettingsSnapshot, error) {
	snapshot := m.settings.Snapshot()
	previous := snapshot.Paths.BackgroundImage
	snapshot.Paths.BackgroundImage = ""
	snapshot.Theme.BackgroundMode = "theme"
	if err := m.settings.Save(ctx, snapshot); err != nil {
		return model.SettingsSnapshot{}, err
	}
	m.removeControlled(previous)
	return m.settings.Snapshot(), nil
}

func (m *BackgroundManager) controlledPath(relative string) (string, bool) {
	if filepath.IsAbs(relative) {
		return "", false
	}
	cleaned := filepath.Clean(filepath.FromSlash(relative))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", false
	}
	path := filepath.Join(m.dataDirectory, cleaned)
	relativeToData, err := filepath.Rel(m.dataDirectory, path)
	return path, err == nil && relativeToData != ".." && !strings.HasPrefix(relativeToData, ".."+string(filepath.Separator))
}

func (m *BackgroundManager) removeControlled(relative string) {
	path, ok := m.controlledPath(relative)
	if ok && filepath.Dir(path) == filepath.Join(m.dataDirectory, "backgrounds") {
		_ = os.Remove(path)
	}
}

func supportedBackgroundType(payload []byte) (string, string) {
	mediaType := http.DetectContentType(payload)
	switch mediaType {
	case "image/png":
		return mediaType, ".png"
	case "image/jpeg":
		return mediaType, ".jpg"
	case "image/webp":
		return mediaType, ".webp"
	default:
		return "", ""
	}
}
