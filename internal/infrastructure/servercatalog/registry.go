// Package servercatalog 实现按供应方隔离的 Minecraft 服务端版本目录与动态注册表。
package servercatalog

import (
	"context"
	"sort"
	"sync"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/port"
)

// SourceResolver 为某个供应方选出优先级最高且已启用的官方或镜像端点。
type SourceResolver interface {
	ResolveSource(string, string) string
}

// Registry 持有动态注册的服务端目录与安装器能力元数据。
type Registry struct {
	mu         sync.RWMutex
	catalogs   map[enums.MinecraftServerType]port.ServerCatalog
	installers map[enums.MinecraftServerType]port.Installer
	ready      map[enums.MinecraftServerType]bool
	names      map[enums.MinecraftServerType]string
}

// NewRegistry 创建完整的首发服务端类型注册表。
func NewRegistry(catalogs ...port.ServerCatalog) *Registry {
	registry := &Registry{
		catalogs:   make(map[enums.MinecraftServerType]port.ServerCatalog),
		installers: make(map[enums.MinecraftServerType]port.Installer),
		ready:      make(map[enums.MinecraftServerType]bool),
		names: map[enums.MinecraftServerType]string{
			enums.ServerVanilla: "Vanilla", enums.ServerPaper: "Paper", enums.ServerPurpur: "Purpur",
			enums.ServerSpigot: "Spigot", enums.ServerFabric: "Fabric", enums.ServerForge: "Forge",
			enums.ServerNeoForge: "NeoForge", enums.ServerQuilt: "Quilt", enums.ServerFolia: "Folia",
			enums.ServerVelocity: "Velocity", enums.ServerWaterfall: "Waterfall", enums.ServerBungee: "BungeeCord",
		},
	}
	for _, catalog := range catalogs {
		if catalog != nil {
			registry.catalogs[catalog.Distribution()] = catalog
		}
	}
	return registry
}

// EnableGenericInstallers 标记由共享的、经校验 Jar 安装流水线处理的发行版。
func (r *Registry) EnableGenericInstallers(distributions ...enums.MinecraftServerType) {
	if r == nil {
		return
	}
	r.mu.Lock()
	for _, distribution := range distributions {
		if distribution.Valid() {
			r.ready[distribution] = true
		}
	}
	r.mu.Unlock()
}

// RegisterInstaller 为某个发行版新增或替换安装器实现。
func (r *Registry) RegisterInstaller(installer port.Installer) {
	if r == nil || installer == nil {
		return
	}
	r.mu.Lock()
	r.installers[installer.Distribution()] = installer
	r.ready[installer.Distribution()] = true
	r.mu.Unlock()
}

// List 返回全部受支持的首发发行版,前端无需硬编码。
func (r *Registry) List() []port.ServerDistribution {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]port.ServerDistribution, 0, len(r.names))
	for distribution, name := range r.names {
		_, catalogReady := r.catalogs[distribution]
		installerReady := r.ready[distribution]
		result = append(result, port.ServerDistribution{
			Type: distribution, DisplayName: name,
			Proxy:        distribution == enums.ServerVelocity || distribution == enums.ServerWaterfall || distribution == enums.ServerBungee,
			CatalogReady: catalogReady, InstallerReady: installerReady,
		})
	}
	sort.Slice(result, func(left, right int) bool { return result[left].DisplayName < result[right].DisplayName })
	return result
}

// ResolveVersions 委托给已注册的供应方适配器。
func (r *Registry) ResolveVersions(ctx context.Context, distribution enums.MinecraftServerType) ([]port.ServerVersion, error) {
	r.mu.RLock()
	catalog := r.catalogs[distribution]
	r.mu.RUnlock()
	if catalog == nil {
		return nil, apperror.New(apperror.CodeValidationConflict, "服务端类型尚未注册版本目录").WithDetails(map[string]any{
			"distribution": distribution,
		})
	}
	return catalog.ResolveVersions(ctx)
}

// ResolveArtifact 把不可变构件的解析委托给已注册的供应方适配器。
func (r *Registry) ResolveArtifact(ctx context.Context, distribution enums.MinecraftServerType, version, build string) (port.ServerArtifact, error) {
	r.mu.RLock()
	catalog := r.catalogs[distribution]
	r.mu.RUnlock()
	if catalog == nil {
		return port.ServerArtifact{}, apperror.New(apperror.CodeValidationConflict, "服务端类型尚未注册 Artifact 目录")
	}
	return catalog.ResolveArtifact(ctx, version, build)
}

// Installer 返回该发行版已注册的专属安装器。
func (r *Registry) Installer(distribution enums.MinecraftServerType) (port.Installer, error) {
	r.mu.RLock()
	installer := r.installers[distribution]
	r.mu.RUnlock()
	if installer == nil {
		return nil, apperror.New(apperror.CodeValidationConflict, "服务端类型尚未注册 Installer")
	}
	return installer, nil
}
