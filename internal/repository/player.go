package repository

import (
	"context"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/model"
)

// PlayerIdentityQuery contains UUID/name fallback identity evidence.
type PlayerIdentityQuery struct {
	ServerID             model.ID
	UUID, NormalizedName string
}

// PlayerSessionQuery contains bounded Server/player session filters.
type PlayerSessionQuery struct {
	ServerID, PlayerIdentityID model.ID
	OpenOnly                   bool
	Limit, Offset              int
}

// PlayerRepository persists player identities, evidence, sessions, projections, and collector checkpoints.
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
