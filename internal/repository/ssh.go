package repository

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/model"
)

// SSHSessionQuery contains search, grouping, favourite, and pagination filters.
type SSHSessionQuery struct {
	Search        string
	Group         string
	FavouriteOnly bool
	Limit         int
	Offset        int
}

// SSHSessionRepository persists reusable SSH connection metadata without secrets.
type SSHSessionRepository interface {
	Create(context.Context, *model.SSHSession) error
	Update(context.Context, *model.SSHSession) error
	Get(context.Context, model.ID) (*model.SSHSession, error)
	List(context.Context, SSHSessionQuery) ([]model.SSHSession, error)
	UpdateHostSpecs(context.Context, model.ID, model.SSHHostSpecs) error
	Delete(context.Context, model.ID) error
	CountServerReferences(context.Context, model.ID) (int64, error)
}

// SSHCredentialRepository persists secrets inside the SQLCipher-encrypted database.
type SSHCredentialRepository interface {
	Create(context.Context, *model.SSHCredential) error
	Update(context.Context, *model.SSHCredential) error
	Get(context.Context, model.ID) (*model.SSHCredential, error)
	Delete(context.Context, model.ID) error
}

// KnownHostRepository persists trusted keys and fingerprint replacement history.
type KnownHostRepository interface {
	Create(context.Context, *model.KnownHost) error
	Update(context.Context, *model.KnownHost) error
	GetActive(context.Context, string) (*model.KnownHost, error)
	FindActive(context.Context, string) (*model.KnownHost, error)
	List(context.Context, string, int, int) ([]model.KnownHost, error)
	Replace(context.Context, *model.KnownHost, *model.KnownHost) error
	Delete(context.Context, model.ID) error
}
