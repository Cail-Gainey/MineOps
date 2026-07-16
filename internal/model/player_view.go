package model

import (
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
)

// PlayerQuery contains bounded unified-player filters and sorting.
type PlayerQuery struct {
	ServerID  ID     `json:"serverID"`
	Search    string `json:"search,omitempty"`
	Filter    string `json:"filter,omitempty"`
	Sort      string `json:"sort,omitempty"`
	Direction string `json:"direction,omitempty"`
	Limit     int    `json:"limit"`
	Offset    int    `json:"offset"`
}

// PlayerSessionQuery contains one bounded recent-session request.
type PlayerSessionQuery struct {
	ServerID         ID  `json:"serverID"`
	PlayerIdentityID ID  `json:"playerIdentityID"`
	Limit            int `json:"limit"`
	Offset           int `json:"offset"`
}

// PlayerActionInput contains one validated player management request.
type PlayerActionInput struct {
	ServerID         ID         `json:"serverID"`
	PlayerIdentityID ID         `json:"playerIdentityID"`
	Reason           string     `json:"reason,omitempty"`
	ExpiresAt        *time.Time `json:"expiresAt,omitempty"`
}

// PlayerOverview is the unified Server-scoped player list and detail projection.
type PlayerOverview struct {
	IdentityID            ID                           `json:"identityID"`
	ServerID              ID                           `json:"serverID"`
	UUID                  string                       `json:"uuid,omitempty"`
	Name                  string                       `json:"name"`
	NormalizedName        string                       `json:"normalizedName"`
	IdentityKind          enums.PlayerIdentityKind     `json:"identityKind"`
	Online                bool                         `json:"online"`
	CurrentJoinedAt       *time.Time                   `json:"currentJoinedAt,omitempty"`
	TotalDurationSeconds  int64                        `json:"totalDurationSeconds"`
	FirstActivityAt       *time.Time                   `json:"firstActivityAt,omitempty"`
	LastActivityAt        *time.Time                   `json:"lastActivityAt,omitempty"`
	CompletedSessionCount int64                        `json:"completedSessionCount"`
	LongestSessionSeconds int64                        `json:"longestSessionSeconds"`
	AverageSessionSeconds int64                        `json:"averageSessionSeconds"`
	Whitelisted           bool                         `json:"whitelisted"`
	Operator              bool                         `json:"operator"`
	Banned                bool                         `json:"banned"`
	BanReason             string                       `json:"banReason,omitempty"`
	BanSource             string                       `json:"banSource,omitempty"`
	BanExpiresAt          string                       `json:"banExpiresAt,omitempty"`
	Accuracy              enums.PlayerActivityAccuracy `json:"accuracy"`
	CollectorStatus       string                       `json:"collectorStatus,omitempty"`
	DirectoryError        string                       `json:"directoryError,omitempty"`
}

// PlayerListResult contains one bounded page and collector quality evidence.
type PlayerListResult struct {
	Players         []PlayerOverview `json:"players"`
	Total           int64            `json:"total"`
	Limit           int              `json:"limit"`
	Offset          int              `json:"offset"`
	CollectorStatus string           `json:"collectorStatus,omitempty"`
	DirectoryError  string           `json:"directoryError,omitempty"`
}

// PlayerEvent is the versioned realtime player state contract.
type PlayerEvent struct {
	Version          int             `json:"version"`
	Type             string          `json:"type"`
	ServerID         ID              `json:"serverID"`
	PlayerIdentityID ID              `json:"playerIdentityID,omitempty"`
	Player           *PlayerOverview `json:"player,omitempty"`
	Message          string          `json:"message,omitempty"`
	EmittedAt        time.Time       `json:"emittedAt"`
}

// Validate checks supported player filters, sorting, and bounded pagination.
func (q PlayerQuery) Validate() error {
	if !q.ServerID.Valid() || q.Limit < 0 || q.Limit > 200 || q.Offset < 0 || len(q.Search) > 64 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "玩家查询身份、搜索或分页参数无效")
	}
	if !oneOf(strings.TrimSpace(q.Filter), "", "all", "online", "whitelist", "operator", "banned") ||
		!oneOf(strings.TrimSpace(q.Sort), "", "name", "current_duration", "total_duration", "last_activity") ||
		!oneOf(strings.TrimSpace(q.Direction), "", "asc", "desc") {
		return apperror.New(apperror.CodeValidationInvalidArgument, "玩家查询筛选或排序参数无效")
	}
	return nil
}

// Validate checks recent-session ownership and bounded pagination.
func (q PlayerSessionQuery) Validate() error {
	if !q.ServerID.Valid() || !q.PlayerIdentityID.Valid() || q.Limit < 0 || q.Limit > 100 || q.Offset < 0 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "玩家会话查询身份或分页参数无效")
	}
	return nil
}

// Validate checks player management ownership and optional Ban metadata.
func (i PlayerActionInput) Validate() error {
	if !i.ServerID.Valid() || !i.PlayerIdentityID.Valid() || len(strings.TrimSpace(i.Reason)) > 512 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "玩家操作身份或原因无效")
	}
	return nil
}

func oneOf(value string, values ...string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
