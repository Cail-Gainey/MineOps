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

const quiltDefaultBaseURL = "https://meta.quiltmc.org"

// QuiltCatalog resolves stable Quilt loader and installer combinations.
type QuiltCatalog struct {
	client  *httpclient.Client
	sources SourceResolver
}

// NewQuiltCatalog creates the official Quilt Meta adapter.
func NewQuiltCatalog(client *httpclient.Client, sources SourceResolver) *QuiltCatalog {
	return &QuiltCatalog{client: client, sources: sources}
}

// Distribution returns Quilt.
func (c *QuiltCatalog) Distribution() enums.MinecraftServerType { return enums.ServerQuilt }

// ResolveVersions returns stable Minecraft versions with the preferred loader version.
func (c *QuiltCatalog) ResolveVersions(ctx context.Context) ([]port.ServerVersion, error) {
	if c == nil || c.client == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Quilt HTTP Client 不能为空")
	}
	var games []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}
	if err := c.client.GetJSON(ctx, c.metaBaseURL()+"/game", &games); err != nil {
		return nil, err
	}
	loader, err := c.latestVersion(ctx, "/loader")
	if err != nil {
		return nil, err
	}
	result := make([]port.ServerVersion, 0, len(games))
	for _, game := range games {
		if game.Stable {
			result = append(result, port.ServerVersion{
				Distribution: enums.ServerQuilt, Version: game.Version, Build: loader,
				Stable: true, JavaMajor: javaMajorForMinecraft(game.Version),
			})
		}
	}
	if len(result) == 0 {
		return nil, apperror.New(apperror.CodeIONotFound, "Quilt Meta 没有稳定版本组合")
	}
	return result, nil
}

// ResolveArtifact returns the Quilt server launcher endpoint for an exact combination.
func (c *QuiltCatalog) ResolveArtifact(ctx context.Context, version, loader string) (port.ServerArtifact, error) {
	if strings.TrimSpace(loader) == "" {
		var err error
		loader, err = c.latestVersion(ctx, "/loader")
		if err != nil {
			return port.ServerArtifact{}, err
		}
	}
	installer, err := c.latestVersion(ctx, "/installer")
	if err != nil {
		return port.ServerArtifact{}, err
	}
	return port.ServerArtifact{
		Distribution: enums.ServerQuilt, GameVersion: version, Build: loader,
		FileName:   fmt.Sprintf("quilt-server-%s-%s-%s.jar", version, loader, installer),
		URL:        fmt.Sprintf("%s/loader/%s/%s/%s/server/jar", c.metaBaseURL(), url.PathEscape(version), url.PathEscape(loader), url.PathEscape(installer)),
		SourceRisk: "Quilt Meta Server Launcher 未提供 SHA-256；MineOps 下载后记录实际 SHA-256",
	}, nil
}

func (c *QuiltCatalog) latestVersion(ctx context.Context, endpoint string) (string, error) {
	var versions []struct {
		Version string `json:"version"`
	}
	if err := c.client.GetJSON(ctx, c.metaBaseURL()+endpoint, &versions); err != nil {
		return "", err
	}
	for _, version := range versions {
		if version.Version != "" {
			return version.Version, nil
		}
	}
	return "", apperror.New(apperror.CodeIONotFound, "Quilt Meta 版本列表为空")
}

func (c *QuiltCatalog) metaBaseURL() string {
	baseURL := quiltDefaultBaseURL
	if c.sources != nil {
		baseURL = c.sources.ResolveSource("quilt", baseURL)
	}
	return strings.TrimRight(baseURL, "/") + "/v3/versions"
}
