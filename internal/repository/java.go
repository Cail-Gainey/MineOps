package repository

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/model"
)

// JavaRuntimeQuery contains SSH, major-version, source, and pagination filters.
type JavaRuntimeQuery struct {
	SSHSessionID model.ID
	MajorVersion int
	Source       model.JavaRuntimeSource
	Limit        int
	Offset       int
}

// JavaRuntimeRepository persists reusable remote Java installations.
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
