package repository

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

// MinecraftServerQuery 承载搜索、SSH、分组、标签、状态与删除过滤条件。
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

// MinecraftServerRepository 持久化绑定 SSH 的 Minecraft Server 元数据与启动配置。
type MinecraftServerRepository interface {
	Create(context.Context, *model.MinecraftServer) error
	Update(context.Context, *model.MinecraftServer) error
	Get(context.Context, model.ID, bool) (*model.MinecraftServer, error)
	List(context.Context, MinecraftServerQuery) ([]model.MinecraftServer, error)
	SoftDelete(context.Context, model.ID, model.Clock) error
	Restore(context.Context, model.ID, model.Clock) error
	HardDelete(context.Context, model.ID) error
}
