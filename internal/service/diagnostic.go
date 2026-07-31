package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/global/appsettings"
	"github.com/Cail-Gainey/MineOps/internal/global/constants"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
)

const (
	diagnosticMaximumServers     = 500
	diagnosticMaximumSparkRows   = 1000
	diagnosticMaximumLogFiles    = 5
	diagnosticMaximumLogFileSize = 512 * 1024
	diagnosticMaximumLogBytes    = 2 * 1024 * 1024
)

var (
	privateKeyBlockPattern = regexp.MustCompile(`(?is)-----BEGIN(?: [A-Z0-9]+)? PRIVATE KEY-----.*?-----END(?: [A-Z0-9]+)? PRIVATE KEY-----`)
	sensitiveValuePattern  = regexp.MustCompile(`(?i)(?:"?(?:password|passphrase|token|secret|authorization|credential(?:id)?|private[_ .-]?key)"?\s*[:=]\s*)(?:"(?:\\.|[^"])*"|'[^']*'|[^\s,;}\]]+)`)
	bearerValuePattern     = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]+`)
	urlUserInfoPattern     = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^/@\s:]+:[^/@\s]+@`)
	opaqueValuePattern     = regexp.MustCompile(`\b[A-Za-z0-9+/=]{40,}\b`)
)

// DiagnosticPackage 描述一份已完成的脱敏诊断归档。
type DiagnosticPackage struct {
	Path        string    `json:"path"`
	SizeBytes   int64     `json:"sizeBytes"`
	GeneratedAt time.Time `json:"generatedAt"`
	LogFiles    int       `json:"logFiles"`
	Warnings    []string  `json:"warnings"`
}

// DiagnosticManager 导出有界的平台、设置、采集、Spark 与脱敏日志证据。
type DiagnosticManager struct {
	clock        model.Clock
	store        repository.Store
	settings     *appsettings.Manager
	logDirectory string
	runtimeMode  applog.RuntimeMode
}

// NewDiagnosticManager 创建诊断导出边界,不接收任何凭据仓储。
func NewDiagnosticManager(clock model.Clock, store repository.Store, settings *appsettings.Manager, logDirectory string, runtimeMode applog.RuntimeMode) (*DiagnosticManager, error) {
	if clock == nil || store == nil || settings == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Diagnostic Clock、Store 和 Settings 不能为空")
	}
	logDirectory = strings.TrimSpace(logDirectory)
	if logDirectory == "" {
		return nil, apperror.New(apperror.CodeValidationRequired, "Diagnostic 日志目录不能为空")
	}
	if runtimeMode != applog.RuntimeDevelopment && runtimeMode != applog.RuntimePackaged {
		return nil, apperror.New(apperror.CodeValidationInvalidArgument, "Diagnostic 运行模式无效")
	}
	return &DiagnosticManager{clock: clock, store: store, settings: settings, logDirectory: filepath.Clean(logDirectory), runtimeMode: runtimeMode}, nil
}

// Export 把一份 .zip 或 .mineops-diagnostic.zip 归档原子写入所选本地路径。
func (m *DiagnosticManager) Export(ctx context.Context, destination string) (DiagnosticPackage, error) {
	destination, err := normalizeDiagnosticDestination(destination)
	if err != nil {
		return DiagnosticPackage{}, err
	}
	if err := ctx.Err(); err != nil {
		return DiagnosticPackage{}, apperror.Wrap(apperror.CodeIOWriteFailed, "诊断导出已取消", err)
	}

	generatedAt := m.clock.Now().UTC()
	warnings := make([]string, 0, 4)
	spark, sparkWarnings := m.sparkSummary(ctx)
	warnings = append(warnings, sparkWarnings...)
	logs, logWarning := m.redactedLogs(ctx)
	if logWarning != "" {
		warnings = append(warnings, logWarning)
	}

	settings := diagnosticSettingsSummary(m.settings.Snapshot())
	manifest := diagnosticManifest{
		Application:   constants.ApplicationName,
		Version:       constants.ApplicationVersion,
		GOOS:          runtime.GOOS,
		GOARCH:        runtime.GOARCH,
		RuntimeMode:   string(m.runtimeMode),
		GeneratedAt:   generatedAt,
		LogDirectory:  m.logDirectory,
		IncludedLogs:  len(logs),
		Warnings:      warnings,
		FormatVersion: 1,
	}

	files := make([]diagnosticArchiveFile, 0, 4+len(logs))
	for _, value := range []struct {
		name  string
		value any
	}{
		{name: "diagnostic.json", value: manifest},
		{name: "settings-summary.json", value: settings},
		{name: "spark-summary.json", value: spark},
	} {
		payload, marshalErr := marshalDiagnosticJSON(value.value)
		if marshalErr != nil {
			return DiagnosticPackage{}, apperror.Wrap(apperror.CodeInternal, "编码诊断摘要失败", marshalErr)
		}
		files = append(files, diagnosticArchiveFile{name: value.name, payload: append(payload, '\n')})
	}
	files = append(files, logs...)

	if err := writeDiagnosticArchive(ctx, destination, generatedAt, files); err != nil {
		return DiagnosticPackage{}, err
	}
	info, err := os.Stat(destination)
	if err != nil {
		return DiagnosticPackage{}, apperror.Wrap(apperror.CodeIOReadFailed, "读取诊断包信息失败", err)
	}
	return DiagnosticPackage{
		Path: destination, SizeBytes: info.Size(), GeneratedAt: generatedAt,
		LogFiles: len(logs), Warnings: append([]string(nil), warnings...),
	}, nil
}

