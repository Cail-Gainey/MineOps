package model

import (
	"net/url"
	"path/filepath"
	"strings"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
)

const SettingsSchemaVersion = 5

// SettingsSnapshot is the complete validated application configuration loaded from encrypted SQLite.
type SettingsSnapshot struct {
	SchemaVersion int                `json:"schemaVersion"`
	General       GeneralSettings    `json:"general"`
	Theme         ThemeSettings      `json:"theme"`
	Paths         PathSettings       `json:"paths"`
	Mirrors       MirrorSettings     `json:"mirrors"`
	Logging       LoggingSettings    `json:"logging"`
	Monitoring    MonitoringSettings `json:"monitoring"`
	Firewall      FirewallSettings   `json:"firewall"`
	Layout        LayoutSettings     `json:"layout"`
	SSH           SSHSettings        `json:"ssh"`
	Terminal      TerminalSettings   `json:"terminal"`
	Downloads     DownloadSettings   `json:"downloads"`
	Storage       StorageSettings    `json:"storage"`
}

// GeneralSettings contains language and common desktop behavior.
type GeneralSettings struct {
	Language         string `json:"language"`
	LaunchAtStartup  bool   `json:"launchAtStartup"`
	CloseBehavior    string `json:"closeBehavior"`
	TimeFormat       string `json:"timeFormat"`
	UpdateChannel    string `json:"updateChannel"`
	AutoCheckUpdates bool   `json:"autoCheckUpdates"`
	UpdatePolicy     string `json:"updatePolicy"`
}

// ThemeSettings contains the persisted semantic theme selection.
type ThemeSettings struct {
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
}

// PathSettings contains controlled local default directories.
type PathSettings struct {
	ServersDirectory   string `json:"serversDirectory"`
	DownloadsDirectory string `json:"downloadsDirectory"`
	BackgroundImage    string `json:"backgroundImage"`
}

// MirrorSettings contains approved source overrides without credential material.
type MirrorSettings struct {
	Java      string `json:"java"`
	Minecraft string `json:"minecraft"`
	Spark     string `json:"spark"`
}

// DownloadSourceSettings is one ordered official or mirror endpoint in the download source registry.
type DownloadSourceSettings struct {
	Category string `json:"category"`
	Provider string `json:"provider"`
	Name     string `json:"name"`
	BaseURL  string `json:"baseURL"`
	ProbeURL string `json:"probeURL"`
	Official bool   `json:"official"`
	Enabled  bool   `json:"enabled"`
	Priority int    `json:"priority"`
}

// ProxySettings contains non-secret proxy routing and an encrypted credential reference.
type ProxySettings struct {
	Mode         enums.ProxyMode `json:"mode"`
	Host         string          `json:"host"`
	Port         uint16          `json:"port"`
	Bypass       []string        `json:"bypass"`
	CredentialID string          `json:"credentialID,omitempty"`
}

// DownloadSettings contains source, timeout, retry, concurrency, proxy, and cache policy.
type DownloadSettings struct {
	Sources               []DownloadSourceSettings `json:"sources"`
	TimeoutSeconds        int                      `json:"timeoutSeconds"`
	OverallTimeoutSeconds int                      `json:"overallTimeoutSeconds"`
	Retries               int                      `json:"retries"`
	RetryBackoffSeconds   int                      `json:"retryBackoffSeconds"`
	Concurrency           int                      `json:"concurrency"`
	MaxArtifactMiB        int                      `json:"maxArtifactMiB"`
	CacheCapacityMiB      int                      `json:"cacheCapacityMiB"`
	BandwidthLimitKiB     int                      `json:"bandwidthLimitKiB"`
	AutoCleanup           bool                     `json:"autoCleanup"`
	Proxy                 ProxySettings            `json:"proxy"`
}

// LoggingSettings contains runtime log level and retention limits.
type LoggingSettings struct {
	Level            string `json:"level"`
	MaxFileMiB       int    `json:"maxFileMiB"`
	RetentionDays    int    `json:"retentionDays"`
	TotalCapacityMiB int    `json:"totalCapacityMiB"`
}

