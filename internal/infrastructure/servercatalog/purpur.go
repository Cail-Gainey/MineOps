package servercatalog

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/infrastructure/httpclient"
	"github.com/Cail-Gainey/MineOps/internal/port"
)

const purpurDefaultBaseURL = "https://api.purpurmc.org"

// PurpurCatalog resolves Purpur versions and exact builds.
type PurpurCatalog struct {
	client  *httpclient.Client
	sources SourceResolver
}

// NewPurpurCatalog creates the official Purpur catalog adapter.
func NewPurpurCatalog(client *httpclient.Client, sources SourceResolver) *PurpurCatalog {
	return &PurpurCatalog{client: client, sources: sources}
}

// Distribution returns Purpur.
func (c *PurpurCatalog) Distribution() enums.MinecraftServerType { return enums.ServerPurpur }

// ResolveVersions returns Purpur game versions without resolving every version's build eagerly.
func (c *PurpurCatalog) ResolveVersions(ctx context.Context) ([]port.ServerVersion, error) {
	var project struct {
		Versions []string `json:"versions"`
	}
	if c == nil || c.client == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Purpur HTTP Client 不能为空")
	}
	apiBaseURL := c.apiBaseURL()
	if err := c.client.GetJSON(ctx, apiBaseURL, &project); err != nil {
		return nil, err
	}
	result := make([]port.ServerVersion, 0, len(project.Versions))
	for index := len(project.Versions) - 1; index >= 0; index-- {
		version := project.Versions[index]
		if strings.TrimSpace(version) == "" {
			continue
		}
		result = append(result, port.ServerVersion{
			Distribution: enums.ServerPurpur, Version: version,
			Stable: true, JavaMajor: javaMajorForMinecraft(version),
		})
	}
	if len(result) == 0 {
		return nil, apperror.New(apperror.CodeIONotFound, "Purpur Catalog 没有可用版本")
	}
	return result, nil
}

// ResolveArtifact returns the exact Purpur build download; actual SHA-256 is recorded after download.
func (c *PurpurCatalog) ResolveArtifact(ctx context.Context, version, build string) (port.ServerArtifact, error) {
	if strings.TrimSpace(build) == "" {
		var metadata struct {
			Builds struct {
				Latest string `json:"latest"`
			} `json:"builds"`
		}
		endpoint := fmt.Sprintf("%s/%s", c.apiBaseURL(), url.PathEscape(version))
		if err := c.client.GetJSON(ctx, endpoint, &metadata); err != nil {
			return port.ServerArtifact{}, err
		}
		build = metadata.Builds.Latest
	}
	if build == "" {
		return port.ServerArtifact{}, apperror.New(apperror.CodeIONotFound, "Purpur 版本没有可用 Build")
	}
	return port.ServerArtifact{
		Distribution: enums.ServerPurpur, GameVersion: version, Build: build,
		FileName:   fmt.Sprintf("purpur-%s-%s.jar", version, build),
		URL:        fmt.Sprintf("%s/%s/%s/download", c.apiBaseURL(), url.PathEscape(version), url.PathEscape(build)),
		SourceRisk: "Purpur API 未提供 SHA-256；MineOps 下载后记录实际 SHA-256",
	}, nil
}

func (c *PurpurCatalog) apiBaseURL() string {
	baseURL := purpurDefaultBaseURL
	if c.sources != nil {
		baseURL = c.sources.ResolveSource("purpur", baseURL)
	}
	return strings.TrimRight(baseURL, "/") + "/v2/purpur"
}
