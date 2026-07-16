package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
)

const (
	alertMaximumRulesPerSample = 256
	alertEvaluationQueueSize   = 64
)

// AlertEventPublisher emits durable active, updated, recovered, and acknowledged incidents.
type AlertEventPublisher interface {
	PublishAlert(context.Context, model.AlertEvent) error
}

type alertPendingMatch struct {
	firstMatchedAt time.Time
	latestValue    float64
}

// AlertManager owns bounded threshold evaluation, duration debounce, cooldown, recovery, and acknowledgement.
type AlertManager struct {
	clock     model.Clock
	store     repository.Store
	publisher AlertEventPublisher
	logger    *applog.Logger
	queue     chan []model.MetricSample

	mu      sync.Mutex
	pending map[model.ID]alertPendingMatch
}

// NewAlertManager creates the threshold rule and incident application service.
func NewAlertManager(clock model.Clock, store repository.Store, publisher AlertEventPublisher, logger *applog.Logger) (*AlertManager, error) {
	if clock == nil || store == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Alert Service 依赖不能为空")
	}
	if logger == nil {
		logger = applog.Default()
	}
	return &AlertManager{clock: clock, store: store, publisher: publisher, logger: logger, queue: make(chan []model.MetricSample, alertEvaluationQueueSize), pending: make(map[model.ID]alertPendingMatch)}, nil
}

// ObserveMetrics enqueues a stable bounded copy and never blocks Metric ingest.
func (m *AlertManager) ObserveMetrics(samples []model.MetricSample) {
	if m == nil || len(samples) == 0 {
		return
	}
	copyOfSamples := append([]model.MetricSample(nil), samples...)
	select {
	case m.queue <- copyOfSamples:
	default:
		m.logger.Warn(context.Background(), "Alert 评估队列已满，丢弃一个 Metric 批次", applog.Fields{"samples": len(samples)})
	}
}

// Run evaluates queued samples until the application context is cancelled.
func (m *AlertManager) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case samples := <-m.queue:
			for _, sample := range latestSamplesFromBatch(samples) {
				if err := m.evaluate(ctx, sample); err != nil {
					m.logger.Error(ctx, "Alert 阈值评估失败", err, applog.Fields{"server_id": sample.ServerID.String(), "metric": sample.Metric.String()})
				}
			}
		}
	}
}

// CreateRule validates one bounded threshold rule and persists it in encrypted SQLite.
func (m *AlertManager) CreateRule(ctx context.Context, rule model.AlertRule) (model.AlertRule, error) {
	if _, err := m.store.MinecraftServers().Get(ctx, rule.ServerID, false); err != nil {
		return model.AlertRule{}, err
	}
	now := m.clock.Now().UTC()
	id, err := model.NewID(now)
	if err != nil {
		return model.AlertRule{}, err
	}
	rule.ID, rule.CreatedAt, rule.UpdatedAt, rule.SchemaVersion = id, now, now, model.SparkSchemaVersion
	if err := rule.Validate(); err != nil {
		return model.AlertRule{}, err
	}
	if err := m.store.Alerts().CreateRule(ctx, &rule); err != nil {
		return model.AlertRule{}, err
	}
	return rule, nil
}

// UpdateRule updates threshold controls and clears any pending duration debounce when disabled.
func (m *AlertManager) UpdateRule(ctx context.Context, rule model.AlertRule) (model.AlertRule, error) {
	existing, err := m.store.Alerts().GetRule(ctx, rule.ID)
	if err != nil {
		return model.AlertRule{}, err
	}
	rule.CreatedAt = existing.CreatedAt
	rule.UpdatedAt = m.clock.Now().UTC()
	rule.SchemaVersion = model.SparkSchemaVersion
	if err := rule.Validate(); err != nil {
		return model.AlertRule{}, err
	}
	if err := m.store.Alerts().UpdateRule(ctx, &rule); err != nil {
		return model.AlertRule{}, err
	}
	if !rule.Enabled {
		m.mu.Lock()
		delete(m.pending, rule.ID)
		m.mu.Unlock()
	}
	return rule, nil
}

// DeleteRule removes one rule after recovering any active incident.
func (m *AlertManager) DeleteRule(ctx context.Context, id model.ID) error {
	if active, err := m.store.Alerts().FindActiveEvent(ctx, id); err == nil {
		now := m.clock.Now().UTC()
		active.State, active.RecoveredAt, active.LastSeenAt, active.UpdatedAt = enums.AlertEventRecovered, &now, now, now
		if err := m.store.Alerts().UpdateEvent(ctx, active); err != nil {
			return err
		}
		m.publish(ctx, *active)
	}
	m.mu.Lock()
	delete(m.pending, id)
	m.mu.Unlock()
	return m.store.Alerts().DeleteRule(ctx, id)
}

// ListRules returns bounded rules for one Server and optional Metric.
func (m *AlertManager) ListRules(ctx context.Context, query repository.AlertRuleQuery) ([]model.AlertRule, error) {
	return m.store.Alerts().ListRules(ctx, query)
}

// ListEvents returns bounded active or historical incidents.
func (m *AlertManager) ListEvents(ctx context.Context, query repository.AlertEventQuery) ([]model.AlertEvent, error) {
	return m.store.Alerts().ListEvents(ctx, query)
}

