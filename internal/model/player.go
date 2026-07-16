package model

import (
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
)

// PlayerActivitySchemaVersion is the current durable player activity schema.
const PlayerActivitySchemaVersion = 1

// PlayerIdentity is one Server-scoped UUID-backed or temporary name-backed player.
type PlayerIdentity struct {
	ID, ServerID                      ID
	UUID, CurrentName, NormalizedName string
	Kind                              enums.PlayerIdentityKind
	CreatedAt, UpdatedAt              time.Time
	SchemaVersion                     int
}

// PlayerActivityEvent is immutable idempotent evidence collected from one managed process cycle.
type PlayerActivityEvent struct {
	ID, ServerID, ProcessIdentityID         ID
	PlayerIdentityID                        *ID
	SourceSequence                          uint64
	Type                                    enums.PlayerActivityEventType
	PlayerName, NormalizedName, RawEvidence string
	ObservedAt, CreatedAt                   time.Time
	SchemaVersion                           int
}

// PlayerSession is one persisted player connection and its settlement evidence.
type PlayerSession struct {
	ID                ID                             `json:"id"`
	ServerID          ID                             `json:"serverID"`
	PlayerIdentityID  ID                             `json:"playerIdentityID"`
	ProcessIdentityID ID                             `json:"processIdentityID"`
	JoinedAt          time.Time                      `json:"joinedAt"`
	LeftAt            *time.Time                     `json:"leftAt,omitempty"`
	DurationSeconds   int64                          `json:"durationSeconds"`
	State             enums.PlayerSessionState       `json:"state"`
	CloseReason       enums.PlayerSessionCloseReason `json:"closeReason,omitempty"`
	Accuracy          enums.PlayerActivityAccuracy   `json:"accuracy"`
	CreatedAt         time.Time                      `json:"createdAt"`
	UpdatedAt         time.Time                      `json:"updatedAt"`
	SchemaVersion     int                            `json:"schemaVersion"`
}

// PlayerStatistics is the rebuildable fast-query projection of closed sessions.
type PlayerStatistics struct {
	PlayerIdentityID, ServerID                  ID
	TotalDurationSeconds, LongestSessionSeconds int64
	CompletedSessionCount                       int64
	FirstActivityAt, LastActivityAt             *time.Time
	Accuracy                                    enums.PlayerActivityAccuracy
	UpdatedAt                                   time.Time
	SchemaVersion                               int
}

// PlayerDirectorySnapshot records authoritative Minecraft directory and permission evidence.
type PlayerDirectorySnapshot struct {
	ID, ServerID, PlayerIdentityID                    ID
	Known, Whitelisted, Operator, Banned              bool
	BanReason, BanSource, BanExpiresAt, SourceVersion string
	ObservedAt                                        time.Time
	SchemaVersion                                     int
}

// PlayerCollectorStatus records the durable synchronization checkpoint and quality state for one Server.
type PlayerCollectorStatus struct {
	ServerID                              ID
	ProcessIdentityID                     *ID
	LastSourceSequence, DroppedEventCount uint64
	LastObservedAt, LastSynchronizedAt    *time.Time
	Accuracy                              enums.PlayerActivityAccuracy
	LastError                             string
	UpdatedAt                             time.Time
	SchemaVersion                         int
}

