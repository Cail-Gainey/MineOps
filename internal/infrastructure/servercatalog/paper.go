package servercatalog

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/infrastructure/httpclient"
	"github.com/Cail-Gainey/MineOps/internal/port"
)

const paperDefaultBaseURL = "https://fill.papermc.io"

// PaperCatalog 从 PaperMC API 解析 Paper、Folia、Velocity 或 Waterfall 的构建。
type PaperCatalog struct {
	client       *httpclient.Client
	sources      SourceResolver
	distribution enums.MinecraftServerType
	project      string
}

// NewPaperCatalog 为 Paper、Folia、Velocity 或 Waterfall 创建 PaperMC 项目适配器。
func NewPaperCatalog(client *httpclient.Client, sources SourceResolver, distribution enums.MinecraftServerType) *PaperCatalog {
	project := distribution.String()
	return &PaperCatalog{client: client, sources: sources, distribution: distribution, project: project}
}

// Distribution 返回已配置的 PaperMC 发行版。
func (c *PaperCatalog) Distribution() enums.MinecraftServerType { return c.distribution }

// ResolveVersions 为项目的每个版本返回最新可用构建。
func (c *PaperCatalog) ResolveVersions(ctx context.Context) ([]port.ServerVersion, error) {
	if c == nil || c.client == nil || c.distribution != enums.ServerPaper && c.distribution != enums.ServerFolia && c.distribution != enums.ServerVelocity && c.distribution != enums.ServerWaterfall {
		return nil, apperror.New(apperror.CodeValidationInvalidArgument, "PaperMC Catalog 配置无效")
	}
	var project struct {
		Versions []struct {
			Version struct {
				ID      string `json:"id"`
				Support struct {
					Status string `json:"status"`
				} `json:"support"`
				Java struct {
					Version struct {
						Minimum int `json:"minimum"`
					} `json:"version"`
				} `json:"java"`
			} `json:"version"`
			Builds []int `json:"builds"`
		} `json:"versions"`
	}
	apiBaseURL := c.apiBaseURL()
	if err := c.client.GetJSON(ctx, fmt.Sprintf("%s/%s/versions", apiBaseURL, url.PathEscape(c.project)), &project); err != nil {
		return nil, err
	}
	result := make([]port.ServerVersion, 0, len(project.Versions))
	for _, item := range project.Versions {
		if item.Version.ID == "" || len(item.Builds) == 0 {
			continue
		}
		latestBuild := item.Builds[0]
		for _, build := range item.Builds[1:] {
			if build > latestBuild {
				latestBuild = build
			}
		}
		javaMajor := item.Version.Java.Version.Minimum
		if javaMajor < 1 {
			javaMajor = javaMajorForMinecraft(item.Version.ID)
		}
		result = append(result, port.ServerVersion{
			Distribution: c.distribution, Version: item.Version.ID,
			Build: strconv.Itoa(latestBuild), Stable: strings.EqualFold(item.Version.Support.Status, "SUPPORTED"),
			JavaMajor: javaMajor,
		})
	}
	if len(result) == 0 {
		return nil, apperror.New(apperror.CodeIONotFound, "PaperMC Catalog 没有可用版本")
	}
	return result, nil
}

// ResolveArtifact 解析一个确切的 PaperMC 构建及其 SHA-256 元数据。
func (c *PaperCatalog) ResolveArtifact(ctx context.Context, version, build string) (port.ServerArtifact, error) {
	if strings.TrimSpace(build) == "" {
		versions, err := c.ResolveVersions(ctx)
		if err != nil {
			return port.ServerArtifact{}, err
		}
		for _, candidate := range versions {
			if candidate.Version == version {
				build = candidate.Build
				break
			}
		}
	}
	if build == "" {
		return port.ServerArtifact{}, apperror.New(apperror.CodeIONotFound, "PaperMC 版本没有可用 Build")
	}
	var metadata struct {
		Downloads map[string]struct {
			Name      string `json:"name"`
			Size      int64  `json:"size"`
			URL       string `json:"url"`
			Checksums struct {
				SHA256 string `json:"sha256"`
			} `json:"checksums"`
		} `json:"downloads"`
	}
	base := fmt.Sprintf("%s/%s/versions/%s/builds/%s", c.apiBaseURL(), url.PathEscape(c.project), url.PathEscape(version), url.PathEscape(build))
	if err := c.client.GetJSON(ctx, base, &metadata); err != nil {
		return port.ServerArtifact{}, err
	}
	download, found := metadata.Downloads["server:default"]
	if !found || download.Name == "" || download.Checksums.SHA256 == "" || download.URL == "" {
		return port.ServerArtifact{}, apperror.New(apperror.CodeIONotFound, "PaperMC Build 缺少 Artifact 或 SHA-256")
	}
	return port.ServerArtifact{
		Distribution: c.distribution, GameVersion: version, Build: build,
		FileName: download.Name, Size: download.Size, SHA256: strings.ToLower(download.Checksums.SHA256),
		URL: download.URL,
	}, nil
}

func (c *PaperCatalog) apiBaseURL() string {
	baseURL := paperDefaultBaseURL
	if c.sources != nil {
		baseURL = c.sources.ResolveSource("papermc", baseURL)
	}
	return strings.TrimRight(baseURL, "/") + "/v3/projects"
}
