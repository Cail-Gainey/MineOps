package repository

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

// MinecraftServerQuery contains search, SSH, group, tag, state, and deletion filters.
type MinecraftServerQuery struct {
	Search         string
	SSHSessionID   model.ID
	Group          string
	Tag            string
	State          enums.LifecycleState
	IncludeDeleted bool
	Limit          int
	Offset         int
}

// MinecraftServerRepository persists SSH-bound Minecraft Server metadata and launch profiles.
type MinecraftServerRepository interface {
	Create(context.Context, *model.MinecraftServer) error
	Update(context.Context, *model.MinecraftServer) error
	Get(context.Context, model.ID, bool) (*model.MinecraftServer, error)
	List(context.Context, MinecraftServerQuery) ([]model.MinecraftServer, error)
	SoftDelete(context.Context, model.ID, model.Clock) error
	Restore(context.Context, model.ID, model.Clock) error
	HardDelete(context.Context, model.ID) error
}
