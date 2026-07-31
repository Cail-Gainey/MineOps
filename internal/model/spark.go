package model

import (
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
)

const SparkSchemaVersion = 1

// SparkCapability 记录独立探测得到的安装、兼容性、权限与采集方式证据。
type SparkCapability struct {
	ServerID          ID                        `json:"serverID"`
	Status            enums.SparkStatus         `json:"status"`
	ServerType        enums.MinecraftServerType `json:"serverType"`
	Platform          string                    `json:"platform"`
	Distribution      string                    `json:"distribution"`
	Installed         bool                      `json:"installed"`
	PluginVersion     string                    `json:"pluginVersion,omitempty"`
	ParserVersion     string                    `json:"parserVersion,omitempty"`
	CollectionMethod  string                    `json:"collectionMethod,omitempty"`
	SourceSchemaHash  string                    `json:"sourceSchemaHash,omitempty"`
	TPSSupported      bool                      `json:"tpsSupported"`
	MSPTSupported     bool                      `json:"msptSupported"`
	ReportSupported   bool                      `json:"reportSupported"`
	PermissionGranted bool                      `json:"permissionGranted"`
	ArtifactPath      string                    `json:"artifactPath,omitempty"`
	BackupPath        string                    `json:"backupPath,omitempty"`
	InstallManifest   string                    `json:"installManifest,omitempty"`
	RestartRequired   bool                      `json:"restartRequired"`
	DetectedAt        time.Time                 `json:"detectedAt"`
	LastErrorCode     string                    `json:"lastErrorCode,omitempty"`
	LastError         string                    `json:"lastError,omitempty"`
	SchemaVersion     int                       `json:"schemaVersion"`
}

// SparkSnapshot 存放一条带版本的 TPS/MSPT 健康观测,不臆造缺失字段。
type SparkSnapshot struct {
	ID                 ID                        `json:"id"`
	ServerID           ID                        `json:"serverID"`
	SourceID           ID                        `json:"sourceID"`
	ServerType         enums.MinecraftServerType `json:"serverType"`
	Platform           string                    `json:"platform"`
	PluginVersion      string                    `json:"pluginVersion"`
	ParserVersion      string                    `json:"parserVersion"`
	CollectionMethod   string                    `json:"collectionMethod"`
	SourceSchemaHash   string                    `json:"sourceSchemaHash"`
	TPS5Seconds        float64                   `json:"tps5Seconds"`
	TPS5SecondsCapped  bool                      `json:"tps5SecondsCapped"`
	TPS10Seconds       float64                   `json:"tps10Seconds"`
	TPS10SecondsCapped bool                      `json:"tps10SecondsCapped"`
	TPS1Minute         float64                   `json:"tps1Minute"`
	TPS1MinuteCapped   bool                      `json:"tps1MinuteCapped"`
	TPS5Minutes        float64                   `json:"tps5Minutes"`
	TPS5MinutesCapped  bool                      `json:"tps5MinutesCapped"`
	TPS15Minutes       float64                   `json:"tps15Minutes"`
	TPS15MinutesCapped bool                      `json:"tps15MinutesCapped"`
	MSPTAvailable      bool                      `json:"msptAvailable"`
	MSPTMinimum        float64                   `json:"msptMinimum,omitempty"`
	MSPTMedian         float64                   `json:"msptMedian,omitempty"`
	MSPTP95            float64                   `json:"msptP95,omitempty"`
	MSPTMaximum        float64                   `json:"msptMaximum,omitempty"`
	CollectedAt        time.Time                 `json:"collectedAt"`
	SchemaVersion      int                       `json:"schemaVersion"`
}

// SparkReport 存放持久化的健康或性能分析流程状态,以及涉及隐私的查看器引用。
type SparkReport struct {
	ID                  ID                     `json:"id"`
	ServerID            ID                     `json:"serverID"`
	OperationID         *ID                    `json:"operationID,omitempty"`
	Kind                enums.SparkReportKind  `json:"kind"`
	State               enums.SparkReportState `json:"state"`
	DurationSeconds     int                    `json:"durationSeconds,omitempty"`
	ReportURL           string                 `json:"reportURL,omitempty"`
	PrivacyAcknowledged bool                   `json:"privacyAcknowledged"`
	PluginVersion       string                 `json:"pluginVersion"`
	ParserVersion       string                 `json:"parserVersion"`
	StartedAt           *time.Time             `json:"startedAt,omitempty"`
	FinishedAt          *time.Time             `json:"finishedAt,omitempty"`
	ErrorCode           string                 `json:"errorCode,omitempty"`
	ErrorMessage        string                 `json:"errorMessage,omitempty"`
	CreatedAt           time.Time              `json:"createdAt"`
	UpdatedAt           time.Time              `json:"updatedAt"`
	SchemaVersion       int                    `json:"schemaVersion"`
}

// AlertRule 定义一条有界的指标阈值,含持续时长、冷却与启停控制。
type AlertRule struct {
	ID              ID                    `json:"id"`
	ServerID        ID                    `json:"serverID"`
	Name            string                `json:"name"`
	Metric          enums.MetricType      `json:"metric"`
	Comparison      enums.AlertComparison `json:"comparison"`
	Threshold       float64               `json:"threshold"`
	DurationSeconds int                   `json:"durationSeconds"`
	CooldownSeconds int                   `json:"cooldownSeconds"`
	Enabled         bool                  `json:"enabled"`
	CreatedAt       time.Time             `json:"createdAt"`
	UpdatedAt       time.Time             `json:"updatedAt"`
	SchemaVersion   int                   `json:"schemaVersion"`
}