type diagnosticManifest struct {
	Application   string    `json:"application"`
	Version       string    `json:"version"`
	GOOS          string    `json:"goos"`
	GOARCH        string    `json:"goarch"`
	RuntimeMode   string    `json:"runtimeMode"`
	GeneratedAt   time.Time `json:"generatedAt"`
	LogDirectory  string    `json:"logDirectory"`
	IncludedLogs  int       `json:"includedLogs"`
	Warnings      []string  `json:"warnings"`
	FormatVersion int       `json:"formatVersion"`
}

type diagnosticSettings struct {
	SchemaVersion int                          `json:"schemaVersion"`
	General       diagnosticGeneralSettings    `json:"general"`
	Theme         diagnosticThemeSettings      `json:"theme"`
	Paths         diagnosticPathSettings       `json:"paths"`
	Mirrors       diagnosticMirrorSettings     `json:"mirrors"`
	Logging       model.LoggingSettings        `json:"logging"`
	Monitoring    diagnosticMonitoringSettings `json:"monitoring"`
	Firewall      model.FirewallSettings       `json:"firewall"`
	Layout        model.LayoutSettings         `json:"layout"`
	SSH           diagnosticSSHSettings        `json:"ssh"`
	Terminal      model.TerminalSettings       `json:"terminal"`
	Downloads     diagnosticDownloadSettings   `json:"downloads"`
	Storage       diagnosticStorageSettings    `json:"storage"`
}

type diagnosticGeneralSettings struct {
	Language         string `json:"language"`
	LaunchAtStartup  bool   `json:"launchAtStartup"`
	CloseBehavior    string `json:"closeBehavior"`
	TimeFormat       string `json:"timeFormat"`
	UpdateChannel    string `json:"updateChannel"`
	AutoCheckUpdates bool   `json:"autoCheckUpdates"`
}

type diagnosticThemeSettings struct {
	Mode              string  `json:"mode"`
	Preset            string  `json:"preset"`
	Accent            string  `json:"accent"`
	BackgroundMode    string  `json:"backgroundMode"`
	BackgroundColor   string  `json:"backgroundColor"`
	BackgroundFit     string  `json:"backgroundFit"`
	BackgroundOpacity float64 `json:"backgroundOpacity"`
	OverlayStrength   float64 `json:"overlayStrength"`
	BlurPixels        int     `json:"blurPixels"`
	PanelOpacity      float64 `json:"panelOpacity"`
	HighContrast      bool    `json:"highContrast"`
	BackgroundImage   bool    `json:"backgroundImageConfigured"`
}

type diagnosticPathSettings struct {
	ServersDirectory   string `json:"serversDirectory"`
	DownloadsDirectory string `json:"downloadsDirectory"`
}

type diagnosticMirrorSettings struct {
	Java      string `json:"java"`
	Minecraft string `json:"minecraft"`
	Spark     string `json:"spark"`
}

type diagnosticDownloadSource struct {
	Category string `json:"category"`
	Provider string `json:"provider"`
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	Official bool   `json:"official"`
	Enabled  bool   `json:"enabled"`
	Priority int    `json:"priority"`
}

type diagnosticProxySettings struct {
	Mode   string   `json:"mode"`
	Host   string   `json:"host,omitempty"`
	Port   uint16   `json:"port,omitempty"`
	Bypass []string `json:"bypass,omitempty"`
}

