package repository

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/model"
)

// JavaRuntimeQuery 承载 SSH、主版本、来源与分页过滤条件。
type JavaRuntimeQuery struct {
	SSHSessionID model.ID
	MajorVersion int
	Source       model.JavaRuntimeSource
	Limit        int
	Offset       int
}

// JavaRuntimeRepository 持久化可复用的远端 Java 安装。
type JavaRuntimeRepository interface {
	Create(context.Context, *model.JavaRuntime) error
	Update(context.Context, *model.JavaRuntime) error
	Get(context.Context, model.ID) (*model.JavaRuntime, error)
	GetByPath(context.Context, model.ID, string) (*model.JavaRuntime, error)
	List(context.Context, JavaRuntimeQuery) ([]model.JavaRuntime, error)
	SetDefault(context.Context, model.ID, model.ID) error
	CountServerReferences(context.Context, model.ID) (int64, error)
	Delete(context.Context, model.ID) error
}
