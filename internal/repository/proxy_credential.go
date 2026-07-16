package repository

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/model"
)

// ProxyCredentialRepository persists proxy secrets only in SQLCipher-encrypted SQLite.
type ProxyCredentialRepository interface {
	Create(context.Context, *model.ProxyCredential) error
	Update(context.Context, *model.ProxyCredential) error
	Get(context.Context, model.ID) (*model.ProxyCredential, error)
	Delete(context.Context, model.ID) error
}