type diagnosticDownloadSettings struct {
	Sources               []diagnosticDownloadSource `json:"sources"`
	TimeoutSeconds        int                        `json:"timeoutSeconds"`
	OverallTimeoutSeconds int                        `json:"overallTimeoutSeconds"`
	Retries               int                        `json:"retries"`
	RetryBackoffSeconds   int                        `json:"retryBackoffSeconds"`
	Concurrency           int                        `json:"concurrency"`
	MaxArtifactMiB        int                        `json:"maxArtifactMiB"`
	CacheCapacityMiB      int                        `json:"cacheCapacityMiB"`
	BandwidthLimitKiB     int                        `json:"bandwidthLimitKiB"`
	AutoCleanup           bool                       `json:"autoCleanup"`
	Proxy                 diagnosticProxySettings    `json:"proxy"`
}

type diagnosticMonitoringSettings struct {
	IntervalSeconds            int    `json:"intervalSeconds"`
	RealtimeThrottleMillis     int    `json:"realtimeThrottleMillis"`
	OfflineAfterSeconds        int    `json:"offlineAfterSeconds"`
	RawRetentionDays           int    `json:"rawRetentionDays"`
	MinuteRetentionDays        int    `json:"minuteRetentionDays"`
	HourRetentionDays          int    `json:"hourRetentionDays"`
	DatabaseCapacityMiB        int    `json:"databaseCapacityMiB"`
	MinimumFreeDiskMiB         int    `json:"minimumFreeDiskMiB"`
	MaintenanceIntervalSeconds int    `json:"maintenanceIntervalSeconds"`
	SparkIntervalSeconds       int    `json:"sparkIntervalSeconds"`
	ProfilerDefaultSeconds     int    `json:"profilerDefaultSeconds"`
	ReportPrivacyConfirmation  bool   `json:"reportPrivacyConfirmation"`
	AlertCooldownSeconds       int    `json:"alertCooldownSeconds"`
	AlertNotificationCount     int    `json:"alertNotificationCount"`
	QuietHoursStart            string `json:"quietHoursStart"`
	QuietHoursEnd              string `json:"quietHoursEnd"`
}

type diagnosticSSHSettings struct {
	DefaultPort          uint16   `json:"defaultPort"`
	ConnectTimeoutSec    int      `json:"connectTimeoutSec"`
	HandshakeTimeoutSec  int      `json:"handshakeTimeoutSec"`
	KeepAliveSec         int      `json:"keepAliveSec"`
	MaxFailures          int      `json:"maxFailures"`
	AutoReconnect        bool     `json:"autoReconnect"`
	ReconnectAttempts    int      `json:"reconnectAttempts"`
	ReconnectBackoffSec  int      `json:"reconnectBackoffSec"`
	AuthPriority         []string `json:"authPriority"`
	Compression          bool     `json:"compression"`
	DefaultHostKeyPolicy string   `json:"defaultHostKeyPolicy"`
	PTYTerminalType      string   `json:"ptyTerminalType"`
	DefaultEncoding      string   `json:"defaultEncoding"`
	JumpHostConfigured   bool     `json:"jumpHostConfigured"`
}

type diagnosticStorageSettings struct {
	BackupDirectoryConfigured bool `json:"backupDirectoryConfigured"`
	VerifyBackupAfterCreate   bool `json:"verifyBackupAfterCreate"`
	RequireRestoreConfirm     bool `json:"requireRestoreConfirm"`
	RequireKeyRotateConfirm   bool `json:"requireKeyRotateConfirm"`
}

type diagnosticSparkSummary struct {
	Servers          []diagnosticSparkServer `json:"servers"`
	ServersTruncated bool                    `json:"serversTruncated"`
	HistoryTruncated bool                    `json:"historyTruncated"`
}

type diagnosticSparkServer struct {
	ServerID          string                 `json:"serverID"`
	ServerType        string                 `json:"serverType"`
	Status            string                 `json:"status,omitempty"`
	Platform          string                 `json:"platform,omitempty"`
	Distribution      string                 `json:"distribution,omitempty"`
	Installed         bool                   `json:"installed"`
	PluginVersion     string                 `json:"pluginVersion,omitempty"`
	ParserVersion     string                 `json:"parserVersion,omitempty"`
	CollectionMethod  string                 `json:"collectionMethod,omitempty"`
	TPSSupported      bool                   `json:"tpsSupported"`
	MSPTSupported     bool                   `json:"msptSupported"`
	ReportSupported   bool                   `json:"reportSupported"`
	PermissionGranted bool                   `json:"permissionGranted"`
	RestartRequired   bool                   `json:"restartRequired"`
	DetectedAt        *time.Time             `json:"detectedAt,omitempty"`
	LastErrorCode     string                 `json:"lastErrorCode,omitempty"`
	LatestSnapshot    *diagnosticSparkSample `json:"latestSnapshot,omitempty"`
	LatestReport      *diagnosticSparkReport `json:"latestReport,omitempty"`
	RecentReports     int                    `json:"recentReports"`
}