// DeleteEvent permanently removes one incident and publishes recovery for an active event.
func (m *AlertManager) DeleteEvent(ctx context.Context, eventID model.ID) error {
	event, err := m.store.Alerts().GetEvent(ctx, eventID)
	if err != nil {
		return err
	}
	if err := m.store.Alerts().DeleteEvent(ctx, eventID); err != nil {
		return err
	}
	if event.State == enums.AlertEventActive {
		now := m.clock.Now().UTC()
		event.State, event.RecoveredAt, event.LastSeenAt, event.UpdatedAt = enums.AlertEventRecovered, &now, now, now
		m.publish(ctx, *event)
	}
	return nil
}

// Acknowledge marks one incident as reviewed without changing active/recovered state.
func (m *AlertManager) Acknowledge(ctx context.Context, eventID model.ID) (model.AlertEvent, error) {
	event, err := m.store.Alerts().GetEvent(ctx, eventID)
	if err != nil {
		return model.AlertEvent{}, err
	}
	if event.AcknowledgedAt == nil {
		now := m.clock.Now().UTC()
		event.AcknowledgedAt, event.UpdatedAt = &now, now
		if err := m.store.Alerts().UpdateEvent(ctx, event); err != nil {
			return model.AlertEvent{}, err
		}
		m.publish(ctx, *event)
	}
	return *event, nil
}

func (m *AlertManager) evaluate(ctx context.Context, sample model.MetricSample) error {
	enabled := true
	rules, err := m.store.Alerts().ListRules(ctx, repository.AlertRuleQuery{ServerID: sample.ServerID, Metric: sample.Metric, Enabled: &enabled, Limit: alertMaximumRulesPerSample})
	if err != nil {
		return err
	}
	if len(rules) == 0 {
		return nil
	}
	for _, rule := range rules {
		active, activeErr := m.store.Alerts().FindActiveEvent(ctx, rule.ID)
		if activeErr != nil && apperror.ToDTO(activeErr).Code != apperror.CodeIONotFound.String() {
			return activeErr
		}
		matches := rule.Comparison.Match(sample.Value, rule.Threshold)
		if !matches {
			m.mu.Lock()
			delete(m.pending, rule.ID)
			m.mu.Unlock()
			if active != nil {
				now := sample.Timestamp.UTC()
				active.State, active.LatestValue, active.LastSeenAt, active.RecoveredAt, active.UpdatedAt = enums.AlertEventRecovered, sample.Value, now, &now, m.clock.Now().UTC()
				if err := m.store.Alerts().UpdateEvent(ctx, active); err != nil {
					return err
				}
				m.publish(ctx, *active)
			}
			continue
		}
		if active != nil {
			active.LatestValue, active.LastSeenAt, active.UpdatedAt = sample.Value, sample.Timestamp.UTC(), m.clock.Now().UTC()
			if err := m.store.Alerts().UpdateEvent(ctx, active); err != nil {
				return err
			}
			continue
		}
		m.mu.Lock()
		pending, found := m.pending[rule.ID]
		if !found || sample.Timestamp.Before(pending.firstMatchedAt) {
			pending = alertPendingMatch{firstMatchedAt: sample.Timestamp.UTC(), latestValue: sample.Value}
		} else {
			pending.latestValue = sample.Value
		}
		m.pending[rule.ID] = pending
		m.mu.Unlock()
		if sample.Timestamp.Sub(pending.firstMatchedAt) < time.Duration(rule.DurationSeconds)*time.Second {
			continue
		}
		history, historyErr := m.store.Alerts().ListEvents(ctx, repository.AlertEventQuery{RuleID: rule.ID, Limit: 1})
		if historyErr != nil {
			return historyErr
		}
		if len(history) > 0 && sample.Timestamp.Sub(history[0].TriggeredAt) < time.Duration(rule.CooldownSeconds)*time.Second {
			continue
		}
		now := m.clock.Now().UTC()
		id, idErr := model.NewID(now)
		if idErr != nil {
			return idErr
		}
		event := model.AlertEvent{
			ID: id, RuleID: rule.ID, ServerID: rule.ServerID, Metric: rule.Metric, State: enums.AlertEventActive,
			Threshold: rule.Threshold, TriggerValue: sample.Value, LatestValue: sample.Value,
			FirstMatchedAt: pending.firstMatchedAt, TriggeredAt: sample.Timestamp.UTC(), LastSeenAt: sample.Timestamp.UTC(),
			CreatedAt: now, UpdatedAt: now, SchemaVersion: model.SparkSchemaVersion,
		}
		if err := m.store.Alerts().CreateEvent(ctx, &event); err != nil {
			return err
		}
		m.mu.Lock()
		delete(m.pending, rule.ID)
		m.mu.Unlock()
		m.publish(ctx, event)
	}
	return nil
}

func (m *AlertManager) publish(ctx context.Context, event model.AlertEvent) {
	if m.publisher == nil {
		return
	}
	if err := m.publisher.PublishAlert(ctx, event); err != nil && !errors.Is(err, context.Canceled) {
		m.logger.Warn(ctx, "发布 Alert Event 失败", applog.Fields{"event_id": event.ID.String(), "error": err.Error()})
	}
}

var _ MetricObserver = (*AlertManager)(nil)
