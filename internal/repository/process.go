package repository

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/model"
)

// ProcessIdentityRepository persists the latest remote process identity per Server.
type ProcessIdentityRepository interface {
	Save(context.Context, *model.RemoteProcessIdentity) error
	GetByServer(context.Context, model.ID) (*model.RemoteProcessIdentity, error)
	ListActive(context.Context) ([]model.RemoteProcessIdentity, error)
	DeleteByServer(context.Context, model.ID) error
}
