package gormrepo

import (
	"context"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
	"gorm.io/gorm"
)

// AlertRuleRecord is the indexed durable metric threshold representation.
type AlertRuleRecord struct {
	ID              string `gorm:"primaryKey;size:36"`
	ServerID        string `gorm:"index:idx_alert_rule_server_metric_enabled,priority:1;size:36"`
	Name            string `gorm:"size:160"`
	Metric          string `gorm:"index:idx_alert_rule_server_metric_enabled,priority:2;size:96"`
	Comparison      string `gorm:"size:32"`
	Threshold       float64
	DurationSeconds int
	CooldownSeconds int
	Enabled         bool `gorm:"index:idx_alert_rule_server_metric_enabled,priority:3"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
	SchemaVersion   int
}

// AlertEventRecord is the indexed durable active/recovered incident representation.
type AlertEventRecord struct {
	ID             string `gorm:"primaryKey;size:36"`
	RuleID         string `gorm:"index:idx_alert_event_rule_state,priority:1;size:36"`
	ServerID       string `gorm:"index:idx_alert_event_server_created,priority:1;size:36"`
	Metric         string `gorm:"index;size:96"`
	State          string `gorm:"index:idx_alert_event_rule_state,priority:2;size:32"`
	Threshold      float64
	TriggerValue   float64
	LatestValue    float64
	FirstMatchedAt time.Time
	TriggeredAt    time.Time
	LastSeenAt     time.Time
	RecoveredAt    *time.Time
	AcknowledgedAt *time.Time
	CreatedAt      time.Time `gorm:"index:idx_alert_event_server_created,priority:2"`
	UpdatedAt      time.Time
	SchemaVersion  int
}

type alertRepository struct{ database *gorm.DB }

func (r *alertRepository) CreateRule(ctx context.Context, rule *model.AlertRule) error {
	if rule == nil {
		return apperror.New(apperror.CodeValidationRequired, "Alert Rule 不能为空")
	}
	if err := rule.Validate(); err != nil {
		return err
	}
	record := alertRuleToRecord(*rule)
	if err := r.database.WithContext(ctx).Create(&record).Error; err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建 Alert Rule 失败", err)
	}
	return nil
}

func (r *alertRepository) UpdateRule(ctx context.Context, rule *model.AlertRule) error {
	if rule == nil {
		return apperror.New(apperror.CodeValidationRequired, "Alert Rule 不能为空")
	}
	if err := rule.Validate(); err != nil {
		return err
	}
	record := alertRuleToRecord(*rule)
	result := r.database.WithContext(ctx).Model(&AlertRuleRecord{}).Where("id = ?", rule.ID.String()).Select("*").Updates(&record)
	if result.Error != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "更新 Alert Rule 失败", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperror.New(apperror.CodeIONotFound, "Alert Rule 不存在")
	}
	return nil
}

func (r *alertRepository) DeleteRule(ctx context.Context, id model.ID) error {
	result := r.database.WithContext(ctx).Delete(&AlertRuleRecord{}, "id = ?", id.String())
	if result.Error != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "删除 Alert Rule 失败", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperror.New(apperror.CodeIONotFound, "Alert Rule 不存在")
	}
	return nil
}

func (r *alertRepository) GetRule(ctx context.Context, id model.ID) (*model.AlertRule, error) {
	var record AlertRuleRecord
	if err := r.database.WithContext(ctx).First(&record, "id = ?", id.String()).Error; err != nil {
		return nil, mapSparkNotFound("Alert Rule 不存在", err)
	}
	rule := recordToAlertRule(record)
	return &rule, nil
}

func (r *alertRepository) ListRules(ctx context.Context, query repository.AlertRuleQuery) ([]model.AlertRule, error) {
	database := r.database.WithContext(ctx).Order("created_at desc")
	if query.ServerID.Valid() {
		database = database.Where("server_id = ?", query.ServerID.String())
	}
	if query.Metric.Valid() {
		database = database.Where("metric = ?", query.Metric.String())
	}
	if query.Enabled != nil {
		database = database.Where("enabled = ?", *query.Enabled)
	}
	var records []AlertRuleRecord
	if err := applySparkPagination(database, query.Limit, query.Offset).Find(&records).Error; err != nil {
		return nil, apperror.Wrap(apperror.CodeIOReadFailed, "查询 Alert Rules 失败", err)
	}
	result := make([]model.AlertRule, len(records))
	for index, record := range records {
		result[index] = recordToAlertRule(record)
	}
	return result, nil
}

func (r *alertRepository) CreateEvent(ctx context.Context, event *model.AlertEvent) error {
	if event == nil {
		return apperror.New(apperror.CodeValidationRequired, "Alert Event 不能为空")
	}
	if err := event.Validate(); err != nil {
		return err
	}
	record := alertEventToRecord(*event)
	if err := r.database.WithContext(ctx).Create(&record).Error; err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建 Alert Event 失败", err)
	}
	return nil
}

func (r *alertRepository) UpdateEvent(ctx context.Context, event *model.AlertEvent) error {
	if event == nil {
		return apperror.New(apperror.CodeValidationRequired, "Alert Event 不能为空")
	}
	if err := event.Validate(); err != nil {
		return err
	}
	record := alertEventToRecord(*event)
	result := r.database.WithContext(ctx).Model(&AlertEventRecord{}).Where("id = ?", event.ID.String()).Select("*").Updates(&record)
	if result.Error != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "更新 Alert Event 失败", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperror.New(apperror.CodeIONotFound, "Alert Event 不存在")
	}
	return nil
}

func (r *alertRepository) DeleteEvent(ctx context.Context, id model.ID) error {
	result := r.database.WithContext(ctx).Delete(&AlertEventRecord{}, "id = ?", id.String())
	if result.Error != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "删除 Alert Event 失败", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperror.New(apperror.CodeIONotFound, "Alert Event 不存在")
	}
	return nil
}

func (r *alertRepository) GetEvent(ctx context.Context, id model.ID) (*model.AlertEvent, error) {
	var record AlertEventRecord
	if err := r.database.WithContext(ctx).First(&record, "id = ?", id.String()).Error; err != nil {
		return nil, mapSparkNotFound("Alert Event 不存在", err)
	}
	event := recordToAlertEvent(record)
	return &event, nil
}

func (r *alertRepository) FindActiveEvent(ctx context.Context, ruleID model.ID) (*model.AlertEvent, error) {
	var record AlertEventRecord
	if err := r.database.WithContext(ctx).Where("rule_id = ? AND state = ?", ruleID.String(), enums.AlertEventActive.String()).Order("triggered_at desc").First(&record).Error; err != nil {
		return nil, mapSparkNotFound("活动 Alert Event 不存在", err)
	}
	event := recordToAlertEvent(record)
	return &event, nil
}

func (r *alertRepository) ListEvents(ctx context.Context, query repository.AlertEventQuery) ([]model.AlertEvent, error) {
	database := r.database.WithContext(ctx).Order("created_at desc")
	if query.ServerID.Valid() {
		database = database.Where("server_id = ?", query.ServerID.String())
	}
	if query.RuleID.Valid() {
		database = database.Where("rule_id = ?", query.RuleID.String())
	}
	if query.State.Valid() {
		database = database.Where("state = ?", query.State.String())
	}
	var records []AlertEventRecord
	if err := applySparkPagination(database, query.Limit, query.Offset).Find(&records).Error; err != nil {
		return nil, apperror.Wrap(apperror.CodeIOReadFailed, "查询 Alert Events 失败", err)
	}
	result := make([]model.AlertEvent, len(records))
	for index, record := range records {
		result[index] = recordToAlertEvent(record)
	}
	return result, nil
}

func alertRuleToRecord(value model.AlertRule) AlertRuleRecord {
	return AlertRuleRecord{
		ID: value.ID.String(), ServerID: value.ServerID.String(), Name: value.Name, Metric: value.Metric.String(), Comparison: value.Comparison.String(),
		Threshold: value.Threshold, DurationSeconds: value.DurationSeconds, CooldownSeconds: value.CooldownSeconds, Enabled: value.Enabled,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, SchemaVersion: value.SchemaVersion,
	}
}

func recordToAlertRule(value AlertRuleRecord) model.AlertRule {
	return model.AlertRule{
		ID: model.ID(value.ID), ServerID: model.ID(value.ServerID), Name: value.Name, Metric: enums.MetricType(value.Metric), Comparison: enums.AlertComparison(value.Comparison),
		Threshold: value.Threshold, DurationSeconds: value.DurationSeconds, CooldownSeconds: value.CooldownSeconds, Enabled: value.Enabled,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, SchemaVersion: value.SchemaVersion,
	}
}

func alertEventToRecord(value model.AlertEvent) AlertEventRecord {
	return AlertEventRecord{
		ID: value.ID.String(), RuleID: value.RuleID.String(), ServerID: value.ServerID.String(), Metric: value.Metric.String(), State: value.State.String(),
		Threshold: value.Threshold, TriggerValue: value.TriggerValue, LatestValue: value.LatestValue, FirstMatchedAt: value.FirstMatchedAt,
		TriggeredAt: value.TriggeredAt, LastSeenAt: value.LastSeenAt, RecoveredAt: value.RecoveredAt, AcknowledgedAt: value.AcknowledgedAt,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, SchemaVersion: value.SchemaVersion,
	}
}

func recordToAlertEvent(value AlertEventRecord) model.AlertEvent {
	return model.AlertEvent{
		ID: model.ID(value.ID), RuleID: model.ID(value.RuleID), ServerID: model.ID(value.ServerID), Metric: enums.MetricType(value.Metric), State: enums.AlertEventState(value.State),
		Threshold: value.Threshold, TriggerValue: value.TriggerValue, LatestValue: value.LatestValue, FirstMatchedAt: value.FirstMatchedAt,
		TriggeredAt: value.TriggeredAt, LastSeenAt: value.LastSeenAt, RecoveredAt: value.RecoveredAt, AcknowledgedAt: value.AcknowledgedAt,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, SchemaVersion: value.SchemaVersion,
	}
}

var _ repository.AlertRepository = (*alertRepository)(nil)
