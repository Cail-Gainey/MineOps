package service

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/global/appsettings"
	"github.com/Cail-Gainey/MineOps/internal/global/constants"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/infrastructure/desktoprelease"
	"github.com/Cail-Gainey/MineOps/internal/infrastructure/releaseversion"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

const (
	desktopReleaseDefaultBaseURL = "https://api.github.com/repos/Cail-Gainey/MineOps/releases"
	desktopUpdateCheckInterval   = 12 * time.Hour
)

const (
	// DesktopUpdatePhaseIdle 表示当前没有进行中的更新操作。
	DesktopUpdatePhaseIdle = "idle"
	// DesktopUpdatePhaseChecking 表示正在解析发行版元数据。
	DesktopUpdatePhaseChecking = "checking"
	// DesktopUpdatePhaseAvailable 表示存在可用的新版本。
	DesktopUpdatePhaseAvailable = "available"
	// DesktopUpdatePhaseDownloading 表示正在下载构件。
	DesktopUpdatePhaseDownloading = "downloading"
	// DesktopUpdatePhaseVerifying 表示正在校验已下载的构件。
	DesktopUpdatePhaseVerifying = "verifying"
	// DesktopUpdatePhaseReady 表示已校验的更新正等待重启生效。
	DesktopUpdatePhaseReady = "ready"
	// DesktopUpdatePhaseRestarting 表示更新助手正在重启 MineOps。
	DesktopUpdatePhaseRestarting = "restarting"
	// DesktopUpdatePhaseUpToDate 表示未发现更新版本。
	DesktopUpdatePhaseUpToDate = "up-to-date"
	// DesktopUpdatePhaseError 表示最近一次更新操作失败。
	DesktopUpdatePhaseError = "error"
)

// DesktopUpdateStatus 描述已配置的通道与完整的桌面自更新生命周期。
type DesktopUpdateStatus struct {
	Enabled          bool       `json:"enabled"`
	Channel          string     `json:"channel"`
	ManifestURL      string     `json:"manifestURL"`
	Running          bool       `json:"running"`
	CurrentVersion   string     `json:"currentVersion"`
	LatestVersion    string     `json:"latestVersion,omitempty"`
	UpdateAvailable  bool       `json:"updateAvailable"`
	ReleaseURL       string     `json:"releaseURL,omitempty"`
	PublishedAt      string     `json:"publishedAt,omitempty"`
	Notes            string     `json:"notes,omitempty"`
	LastCheckedAt    *time.Time `json:"lastCheckedAt,omitempty"`
	LastErrorCode    string     `json:"lastErrorCode,omitempty"`
	LastErrorMessage string     `json:"lastErrorMessage,omitempty"`
	Phase            string     `json:"phase"`
	Platform         string     `json:"platform"`
	Architecture     string     `json:"architecture"`
	InstallSupported bool       `json:"installSupported"`
	InstallMessage   string     `json:"installMessage,omitempty"`
	DownloadedBytes  int64      `json:"downloadedBytes"`
	TotalBytes       int64      `json:"totalBytes"`
	Progress         float64    `json:"progress"`
	ReadyToRestart   bool       `json:"readyToRestart"`
}

// DesktopUpdateProgress 描述一次有界的更新下载进度采样。
type DesktopUpdateProgress struct {
	DownloadedBytes int64   `json:"downloadedBytes"`
	TotalBytes      int64   `json:"totalBytes"`
	Progress        float64 `json:"progress"`
}

// DesktopUpdateDriver 把领域层的更新协调器接到具体平台的 updater 上。
type DesktopUpdateDriver interface {
	Check(context.Context) (DesktopUpdateStatus, error)
	DownloadAndInstall(context.Context, func(DesktopUpdateProgress)) error
	Cancel()
	Restart(context.Context, bool) error
}

// DesktopUpdateManager 只检查受信任的桌面发行元数据,不下载也不安装。
type DesktopUpdateManager struct {
	clock     model.Clock
	settings  *appsettings.Manager
	downloads *DownloadManager
	catalog   *desktoprelease.Catalog
	logger    *applog.Logger
	trigger   chan struct{}
	checkMu   sync.Mutex
	statusMu  sync.Mutex
	status    DesktopUpdateStatus
	driverMu  sync.RWMutex
	driver    DesktopUpdateDriver
}

// AttachDriver 接入平台专属 updater 并触发一次状态刷新。
func (m *DesktopUpdateManager) AttachDriver(driver DesktopUpdateDriver) {
	if m == nil {
		return
	}
	m.driverMu.Lock()
	m.driver = driver
	m.driverMu.Unlock()
	m.Trigger()
}

func (m *DesktopUpdateManager) updateDriver() DesktopUpdateDriver {
	m.driverMu.RLock()
	defer m.driverMu.RUnlock()
	return m.driver
}

