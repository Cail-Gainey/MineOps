// Package servercatalog implements provider-isolated Minecraft server version catalogs and a dynamic registry.
package servercatalog

import (
	"context"
	"sort"
	"sync"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/port"
)

// SourceResolver selects the highest-priority enabled official or mirror endpoint for a provider.
type SourceResolver interface {
	ResolveSource(string, string) string
}

// Registry owns dynamically registered server catalogs and installer capability metadata.
type Registry struct {
	mu         sync.RWMutex
	catalogs   map[enums.MinecraftServerType]port.ServerCatalog
	installers map[enums.MinecraftServerType]port.Installer
	ready      map[enums.MinecraftServerType]bool
	names      map[enums.MinecraftServerType]string
}

// NewRegistry creates the complete first-release server type registry.
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

// EnableGenericInstallers marks distributions handled by the shared verified Jar installation pipeline.
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

// RegisterInstaller adds or replaces the installer implementation for one distribution.
func (r *Registry) RegisterInstaller(installer port.Installer) {
	if r == nil || installer == nil {
		return
	}
	r.mu.Lock()
	r.installers[installer.Distribution()] = installer
	r.ready[installer.Distribution()] = true
	r.mu.Unlock()
}

// List returns every supported first-release distribution without frontend hard-coding.
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

// ResolveVersions delegates to the registered provider adapter.
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

// ResolveArtifact delegates immutable artifact resolution to the registered provider adapter.
func (r *Registry) ResolveArtifact(ctx context.Context, distribution enums.MinecraftServerType, version, build string) (port.ServerArtifact, error) {
	r.mu.RLock()
	catalog := r.catalogs[distribution]
	r.mu.RUnlock()
	if catalog == nil {
		return port.ServerArtifact{}, apperror.New(apperror.CodeValidationConflict, "服务端类型尚未注册 Artifact 目录")
	}
	return catalog.ResolveArtifact(ctx, version, build)
}

// Installer returns the registered distribution-specific installer.
func (r *Registry) Installer(distribution enums.MinecraftServerType) (port.Installer, error) {
	r.mu.RLock()
	installer := r.installers[distribution]
	r.mu.RUnlock()
	if installer == nil {
		return nil, apperror.New(apperror.CodeValidationConflict, "服务端类型尚未注册 Installer")
	}
	return installer, nil
}
