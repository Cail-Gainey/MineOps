package servercatalog

import (
	"context"
	"strings"

	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/port"
)

const bungeeDefaultBaseURL = "https://ci.md-5.net"

// BungeeCatalog exposes the intentionally floating official Jenkins artifact strategy.
type BungeeCatalog struct{ sources SourceResolver }

// NewBungeeCatalog creates the BungeeCord Jenkins artifact adapter.
func NewBungeeCatalog(sources SourceResolver) *BungeeCatalog { return &BungeeCatalog{sources: sources} }

// Distribution returns BungeeCord.
func (c *BungeeCatalog) Distribution() enums.MinecraftServerType { return enums.ServerBungee }

// ResolveVersions returns the only supported floating latest strategy.
func (c *BungeeCatalog) ResolveVersions(context.Context) ([]port.ServerVersion, error) {
	return []port.ServerVersion{{
		Distribution: enums.ServerBungee, Version: "latest", Build: "lastSuccessfulBuild", Stable: true, JavaMajor: 17,
	}}, nil
}

// ResolveArtifact returns the official Jenkins bootstrap Jar and explicit lock-version risk.
func (c *BungeeCatalog) ResolveArtifact(context.Context, string, string) (port.ServerArtifact, error) {
	baseURL := bungeeDefaultBaseURL
	if c.sources != nil {
		baseURL = c.sources.ResolveSource("bungeecord", baseURL)
	}
	return port.ServerArtifact{
		Distribution: enums.ServerBungee, GameVersion: "latest", Build: "lastSuccessfulBuild",
		FileName:   "BungeeCord.jar",
		URL:        strings.TrimRight(baseURL, "/") + "/job/BungeeCord/lastSuccessfulBuild/artifact/bootstrap/target/BungeeCord.jar",
		SourceRisk: "BungeeCord Jenkins latest 不可精确锁版；MineOps 保存实际 Artifact SHA-256",
	}, nil
}
