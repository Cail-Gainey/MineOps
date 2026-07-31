package servercatalog

import (
	"context"
	"strings"

	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/port"
)

const bungeeDefaultBaseURL = "https://ci.md-5.net"

// BungeeCatalog 暴露官方 Jenkins 构件那套有意浮动的策略。
type BungeeCatalog struct{ sources SourceResolver }

// NewBungeeCatalog 创建 BungeeCord 的 Jenkins 构件适配器。
func NewBungeeCatalog(sources SourceResolver) *BungeeCatalog { return &BungeeCatalog{sources: sources} }

// Distribution 返回 BungeeCord。
func (c *BungeeCatalog) Distribution() enums.MinecraftServerType { return enums.ServerBungee }

// ResolveVersions 返回唯一受支持的浮动最新版策略。
func (c *BungeeCatalog) ResolveVersions(context.Context) ([]port.ServerVersion, error) {
	return []port.ServerVersion{{
		Distribution: enums.ServerBungee, Version: "latest", Build: "lastSuccessfulBuild", Stable: true, JavaMajor: 17,
	}}, nil
}

// ResolveArtifact 返回官方 Jenkins 引导 Jar,并显式标注无法锁版本的风险。
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
