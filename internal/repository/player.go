package repository

import (
	"context"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/model"
)

// PlayerIdentityQuery 承载 UUID 与名称兜底的身份证据。
type PlayerIdentityQuery struct {
	ServerID             model.ID
	UUID, NormalizedName string
}

// PlayerSessionQuery 承载有界的 Server 与玩家会话过滤条件。
type PlayerSessionQuery struct {
	ServerID, PlayerIdentityID model.ID
	OpenOnly                   bool
	Limit, Offset              int
}

// PlayerRepository 持久化玩家身份、活动证据、会话、投影与采集器检查点。
type PlayerRepository interface {
	CreateIdentity(context.Context, *model.PlayerIdentity) error
	FindIdentity(context.Context, PlayerIdentityQuery) (*model.PlayerIdentity, error)
	ListIdentities(context.Context, model.ID, int, int) ([]model.PlayerIdentity, error)
	UpdateIdentity(context.Context, *model.PlayerIdentity) error
	DeleteIdentity(context.Context, model.ID) error
	ReassignIdentity(context.Context, model.ID, model.ID) error
	InsertEvent(context.Context, *model.PlayerActivityEvent) (bool, error)
	CreateSession(context.Context, *model.PlayerSession) error
	UpdateSession(context.Context, *model.PlayerSession) error
	GetOpenSession(context.Context, model.ID, model.ID) (*model.PlayerSession, error)
	ListSessions(context.Context, PlayerSessionQuery) ([]model.PlayerSession, error)
	SaveStatistics(context.Context, *model.PlayerStatistics) error
	GetStatistics(context.Context, model.ID) (*model.PlayerStatistics, error)
	ListClosedSessionsForRebuild(context.Context, model.ID, int, int) ([]model.PlayerSession, error)
	SaveDirectorySnapshot(context.Context, *model.PlayerDirectorySnapshot) error
	GetDirectorySnapshot(context.Context, model.ID) (*model.PlayerDirectorySnapshot, error)
	SaveCollectorStatus(context.Context, *model.PlayerCollectorStatus) error
	GetCollectorStatus(context.Context, model.ID) (*model.PlayerCollectorStatus, error)
	DeleteEventsBefore(context.Context, model.ID, time.Time, int) (int64, error)
}