// MonitoringSettings contains collection and retention defaults.
type MonitoringSettings struct {
	IntervalSeconds            int      `json:"intervalSeconds"`
	RealtimeThrottleMillis     int      `json:"realtimeThrottleMillis"`
	OfflineAfterSeconds        int      `json:"offlineAfterSeconds"`
	RawRetentionDays           int      `json:"rawRetentionDays"`
	MinuteRetentionDays        int      `json:"minuteRetentionDays"`
	HourRetentionDays          int      `json:"hourRetentionDays"`
	DatabaseCapacityMiB        int      `json:"databaseCapacityMiB"`
	MinimumFreeDiskMiB         int      `json:"minimumFreeDiskMiB"`
	MaintenanceIntervalSeconds int      `json:"maintenanceIntervalSeconds"`
	SparkIntervalSeconds       int      `json:"sparkIntervalSeconds"`
	ProfilerDefaultSeconds     int      `json:"profilerDefaultSeconds"`
	ReportPrivacyConfirmation  bool     `json:"reportPrivacyConfirmation"`
	AlertCooldownSeconds       int      `json:"alertCooldownSeconds"`
	AlertNotifications         []string `json:"alertNotifications"`
	QuietHoursStart            string   `json:"quietHoursStart"`
	QuietHoursEnd              string   `json:"quietHoursEnd"`
}

// FirewallSettings contains the preferred firewall backend and confirmation behavior.
type FirewallSettings struct {
	Provider                  string `json:"provider"`
	DefaultPolicy             string `json:"defaultPolicy"`
	AutoOpenOnInstall         bool   `json:"autoOpenOnInstall"`
	SyncOnPortChange          bool   `json:"syncOnPortChange"`
	RemoveOldPort             bool   `json:"removeOldPort"`
	RequireDestructiveConfirm bool   `json:"requireDestructiveConfirm"`
}

// LayoutSettings contains the persisted Root Layout visibility and sizing preferences.
type LayoutSettings struct {
	SidebarCollapsed bool `json:"sidebarCollapsed"`
	SidebarVisible   bool `json:"sidebarVisible"`
	SidebarWidth     int  `json:"sidebarWidth"`
	TopBarVisible    bool `json:"topBarVisible"`
	BottomBarVisible bool `json:"bottomBarVisible"`
}

// SSHSettings contains secure global defaults inherited by SSH Sessions without explicit overrides.
type SSHSettings struct {
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
	DefaultJumpHostID    string   `json:"defaultJumpHostID,omitempty"`
}

// StorageSettings contains non-secret backup preferences and destructive-operation safeguards.
type StorageSettings struct {
	BackupDirectory         string `json:"backupDirectory"`
	VerifyBackupAfterCreate bool   `json:"verifyBackupAfterCreate"`
	RequireRestoreConfirm   bool   `json:"requireRestoreConfirm"`
	RequireKeyRotateConfirm bool   `json:"requireKeyRotateConfirm"`
}

// TerminalSettings contains xterm typography, cursor, behavior, and colour preferences.
type TerminalSettings struct {
	FontFamily    string   `json:"fontFamily"`
	FontSize      int      `json:"fontSize"`
	LetterSpacing int      `json:"letterSpacing"`
	LineHeight    float64  `json:"lineHeight"`
	Scrollback    int      `json:"scrollback"`
	CursorStyle   string   `json:"cursorStyle"`
	CursorBlink   bool     `json:"cursorBlink"`
	CursorWidth   int      `json:"cursorWidth"`
	ThemePreset   string   `json:"themePreset"`
	Foreground    string   `json:"foreground"`
	Background    string   `json:"background"`
	Cursor        string   `json:"cursor"`
	Selection     string   `json:"selection"`
	ANSIColours   []string `json:"ansiColours"`
	CopyOnSelect  bool     `json:"copyOnSelect"`
	AutoFocus     bool     `json:"autoFocus"`
	BellStyle     string   `json:"bellStyle"`
}

