package service

import (
	"context"
	"path"
	"strings"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/appsettings"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
)

// MinecraftServerCommand 承载可编辑的服务器元数据与结构化启动设置。
type MinecraftServerCommand struct {
	SSHSessionID   model.ID
	JavaRuntimeID  *model.ID
	Name           string
	Type           enums.MinecraftServerType
	Version        string
	RemotePath     string
	Group          string
	Tags           []string
	Favourite      bool
	LaunchProfile  model.LaunchProfile
	FirewallPolicy enums.FirewallPolicy
	EULAAccepted   bool
}

// MinecraftServerManager 统筹 SSH 与 Java 引用以及持久化的 Server 元数据。
type MinecraftServerManager struct {
	clock    model.Clock
	store    repository.Store
	settings *appsettings.Manager
	clients  *SSHClientFactory
	runner   *OperationRunner
	firewall *FirewallManager
}

// NewMinecraftServerManager 创建 Server 应用服务。
func NewMinecraftServerManager(clock model.Clock, store repository.Store, settings *appsettings.Manager, clients *SSHClientFactory, runner *OperationRunner, firewall *FirewallManager) (*MinecraftServerManager, error) {
	if clock == nil || store == nil || settings == nil || clients == nil || runner == nil || firewall == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Minecraft Server Manager 依赖不能为空")
	}
	return &MinecraftServerManager{clock: clock, store: store, settings: settings, clients: clients, runner: runner, firewall: firewall}, nil
}

// Create 校验 SSH 与 Java 引用,并持久化一条创建中状态的服务器记录。
func (m *MinecraftServerManager) Create(ctx context.Context, command MinecraftServerCommand) (*model.MinecraftServer, error) {
	if err := m.validateReferences(ctx, command.SSHSessionID, command.JavaRuntimeID); err != nil {
		return nil, err
	}
	if command.FirewallPolicy == "" {
		command.FirewallPolicy = enums.FirewallPolicy(m.settings.Snapshot().Firewall.DefaultPolicy)
	}
	server, err := model.NewMinecraftServer(m.clock, model.MinecraftServer{
		SSHSessionID: command.SSHSessionID, JavaRuntimeID: command.JavaRuntimeID,
		Name: command.Name, Type: command.Type, Version: command.Version,
		RemotePath: path.Clean(command.RemotePath), Group: command.Group, Tags: normalizeTags(command.Tags),
		Favourite: command.Favourite, State: enums.LifecycleCreating,
		LaunchProfile: command.LaunchProfile, FirewallPolicy: command.FirewallPolicy,
		EULAAccepted: command.EULAAccepted,
	})
	if err != nil {
		return nil, err
	}
	if err := m.store.MinecraftServers().Create(ctx, server); err != nil {
		return nil, err
	}
	return server, nil
}

// Update 修改元数据与 LaunchProfile,不会隐式移动远端目录。
func (m *MinecraftServerManager) Update(ctx context.Context, id model.ID, command MinecraftServerCommand) (*model.MinecraftServer, error) {
	server, err := m.store.MinecraftServers().Get(ctx, id, false)
	if err != nil {
		return nil, err
	}
	if command.SSHSessionID != server.SSHSessionID || path.Clean(command.RemotePath) != server.RemotePath {
		return nil, apperror.New(apperror.CodeValidationConflict, "更新 Server 元数据不能隐式更换 SSH Session 或移动远程目录")
	}
	if err := m.validateReferences(ctx, command.SSHSessionID, command.JavaRuntimeID); err != nil {
		return nil, err
	}
	server.JavaRuntimeID = command.JavaRuntimeID
	server.Name = command.Name
	server.Type = command.Type
	server.Version = command.Version
	server.Group = command.Group
	server.Tags = normalizeTags(command.Tags)
	server.Favourite = command.Favourite
	server.LaunchProfile = command.LaunchProfile
	server.FirewallPolicy = command.FirewallPolicy
	server.EULAAccepted = command.EULAAccepted
	server.UpdatedAt = m.clock.Now().UTC()
	if err := server.Validate(); err != nil {
		return nil, err
	}
	if err := m.store.MinecraftServers().Update(ctx, server); err != nil {
		return nil, err
	}
	return server, nil
}

// SoftDelete 把已停止或未运行的 Server 标记为已删除,同时保留其远端目录。
func (m *MinecraftServerManager) SoftDelete(ctx context.Context, id model.ID) error {
	server, err := m.store.MinecraftServers().Get(ctx, id, false)
	if err != nil {
		return err
	}
	if !server.CanSoftDelete() {
		return apperror.New(apperror.CodeValidationConflict, "运行中或正在转换状态的 Server 不能软删除")
	}
	operations, err := m.store.Operations().ListActive(ctx, id)
	if err != nil {
		return err
	}
	if len(operations) > 0 {
		return apperror.New(apperror.CodeValidationConflict, "Server 存在活动 Operation，不能软删除")
	}
	return m.store.MinecraftServers().SoftDelete(ctx, id, m.clock)
}

// Restore 把已软删除的 Server 记录恢复为已停止状态。
func (m *MinecraftServerManager) Restore(ctx context.Context, id model.ID) error {
	return m.store.MinecraftServers().Restore(ctx, id, m.clock)
}

// HardDeleteRegistration 在精确确认后,仅删除已软删除的数据库记录。
func (m *MinecraftServerManager) HardDeleteRegistration(ctx context.Context, id model.ID, confirmedName, confirmedPath string) error {
	server, err := m.store.MinecraftServers().Get(ctx, id, true)
	if err != nil {
		return err
	}
	if server.DeletedAt == nil || confirmedName != server.Name || path.Clean(confirmedPath) != server.RemotePath {
		return apperror.New(apperror.CodeValidationConflict, "硬删除确认对象与实际 Server 不一致")
	}
	return m.store.MinecraftServers().HardDelete(ctx, id)
}

func (m *MinecraftServerManager) validateReferences(ctx context.Context, sshSessionID model.ID, javaRuntimeID *model.ID) error {
	if _, err := m.store.SSHSessions().Get(ctx, sshSessionID); err != nil {
		return err
	}
	if javaRuntimeID == nil {
		return nil
	}
	javaRuntime, err := m.store.JavaRuntimes().Get(ctx, *javaRuntimeID)
	if err != nil {
		return err
	}
	if javaRuntime.SSHSessionID != sshSessionID {
		return apperror.New(apperror.CodeValidationConflict, "Java Runtime 与 Minecraft Server 不属于同一 SSH Session")
	}
	return nil
}

func normalizeTags(tags []string) []string {
	seen := make(map[string]bool, len(tags))
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag != "" && !seen[tag] {
			seen[tag] = true
			result = append(result, tag)
		}
	}
	return result
}