// NewDesktopUpdateManager 创建桌面版本检查协调器。
func NewDesktopUpdateManager(clock model.Clock, settings *appsettings.Manager, downloads *DownloadManager, catalog *desktoprelease.Catalog, logger *applog.Logger) (*DesktopUpdateManager, error) {
	if clock == nil || settings == nil || downloads == nil || catalog == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Desktop Update 依赖不能为空")
	}
	if logger == nil {
		logger = applog.Default()
	}
	manager := &DesktopUpdateManager{
		clock: clock, settings: settings, downloads: downloads, catalog: catalog, logger: logger,
		trigger: make(chan struct{}, 1),
	}
	manager.status = manager.configuredStatus()
	return manager, nil
}

// Status 返回最近一次桌面发行版检查结果,不发起网络请求。
func (m *DesktopUpdateManager) Status() DesktopUpdateStatus {
	m.statusMu.Lock()
	defer m.statusMu.Unlock()
	return m.status
}

// CheckNow 立即检查已配置通道,不下载也不安装更新。
func (m *DesktopUpdateManager) CheckNow(ctx context.Context) (DesktopUpdateStatus, error) {
	if driver := m.updateDriver(); driver != nil {
		if !m.checkMu.TryLock() {
			return m.Status(), nil
		}
		defer m.checkMu.Unlock()
		status, err := driver.Check(ctx)
		if status.CurrentVersion == "" {
			status.CurrentVersion = constants.ApplicationVersion
		}
		if status.Channel == "" {
			status.Channel = m.settings.Snapshot().General.UpdateChannel
		}
		return m.finishCheck(status, err)
	}
	if !m.checkMu.TryLock() {
		return m.Status(), nil
	}
	defer m.checkMu.Unlock()
	status := m.configuredStatus()
	status.Running = true
	status.Phase = DesktopUpdatePhaseChecking
	m.storeStatus(status)
	release, err := m.catalog.Resolve(ctx, m.releaseBaseURL(), status.Channel)
	if err != nil {
		return m.finishCheck(status, err)
	}
	status.LatestVersion = release.Version
	status.ReleaseURL = release.ReleaseURL
	status.PublishedAt = release.PublishedAt
	status.Notes = release.Notes
	comparison, err := releaseversion.Compare(constants.ApplicationVersion, release.Version)
	if err != nil {
		return m.finishCheck(status, err)
	}
	status.UpdateAvailable = comparison < 0
	if status.UpdateAvailable {
		status.Phase = DesktopUpdatePhaseAvailable
	} else {
		status.Phase = DesktopUpdatePhaseUpToDate
	}
	return m.finishCheck(status, nil)
}

// DownloadNow 下载并校验可用的桌面更新,不重启应用。
func (m *DesktopUpdateManager) DownloadNow(ctx context.Context) (DesktopUpdateStatus, error) {
	driver := m.updateDriver()
	if driver == nil {
		return m.Status(), apperror.New(apperror.CodeDesktopUpdateUnsupported, "Desktop 自更新安装器尚未初始化")
	}
	status := m.Status()
	if !status.UpdateAvailable {
		return status, apperror.New(apperror.CodeValidationConflict, "当前没有可准备的 Desktop 更新")
	}
	status.Running = true
	status.Phase = DesktopUpdatePhaseDownloading
	status.LastErrorCode = ""
	status.LastErrorMessage = ""
	m.storeStatus(status)
	err := driver.DownloadAndInstall(ctx, func(progress DesktopUpdateProgress) {
		next := m.Status()
		next.Running = true
		next.Phase = DesktopUpdatePhaseDownloading
		next.DownloadedBytes = progress.DownloadedBytes
		next.TotalBytes = progress.TotalBytes
		next.Progress = progress.Progress
		if progress.Progress >= 1 {
			next.Phase = DesktopUpdatePhaseVerifying
		}
		m.storeStatus(next)
	})
	if err != nil {
		return m.finishCheck(m.Status(), err)
	}
	status = m.Status()
	status.Running = false
	status.Phase = DesktopUpdatePhaseReady
	status.ReadyToRestart = true
	status.Progress = 1
	m.storeStatus(status)
	return status, nil
}

// CancelDownload 在受支持时取消进行中的桌面构件下载。
func (m *DesktopUpdateManager) CancelDownload() {
	if driver := m.updateDriver(); driver != nil {
		driver.Cancel()
	}
}

// RestartAndApply 请求一次受保护的重启,进入已校验的桌面更新。
// RestartAndApply 请求一次受保护的重启,进入已校验的桌面更新。
func (m *DesktopUpdateManager) RestartAndApply(ctx context.Context, discardUnsaved bool) (DesktopUpdateStatus, error) {
	driver := m.updateDriver()
	if driver == nil {
		return m.Status(), apperror.New(apperror.CodeDesktopUpdateUnsupported, "Desktop 自更新安装器尚未初始化")
	}
	status := m.Status()
	if !status.ReadyToRestart {
		return status, apperror.New(apperror.CodeValidationConflict, "Desktop 更新尚未准备完成")
	}
	status.Phase = DesktopUpdatePhaseRestarting
	status.Running = true
	m.storeStatus(status)
	if err := driver.Restart(ctx, discardUnsaved); err != nil {
		return m.finishCheck(status, apperror.Wrap(apperror.CodeDesktopUpdateApplyFailed, "启动 Desktop 更新重启失败", err))
	}
	return m.Status(), nil
}