// DefaultSettings returns the embedded versioned settings baseline.
func DefaultSettings() SettingsSnapshot {
	return SettingsSnapshot{
		SchemaVersion: SettingsSchemaVersion,
		General: GeneralSettings{
			Language: "zh-CN", CloseBehavior: "quit", TimeFormat: "24h",
			UpdateChannel: "stable", AutoCheckUpdates: true, UpdatePolicy: "notify",
		},
		Theme: ThemeSettings{
			Mode: "system", Preset: "mineops", Accent: "emerald", BackgroundMode: "theme", BackgroundColor: "#0f172a",
			BackgroundFit: "cover", BackgroundOpacity: 1, OverlayStrength: 0.1, PanelOpacity: 0.94,
		},
		Paths:   PathSettings{ServersDirectory: "MineOps/Servers", DownloadsDirectory: "MineOps/Downloads"},
		Logging: LoggingSettings{Level: "info", MaxFileMiB: 20, RetentionDays: 14, TotalCapacityMiB: 500},
		Monitoring: MonitoringSettings{
			IntervalSeconds: 5, RealtimeThrottleMillis: 1000, OfflineAfterSeconds: 30,
			RawRetentionDays: 7, MinuteRetentionDays: 90, HourRetentionDays: 730,
			DatabaseCapacityMiB: 2048, MinimumFreeDiskMiB: 256, MaintenanceIntervalSeconds: 60,
			SparkIntervalSeconds: 15, ProfilerDefaultSeconds: 60, ReportPrivacyConfirmation: true,
			AlertCooldownSeconds: 300, AlertNotifications: []string{"desktop"},
		},
		Firewall: FirewallSettings{
			Provider: "auto", DefaultPolicy: enums.FirewallAutomatic.String(), AutoOpenOnInstall: true,
			SyncOnPortChange: true, RemoveOldPort: true, RequireDestructiveConfirm: true,
		},
		Layout: LayoutSettings{SidebarVisible: true, SidebarWidth: 232, TopBarVisible: true, BottomBarVisible: true},
		SSH: SSHSettings{
			DefaultPort: 22, ConnectTimeoutSec: 10, HandshakeTimeoutSec: 15, KeepAliveSec: 30,
			MaxFailures: 3, AutoReconnect: true, ReconnectAttempts: 3, ReconnectBackoffSec: 2,
			AuthPriority: []string{"private_key", "agent", "password"}, DefaultHostKeyPolicy: "strict",
			PTYTerminalType: "xterm-256color", DefaultEncoding: "UTF-8",
		},
		Terminal: TerminalSettings{
			FontFamily: "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace",
			FontSize:   13, LineHeight: 1.2, Scrollback: 10_000, CursorStyle: "block",
			CursorBlink: true, CursorWidth: 1, ThemePreset: "semantic",
			Foreground: "#e5e7eb", Background: "#0d1014", Cursor: "#34d399", Selection: "#14532d",
			ANSIColours: []string{
				"#0f172a", "#dc2626", "#16a34a", "#ca8a04", "#2563eb", "#9333ea", "#0891b2", "#d1d5db",
				"#64748b", "#f87171", "#4ade80", "#facc15", "#60a5fa", "#c084fc", "#22d3ee", "#f8fafc",
			},
			AutoFocus: true, BellStyle: "none",
		},
		Downloads: DownloadSettings{
			Sources: []DownloadSourceSettings{
				{Category: "minecraft", Provider: "mojang", Name: "Mojang", BaseURL: "https://piston-meta.mojang.com", ProbeURL: "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json", Official: true, Enabled: true, Priority: 10},
				{Category: "minecraft", Provider: "papermc", Name: "PaperMC", BaseURL: "https://fill.papermc.io", ProbeURL: "https://fill.papermc.io/v3/projects/paper", Official: true, Enabled: true, Priority: 10},
				{Category: "minecraft", Provider: "purpur", Name: "Purpur", BaseURL: "https://api.purpurmc.org", ProbeURL: "https://api.purpurmc.org/v2/purpur", Official: true, Enabled: true, Priority: 10},
				{Category: "minecraft", Provider: "fabric", Name: "Fabric", BaseURL: "https://meta.fabricmc.net", ProbeURL: "https://meta.fabricmc.net/v2/versions/game", Official: true, Enabled: true, Priority: 10},
				{Category: "minecraft", Provider: "quilt", Name: "Quilt", BaseURL: "https://meta.quiltmc.org", ProbeURL: "https://meta.quiltmc.org/v3/versions/game", Official: true, Enabled: true, Priority: 10},
				{Category: "minecraft", Provider: "spigot", Name: "Spigot BuildTools", BaseURL: "https://hub.spigotmc.org", ProbeURL: "https://hub.spigotmc.org/jenkins/job/BuildTools/lastSuccessfulBuild/api/json", Official: true, Enabled: true, Priority: 10},
				{Category: "minecraft", Provider: "bungeecord", Name: "BungeeCord Jenkins", BaseURL: "https://ci.md-5.net", ProbeURL: "https://ci.md-5.net/job/BungeeCord/lastSuccessfulBuild/api/json", Official: true, Enabled: true, Priority: 10},
				{Category: "minecraft", Provider: "forge", Name: "Minecraft Forge Maven", BaseURL: "https://maven.minecraftforge.net", ProbeURL: "https://maven.minecraftforge.net/net/minecraftforge/forge/maven-metadata.xml", Official: true, Enabled: true, Priority: 10},
				{Category: "minecraft", Provider: "neoforge", Name: "NeoForge Maven", BaseURL: "https://maven.neoforged.net/releases", ProbeURL: "https://maven.neoforged.net/releases/net/neoforged/neoforge/maven-metadata.xml", Official: true, Enabled: true, Priority: 10},
				{Category: "java", Provider: "adoptium", Name: "Eclipse Adoptium", BaseURL: "https://api.adoptium.net", ProbeURL: "https://api.adoptium.net/v3/info/available_releases", Official: true, Enabled: true, Priority: 10},
				{Category: "spark", Provider: "spark", Name: "Lucko spark Jenkins", BaseURL: "https://ci.lucko.me", ProbeURL: "https://ci.lucko.me/job/spark/525/api/json", Official: true, Enabled: true, Priority: 10},
				{Category: "spark", Provider: "spark-modrinth", Name: "Modrinth spark CDN", BaseURL: "https://cdn.modrinth.com", ProbeURL: "https://api.modrinth.com/v2/project/l6YH9Als", Official: true, Enabled: true, Priority: 10},
				{Category: "desktop", Provider: "desktop", Name: "MineOps Desktop", BaseURL: "https://api.github.com/repos/Cail-Gainey/MineOps/releases", ProbeURL: "https://api.github.com/repos/Cail-Gainey/MineOps/releases?per_page=1", Official: true, Enabled: true, Priority: 10},
			},
			TimeoutSeconds: 30, OverallTimeoutSeconds: 1800, Retries: 2, RetryBackoffSeconds: 2,
			Concurrency: 3, MaxArtifactMiB: 2048, CacheCapacityMiB: 10_240,
			AutoCleanup: true, Proxy: ProxySettings{Mode: enums.ProxySystem},
		},
		Storage: StorageSettings{
			BackupDirectory: "MineOps/Backups", VerifyBackupAfterCreate: true,
			RequireRestoreConfirm: true, RequireKeyRotateConfirm: true,
		},
	}
}

