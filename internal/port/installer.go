package port

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

// ServerVersion 是服务端发行版目录暴露的、与供应方无关的一个版本。
type ServerVersion struct {
	Distribution enums.MinecraftServerType `json:"distribution"`
	Version      string                    `json:"version"`
	Build        string                    `json:"build,omitempty"`
	Stable       bool                      `json:"stable"`
	JavaMajor    int                       `json:"javaMajor"`
}

// ServerDistribution 描述一个动态注册的服务端类型及其安装能力。
type ServerDistribution struct {
	Type           enums.MinecraftServerType `json:"type"`
	DisplayName    string                    `json:"displayName"`
	Proxy          bool                      `json:"proxy"`
	CatalogReady   bool                      `json:"catalogReady"`
	InstallerReady bool                      `json:"installerReady"`
}

// ServerArtifact 是为安装选定的、不可变且与供应方无关的下载构件。
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

// InstallationPreparation 承载持久化任务已解析的运行时与构件输入。
type InstallationPreparation struct {
	Server    model.MinecraftServer `json:"server"`
	Artifact  ServerArtifact        `json:"artifact"`
	JavaMajor int                   `json:"javaMajor"`
}

// InstallationResult 记录实际安装的构件与首次启动证据。
type InstallationResult struct {
	ArtifactSHA256 string            `json:"artifactSHA256"`
	JarPath        string            `json:"jarPath"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// ServerCatalog 把供应方 API 与应用层、桌面层隔离开。
type ServerCatalog interface {
	Distribution() enums.MinecraftServerType
	ResolveVersions(context.Context) ([]ServerVersion, error)
	ResolveArtifact(context.Context, string, string) (ServerArtifact, error)
}

// Installer 把发行版专属的准备、安装与校验挡在应用服务之外。
type Installer interface {
	Distribution() enums.MinecraftServerType
	Prepare(context.Context, model.MinecraftServer, ServerArtifact) (InstallationPreparation, error)
	Install(context.Context, InstallationPreparation) (InstallationResult, error)
	Verify(context.Context, InstallationPreparation, InstallationResult) error
}
