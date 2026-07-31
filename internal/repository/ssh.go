package repository

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/model"
)

// SSHSessionQuery 承载搜索、分组、收藏与分页过滤条件。
type SSHSessionQuery struct {
	Search        string
	Group         string
	FavouriteOnly bool
	Limit         int
	Offset        int
}

// SSHSessionRepository 持久化可复用的 SSH 连接元数据,不含密文。
type SSHSessionRepository interface {
	Create(context.Context, *model.SSHSession) error
	Update(context.Context, *model.SSHSession) error
	Get(context.Context, model.ID) (*model.SSHSession, error)
	List(context.Context, SSHSessionQuery) ([]model.SSHSession, error)
	UpdateHostSpecs(context.Context, model.ID, model.SSHHostSpecs) error
	Delete(context.Context, model.ID) error
	CountServerReferences(context.Context, model.ID) (int64, error)
}

// SSHCredentialRepository 把密文持久化在 SQLCipher 加密的数据库内。
type SSHCredentialRepository interface {
	Create(context.Context, *model.SSHCredential) error
	Update(context.Context, *model.SSHCredential) error
	Get(context.Context, model.ID) (*model.SSHCredential, error)
	Delete(context.Context, model.ID) error
}

// KnownHostRepository 持久化受信任密钥与指纹替换历史。
type KnownHostRepository interface {
	Create(context.Context, *model.KnownHost) error
	Update(context.Context, *model.KnownHost) error
	GetActive(context.Context, string) (*model.KnownHost, error)
	FindActive(context.Context, string) (*model.KnownHost, error)
	List(context.Context, string, int, int) ([]model.KnownHost, error)
	Replace(context.Context, *model.KnownHost, *model.KnownHost) error
	Delete(context.Context, model.ID) error
}