// Validate checks the complete settings snapshot without performing I/O.
func (s SettingsSnapshot) Validate() error {
	if s.SchemaVersion != SettingsSchemaVersion {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Settings Schema Version 不受支持")
	}
	if s.General.Language != "zh-CN" && s.General.Language != "en-US" {
		return apperror.New(apperror.CodeValidationInvalidArgument, "语言设置无效")
	}
	if s.General.CloseBehavior != "quit" && s.General.CloseBehavior != "minimize" || s.General.TimeFormat != "24h" && s.General.TimeFormat != "12h" || s.General.UpdateChannel != "stable" && s.General.UpdateChannel != "beta" || s.General.UpdatePolicy != "notify" && s.General.UpdatePolicy != "download" && s.General.UpdatePolicy != "prompt_restart" {
		return apperror.New(apperror.CodeValidationInvalidArgument, "通用桌面行为设置无效")
	}
	if s.Theme.Mode != "light" && s.Theme.Mode != "dark" && s.Theme.Mode != "system" {
		return apperror.New(apperror.CodeValidationInvalidArgument, "主题模式无效")
	}
	if s.Theme.Preset != "mineops" && s.Theme.Preset != "forest" && s.Theme.Preset != "ocean" && s.Theme.Preset != "amethyst" && s.Theme.Preset != "graphite" && s.Theme.Preset != "sunset" {
		return apperror.New(apperror.CodeValidationInvalidArgument, "主题套装无效")
	}
	if s.Theme.Accent != "emerald" && s.Theme.Accent != "amber" && s.Theme.Accent != "azure" && s.Theme.Accent != "violet" && s.Theme.Accent != "rose" {
		return apperror.New(apperror.CodeValidationInvalidArgument, "强调色无效")
	}
	if s.Theme.BackgroundMode != "theme" && s.Theme.BackgroundMode != "color" && s.Theme.BackgroundMode != "image" || s.Theme.BackgroundFit != "cover" && s.Theme.BackgroundFit != "contain" && s.Theme.BackgroundFit != "center" && s.Theme.BackgroundFit != "tile" {
		return apperror.New(apperror.CodeValidationInvalidArgument, "背景模式或适配方式无效")
	}
	if !validHexColour(s.Theme.BackgroundColor) || s.Theme.BackgroundOpacity < 0 || s.Theme.BackgroundOpacity > 1 || s.Theme.OverlayStrength < 0 || s.Theme.OverlayStrength > 1 || s.Theme.BlurPixels < 0 || s.Theme.BlurPixels > 40 || s.Theme.PanelOpacity < 0.65 || s.Theme.PanelOpacity > 1 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "背景颜色、透明度、遮罩、模糊或面板透明度无效")
	}
	if invalidControlledPath(s.Paths.ServersDirectory) || invalidControlledPath(s.Paths.DownloadsDirectory) {
		return apperror.New(apperror.CodeValidationInvalidArgument, "默认目录必须是受控相对路径")
	}
	if !validOptionalHTTPSBaseURL(s.Mirrors.Java) || !validOptionalHTTPSBaseURL(s.Mirrors.Minecraft) || !validOptionalHTTPSBaseURL(s.Mirrors.Spark) {
		return apperror.New(apperror.CodeValidationInvalidArgument, "镜像地址必须是无凭据 HTTPS URL")
	}
	if s.Logging.MaxFileMiB < 1 || s.Logging.RetentionDays < 1 || s.Logging.TotalCapacityMiB < s.Logging.MaxFileMiB {
		return apperror.New(apperror.CodeValidationInvalidArgument, "日志轮转设置无效")
	}
	if s.Monitoring.IntervalSeconds < 1 || s.Monitoring.IntervalSeconds > 300 || s.Monitoring.RealtimeThrottleMillis < 100 || s.Monitoring.OfflineAfterSeconds < s.Monitoring.IntervalSeconds*2 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "监控采样、实时节流或离线判定设置无效")
	}
	if s.Monitoring.RawRetentionDays < 1 || s.Monitoring.MinuteRetentionDays < s.Monitoring.RawRetentionDays || s.Monitoring.HourRetentionDays < s.Monitoring.MinuteRetentionDays {
		return apperror.New(apperror.CodeValidationInvalidArgument, "监控保留设置无效")
	}
	if s.Monitoring.DatabaseCapacityMiB < 128 || s.Monitoring.MinimumFreeDiskMiB < 64 || s.Monitoring.MaintenanceIntervalSeconds < 30 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "监控容量或维护周期设置无效")
	}
	quietHoursIncomplete := (s.Monitoring.QuietHoursStart == "") != (s.Monitoring.QuietHoursEnd == "")
	quietHoursAmbiguous := s.Monitoring.QuietHoursStart != "" && s.Monitoring.QuietHoursStart == s.Monitoring.QuietHoursEnd
	if s.Monitoring.SparkIntervalSeconds < 5 || s.Monitoring.ProfilerDefaultSeconds < 10 || s.Monitoring.ProfilerDefaultSeconds > 3600 || s.Monitoring.AlertCooldownSeconds < 0 || !validAlertNotifications(s.Monitoring.AlertNotifications) || !validClockTime(s.Monitoring.QuietHoursStart) || !validClockTime(s.Monitoring.QuietHoursEnd) || quietHoursIncomplete || quietHoursAmbiguous {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Spark、Profiler 或告警默认设置无效")
	}
	if s.Firewall.Provider != "auto" && s.Firewall.Provider != "ufw" && s.Firewall.Provider != "firewalld" && s.Firewall.Provider != "disabled" || !enums.FirewallPolicy(s.Firewall.DefaultPolicy).Valid() {
		return apperror.New(apperror.CodeValidationInvalidArgument, "防火墙 Provider 或默认策略无效")
	}
	if s.Layout.SidebarWidth < 180 || s.Layout.SidebarWidth > 360 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "侧栏宽度必须在 180 到 360 之间")
	}
	if s.SSH.DefaultPort == 0 || s.SSH.ConnectTimeoutSec < 1 || s.SSH.HandshakeTimeoutSec < 1 || s.SSH.KeepAliveSec < 0 || s.SSH.MaxFailures < 1 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "SSH 全局超时或失败次数无效")
	}
	if s.SSH.ReconnectAttempts < 0 || s.SSH.ReconnectBackoffSec < 0 || s.SSH.DefaultHostKeyPolicy != "strict" && s.SSH.DefaultHostKeyPolicy != "trust_on_first_use" {
		return apperror.New(apperror.CodeValidationInvalidArgument, "SSH 重连或主机密钥策略无效")
	}
	if strings.TrimSpace(s.SSH.PTYTerminalType) == "" || s.SSH.DefaultEncoding != "UTF-8" {
		return apperror.New(apperror.CodeValidationInvalidArgument, "SSH PTY Terminal 类型或字符编码无效")
	}
	if s.SSH.DefaultJumpHostID != "" && !ID(s.SSH.DefaultJumpHostID).Valid() {
		return apperror.New(apperror.CodeValidationInvalidArgument, "默认 Jump Host ID 无效")
	}
	if !validAuthPriority(s.SSH.AuthPriority) {
		return apperror.New(apperror.CodeValidationInvalidArgument, "SSH 认证优先级无效")
	}
	if s.Terminal.FontSize < 8 || s.Terminal.FontSize > 40 || s.Terminal.LetterSpacing < -2 || s.Terminal.LetterSpacing > 10 || s.Terminal.LineHeight < 1 || s.Terminal.LineHeight > 2 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Terminal 字体或行距设置无效")
	}
	if s.Terminal.Scrollback < 100 || s.Terminal.Scrollback > 200_000 || s.Terminal.CursorWidth < 1 || s.Terminal.CursorWidth > 5 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Terminal Scrollback 或光标宽度无效")
	}
	if s.Terminal.CursorStyle != "block" && s.Terminal.CursorStyle != "underline" && s.Terminal.CursorStyle != "bar" {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Terminal 光标样式无效")
	}
	if s.Terminal.ThemePreset != "semantic" && s.Terminal.ThemePreset != "nord" && s.Terminal.ThemePreset != "solarized" && s.Terminal.ThemePreset != "custom" {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Terminal 配色预设无效")
	}
	if !validTerminalColours(s.Terminal) || s.Terminal.BellStyle != "none" && s.Terminal.BellStyle != "sound" && s.Terminal.BellStyle != "visual" && s.Terminal.BellStyle != "both" {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Terminal 颜色或 Bell 设置无效")
	}
	if s.Downloads.TimeoutSeconds < 1 || s.Downloads.TimeoutSeconds > 3600 || s.Downloads.OverallTimeoutSeconds < s.Downloads.TimeoutSeconds || s.Downloads.OverallTimeoutSeconds > 86400 || s.Downloads.Retries < 0 || s.Downloads.Retries > 3 || s.Downloads.RetryBackoffSeconds < 0 || s.Downloads.RetryBackoffSeconds > 300 || s.Downloads.Concurrency < 1 || s.Downloads.Concurrency > 16 || s.Downloads.BandwidthLimitKiB < 0 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "下载超时、重试或并发设置无效")
	}
	if invalidControlledPath(s.Storage.BackupDirectory) {
		return apperror.New(apperror.CodeValidationInvalidArgument, "备份目录必须是受控相对路径")
	}
	if s.Downloads.MaxArtifactMiB < 1 || s.Downloads.CacheCapacityMiB < s.Downloads.MaxArtifactMiB || !s.Downloads.Proxy.Mode.Valid() {
		return apperror.New(apperror.CodeValidationInvalidArgument, "下载大小、缓存容量或代理模式无效")
	}
	if s.Downloads.Proxy.Mode == enums.ProxyHTTP || s.Downloads.Proxy.Mode == enums.ProxyHTTPS || s.Downloads.Proxy.Mode == enums.ProxySOCKS5 {
		if strings.TrimSpace(s.Downloads.Proxy.Host) == "" || s.Downloads.Proxy.Port == 0 {
			return apperror.New(apperror.CodeValidationRequired, "手动代理 Host 和 Port 不能为空")
		}
	}
	seenSources := make(map[string]bool, len(s.Downloads.Sources))
	for _, source := range s.Downloads.Sources {
		parsed, err := url.Parse(source.BaseURL)
		probe, probeErr := url.Parse(source.ProbeURL)
		key := source.Category + "\x00" + source.Name
		if strings.TrimSpace(source.Category) == "" || strings.TrimSpace(source.Provider) == "" || strings.TrimSpace(source.Name) == "" || source.Priority < 0 || seenSources[key] || err != nil || probeErr != nil || parsed.Scheme != "https" && parsed.Scheme != "http" || parsed.Host == "" || probe.Scheme != "https" && probe.Scheme != "http" || probe.Host == "" {
			return apperror.New(apperror.CodeValidationInvalidArgument, "下载源注册表存在无效或重复条目")
		}
		seenSources[key] = true
	}
	return nil
}

