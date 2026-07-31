package servercatalog

import (
	"context"
	"strings"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/infrastructure/httpclient"
	"github.com/Cail-Gainey/MineOps/internal/port"
)

const spigotDefaultBaseURL = "https://hub.spigotmc.org"

// SpigotCatalog 解析 Mojang 的发行修订版与官方 BuildTools 构件。
type SpigotCatalog struct {
	client  *httpclient.Client
	sources SourceResolver
}

// NewSpigotCatalog 创建基于 BuildTools 的 Spigot 目录。
func NewSpigotCatalog(client *httpclient.Client, sources SourceResolver) *SpigotCatalog {
	return &SpigotCatalog{client: client, sources: sources}
}

// Distribution 返回 Spigot。
func (c *SpigotCatalog) Distribution() enums.MinecraftServerType { return enums.ServerSpigot }

// ResolveVersions 返回 BuildTools 接受的发行修订版。
func (c *SpigotCatalog) ResolveVersions(ctx context.Context) ([]port.ServerVersion, error) {
	mojang := NewMojangCatalog(c.client, c.sources)
	versions, err := mojang.ResolveVersions(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]port.ServerVersion, len(versions))
	for index, version := range versions {
		result[index] = port.ServerVersion{
			Distribution: enums.ServerSpigot, Version: version.Version,
			Build: "BuildTools-lastSuccessful", Stable: true, JavaMajor: version.JavaMajor,
		}
	}
	return result, nil
}

// ResolveArtifact 返回官方浮动的 BuildTools Jar,并记录该来源无法锁定的风险。
func (c *SpigotCatalog) ResolveArtifact(_ context.Context, version, _ string) (port.ServerArtifact, error) {
	if strings.TrimSpace(version) == "" {
		return port.ServerArtifact{}, apperror.New(apperror.CodeValidationRequired, "Spigot Minecraft 版本不能为空")
	}
	baseURL := spigotDefaultBaseURL
	if c.sources != nil {
		baseURL = c.sources.ResolveSource("spigot", baseURL)
	}
	return port.ServerArtifact{
		Distribution: enums.ServerSpigot, GameVersion: version, Build: "lastSuccessfulBuild",
		FileName:   "BuildTools.jar",
		URL:        strings.TrimRight(baseURL, "/") + "/jenkins/job/BuildTools/lastSuccessfulBuild/artifact/target/BuildTools.jar",
		SourceRisk: "BuildTools lastSuccessfulBuild 不可精确锁定；MineOps 保存实际 SHA-256 与构建日志",
	}, nil
}