// NormalizePlayerName returns the canonical Server-local fallback identity key.
func NormalizePlayerName(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

// Validate checks the durable player identity and its UUID/name evidence.
func (p PlayerIdentity) Validate() error {
	if !p.ID.Valid() || !p.ServerID.Valid() || !p.Kind.Valid() || p.SchemaVersion != PlayerActivitySchemaVersion || p.CreatedAt.IsZero() || p.UpdatedAt.IsZero() {
		return apperror.New(apperror.CodeValidationInvalidArgument, "玩家身份、类型、时间或 Schema 无效")
	}
	if p.CurrentName == "" || len(p.CurrentName) > 64 || p.NormalizedName != NormalizePlayerName(p.CurrentName) || len(p.UUID) > 36 || p.Kind == enums.PlayerIdentityUUID && p.UUID == "" || p.Kind == enums.PlayerIdentityNameOnly && p.UUID != "" {
		return apperror.New(apperror.CodeValidationInvalidArgument, "玩家 UUID 或名称证据无效")
	}
	return nil
}

// Validate checks immutable player activity evidence.
func (e PlayerActivityEvent) Validate() error {
	if !e.ID.Valid() || !e.ServerID.Valid() || !e.ProcessIdentityID.Valid() || e.PlayerIdentityID != nil && !e.PlayerIdentityID.Valid() || !e.Type.Valid() || e.SourceSequence == 0 || e.ObservedAt.IsZero() || e.CreatedAt.IsZero() || e.SchemaVersion != PlayerActivitySchemaVersion {
		return apperror.New(apperror.CodeValidationInvalidArgument, "玩家事件身份、序号、类型、时间或 Schema 无效")
	}
	if e.PlayerName == "" || len(e.PlayerName) > 64 || e.NormalizedName != NormalizePlayerName(e.PlayerName) || len(e.RawEvidence) > 8192 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "玩家事件名称或原始证据无效")
	}
	return nil
}

// Validate checks player session ownership, state, duration, and settlement evidence.
func (s PlayerSession) Validate() error {
	if !s.ID.Valid() || !s.ServerID.Valid() || !s.PlayerIdentityID.Valid() || !s.ProcessIdentityID.Valid() || !s.State.Valid() || !s.Accuracy.Valid() || s.JoinedAt.IsZero() || s.CreatedAt.IsZero() || s.UpdatedAt.IsZero() || s.DurationSeconds < 0 || s.SchemaVersion != PlayerActivitySchemaVersion {
		return apperror.New(apperror.CodeValidationInvalidArgument, "玩家会话身份、状态、时间、时长或 Schema 无效")
	}
	if s.State == enums.PlayerSessionOpen && (s.LeftAt != nil || s.CloseReason != "" || s.DurationSeconds != 0) || s.State != enums.PlayerSessionOpen && (s.LeftAt == nil || !s.CloseReason.Valid() || s.LeftAt.Before(s.JoinedAt)) {
		return apperror.New(apperror.CodeValidationInvalidArgument, "玩家会话关闭证据无效")
	}
	return nil
}

// Validate checks the rebuildable player statistics projection.
func (s PlayerStatistics) Validate() error {
	if !s.PlayerIdentityID.Valid() || !s.ServerID.Valid() || s.TotalDurationSeconds < 0 || s.LongestSessionSeconds < 0 || s.CompletedSessionCount < 0 || !s.Accuracy.Valid() || s.UpdatedAt.IsZero() || s.SchemaVersion != PlayerActivitySchemaVersion {
		return apperror.New(apperror.CodeValidationInvalidArgument, "玩家统计身份、计数、准确性或 Schema 无效")
	}
	return nil
}

// Validate checks one player directory snapshot.
func (s PlayerDirectorySnapshot) Validate() error {
	if !s.ID.Valid() || !s.ServerID.Valid() || !s.PlayerIdentityID.Valid() || s.ObservedAt.IsZero() || len(s.BanReason) > 2048 || len(s.BanSource) > 256 || len(s.SourceVersion) > 256 || s.SchemaVersion != PlayerActivitySchemaVersion {
		return apperror.New(apperror.CodeValidationInvalidArgument, "玩家目录快照身份、证据或 Schema 无效")
	}
	return nil
}

// Validate checks one collector synchronization checkpoint.
func (s PlayerCollectorStatus) Validate() error {
	if !s.ServerID.Valid() || s.ProcessIdentityID != nil && !s.ProcessIdentityID.Valid() || !s.Accuracy.Valid() || len(s.LastError) > 2048 || s.UpdatedAt.IsZero() || s.SchemaVersion != PlayerActivitySchemaVersion {
		return apperror.New(apperror.CodeValidationInvalidArgument, "玩家采集状态身份、准确性或 Schema 无效")
	}
	return nil
}