func validTerminalColours(settings TerminalSettings) bool {
	colours := append([]string{settings.Foreground, settings.Background, settings.Cursor, settings.Selection}, settings.ANSIColours...)
	if len(settings.ANSIColours) != 16 {
		return false
	}
	for _, colour := range colours {
		if len(colour) != 7 || colour[0] != '#' {
			return false
		}
		for _, character := range colour[1:] {
			if !isHexDigit(character) {
				return false
			}
		}
	}
	return true
}

func validHexColour(value string) bool {
	if len(value) != 7 || value[0] != '#' {
		return false
	}
	for _, character := range value[1:] {
		if !isHexDigit(character) {
			return false
		}
	}
	return true
}

func validOptionalHTTPSBaseURL(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil
}

func validAlertNotifications(values []string) bool {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if value != "desktop" && value != "sound" || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func validClockTime(value string) bool {
	if value == "" {
		return true
	}
	if len(value) != 5 || value[2] != ':' {
		return false
	}
	return value[0] >= '0' && value[0] <= '2' && value[1] >= '0' && value[1] <= '9' && value[3] >= '0' && value[3] <= '5' && value[4] >= '0' && value[4] <= '9' && (value[0] != '2' || value[1] <= '3')
}

func isHexDigit(character rune) bool {
	return character >= '0' && character <= '9' || character >= 'A' && character <= 'F' || character >= 'a' && character <= 'f'
}

func validAuthPriority(values []string) bool {
	if len(values) != 3 {
		return false
	}
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if value != "private_key" && value != "agent" && value != "password" || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func invalidControlledPath(value string) bool {
	cleaned := filepath.Clean(strings.TrimSpace(value))
	return cleaned == "." || filepath.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator))
}
