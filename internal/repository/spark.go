package repository

import (
	"context"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

// SparkSnapshotQuery 承载有界的 Server 历史分页条件。
type SparkSnapshotQuery struct {
	ServerID model.ID
	Limit    int
	Offset   int
}

// SparkReportQuery 承载有界的 Server 与报告类型历史过滤条件。
type SparkReportQuery struct {
	ServerID model.ID
	Kind     enums.SparkReportKind
	Limit    int
	Offset   int
}

// SparkRepository 持久化能力、健康 Snapshot 与报告元数据。
type SparkRepository interface {
	SaveCapability(context.Context, *model.SparkCapability) error
	GetCapability(context.Context, model.ID) (*model.SparkCapability, error)
	CreateSnapshot(context.Context, *model.SparkSnapshot) error
	LatestSnapshot(context.Context, model.ID) (*model.SparkSnapshot, error)
	ListSnapshots(context.Context, SparkSnapshotQuery) ([]model.SparkSnapshot, error)
	// DeleteSnapshotsBefore 删除指定时间之前的一批 Spark Snapshot。
	DeleteSnapshotsBefore(context.Context, time.Time, int) (int64, error)
	// DeleteServerSnapshots 删除指定 Server 的全部 Spark Snapshot 历史。
	DeleteServerSnapshots(context.Context, model.ID, int) (int64, error)
	CreateReport(context.Context, *model.SparkReport) error
	UpdateReport(context.Context, *model.SparkReport) error
	GetReport(context.Context, model.ID) (*model.SparkReport, error)
	ListReports(context.Context, SparkReportQuery) ([]model.SparkReport, error)
	DeleteReport(context.Context, model.ID) error
}

// AlertRuleQuery 承载有界的 Server、指标与启用状态过滤条件。
type AlertRuleQuery struct {
	ServerID model.ID
	Metric   enums.MetricType
	Enabled  *bool
	Limit    int
	Offset   int
}

// AlertEventQuery 承载有界的 Server、规则、状态与分页过滤条件。
type AlertEventQuery struct {
	ServerID model.ID
	RuleID   model.ID
	State    enums.AlertEventState
	Limit    int
	Offset   int
}

// AlertRepository 持久化阈值规则与活跃、已恢复的告警事件。
type AlertRepository interface {
	CreateRule(context.Context, *model.AlertRule) error
	UpdateRule(context.Context, *model.AlertRule) error
	DeleteRule(context.Context, model.ID) error
	GetRule(context.Context, model.ID) (*model.AlertRule, error)
	ListRules(context.Context, AlertRuleQuery) ([]model.AlertRule, error)
	CreateEvent(context.Context, *model.AlertEvent) error
	UpdateEvent(context.Context, *model.AlertEvent) error
	DeleteEvent(context.Context, model.ID) error
	GetEvent(context.Context, model.ID) (*model.AlertEvent, error)
	FindActiveEvent(context.Context, model.ID) (*model.AlertEvent, error)
	ListEvents(context.Context, AlertEventQuery) ([]model.AlertEvent, error)
}
