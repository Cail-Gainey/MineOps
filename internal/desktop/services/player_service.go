package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/service"
)

// PlayerListServiceResult contains one unified player page or stable error.
type PlayerListServiceResult struct {
	Result *model.PlayerListResult `json:"result,omitempty"`
	Error  *apperror.DTO           `json:"error,omitempty"`
}

// PlayerDetailServiceResult contains one player overview or stable error.
type PlayerDetailServiceResult struct {
	Player *model.PlayerOverview `json:"player,omitempty"`
	Error  *apperror.DTO         `json:"error,omitempty"`
}

// PlayerSessionsServiceResult contains bounded recent player sessions.
type PlayerSessionsServiceResult struct {
	Sessions []model.PlayerSession `json:"sessions"`
	Error    *apperror.DTO         `json:"error,omitempty"`
}

// PlayerActionServiceResult contains the refreshed player after a command.
type PlayerActionServiceResult struct {
	Player *model.PlayerOverview `json:"player,omitempty"`
	Error  *apperror.DTO         `json:"error,omitempty"`
}

// PlayerService exposes unified player queries, synchronization, and management actions.
type PlayerService struct {
	manager *service.PlayerActivityManager
	logger  *applog.Logger
}

// NewPlayerService creates the desktop player facade.
func NewPlayerService(manager *service.PlayerActivityManager, logger *applog.Logger) *PlayerService {
	return &PlayerService{manager: manager, logger: logger}
}

// List returns one bounded unified player page.
func (s *PlayerService) List(ctx context.Context, query model.PlayerQuery) PlayerListServiceResult {
	result, err := s.manager.ListPlayers(ctx, query)
	if err != nil {
		return PlayerListServiceResult{Error: playerServiceError(err)}
	}
	return PlayerListServiceResult{Result: &result}
}

// Detail returns one unified player overview.
func (s *PlayerService) Detail(ctx context.Context, serverID, playerID string) PlayerDetailServiceResult {
	player, err := s.manager.PlayerDetail(ctx, model.ID(serverID), model.ID(playerID))
	if err != nil {
		return PlayerDetailServiceResult{Error: playerServiceError(err)}
	}
	return PlayerDetailServiceResult{Player: &player}
}

// RecentSessions returns bounded recent sessions in newest-first order.
func (s *PlayerService) RecentSessions(ctx context.Context, query model.PlayerSessionQuery) PlayerSessionsServiceResult {
	sessions, err := s.manager.RecentPlayerSessions(ctx, query)
	if err != nil {
		return PlayerSessionsServiceResult{Error: playerServiceError(err)}
	}
	return PlayerSessionsServiceResult{Sessions: sessions}
}

// RefreshDirectory synchronizes authoritative Minecraft player files.
func (s *PlayerService) RefreshDirectory(ctx context.Context, serverID string) PlayerListServiceResult {
	if err := s.manager.SynchronizeDirectory(ctx, model.ID(serverID)); err != nil {
		return PlayerListServiceResult{Error: playerServiceError(err)}
	}
	return s.List(ctx, model.PlayerQuery{ServerID: model.ID(serverID), Limit: 50})
}

// SynchronizeActivity claims and applies pending remote player evidence.
func (s *PlayerService) SynchronizeActivity(ctx context.Context, serverID string) PlayerListServiceResult {
	if err := s.manager.SynchronizeServer(ctx, model.ID(serverID)); err != nil {
		return PlayerListServiceResult{Error: playerServiceError(err)}
	}
	return s.List(ctx, model.PlayerQuery{ServerID: model.ID(serverID), Limit: 50})
}

// AddWhitelist adds a player to the Minecraft whitelist.
func (s *PlayerService) AddWhitelist(ctx context.Context, input model.PlayerActionInput) PlayerActionServiceResult {
	return s.action(ctx, "add_whitelist", input)
}

// RemoveWhitelist removes a player from the Minecraft whitelist.
func (s *PlayerService) RemoveWhitelist(ctx context.Context, input model.PlayerActionInput) PlayerActionServiceResult {
	return s.action(ctx, "remove_whitelist", input)
}

// GrantOperator grants Minecraft operator permissions.
func (s *PlayerService) GrantOperator(ctx context.Context, input model.PlayerActionInput) PlayerActionServiceResult {
	return s.action(ctx, "grant_operator", input)
}

// RevokeOperator revokes Minecraft operator permissions.
func (s *PlayerService) RevokeOperator(ctx context.Context, input model.PlayerActionInput) PlayerActionServiceResult {
	return s.action(ctx, "revoke_operator", input)
}

// Ban bans one player with an optional reason.
func (s *PlayerService) Ban(ctx context.Context, input model.PlayerActionInput) PlayerActionServiceResult {
	return s.action(ctx, "ban", input)
}

// Pardon removes one player Ban.
func (s *PlayerService) Pardon(ctx context.Context, input model.PlayerActionInput) PlayerActionServiceResult {
	return s.action(ctx, "pardon", input)
}

// Kick disconnects one currently online player.
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
