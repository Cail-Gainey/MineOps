package servercatalog

import (
	"context"
	"strings"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/infrastructure/httpclient"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/port"
)

const mojangBaseURL = "https://piston-meta.mojang.com"

// MojangCatalog 只解析正式发行版的 Vanilla 服务端构件。
type MojangCatalog struct {
	client  *httpclient.Client
	sources SourceResolver
}

// NewMojangCatalog 创建官方 Mojang 目录适配器。
func NewMojangCatalog(client *httpclient.Client, sources SourceResolver) *MojangCatalog {
	return &MojangCatalog{client: client, sources: sources}
}

// Distribution 返回 Vanilla。
func (c *MojangCatalog) Distribution() enums.MinecraftServerType { return enums.ServerVanilla }

// ResolveVersions 只返回正式发行版本,并保留供应方的排序。
func (c *MojangCatalog) ResolveVersions(ctx context.Context) ([]port.ServerVersion, error) {
	manifest, err := c.manifest(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]port.ServerVersion, 0, len(manifest.Versions))
	for _, version := range manifest.Versions {
		if version.Type == "release" {
			result = append(result, port.ServerVersion{
				Distribution: enums.ServerVanilla, Version: version.ID, Stable: true, JavaMajor: javaMajorForMinecraft(version.ID),
			})
		}
	}
	if len(result) == 0 {
		return nil, apperror.New(apperror.CodeIONotFound, "Mojang Catalog 未返回 release 版本")
	}
	return result, nil
}

// ResolveArtifact 解析某个发行版对应的官方服务端 Jar 元数据。
func (c *MojangCatalog) ResolveArtifact(ctx context.Context, version, _ string) (port.ServerArtifact, error) {
	manifest, err := c.manifest(ctx)
	if err != nil {
		return port.ServerArtifact{}, err
	}
	metadataURL := ""
	for _, item := range manifest.Versions {
		if item.ID == strings.TrimSpace(version) && item.Type == "release" {
			metadataURL = item.URL
			break
		}
	}
	if metadataURL == "" {
		return port.ServerArtifact{}, apperror.New(apperror.CodeIONotFound, "Mojang release 版本不存在")
	}
	var metadata struct {
		Downloads struct {
			Server struct {
				URL  string `json:"url"`
				Size int64  `json:"size"`
				SHA1 string `json:"sha1"`
			} `json:"server"`
		} `json:"downloads"`
	}
	if err := c.client.GetJSON(ctx, metadataURL, &metadata); err != nil {
		return port.ServerArtifact{}, err
	}
	if metadata.Downloads.Server.URL == "" {
		return port.ServerArtifact{}, apperror.New(apperror.CodeIONotFound, "Mojang 版本缺少 Server Artifact")
	}
	return port.ServerArtifact{
		Distribution: enums.ServerVanilla, GameVersion: version, FileName: "server.jar",
		URL: metadata.Downloads.Server.URL, Size: metadata.Downloads.Server.Size,
		SourceRisk: "Mojang 仅提供 SHA-1；MineOps 下载后记录实际 SHA-256",
	}, nil
}

func (c *MojangCatalog) manifest(ctx context.Context) (struct {
	Versions []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		URL  string `json:"url"`
	} `json:"versions"`
}, error) {
	var manifest struct {
		Versions []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"versions"`
	}
	if c == nil || c.client == nil {
		return manifest, apperror.New(apperror.CodeValidationRequired, "Mojang HTTP Client 不能为空")
	}
	baseURL := mojangBaseURL
	if c.sources != nil {
		baseURL = c.sources.ResolveSource("mojang", baseURL)
	}
	return manifest, c.client.GetJSON(ctx, strings.TrimRight(baseURL, "/")+"/mc/game/version_manifest_v2.json", &manifest)
}

func javaMajorForMinecraft(version string) int {
	requirement, err := model.JavaRequirementForMinecraft("", version)
	if err != nil {
		return 0
	}
	return requirement.PreferredMajor
}
