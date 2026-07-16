package repository

import (
	"context"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

// SparkSnapshotQuery contains bounded Server history pagination.
type SparkSnapshotQuery struct {
	ServerID model.ID
	Limit    int
	Offset   int
}

// SparkReportQuery contains bounded Server and report-kind history filters.
type SparkReportQuery struct {
	ServerID model.ID
	Kind     enums.SparkReportKind
	Limit    int
	Offset   int
}

// SparkRepository persists capability, health snapshots, and report metadata.
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

// AlertRuleQuery contains bounded Server, Metric, and enabled filters.
type AlertRuleQuery struct {
	ServerID model.ID
	Metric   enums.MetricType
	Enabled  *bool
	Limit    int
	Offset   int
}

// AlertEventQuery contains bounded Server, Rule, state, and pagination filters.
type AlertEventQuery struct {
	ServerID model.ID
	RuleID   model.ID
	State    enums.AlertEventState
	Limit    int
	Offset   int
}

// AlertRepository persists threshold rules and active/recovered incidents.
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