// AlertEvent 是一次持久化的阈值告警,含活跃、恢复与确认时间。
type AlertEvent struct {
	ID             ID                    `json:"id"`
	RuleID         ID                    `json:"ruleID"`
	ServerID       ID                    `json:"serverID"`
	Metric         enums.MetricType      `json:"metric"`
	State          enums.AlertEventState `json:"state"`
	Threshold      float64               `json:"threshold"`
	TriggerValue   float64               `json:"triggerValue"`
	LatestValue    float64               `json:"latestValue"`
	FirstMatchedAt time.Time             `json:"firstMatchedAt"`
	TriggeredAt    time.Time             `json:"triggeredAt"`
	LastSeenAt     time.Time             `json:"lastSeenAt"`
	RecoveredAt    *time.Time            `json:"recoveredAt,omitempty"`
	AcknowledgedAt *time.Time            `json:"acknowledgedAt,omitempty"`
	CreatedAt      time.Time             `json:"createdAt"`
	UpdatedAt      time.Time             `json:"updatedAt"`
	SchemaVersion  int                   `json:"schemaVersion"`
}

// Validate 校验持久化的 Spark 能力身份与带版本的证据。
func (c SparkCapability) Validate() error {
	if !c.ServerID.Valid() || !c.Status.Valid() || !c.ServerType.Valid() || c.SchemaVersion != SparkSchemaVersion || c.DetectedAt.IsZero() {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Spark Capability 身份、状态或 Schema 无效")
	}
	if strings.TrimSpace(c.Platform) == "" || len(c.Platform) > 80 || len(c.Distribution) > 80 || len(c.PluginVersion) > 80 || len(c.ParserVersion) > 80 || len(c.CollectionMethod) > 80 || len(c.SourceSchemaHash) > 128 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Spark Capability 平台或版本字段无效")
	}
	if len(c.ArtifactPath) > 1024 || len(c.BackupPath) > 1024 || len(c.InstallManifest) > 64*1024 || strings.ContainsAny(c.ArtifactPath+c.BackupPath, "\x00\r\n") || len(c.LastError) > 2048 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Spark Capability 路径或错误摘要无效")
	}
	return nil
}

// Validate 校验带版本的 Spark Snapshot 身份、时间与有限的 TPS/MSPT 分布。
func (s SparkSnapshot) Validate() error {
	if !s.ID.Valid() || !s.ServerID.Valid() || !s.SourceID.Valid() || !s.ServerType.Valid() || s.CollectedAt.IsZero() || s.SchemaVersion != SparkSchemaVersion {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Spark Snapshot 身份、时间或 Schema 无效")
	}
	values := []float64{s.TPS5Seconds, s.TPS10Seconds, s.TPS1Minute, s.TPS5Minutes, s.TPS15Minutes}
	if s.MSPTAvailable {
		values = append(values, s.MSPTMinimum, s.MSPTMedian, s.MSPTP95, s.MSPTMaximum)
		if s.MSPTMinimum > s.MSPTMedian || s.MSPTMedian > s.MSPTP95 || s.MSPTP95 > s.MSPTMaximum {
			return apperror.New(apperror.CodeValidationInvalidArgument, "Spark Snapshot MSPT 分布顺序无效")
		}
	}
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return apperror.New(apperror.CodeValidationInvalidArgument, "Spark Snapshot 数值无效")
		}
	}
	return nil
}

// Validate 校验持久化报告的生命周期与已核准的查看器 URL 形态。
func (r SparkReport) Validate() error {
	if !r.ID.Valid() || !r.ServerID.Valid() || !r.Kind.Valid() || !r.State.Valid() || r.SchemaVersion != SparkSchemaVersion || r.CreatedAt.IsZero() || r.UpdatedAt.IsZero() {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Spark Report 身份、状态或 Schema 无效")
	}
	if r.OperationID != nil && !r.OperationID.Valid() || r.DurationSeconds < 0 || r.DurationSeconds > 3600 || len(r.ErrorMessage) > 2048 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Spark Report Operation、时长或错误摘要无效")
	}
	if r.ReportURL != "" {
		parsed, err := url.Parse(r.ReportURL)
		if err != nil || parsed.Scheme != "https" || parsed.Hostname() != "spark.lucko.me" {
			return apperror.New(apperror.CodeValidationInvalidArgument, "Spark Report URL 必须是官方 HTTPS Viewer")
		}
	}
	return nil
}

// Validate 校验一条告警规则的身份、指标定义、有限阈值、持续时长与冷却。
func (r AlertRule) Validate() error {
	if !r.ID.Valid() || !r.ServerID.Valid() || strings.TrimSpace(r.Name) == "" || len(r.Name) > 160 || !r.Metric.Valid() || !r.Comparison.Valid() || r.SchemaVersion != SparkSchemaVersion {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Alert Rule 身份、名称、Metric、比较符或 Schema 无效")
	}
	if math.IsNaN(r.Threshold) || math.IsInf(r.Threshold, 0) || r.DurationSeconds < 0 || r.DurationSeconds > 86400 || r.CooldownSeconds < 0 || r.CooldownSeconds > 604800 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Alert Rule 阈值、持续时间或冷却时间无效")
	}
	return nil
}

// Validate 校验一条告警事件的身份、状态、时间戳与有限数值。
func (e AlertEvent) Validate() error {
	if !e.ID.Valid() || !e.RuleID.Valid() || !e.ServerID.Valid() || !e.Metric.Valid() || !e.State.Valid() || e.SchemaVersion != SparkSchemaVersion || e.FirstMatchedAt.IsZero() || e.TriggeredAt.IsZero() || e.LastSeenAt.IsZero() {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Alert Event 身份、状态、时间或 Schema 无效")
	}
	for _, value := range []float64{e.Threshold, e.TriggerValue, e.LatestValue} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return apperror.New(apperror.CodeValidationInvalidArgument, "Alert Event 数值无效")
		}
	}
	return nil
}
