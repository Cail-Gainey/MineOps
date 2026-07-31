package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
)

// ListPlayers 返回一页有界的统一玩家数据。
func (m *PlayerActivityManager) ListPlayers(ctx context.Context, query model.PlayerQuery) (model.PlayerListResult, error) {
	if query.Limit == 0 {
		query.Limit = 50
	}
	if err := query.Validate(); err != nil {
		return model.PlayerListResult{}, err
	}
	identities, err := m.store.Players().ListIdentities(ctx, query.ServerID, 200, 0)
	if err != nil {
		return model.PlayerListResult{}, err
	}
	players := make([]model.PlayerOverview, 0, len(identities))
	for index := range identities {
		overview, err := m.playerOverview(ctx, identities[index])
		if err != nil {
			return model.PlayerListResult{}, err
		}
		if playerMatchesQuery(overview, query) {
			players = append(players, overview)
		}
	}
	sortPlayerOverviews(players, query.Sort, query.Direction)
	total := int64(len(players))
	start := min(query.Offset, len(players))
	end := min(start+query.Limit, len(players))
	status, statusErr := m.store.Players().GetCollectorStatus(ctx, query.ServerID)
	collector := "pending"
	directoryError := ""
	if statusErr == nil && status != nil {
		collector = status.Accuracy.String()
		directoryError = status.LastError
	} else if statusErr != nil && !isPlayerNotFound(statusErr) {
		return model.PlayerListResult{}, statusErr
	}
	return model.PlayerListResult{Players: players[start:end], Total: total, Limit: query.Limit, Offset: query.Offset, CollectorStatus: collector, DirectoryError: directoryError}, nil
}

// PlayerDetail 返回一份 Server 范围内统一的玩家投影。
func (m *PlayerActivityManager) PlayerDetail(ctx context.Context, serverID, playerID model.ID) (model.PlayerOverview, error) {
	if !serverID.Valid() || !playerID.Valid() {
		return model.PlayerOverview{}, apperror.New(apperror.CodeValidationInvalidArgument, "玩家详情身份无效")
	}
	identities, err := m.store.Players().ListIdentities(ctx, serverID, 200, 0)
	if err != nil {
		return model.PlayerOverview{}, err
	}
	for index := range identities {
		if identities[index].ID == playerID {
			return m.playerOverview(ctx, identities[index])
		}
	}
	return model.PlayerOverview{}, apperror.New(apperror.CodeIONotFound, "玩家不存在")
}

// RecentPlayerSessions 返回一页按时间倒序的有界会话数据。
func (m *PlayerActivityManager) RecentPlayerSessions(ctx context.Context, query model.PlayerSessionQuery) ([]model.PlayerSession, error) {
	if query.Limit == 0 {
		query.Limit = 20
	}
	if err := query.Validate(); err != nil {
		return nil, err
	}
	return m.store.Players().ListSessions(ctx, repository.PlayerSessionQuery{ServerID: query.ServerID, PlayerIdentityID: query.PlayerIdentityID, Limit: query.Limit, Offset: query.Offset})
}

