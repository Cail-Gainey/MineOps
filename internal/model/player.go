package model

import (
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
)

// PlayerActivitySchemaVersion 是当前持久化玩家活动数据的 schema 版本。
const PlayerActivitySchemaVersion = 1

// PlayerIdentity 是 Server 范围内以 UUID 或临时名称为依据的玩家身份。
type PlayerIdentity struct {
	ID, ServerID                      ID
	UUID, CurrentName, NormalizedName string
	Kind                              enums.PlayerIdentityKind
	CreatedAt, UpdatedAt              time.Time
	SchemaVersion                     int
}

// PlayerActivityEvent 是从一个受管进程周期采集到的不可变幂等证据。
type PlayerActivityEvent struct {
	ID, ServerID, ProcessIdentityID         ID
	PlayerIdentityID                        *ID
	SourceSequence                          uint64
	Type                                    enums.PlayerActivityEventType
	PlayerName, NormalizedName, RawEvidence string
	ObservedAt, CreatedAt                   time.Time
	SchemaVersion                           int
}

// PlayerSession 是一次持久化的玩家连接及其结算证据。
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

// PlayerStatistics 是由已结束会话重算得出的快查投影。
type PlayerStatistics struct {
	PlayerIdentityID, ServerID                  ID
	TotalDurationSeconds, LongestSessionSeconds int64
	CompletedSessionCount                       int64
	FirstActivityAt, LastActivityAt             *time.Time
	Accuracy                                    enums.PlayerActivityAccuracy
	UpdatedAt                                   time.Time
	SchemaVersion                               int
}

// PlayerDirectorySnapshot 记录 Minecraft 权威名录与权限证据。
type PlayerDirectorySnapshot struct {
	ID, ServerID, PlayerIdentityID                    ID
	Known, Whitelisted, Operator, Banned              bool
	BanReason, BanSource, BanExpiresAt, SourceVersion string
	ObservedAt                                        time.Time
	SchemaVersion                                     int
}

// PlayerCollectorStatus 记录某台 Server 的持久化同步检查点与数据质量状态。
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

// NormalizePlayerName 返回 Server 本地兜底身份的规范化键。
func NormalizePlayerName(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

// Validate 校验持久化玩家身份及其 UUID 或名称证据。
func (p PlayerIdentity) Validate() error {
	if !p.ID.Valid() || !p.ServerID.Valid() || !p.Kind.Valid() || p.SchemaVersion != PlayerActivitySchemaVersion || p.CreatedAt.IsZero() || p.UpdatedAt.IsZero() {
		return apperror.New(apperror.CodeValidationInvalidArgument, "玩家身份、类型、时间或 Schema 无效")
	}
	if p.CurrentName == "" || len(p.CurrentName) > 64 || p.NormalizedName != NormalizePlayerName(p.CurrentName) || len(p.UUID) > 36 || p.Kind == enums.PlayerIdentityUUID && p.UUID == "" || p.Kind == enums.PlayerIdentityNameOnly && p.UUID != "" {
		return apperror.New(apperror.CodeValidationInvalidArgument, "玩家 UUID 或名称证据无效")
	}
	return nil
}

// Validate 校验不可变的玩家活动证据。
func (e PlayerActivityEvent) Validate() error {
	if !e.ID.Valid() || !e.ServerID.Valid() || !e.ProcessIdentityID.Valid() || e.PlayerIdentityID != nil && !e.PlayerIdentityID.Valid() || !e.Type.Valid() || e.SourceSequence == 0 || e.ObservedAt.IsZero() || e.CreatedAt.IsZero() || e.SchemaVersion != PlayerActivitySchemaVersion {
		return apperror.New(apperror.CodeValidationInvalidArgument, "玩家事件身份、序号、类型、时间或 Schema 无效")
	}
	if e.PlayerName == "" || len(e.PlayerName) > 64 || e.NormalizedName != NormalizePlayerName(e.PlayerName) || len(e.RawEvidence) > 8192 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "玩家事件名称或原始证据无效")
	}
	return nil
}

// Validate 校验玩家会话的归属、状态、时长与结算证据。
func (s PlayerSession) Validate() error {
	if !s.ID.Valid() || !s.ServerID.Valid() || !s.PlayerIdentityID.Valid() || !s.ProcessIdentityID.Valid() || !s.State.Valid() || !s.Accuracy.Valid() || s.JoinedAt.IsZero() || s.CreatedAt.IsZero() || s.UpdatedAt.IsZero() || s.DurationSeconds < 0 || s.SchemaVersion != PlayerActivitySchemaVersion {
		return apperror.New(apperror.CodeValidationInvalidArgument, "玩家会话身份、状态、时间、时长或 Schema 无效")
	}
	if s.State == enums.PlayerSessionOpen && (s.LeftAt != nil || s.CloseReason != "" || s.DurationSeconds != 0) || s.State != enums.PlayerSessionOpen && (s.LeftAt == nil || !s.CloseReason.Valid() || s.LeftAt.Before(s.JoinedAt)) {
		return apperror.New(apperror.CodeValidationInvalidArgument, "玩家会话关闭证据无效")
	}
	return nil
}

// Validate 校验可重算的玩家统计投影。
func (s PlayerStatistics) Validate() error {
	if !s.PlayerIdentityID.Valid() || !s.ServerID.Valid() || s.TotalDurationSeconds < 0 || s.LongestSessionSeconds < 0 || s.CompletedSessionCount < 0 || !s.Accuracy.Valid() || s.UpdatedAt.IsZero() || s.SchemaVersion != PlayerActivitySchemaVersion {
		return apperror.New(apperror.CodeValidationInvalidArgument, "玩家统计身份、计数、准确性或 Schema 无效")
	}
	return nil
}

// Validate 校验一份玩家名录快照。
func (s PlayerDirectorySnapshot) Validate() error {
	if !s.ID.Valid() || !s.ServerID.Valid() || !s.PlayerIdentityID.Valid() || s.ObservedAt.IsZero() || len(s.BanReason) > 2048 || len(s.BanSource) > 256 || len(s.SourceVersion) > 256 || s.SchemaVersion != PlayerActivitySchemaVersion {
		return apperror.New(apperror.CodeValidationInvalidArgument, "玩家目录快照身份、证据或 Schema 无效")
	}
	return nil
}

// Validate 校验一个采集器同步检查点。
func (s PlayerCollectorStatus) Validate() error {
	if !s.ServerID.Valid() || s.ProcessIdentityID != nil && !s.ProcessIdentityID.Valid() || !s.Accuracy.Valid() || len(s.LastError) > 2048 || s.UpdatedAt.IsZero() || s.SchemaVersion != PlayerActivitySchemaVersion {
		return apperror.New(apperror.CodeValidationInvalidArgument, "玩家采集状态身份、准确性或 Schema 无效")
	}
	return nil
}
