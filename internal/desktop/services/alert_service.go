package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
	"github.com/Cail-Gainey/MineOps/internal/service"
)

// AlertRuleResult contains one threshold rule or a stable error.
type AlertRuleResult struct {
	Rule  *model.AlertRule `json:"rule,omitempty"`
	Error *apperror.DTO    `json:"error,omitempty"`
}

// AlertRuleListResult contains bounded threshold rules.
type AlertRuleListResult struct {
	Rules []model.AlertRule `json:"rules"`
	Error *apperror.DTO     `json:"error,omitempty"`
}

// AlertEventResult contains one active, recovered, or acknowledged incident.
type AlertEventResult struct {
	Event *model.AlertEvent `json:"event,omitempty"`
	Error *apperror.DTO     `json:"error,omitempty"`
}

// AlertEventListResult contains bounded incident history.
type AlertEventListResult struct {
	Events []model.AlertEvent `json:"events"`
	Error  *apperror.DTO      `json:"error,omitempty"`
}

// AlertService exposes threshold rule CRUD, active/history queries, and acknowledgement.
type AlertService struct {
	manager *service.AlertManager
	logger  *applog.Logger
}

// NewAlertService creates the Wails Alert facade.
func NewAlertService(manager *service.AlertManager, logger *applog.Logger) *AlertService {
	return &AlertService{manager: manager, logger: logger}
}

// CreateRule creates one bounded Server Metric threshold rule.
func (s *AlertService) CreateRule(ctx context.Context, rule model.AlertRule) (result AlertRuleResult) {
	defer s.recoverRule(ctx, "AlertService.CreateRule", &result)
	created, err := s.manager.CreateRule(ctx, rule)
	if err != nil {
		dto := apperror.ToDTO(err)
		return AlertRuleResult{Error: &dto}
	}
	return AlertRuleResult{Rule: &created}
}

// UpdateRule updates one threshold rule.
func (s *AlertService) UpdateRule(ctx context.Context, rule model.AlertRule) (result AlertRuleResult) {
	defer s.recoverRule(ctx, "AlertService.UpdateRule", &result)
	updated, err := s.manager.UpdateRule(ctx, rule)
	if err != nil {
		dto := apperror.ToDTO(err)
		return AlertRuleResult{Error: &dto}
	}
	return AlertRuleResult{Rule: &updated}
}

// DeleteRule removes one rule and recovers any active event.
func (s *AlertService) DeleteRule(ctx context.Context, ruleID string) (result ActionResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "AlertService.DeleteRule", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	if err := s.manager.DeleteRule(ctx, model.ID(ruleID)); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

// ListRules returns bounded rules for one Server and optional Metric.
func (s *AlertService) ListRules(ctx context.Context, serverID, metric string, enabledOnly bool, limit, offset int) (result AlertRuleListResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "AlertService.ListRules", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	var enabled *bool
	if enabledOnly {
		value := true
		enabled = &value
	}
	rules, err := s.manager.ListRules(ctx, repository.AlertRuleQuery{ServerID: model.ID(serverID), Metric: enums.MetricType(metric), Enabled: enabled, Limit: limit, Offset: offset})
	if err != nil {
		dto := apperror.ToDTO(err)
		return AlertRuleListResult{Error: &dto}
	}
	return AlertRuleListResult{Rules: rules}
}

// ListEvents returns active or historical incidents for one Server.
func (s *AlertService) ListEvents(ctx context.Context, serverID, state string, limit, offset int) (result AlertEventListResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "AlertService.ListEvents", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	events, err := s.manager.ListEvents(ctx, repository.AlertEventQuery{ServerID: model.ID(serverID), State: enums.AlertEventState(state), Limit: limit, Offset: offset})
	if err != nil {
		dto := apperror.ToDTO(err)
		return AlertEventListResult{Error: &dto}
	}
	return AlertEventListResult{Events: events}
}

// DeleteEvent permanently removes one durable alert incident.
func (s *AlertService) DeleteEvent(ctx context.Context, eventID string) (result ActionResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "AlertService.DeleteEvent", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	if err := s.manager.DeleteEvent(ctx, model.ID(eventID)); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

// Acknowledge marks one incident reviewed without changing recovery state.
func (s *AlertService) Acknowledge(ctx context.Context, eventID string) (result AlertEventResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "AlertService.Acknowledge", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	event, err := s.manager.Acknowledge(ctx, model.ID(eventID))
	if err != nil {
		dto := apperror.ToDTO(err)
		return AlertEventResult{Error: &dto}
	}
	return AlertEventResult{Event: &event}
}

func (s *AlertService) recoverRule(ctx context.Context, boundary string, result *AlertRuleResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}
