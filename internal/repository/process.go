package repository

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/model"
)

// ProcessIdentityRepository 持久化每台 Server 最新的远端进程身份。
type ProcessIdentityRepository interface {
	Save(context.Context, *model.RemoteProcessIdentity) error
	GetByServer(context.Context, model.ID) (*model.RemoteProcessIdentity, error)
	ListActive(context.Context) ([]model.RemoteProcessIdentity, error)
	DeleteByServer(context.Context, model.ID) error
}