func (m *PlayerActivityManager) playerOverview(ctx context.Context, identity model.PlayerIdentity) (model.PlayerOverview, error) {
	overview := model.PlayerOverview{
		IdentityID: identity.ID, ServerID: identity.ServerID, UUID: identity.UUID, Name: identity.CurrentName,
		NormalizedName: identity.NormalizedName, IdentityKind: identity.Kind,
	}
	if statistics, err := m.store.Players().GetStatistics(ctx, identity.ID); err == nil {
		overview.TotalDurationSeconds = statistics.TotalDurationSeconds
		overview.FirstActivityAt = statistics.FirstActivityAt
		overview.LastActivityAt = statistics.LastActivityAt
		overview.CompletedSessionCount = statistics.CompletedSessionCount
		overview.LongestSessionSeconds = statistics.LongestSessionSeconds
		overview.Accuracy = statistics.Accuracy
		if statistics.CompletedSessionCount > 0 {
			overview.AverageSessionSeconds = statistics.TotalDurationSeconds / statistics.CompletedSessionCount
		}
	} else if !isPlayerNotFound(err) {
		return model.PlayerOverview{}, err
	}
	if session, err := m.store.Players().GetOpenSession(ctx, identity.ServerID, identity.ID); err == nil {
		processIdentity, processErr := m.store.ProcessIdentities().GetByServer(ctx, identity.ServerID)
		freshOpenSession := overview.LastActivityAt == nil || !session.JoinedAt.Before(*overview.LastActivityAt)
		if processErr == nil && processIdentity.State == enums.RemoteProcessRunning && processIdentity.ID == session.ProcessIdentityID && freshOpenSession {
			overview.Online = true
			joined := session.JoinedAt
			overview.CurrentJoinedAt = &joined
			if overview.LastActivityAt == nil || joined.After(*overview.LastActivityAt) {
				overview.LastActivityAt = &joined
			}
			if !overview.Accuracy.Valid() {
				overview.Accuracy = session.Accuracy
			}
		} else if processErr != nil && !isPlayerNotFound(processErr) {
			return model.PlayerOverview{}, processErr
		}
	} else if !isPlayerNotFound(err) {
		return model.PlayerOverview{}, err
	}
	if snapshot, err := m.store.Players().GetDirectorySnapshot(ctx, identity.ID); err == nil {
		overview.Whitelisted, overview.Operator, overview.Banned = snapshot.Whitelisted, snapshot.Operator, snapshot.Banned
		overview.BanReason, overview.BanSource, overview.BanExpiresAt = snapshot.BanReason, snapshot.BanSource, snapshot.BanExpiresAt
	} else if !isPlayerNotFound(err) {
		return model.PlayerOverview{}, err
	}
	if status, err := m.store.Players().GetCollectorStatus(ctx, identity.ServerID); err == nil {
		overview.CollectorStatus, overview.DirectoryError = status.Accuracy.String(), status.LastError
		if status.Accuracy.String() == "incomplete" {
			overview.Accuracy = status.Accuracy
		}
	} else if !isPlayerNotFound(err) {
		return model.PlayerOverview{}, err
	}
	if !overview.Accuracy.Valid() {
		overview.Accuracy = "exact"
	}
	return overview, nil
}

func playerMatchesQuery(player model.PlayerOverview, query model.PlayerQuery) bool {
	search := strings.ToLower(strings.TrimSpace(query.Search))
	if search != "" && !strings.Contains(strings.ToLower(player.Name), search) {
		return false
	}
	switch query.Filter {
	case "online":
		return player.Online
	case "whitelist":
		return player.Whitelisted
	case "operator":
		return player.Operator
	case "banned":
		return player.Banned
	default:
		return true
	}
}

func sortPlayerOverviews(players []model.PlayerOverview, field, direction string) {
	descending := direction == "desc"
	sort.SliceStable(players, func(left, right int) bool {
		comparison := 0
		switch field {
		case "total_duration":
			comparison = compareInt64(players[left].TotalDurationSeconds, players[right].TotalDurationSeconds)
		case "current_duration":
			comparison = compareTimePointers(players[left].CurrentJoinedAt, players[right].CurrentJoinedAt)
		case "last_activity":
			comparison = compareTimePointers(players[left].LastActivityAt, players[right].LastActivityAt)
		default:
			comparison = strings.Compare(players[left].NormalizedName, players[right].NormalizedName)
		}
		if comparison == 0 {
			comparison = strings.Compare(players[left].IdentityID.String(), players[right].IdentityID.String())
		}
		if descending {
			return comparison > 0
		}
		return comparison < 0
	})
}

func compareInt64(left, right int64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func compareTimePointers(left, right *time.Time) int {
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return -1
	}
	if right == nil {
		return 1
	}
	return compareInt64(left.UnixNano(), right.UnixNano())
}
