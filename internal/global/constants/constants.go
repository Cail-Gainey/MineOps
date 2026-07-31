// Package constants 存放 MineOps 不可变的事件名、路径、请求头与默认值。
package constants

const (
	ApplicationName            = "MineOps"
	DevelopmentApplicationName = "MineOps-Development"
	LogsDirectoryName          = "logs"
	DataDirectoryName          = "data"
	DatabaseFileName           = "mineops.db"
	// MetricsDatabaseFileName 是未加密的监控时序库:只放数值指标与 Spark Snapshot,不含凭据或隐私。
	MetricsDatabaseFileName    = "mineops-metrics.db"
	BackupFileExtension        = ".mineops-backup"
	CorrelationHeader          = "X-MineOps-Correlation-ID"
	SecondInstanceEventName    = "mineops:lifecycle:second-instance"
	EventSpikeBatchName        = "mineops:spike:event-batch"
	OperationProgressEventName = "mineops:operation:progress"
	TerminalEventName          = "mineops:terminal:event"
	ConsoleEventName           = "mineops:console:event"
	MetricRealtimeEventName    = "mineops:metric:realtime"
	AlertEventName             = "mineops:alert:event"
	PlayerEventName            = "mineops:player:event"
	QuitBlockedEventName       = "mineops:lifecycle:quit-blocked"
	SettingsChangedEventName   = "mineops:settings:changed"
	FileDropEventName          = "mineops:file-drop"
	DefaultBusyTimeoutMillis   = 5_000
	DefaultShutdownTimeoutSec  = 10
	DesktopProtocolVersion     = 1
)

// ApplicationVersion 是构建期注入的 MineOps 版本号。发布构建通过 -ldflags
// 注入 Git tag 值;开发构建使用仓库内的默认值。
var ApplicationVersion = "1.0.0"

// ApplicationUserAgent 用注入的版本号标识 MineOps 发出的请求。
var ApplicationUserAgent = "MineOps/1.0.0"
