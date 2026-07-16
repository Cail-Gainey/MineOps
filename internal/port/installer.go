package port

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

// ServerVersion is one provider-neutral version exposed by a server distribution catalog.
type ServerVersion struct {
	Distribution enums.MinecraftServerType `json:"distribution"`
	Version      string                    `json:"version"`
	Build        string                    `json:"build,omitempty"`
	Stable       bool                      `json:"stable"`
	JavaMajor    int                       `json:"javaMajor"`
}

// ServerDistribution describes one dynamically registered server type and its installation capabilities.
type ServerDistribution struct {
	Type           enums.MinecraftServerType `json:"type"`
	DisplayName    string                    `json:"displayName"`
	Proxy          bool                      `json:"proxy"`
	CatalogReady   bool                      `json:"catalogReady"`
	InstallerReady bool                      `json:"installerReady"`
}

// ServerArtifact is the immutable provider-neutral download selected for installation.
type ServerArtifact struct {
	Distribution enums.MinecraftServerType `json:"distribution"`
	GameVersion  string                    `json:"gameVersion"`
	Build        string                    `json:"build,omitempty"`
	FileName     string                    `json:"fileName"`
	URL          string                    `json:"url"`
	Size         int64                     `json:"size,omitempty"`
	SHA256       string                    `json:"sha256"`
	SourceRisk   string                    `json:"sourceRisk,omitempty"`
}

// InstallationPreparation contains resolved runtime and artifact inputs for a durable task.
type InstallationPreparation struct {
	Server    model.MinecraftServer `json:"server"`
	Artifact  ServerArtifact        `json:"artifact"`
	JavaMajor int                   `json:"javaMajor"`
}

// InstallationResult records actual installed artifact and first-start evidence.
type InstallationResult struct {
	ArtifactSHA256 string            `json:"artifactSHA256"`
	JarPath        string            `json:"jarPath"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// ServerCatalog isolates provider APIs from application and desktop layers.
type ServerCatalog interface {
	Distribution() enums.MinecraftServerType
	ResolveVersions(context.Context) ([]ServerVersion, error)
	ResolveArtifact(context.Context, string, string) (ServerArtifact, error)
}

// Installer keeps distribution-specific preparation, installation, and verification out of the application service.
type Installer interface {
	Distribution() enums.MinecraftServerType
	Prepare(context.Context, model.MinecraftServer, ServerArtifact) (InstallationPreparation, error)
	Install(context.Context, InstallationPreparation) (InstallationResult, error)
	Verify(context.Context, InstallationPreparation, InstallationResult) error
}