type diagnosticSparkSample struct {
	TPS5Seconds   float64   `json:"tps5Seconds"`
	TPS1Minute    float64   `json:"tps1Minute"`
	TPS5Minutes   float64   `json:"tps5Minutes"`
	TPS15Minutes  float64   `json:"tps15Minutes"`
	MSPTAvailable bool      `json:"msptAvailable"`
	MSPTMedian    float64   `json:"msptMedian,omitempty"`
	MSPTP95       float64   `json:"msptP95,omitempty"`
	CollectedAt   time.Time `json:"collectedAt"`
}

type diagnosticSparkReport struct {
	Kind       string     `json:"kind"`
	State      string     `json:"state"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

type diagnosticArchiveFile struct {
	name    string
	payload []byte
}

func diagnosticSettingsSummary(snapshot model.SettingsSnapshot) diagnosticSettings {
	sources := make([]diagnosticDownloadSource, 0, len(snapshot.Downloads.Sources))
	for _, source := range snapshot.Downloads.Sources {
		sources = append(sources, diagnosticDownloadSource{
			Category: redactDiagnosticText(source.Category), Provider: redactDiagnosticText(source.Provider),
			Name: redactDiagnosticText(source.Name), Endpoint: diagnosticEndpoint(source.BaseURL),
			Official: source.Official, Enabled: source.Enabled, Priority: source.Priority,
		})
	}
	return diagnosticSettings{
		SchemaVersion: snapshot.SchemaVersion,
		General: diagnosticGeneralSettings{
			Language: snapshot.General.Language, LaunchAtStartup: snapshot.General.LaunchAtStartup,
			CloseBehavior: snapshot.General.CloseBehavior, TimeFormat: snapshot.General.TimeFormat,
			UpdateChannel: snapshot.General.UpdateChannel, AutoCheckUpdates: snapshot.General.AutoCheckUpdates,
		},
		Theme: diagnosticThemeSettings{
			Mode: snapshot.Theme.Mode, Preset: snapshot.Theme.Preset, Accent: snapshot.Theme.Accent, BackgroundMode: snapshot.Theme.BackgroundMode,
			BackgroundColor: snapshot.Theme.BackgroundColor, BackgroundFit: snapshot.Theme.BackgroundFit,
			BackgroundOpacity: snapshot.Theme.BackgroundOpacity, OverlayStrength: snapshot.Theme.OverlayStrength,
			BlurPixels: snapshot.Theme.BlurPixels, PanelOpacity: snapshot.Theme.PanelOpacity, HighContrast: snapshot.Theme.HighContrast,
			BackgroundImage: strings.TrimSpace(snapshot.Paths.BackgroundImage) != "",
		},
		Paths: diagnosticPathSettings{
			ServersDirectory:   redactDiagnosticText(snapshot.Paths.ServersDirectory),
			DownloadsDirectory: redactDiagnosticText(snapshot.Paths.DownloadsDirectory),
		},
		Mirrors: diagnosticMirrorSettings{
			Java: diagnosticEndpoint(snapshot.Mirrors.Java), Minecraft: diagnosticEndpoint(snapshot.Mirrors.Minecraft),
			Spark: diagnosticEndpoint(snapshot.Mirrors.Spark),
		},
		Logging: snapshot.Logging,
		Monitoring: diagnosticMonitoringSettings{
			IntervalSeconds: snapshot.Monitoring.IntervalSeconds, RealtimeThrottleMillis: snapshot.Monitoring.RealtimeThrottleMillis,
			OfflineAfterSeconds: snapshot.Monitoring.OfflineAfterSeconds, RawRetentionDays: snapshot.Monitoring.RawRetentionDays,
			MinuteRetentionDays: snapshot.Monitoring.MinuteRetentionDays, HourRetentionDays: snapshot.Monitoring.HourRetentionDays,
			DatabaseCapacityMiB: snapshot.Monitoring.DatabaseCapacityMiB, MinimumFreeDiskMiB: snapshot.Monitoring.MinimumFreeDiskMiB,
			MaintenanceIntervalSeconds: snapshot.Monitoring.MaintenanceIntervalSeconds, SparkIntervalSeconds: snapshot.Monitoring.SparkIntervalSeconds,
			ProfilerDefaultSeconds: snapshot.Monitoring.ProfilerDefaultSeconds, ReportPrivacyConfirmation: snapshot.Monitoring.ReportPrivacyConfirmation,
			AlertCooldownSeconds: snapshot.Monitoring.AlertCooldownSeconds, AlertNotificationCount: len(snapshot.Monitoring.AlertNotifications),
			QuietHoursStart: snapshot.Monitoring.QuietHoursStart, QuietHoursEnd: snapshot.Monitoring.QuietHoursEnd,
		},
		Firewall: snapshot.Firewall, Layout: snapshot.Layout,
		SSH: diagnosticSSHSettings{
			DefaultPort: snapshot.SSH.DefaultPort, ConnectTimeoutSec: snapshot.SSH.ConnectTimeoutSec,
			HandshakeTimeoutSec: snapshot.SSH.HandshakeTimeoutSec, KeepAliveSec: snapshot.SSH.KeepAliveSec,
			MaxFailures: snapshot.SSH.MaxFailures, AutoReconnect: snapshot.SSH.AutoReconnect,
			ReconnectAttempts: snapshot.SSH.ReconnectAttempts, ReconnectBackoffSec: snapshot.SSH.ReconnectBackoffSec,
			AuthPriority: redactDiagnosticStrings(snapshot.SSH.AuthPriority), Compression: snapshot.SSH.Compression,
			DefaultHostKeyPolicy: snapshot.SSH.DefaultHostKeyPolicy, PTYTerminalType: snapshot.SSH.PTYTerminalType,
			DefaultEncoding: snapshot.SSH.DefaultEncoding, JumpHostConfigured: strings.TrimSpace(snapshot.SSH.DefaultJumpHostID) != "",
		},
		Terminal: snapshot.Terminal,
		Downloads: diagnosticDownloadSettings{
			Sources: sources, TimeoutSeconds: snapshot.Downloads.TimeoutSeconds, OverallTimeoutSeconds: snapshot.Downloads.OverallTimeoutSeconds,
			Retries: snapshot.Downloads.Retries, RetryBackoffSeconds: snapshot.Downloads.RetryBackoffSeconds,
			Concurrency: snapshot.Downloads.Concurrency, MaxArtifactMiB: snapshot.Downloads.MaxArtifactMiB,
			CacheCapacityMiB: snapshot.Downloads.CacheCapacityMiB, BandwidthLimitKiB: snapshot.Downloads.BandwidthLimitKiB,
			AutoCleanup: snapshot.Downloads.AutoCleanup,
			Proxy: diagnosticProxySettings{
				Mode: snapshot.Downloads.Proxy.Mode.String(), Host: redactDiagnosticText(snapshot.Downloads.Proxy.Host),
				Port: snapshot.Downloads.Proxy.Port, Bypass: redactDiagnosticStrings(snapshot.Downloads.Proxy.Bypass),
			},
		},
		Storage: diagnosticStorageSettings{
			BackupDirectoryConfigured: strings.TrimSpace(snapshot.Storage.BackupDirectory) != "",
			VerifyBackupAfterCreate:   snapshot.Storage.VerifyBackupAfterCreate,
			RequireRestoreConfirm:     snapshot.Storage.RequireRestoreConfirm,
			RequireKeyRotateConfirm:   snapshot.Storage.RequireKeyRotateConfirm,
		},
	}
}

func (m *DiagnosticManager) sparkSummary(ctx context.Context) (diagnosticSparkSummary, []string) {
	servers, err := m.store.MinecraftServers().List(ctx, repository.MinecraftServerQuery{IncludeDeleted: true, Limit: diagnosticMaximumServers})
	if err != nil {
		return diagnosticSparkSummary{Servers: []diagnosticSparkServer{}}, []string{"Minecraft Server 状态摘要读取失败"}
	}

	warnings := make([]string, 0, 3)
	snapshots, snapshotErr := m.store.Spark().ListSnapshots(ctx, repository.SparkSnapshotQuery{Limit: diagnosticMaximumSparkRows})
	if snapshotErr != nil {
		warnings = append(warnings, "Spark Snapshot 摘要读取失败")
		snapshots = nil
	}
	reports, reportErr := m.store.Spark().ListReports(ctx, repository.SparkReportQuery{Limit: diagnosticMaximumSparkRows})
	if reportErr != nil {
		warnings = append(warnings, "Spark Report 摘要读取失败")
		reports = nil
	}

	latestSnapshots := make(map[model.ID]model.SparkSnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		if _, exists := latestSnapshots[snapshot.ServerID]; !exists {
			latestSnapshots[snapshot.ServerID] = snapshot
		}
	}
	latestReports := make(map[model.ID]model.SparkReport, len(reports))
	reportCounts := make(map[model.ID]int, len(reports))
	for _, report := range reports {
		reportCounts[report.ServerID]++
		if _, exists := latestReports[report.ServerID]; !exists {
			latestReports[report.ServerID] = report
		}
	}

	result := make([]diagnosticSparkServer, 0, len(servers))
	capabilityReadFailed := false
	for _, server := range servers {
		if err := ctx.Err(); err != nil {
			break
		}
		summary := diagnosticSparkServer{ServerID: server.ID.String(), ServerType: server.Type.String(), RecentReports: reportCounts[server.ID]}
		capability, capabilityErr := m.store.Spark().GetCapability(ctx, server.ID)
		if capabilityErr == nil {
			detectedAt := capability.DetectedAt
			summary.Status = capability.Status.String()
			summary.Platform = redactDiagnosticText(capability.Platform)
			summary.Distribution = redactDiagnosticText(capability.Distribution)
			summary.Installed = capability.Installed
			summary.PluginVersion = redactDiagnosticText(capability.PluginVersion)
			summary.ParserVersion = redactDiagnosticText(capability.ParserVersion)
			summary.CollectionMethod = redactDiagnosticText(capability.CollectionMethod)
			summary.TPSSupported = capability.TPSSupported
			summary.MSPTSupported = capability.MSPTSupported
			summary.ReportSupported = capability.ReportSupported
			summary.PermissionGranted = capability.PermissionGranted
			summary.RestartRequired = capability.RestartRequired
			summary.DetectedAt = &detectedAt
			summary.LastErrorCode = redactDiagnosticText(capability.LastErrorCode)
		} else if !diagnosticErrorCode(capabilityErr, apperror.CodeIONotFound) {
			capabilityReadFailed = true
		}
		if snapshot, exists := latestSnapshots[server.ID]; exists {
			summary.LatestSnapshot = &diagnosticSparkSample{
				TPS5Seconds: snapshot.TPS5Seconds, TPS1Minute: snapshot.TPS1Minute, TPS5Minutes: snapshot.TPS5Minutes,
				TPS15Minutes: snapshot.TPS15Minutes, MSPTAvailable: snapshot.MSPTAvailable,
				MSPTMedian: snapshot.MSPTMedian, MSPTP95: snapshot.MSPTP95, CollectedAt: snapshot.CollectedAt,
			}
		}
		if report, exists := latestReports[server.ID]; exists {
			summary.LatestReport = &diagnosticSparkReport{
				Kind: report.Kind.String(), State: report.State.String(), StartedAt: report.StartedAt,
				FinishedAt: report.FinishedAt, CreatedAt: report.CreatedAt, UpdatedAt: report.UpdatedAt,
			}
		}
		result = append(result, summary)
	}
	if capabilityReadFailed {
		warnings = append(warnings, "部分 Spark Capability 摘要读取失败")
	}
	if ctx.Err() != nil {
		warnings = append(warnings, "Spark 状态摘要因导出取消而不完整")
	}
	return diagnosticSparkSummary{
		Servers: result, ServersTruncated: len(servers) == diagnosticMaximumServers,
		HistoryTruncated: len(snapshots) == diagnosticMaximumSparkRows || len(reports) == diagnosticMaximumSparkRows,
	}, warnings
}

func (m *DiagnosticManager) redactedLogs(ctx context.Context) ([]diagnosticArchiveFile, string) {
	entries, err := os.ReadDir(m.logDirectory)
	if err != nil {
		return nil, "日志目录不可读取"
	}
	type candidate struct {
		name    string
		path    string
		modTime time.Time
	}
	candidates := make([]candidate, 0, len(entries))
	for _, entry := range entries {
		lowerName := strings.ToLower(entry.Name())
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.HasPrefix(lowerName, "mineops-") || !strings.HasSuffix(lowerName, ".log") {
			continue
		}
		path := filepath.Join(m.logDirectory, entry.Name())
		info, infoErr := os.Lstat(path)
		if infoErr != nil || !info.Mode().IsRegular() {
			continue
		}
		candidates = append(candidates, candidate{name: entry.Name(), path: path, modTime: info.ModTime()})
	}
	sort.Slice(candidates, func(left, right int) bool { return candidates[left].modTime.After(candidates[right].modTime) })
	if len(candidates) > diagnosticMaximumLogFiles {
		candidates = candidates[:diagnosticMaximumLogFiles]
	}

	remaining := diagnosticMaximumLogBytes
	files := make([]diagnosticArchiveFile, 0, len(candidates))
	readFailed := false
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil || remaining <= 0 {
			break
		}
		limit := diagnosticMaximumLogFileSize
		if limit > remaining {
			limit = remaining
		}
		payload, readErr := readLogTail(candidate.path, int64(limit))
		if readErr != nil {
			readFailed = true
			continue
		}
		payload = redactLogPayload(payload)
		if len(payload) > remaining {
			payload = payload[len(payload)-remaining:]
			if newline := bytes.IndexByte(payload, '\n'); newline >= 0 {
				payload = payload[newline+1:]
			}
		}
		if len(payload) == 0 {
			continue
		}
		baseName := strings.TrimSuffix(filepath.Base(candidate.name), filepath.Ext(candidate.name))
		files = append(files, diagnosticArchiveFile{name: filepath.ToSlash(filepath.Join("logs", baseName+".redacted.log")), payload: payload})
		remaining -= len(payload)
	}
	if ctx.Err() != nil {
		return files, "日志收集因导出取消而不完整"
	}
	if readFailed {
		return files, "部分日志文件不可读取"
	}
	return files, ""
}

func normalizeDiagnosticDestination(destination string) (string, error) {
	destination = strings.TrimSpace(destination)
	if destination == "" || strings.ContainsAny(destination, "\x00\r\n") {
		return "", apperror.New(apperror.CodeValidationInvalidArgument, "诊断导出路径无效")
	}
	if !strings.EqualFold(filepath.Ext(destination), ".zip") {
		if strings.HasSuffix(strings.ToLower(destination), ".mineops-diagnostic") {
			destination += ".zip"
		} else {
			destination += ".mineops-diagnostic.zip"
		}
	}
	absolute, err := filepath.Abs(filepath.Clean(destination))
	if err != nil {
		return "", apperror.Wrap(apperror.CodeValidationInvalidArgument, "解析诊断导出路径失败", err)
	}
	parent := filepath.Dir(absolute)
	info, err := os.Stat(parent)
	if err != nil {
		return "", apperror.Wrap(apperror.CodeIOWriteFailed, "诊断导出目录不存在或不可访问", err)
	}
	if !info.IsDir() {
		return "", apperror.New(apperror.CodeValidationInvalidArgument, "诊断导出目标目录无效")
	}
	if targetInfo, targetErr := os.Lstat(absolute); targetErr == nil {
		if targetInfo.Mode()&os.ModeSymlink != 0 || !targetInfo.Mode().IsRegular() {
			return "", apperror.New(apperror.CodeValidationConflict, "诊断导出目标不是普通文件")
		}
	} else if !os.IsNotExist(targetErr) {
		return "", apperror.Wrap(apperror.CodeIOWriteFailed, "检查诊断导出目标失败", targetErr)
	}
	return absolute, nil
}

func writeDiagnosticArchive(ctx context.Context, destination string, generatedAt time.Time, files []diagnosticArchiveFile) error {
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".mineops-diagnostic-*.tmp")
	if err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建诊断包临时文件失败", err)
	}
	temporaryPath := temporary.Name()
	completed := false
	defer func() {
		_ = temporary.Close()
		if !completed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return apperror.Wrap(apperror.CodeIOPermissionDenied, "设置诊断包权限失败", err)
	}

	archive := zip.NewWriter(temporary)
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			_ = archive.Close()
			return apperror.Wrap(apperror.CodeIOWriteFailed, "诊断导出已取消", err)
		}
		header := &zip.FileHeader{Name: file.name, Method: zip.Deflate, Modified: generatedAt}
		header.SetMode(0o600)
		writer, createErr := archive.CreateHeader(header)
		if createErr != nil {
			_ = archive.Close()
			return apperror.Wrap(apperror.CodeIOWriteFailed, "创建诊断包条目失败", createErr)
		}
		if _, writeErr := writer.Write(file.payload); writeErr != nil {
			_ = archive.Close()
			return apperror.Wrap(apperror.CodeIOWriteFailed, "写入诊断包条目失败", writeErr)
		}
	}
	if err := archive.Close(); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "完成诊断包压缩失败", err)
	}
	if err := temporary.Sync(); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "同步诊断包失败", err)
	}
	if err := temporary.Close(); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "关闭诊断包失败", err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		targetInfo, targetErr := os.Lstat(destination)
		if targetErr != nil {
			return apperror.Wrap(apperror.CodeIOWriteFailed, "保存诊断包失败", err)
		}
		if targetInfo.Mode()&os.ModeSymlink != 0 || !targetInfo.Mode().IsRegular() {
			return apperror.New(apperror.CodeValidationConflict, "诊断导出目标已变更为非普通文件")
		}
		if removeErr := os.Remove(destination); removeErr != nil && !os.IsNotExist(removeErr) {
			return apperror.Wrap(apperror.CodeIOWriteFailed, "替换已有诊断包失败", errors.Join(err, removeErr))
		}
		if renameErr := os.Rename(temporaryPath, destination); renameErr != nil {
			return apperror.Wrap(apperror.CodeIOWriteFailed, "保存诊断包失败", renameErr)
		}
	}
	completed = true
	return nil
}

func readLogTail(path string, maximum int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	start := info.Size() - maximum
	if start < 0 {
		start = 0
	}
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}
	payload, err := io.ReadAll(io.LimitReader(file, maximum))
	if err != nil {
		return nil, err
	}
	if start > 0 {
		if newline := bytes.IndexByte(payload, '\n'); newline >= 0 {
			payload = payload[newline+1:]
		} else {
			return nil, nil
		}
	}
	return payload, nil
}

func redactLogPayload(payload []byte) []byte {
	payload = []byte(privateKeyBlockPattern.ReplaceAllString(string(payload), "[REDACTED PRIVATE KEY]"))
	lines := bytes.Split(payload, []byte{'\n'})
	result := bytes.NewBuffer(make([]byte, 0, len(payload)))
	for _, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var value any
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err == nil {
			redacted, marshalErr := json.Marshal(redactDiagnosticValue(value, 0))
			if marshalErr == nil {
				result.Write(redacted)
				result.WriteByte('\n')
				continue
			}
		}
		result.WriteString(redactDiagnosticText(string(line)))
		result.WriteByte('\n')
	}
	return result.Bytes()
}

func marshalDiagnosticJSON(value any) ([]byte, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var decoded any
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	return json.MarshalIndent(redactDiagnosticValue(decoded, 0), "", "  ")
}

func redactDiagnosticValue(value any, depth int) any {
	if value == nil {
		return value
	}
	if depth >= 12 {
		return "[TRUNCATED]"
	}
	switch typed := value.(type) {
	case map[string]any:
		redacted := make(map[string]any, len(typed))
		for key, child := range typed {
			if diagnosticSensitiveKey(key) {
				continue
			}
			redacted[key] = redactDiagnosticValue(child, depth+1)
		}
		return redacted
	case []any:
		redacted := make([]any, len(typed))
		for index, child := range typed {
			redacted[index] = redactDiagnosticValue(child, depth+1)
		}
		return redacted
	case string:
		return redactDiagnosticText(typed)
	default:
		return value
	}
}

func redactDiagnosticText(value string) string {
	value = privateKeyBlockPattern.ReplaceAllString(value, "[REDACTED PRIVATE KEY]")
	value = urlUserInfoPattern.ReplaceAllString(value, `${1}[REDACTED]@`)
	value = bearerValuePattern.ReplaceAllString(value, "Bearer [REDACTED]")
	value = sensitiveValuePattern.ReplaceAllString(value, "[REDACTED]")
	value = opaqueValuePattern.ReplaceAllString(value, "[REDACTED OPAQUE VALUE]")
	return value
}

func redactDiagnosticStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	redacted := make([]string, len(values))
	for index, value := range values {
		redacted[index] = redactDiagnosticText(value)
	}
	return redacted
}

func diagnosticSensitiveKey(key string) bool {
	normalized := strings.NewReplacer("_", "", "-", "", ".", "", " ", "").Replace(strings.ToLower(key))
	for _, fragment := range [...]string{"password", "privatekey", "passphrase", "token", "secret", "authorization", "credential"} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

func diagnosticEndpoint(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "[configured]"
	}
	return parsed.Scheme + "://" + parsed.Host
}

func diagnosticErrorCode(err error, code apperror.Code) bool {
	var applicationError *apperror.Error
	return errors.As(err, &applicationError) && applicationError.Code == code
}