// Run 在启动时、通用与下载设置提交变更时,以及启用后每十二小时各检查一次。
func (m *DesktopUpdateManager) Run(ctx context.Context) error {
	unsubscribe := m.settings.Subscribe(func(change appsettings.Change) {
		if change.Category == enums.SettingsGeneral || change.Category == enums.SettingsDownloads {
			m.Trigger()
		}
	})
	defer unsubscribe()
	ticker := time.NewTicker(desktopUpdateCheckInterval)
	defer ticker.Stop()
	if m.settings.Snapshot().General.AutoCheckUpdates && m.updateDriver() != nil {
		m.Trigger()
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-m.trigger:
			if !m.settings.Snapshot().General.AutoCheckUpdates {
				m.updateConfiguredStatus()
				continue
			}
			m.runAutomaticCheck(ctx, "Desktop 自动更新检查失败")
		case <-ticker.C:
			if !m.settings.Snapshot().General.AutoCheckUpdates {
				m.updateConfiguredStatus()
				continue
			}
			m.runAutomaticCheck(ctx, "Desktop 周期更新检查失败")
		}
	}
}

func (m *DesktopUpdateManager) runAutomaticCheck(ctx context.Context, failureMessage string) {
	status, err := m.CheckNow(ctx)
	if err != nil {
		if ctx.Err() == nil {
			m.logger.Error(context.WithoutCancel(ctx), failureMessage, err, applog.Fields{"component": "desktop-update"})
		}
		return
	}
	policy := m.settings.Snapshot().General.UpdatePolicy
	if !status.UpdateAvailable || policy == "notify" || !status.InstallSupported {
		return
	}
	if _, err := m.DownloadNow(ctx); err != nil && ctx.Err() == nil {
		m.logger.Error(context.WithoutCancel(ctx), "Desktop 自动下载并准备更新失败", err, applog.Fields{"component": "desktop-update", "policy": policy})
	}
}

// Trigger 在相关设置提交变更后请求一次合并的检查。
func (m *DesktopUpdateManager) Trigger() {
	if m == nil {
		return
	}
	select {
	case m.trigger <- struct{}{}:
	default:
	}
}

func (m *DesktopUpdateManager) configuredStatus() DesktopUpdateStatus {
	snapshot := m.settings.Snapshot()
	baseURL := m.releaseBaseURL()
	return DesktopUpdateStatus{
		Enabled: snapshot.General.AutoCheckUpdates, Channel: snapshot.General.UpdateChannel,
		ManifestURL: baseURL, CurrentVersion: constants.ApplicationVersion, Phase: DesktopUpdatePhaseIdle,
		Platform: runtime.GOOS, Architecture: runtime.GOARCH,
		InstallMessage: "当前版本尚未连接自更新安装器",
	}
}

func (m *DesktopUpdateManager) releaseBaseURL() string {
	return m.downloads.ResolveSource("desktop", desktopReleaseDefaultBaseURL)
}

func (m *DesktopUpdateManager) finishCheck(status DesktopUpdateStatus, checkErr error) (DesktopUpdateStatus, error) {
	now := m.clock.Now().UTC()
	status.LastCheckedAt = &now
	status.Running = false
	if checkErr != nil {
		dto := apperror.ToDTO(checkErr)
		status.LastErrorCode, status.LastErrorMessage = dto.Code, dto.Message
		status.Phase = DesktopUpdatePhaseError
	}
	m.storeStatus(status)
	return m.Status(), checkErr
}

func (m *DesktopUpdateManager) updateConfiguredStatus() {
	next := m.configuredStatus()
	previous := m.Status()
	next.LatestVersion = previous.LatestVersion
	next.UpdateAvailable = previous.UpdateAvailable
	next.ReleaseURL = previous.ReleaseURL
	next.PublishedAt = previous.PublishedAt
	next.Notes = previous.Notes
	next.LastCheckedAt = previous.LastCheckedAt
	next.LastErrorCode = previous.LastErrorCode
	next.LastErrorMessage = previous.LastErrorMessage
	next.Phase = previous.Phase
	next.Platform = previous.Platform
	next.Architecture = previous.Architecture
	next.InstallSupported = previous.InstallSupported
	next.InstallMessage = previous.InstallMessage
	next.DownloadedBytes = previous.DownloadedBytes
	next.TotalBytes = previous.TotalBytes
	next.Progress = previous.Progress
	next.ReadyToRestart = previous.ReadyToRestart
	m.storeStatus(next)
}

func (m *DesktopUpdateManager) storeStatus(status DesktopUpdateStatus) {
	m.statusMu.Lock()
	m.status = status
	m.statusMu.Unlock()
}
