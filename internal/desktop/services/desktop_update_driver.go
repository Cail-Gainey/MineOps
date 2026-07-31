package services

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/global/appsettings"
	"github.com/Cail-Gainey/MineOps/internal/global/constants"
	"github.com/Cail-Gainey/MineOps/internal/infrastructure/desktoprelease"
	"github.com/Cail-Gainey/MineOps/internal/service"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
)

const desktopGitHubReleasesURL = "https://api.github.com/repos/Cail-Gainey/MineOps/releases"

type mineOpsUpdateProvider struct {
	catalog   *desktoprelease.Catalog
	settings  *appsettings.Manager
	downloads *service.DownloadManager

	progressMu sync.RWMutex
	progress   func(service.DesktopUpdateProgress)
}

// Name 返回更新源提供方名称。
func (p *mineOpsUpdateProvider) Name() string { return "mineops-github" }

// Check 检查配置通道上是否存在可用的新版本。
func (p *mineOpsUpdateProvider) Check(ctx context.Context, request updater.CheckRequest) (*updater.Release, error) {
	snapshot := p.settings.Snapshot()
	source := p.downloads.ResolveSource("desktop", desktopGitHubReleasesURL)
	release, err := p.catalog.ResolveFor(ctx, source, snapshot.General.UpdateChannel, request.CurrentVersion, request.Platform, request.Arch)
	if err != nil {
		if apperror.ToDTO(err).Code == apperror.CodeIONotFound.String() {
			return nil, nil
		}
		return nil, err
	}
	digest, err := base64.StdEncoding.DecodeString(release.Artifact.Digest)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeDesktopUpdateMetadataUnauthenticated, "解析 Desktop Update Artifact Digest 失败", err)
	}
	signature, err := base64.StdEncoding.DecodeString(release.Artifact.Signature)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeDesktopUpdateMetadataUnauthenticated, "解析 Desktop Update Artifact Signature 失败", err)
	}
	publishedAt, err := time.Parse(time.RFC3339, release.PublishedAt)
	if err != nil {
		return nil, err
	}
	return &updater.Release{
		Version: release.Version, Channel: release.Channel, Notes: release.Notes, PublishedAt: publishedAt,
		Artifact: updater.Artifact{
			Filename: release.Artifact.Filename, Filetype: release.Artifact.Filetype, Size: release.Artifact.Size,
			Platform: release.Artifact.Platform, Arch: release.Artifact.Arch,
		},
		Verification: &updater.Verification{
			DigestAlgo: release.Artifact.DigestAlgo, Digest: digest,
			SignatureAlgo: release.Artifact.SignatureAlgo, Signature: signature,
		},
		Metadata: map[string]any{
			"mineops.artifact.url": release.Artifact.URL,
			"mineops.release.url":  release.ReleaseURL,
			"mineops.manifest.url": release.ManifestURL,
		},
	}, nil
}

// Download 下载并回报进度,把发行包写入目标 writer。
func (p *mineOpsUpdateProvider) Download(ctx context.Context, release *updater.Release, dst io.Writer, onProgress func(int64, int64)) error {
	if release == nil || release.Metadata == nil {
		return apperror.New(apperror.CodeValidationRequired, "Desktop Update Release Metadata 不能为空")
	}
	artifactURL, ok := release.Metadata["mineops.artifact.url"].(string)
	if !ok || artifactURL == "" {
		return apperror.New(apperror.CodeValidationRequired, "Desktop Update Artifact URL 缺失")
	}
	return p.downloads.StreamArtifact(ctx, artifactURL, release.Artifact.Size, dst, func(progress service.ArtifactStreamProgress) {
		onProgress(progress.Downloaded, progress.Total)
		p.progressMu.RLock()
		callback := p.progress
		p.progressMu.RUnlock()
		if callback != nil {
			normalized := float64(0)
			if progress.Total > 0 {
				normalized = float64(progress.Downloaded) / float64(progress.Total)
			}
			callback(service.DesktopUpdateProgress{DownloadedBytes: progress.Downloaded, TotalBytes: progress.Total, Progress: normalized})
		}
	})
}

func (p *mineOpsUpdateProvider) setProgress(callback func(service.DesktopUpdateProgress)) {
	p.progressMu.Lock()
	p.progress = callback
	p.progressMu.Unlock()
}

// WailsDesktopUpdateDriver 把锁定版本的 Wails updater 适配到 MineOps 更新协调器。
type WailsDesktopUpdateDriver struct {
	app       *application.App
	updater   *updater.Updater
	provider  *mineOpsUpdateProvider
	settings  *appsettings.Manager
	downloads *service.DownloadManager
	guard     *service.ExitGuard

	cancelMu sync.Mutex
	cancel   context.CancelFunc
}

// NewWailsDesktopUpdateDriver 用经过认证的 MineOps 提供方配置 Wails updater。
func NewWailsDesktopUpdateDriver(app *application.App, catalog *desktoprelease.Catalog, settings *appsettings.Manager, downloads *service.DownloadManager, guard *service.ExitGuard, publicKey []byte) (*WailsDesktopUpdateDriver, error) {
	if app == nil || app.Updater == nil || catalog == nil || settings == nil || downloads == nil || guard == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Wails Desktop Update Driver 依赖不能为空")
	}
	provider := &mineOpsUpdateProvider{catalog: catalog, settings: settings, downloads: downloads}
	if err := app.Updater.Init(updater.Config{
		CurrentVersion: constants.ApplicationVersion,
		Providers:      []updater.Provider{provider},
		PublicKey:      append([]byte(nil), publicKey...),
		Window:         updater.WindowNone,
	}); err != nil {
		return nil, apperror.Wrap(apperror.CodeDesktopUpdateUnsupported, "初始化 Wails Desktop Updater 失败", err)
	}
	return &WailsDesktopUpdateDriver{
		app: app, updater: app.Updater, provider: provider, settings: settings, downloads: downloads, guard: guard,
	}, nil
}

