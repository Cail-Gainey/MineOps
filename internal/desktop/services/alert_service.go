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

// AlertRuleResult 承载一条阈值规则或稳定错误。
type AlertRuleResult struct {
	Rule  *model.AlertRule `json:"rule,omitempty"`
	Error *apperror.DTO    `json:"error,omitempty"`
}

// AlertRuleListResult 承载有界的阈值规则列表。
type AlertRuleListResult struct {
	Rules []model.AlertRule `json:"rules"`
	Error *apperror.DTO     `json:"error,omitempty"`
}

// AlertEventResult 承载一条活跃、已恢复或已确认的告警事件。
type AlertEventResult struct {
	Event *model.AlertEvent `json:"event,omitempty"`
	Error *apperror.DTO     `json:"error,omitempty"`
}

// AlertEventListResult 承载有界的告警事件历史。
type AlertEventListResult struct {
	Events []model.AlertEvent `json:"events"`
	Error  *apperror.DTO      `json:"error,omitempty"`
}

// AlertService 对外暴露阈值规则增删改查、活跃与历史查询以及确认操作。
type AlertService struct {
	manager *service.AlertManager
	logger  *applog.Logger
}

// NewAlertService 创建 Wails 侧的告警门面。
func NewAlertService(manager *service.AlertManager, logger *applog.Logger) *AlertService {
	return &AlertService{manager: manager, logger: logger}
}

// CreateRule 创建一条有界的 Server 指标阈值规则。
func (s *AlertService) CreateRule(ctx context.Context, rule model.AlertRule) (result AlertRuleResult) {
	defer s.recoverRule(ctx, "AlertService.CreateRule", &result)
	created, err := s.manager.CreateRule(ctx, rule)
	if err != nil {
		dto := apperror.ToDTO(err)
		return AlertRuleResult{Error: &dto}
	}
	return AlertRuleResult{Rule: &created}
}

// UpdateRule 更新一条阈值规则。
func (s *AlertService) UpdateRule(ctx context.Context, rule model.AlertRule) (result AlertRuleResult) {
	defer s.recoverRule(ctx, "AlertService.UpdateRule", &result)
	updated, err := s.manager.UpdateRule(ctx, rule)
	if err != nil {
		dto := apperror.ToDTO(err)
		return AlertRuleResult{Error: &dto}
	}
	return AlertRuleResult{Rule: &updated}
}

// DeleteRule 删除一条规则,并把其活跃告警置为已恢复。
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

// ListRules 返回某台 Server 及可选指标下的有界规则列表。
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

// ListEvents 返回某台 Server 的活跃或历史告警事件。
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

// DeleteEvent 永久删除一条持久化告警事件。
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

// Acknowledge 把一条告警标记为已查看,不改变其恢复状态。
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
