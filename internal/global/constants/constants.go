// Package constants contains immutable MineOps event, path, header, and default values.
package constants

const (
	ApplicationName            = "MineOps"
	DevelopmentApplicationName = "MineOps-Development"
	LogsDirectoryName          = "logs"
	DataDirectoryName          = "data"
	DatabaseFileName           = "mineops.db"
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

// ApplicationVersion is the build-time MineOps version. Release builds inject
// the Git tag value with -ldflags; development builds use the repository default.
var ApplicationVersion = "1.0.0"

// ApplicationUserAgent identifies MineOps requests with the injected version.
var ApplicationUserAgent = "MineOps/1.0.0"