// Check 解析当前通道并把 Wails 发行信息映射成 MineOps 状态模型。
func (d *WailsDesktopUpdateDriver) Check(ctx context.Context) (service.DesktopUpdateStatus, error) {
	snapshot := d.settings.Snapshot()
	installSupported, installMessage := desktopInstallSupport()
	status := service.DesktopUpdateStatus{
		Enabled: snapshot.General.AutoCheckUpdates, Channel: snapshot.General.UpdateChannel,
		ManifestURL:    d.downloads.ResolveSource("desktop", desktopGitHubReleasesURL),
		CurrentVersion: constants.ApplicationVersion, Phase: service.DesktopUpdatePhaseChecking,
		Platform: runtime.GOOS, Architecture: runtime.GOARCH,
		InstallSupported: installSupported, InstallMessage: installMessage,
	}
	release, err := d.updater.Check(ctx)
	if err != nil {
		return status, err
	}
	if release == nil {
		status.Phase = service.DesktopUpdatePhaseUpToDate
		return status, nil
	}
	status.Phase = service.DesktopUpdatePhaseAvailable
	status.UpdateAvailable = true
	status.LatestVersion = release.Version
	status.Notes = release.Notes
	status.PublishedAt = release.PublishedAt.UTC().Format(time.RFC3339)
	if release.Metadata != nil {
		if value, ok := release.Metadata["mineops.release.url"].(string); ok {
			status.ReleaseURL = value
		}
		if value, ok := release.Metadata["mineops.manifest.url"].(string); ok {
			status.ManifestURL = value
		}
	}
	return status, nil
}

// DownloadAndInstall 下载、校验并暂存待安装的 Wails 更新。
func (d *WailsDesktopUpdateDriver) DownloadAndInstall(ctx context.Context, onProgress func(service.DesktopUpdateProgress)) error {
	if supported, message := desktopInstallSupport(); !supported {
		return apperror.New(apperror.CodeDesktopUpdateUnsupported, message)
	}
	d.cancelMu.Lock()
	if d.cancel != nil {
		d.cancelMu.Unlock()
		return apperror.New(apperror.CodeValidationConflict, "Desktop Update 下载已在运行")
	}
	downloadContext, cancel := context.WithCancel(ctx)
	d.cancel = cancel
	d.cancelMu.Unlock()
	d.provider.setProgress(onProgress)
	defer func() {
		d.provider.setProgress(nil)
		d.cancelMu.Lock()
		d.cancel = nil
		d.cancelMu.Unlock()
	}()
	if err := d.updater.DownloadAndInstall(downloadContext); err != nil {
		if errors.Is(downloadContext.Err(), context.Canceled) {
			return apperror.Wrap(apperror.CodeDesktopUpdateCancelled, "Desktop Update 下载已取消", downloadContext.Err())
		}
		return err
	}
	return nil
}

// Cancel 取消进行中的更新下载,不丢弃已暂存完成的更新。
func (d *WailsDesktopUpdateDriver) Cancel() {
	d.cancelMu.Lock()
	cancel := d.cancel
	d.cancelMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Restart 在 ExitGuard 放行后请求 Wails 替换当前程序。
func (d *WailsDesktopUpdateDriver) Restart(ctx context.Context, discardUnsaved bool) error {
	if supported, message := desktopInstallSupport(); !supported {
		return apperror.New(apperror.CodeDesktopUpdateUnsupported, message)
	}
	items := d.guard.Items()
	if len(items) > 0 && !discardUnsaved {
		return apperror.New(apperror.CodeValidationConflict, "存在未保存内容，Desktop Update 重启需要确认").WithDetails(map[string]any{"items": items})
	}
	if len(items) > 0 {
		d.guard.ConfirmQuit()
	}
	if err := d.updater.Restart(ctx); err != nil {
		return apperror.Wrap(apperror.CodeDesktopUpdateApplyFailed, "Wails Desktop Update Restart 失败", err)
	}
	return nil
}

func desktopInstallSupport() (bool, string) {
	if applog.DetectRuntimeMode() != applog.RuntimePackaged {
		return false, "开发模式仅允许检测更新，不允许替换开发可执行文件"
	}
	if runtime.GOOS == "linux" {
		if strings.TrimSpace(os.Getenv("APPIMAGE")) != "" {
			return false, "AppImage 自动替换尚未完成目标路径验证，请从 GitHub Release 手动更新"
		}
		if strings.TrimSpace(os.Getenv("MINEOPS_PROGRAM_DIR")) != "" {
			return false, "deb/rpm 安装由系统包管理器维护，请使用包管理器更新"
		}
	}
	executable, err := os.Executable()
	if err != nil {
		return false, "无法解析当前 MineOps 可执行文件"
	}
	targetDirectory := filepath.Dir(executable)
	if runtime.GOOS == "darwin" {
		cleaned := filepath.Clean(executable)
		separator := string(os.PathSeparator)
		parts := strings.Split(cleaned, separator)
		for index, part := range parts {
			if strings.HasSuffix(part, ".app") {
				bundle := separator + filepath.Join(parts[1:index+1]...)
				targetDirectory = filepath.Dir(bundle)
				break
			}
		}
	}
	probe, err := os.CreateTemp(targetDirectory, ".mineops-update-probe-")
	if err != nil {
		return false, "当前安装目录不可写，请从 GitHub Release 手动安装"
	}
	probePath := probe.Name()
	_ = probe.Close()
	_ = os.Remove(probePath)
	return true, "当前安装形态支持重启后自动替换"
}
