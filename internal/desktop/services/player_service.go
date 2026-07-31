package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/service"
)

// PlayerListServiceResult 承载一页统一玩家数据或稳定错误。
type PlayerListServiceResult struct {
	Result *model.PlayerListResult `json:"result,omitempty"`
	Error  *apperror.DTO           `json:"error,omitempty"`
}

// PlayerDetailServiceResult 承载一份玩家总览或稳定错误。
type PlayerDetailServiceResult struct {
	Player *model.PlayerOverview `json:"player,omitempty"`
	Error  *apperror.DTO         `json:"error,omitempty"`
}

// PlayerSessionsServiceResult 承载有界的近期玩家会话。
type PlayerSessionsServiceResult struct {
	Sessions []model.PlayerSession `json:"sessions"`
	Error    *apperror.DTO         `json:"error,omitempty"`
}

// PlayerActionServiceResult 承载命令执行后刷新的玩家数据。
type PlayerActionServiceResult struct {
	Player *model.PlayerOverview `json:"player,omitempty"`
	Error  *apperror.DTO         `json:"error,omitempty"`
}

// PlayerService 对外暴露统一的玩家查询、同步与管理操作。
type PlayerService struct {
	manager *service.PlayerActivityManager
	logger  *applog.Logger
}

// NewPlayerService 创建桌面侧的玩家门面。
func NewPlayerService(manager *service.PlayerActivityManager, logger *applog.Logger) *PlayerService {
	return &PlayerService{manager: manager, logger: logger}
}

// List 返回一页有界的统一玩家数据。
func (s *PlayerService) List(ctx context.Context, query model.PlayerQuery) PlayerListServiceResult {
	result, err := s.manager.ListPlayers(ctx, query)
	if err != nil {
		return PlayerListServiceResult{Error: playerServiceError(err)}
	}
	return PlayerListServiceResult{Result: &result}
}

// Detail 返回一份统一的玩家总览。
func (s *PlayerService) Detail(ctx context.Context, serverID, playerID string) PlayerDetailServiceResult {
	player, err := s.manager.PlayerDetail(ctx, model.ID(serverID), model.ID(playerID))
	if err != nil {
		return PlayerDetailServiceResult{Error: playerServiceError(err)}
	}
	return PlayerDetailServiceResult{Player: &player}
}

// RecentSessions 按时间倒序返回有界的近期会话。
func (s *PlayerService) RecentSessions(ctx context.Context, query model.PlayerSessionQuery) PlayerSessionsServiceResult {
	sessions, err := s.manager.RecentPlayerSessions(ctx, query)
	if err != nil {
		return PlayerSessionsServiceResult{Error: playerServiceError(err)}
	}
	return PlayerSessionsServiceResult{Sessions: sessions}
}

// RefreshDirectory 同步 Minecraft 权威玩家名录文件。
func (s *PlayerService) RefreshDirectory(ctx context.Context, serverID string) PlayerListServiceResult {
	if err := s.manager.SynchronizeDirectory(ctx, model.ID(serverID)); err != nil {
		return PlayerListServiceResult{Error: playerServiceError(err)}
	}
	return s.List(ctx, model.PlayerQuery{ServerID: model.ID(serverID), Limit: 50})
}

// SynchronizeActivity 认领并应用远端待处理的玩家活动证据。
func (s *PlayerService) SynchronizeActivity(ctx context.Context, serverID string) PlayerListServiceResult {
	if err := s.manager.SynchronizeServer(ctx, model.ID(serverID)); err != nil {
		return PlayerListServiceResult{Error: playerServiceError(err)}
	}
	return s.List(ctx, model.PlayerQuery{ServerID: model.ID(serverID), Limit: 50})
}

// AddWhitelist 把玩家加入 Minecraft 白名单。
func (s *PlayerService) AddWhitelist(ctx context.Context, input model.PlayerActionInput) PlayerActionServiceResult {
	return s.action(ctx, "add_whitelist", input)
}

// RemoveWhitelist 把玩家移出 Minecraft 白名单。
func (s *PlayerService) RemoveWhitelist(ctx context.Context, input model.PlayerActionInput) PlayerActionServiceResult {
	return s.action(ctx, "remove_whitelist", input)
}

// GrantOperator 授予 Minecraft 管理员权限。
func (s *PlayerService) GrantOperator(ctx context.Context, input model.PlayerActionInput) PlayerActionServiceResult {
	return s.action(ctx, "grant_operator", input)
}

// RevokeOperator 撤销 Minecraft 管理员权限。
func (s *PlayerService) RevokeOperator(ctx context.Context, input model.PlayerActionInput) PlayerActionServiceResult {
	return s.action(ctx, "revoke_operator", input)
}

// Ban 封禁一名玩家,原因可选。
func (s *PlayerService) Ban(ctx context.Context, input model.PlayerActionInput) PlayerActionServiceResult {
	return s.action(ctx, "ban", input)
}

// Pardon 解除一名玩家的封禁。
func (s *PlayerService) Pardon(ctx context.Context, input model.PlayerActionInput) PlayerActionServiceResult {
	return s.action(ctx, "pardon", input)
}

// Kick 断开一名当前在线玩家的连接。
func (s *PlayerService) Kick(ctx context.Context, input model.PlayerActionInput) PlayerActionServiceResult {
	return s.action(ctx, "kick", input)
}

func (s *PlayerService) action(ctx context.Context, action string, input model.PlayerActionInput) PlayerActionServiceResult {
	player, err := s.manager.ManagePlayer(ctx, action, input)
	if err != nil {
		return PlayerActionServiceResult{Error: playerServiceError(err)}
	}
	return PlayerActionServiceResult{Player: &player}
}

func playerServiceError(err error) *apperror.DTO {
	dto := apperror.ToDTO(err)
	return &dto
}
