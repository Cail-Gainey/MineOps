package repository

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/model"
)

// ProxyCredentialRepository 仅在 SQLCipher 加密的 SQLite 中持久化代理密文。
type ProxyCredentialRepository interface {
	Create(context.Context, *model.ProxyCredential) error
	Update(context.Context, *model.ProxyCredential) error
	Get(context.Context, model.ID) (*model.ProxyCredential, error)
	Delete(context.Context, model.ID) error
}
