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

const fabricDefaultBaseURL = "https://meta.fabricmc.net"

// FabricCatalog resolves stable game, loader, and installer combinations.
type FabricCatalog struct {
	client  *httpclient.Client
	sources SourceResolver
}

// NewFabricCatalog creates the official Fabric Meta adapter.
func NewFabricCatalog(client *httpclient.Client, sources SourceResolver) *FabricCatalog {
	return &FabricCatalog{client: client, sources: sources}
}

// Distribution returns Fabric.
func (c *FabricCatalog) Distribution() enums.MinecraftServerType { return enums.ServerFabric }

// ResolveVersions returns stable Minecraft versions and defers Loader resolution until installation.
func (c *FabricCatalog) ResolveVersions(ctx context.Context) ([]port.ServerVersion, error) {
	if c == nil || c.client == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Fabric HTTP Client 不能为空")
	}
	var games []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}
	if err := c.client.GetJSON(ctx, c.metaBaseURL()+"/game", &games); err != nil {
		return nil, err
	}
	result := make([]port.ServerVersion, 0, len(games))
	for _, game := range games {
		if !game.Stable {
			continue
		}
		result = append(result, port.ServerVersion{
			Distribution: enums.ServerFabric, Version: game.Version,
			Stable: true, JavaMajor: javaMajorForMinecraft(game.Version),
		})
	}
	if len(result) == 0 {
		return nil, apperror.New(apperror.CodeIONotFound, "Fabric Meta 没有稳定版本组合")
	}
	return result, nil
}

// ResolveArtifact returns the stable Fabric server launcher combination.
func (c *FabricCatalog) ResolveArtifact(ctx context.Context, version, loader string) (port.ServerArtifact, error) {
	if strings.TrimSpace(loader) == "" {
		var err error
		loader, err = c.latestLoader(ctx, version)
		if err != nil {
			return port.ServerArtifact{}, err
		}
	}
	installer, err := c.latestInstaller(ctx)
	if err != nil {
		return port.ServerArtifact{}, err
	}
	return port.ServerArtifact{
		Distribution: enums.ServerFabric, GameVersion: version, Build: loader,
		FileName:   fmt.Sprintf("fabric-server-%s-%s-%s.jar", version, loader, installer),
		URL:        fmt.Sprintf("%s/loader/%s/%s/%s/server/jar", c.metaBaseURL(), url.PathEscape(version), url.PathEscape(loader), url.PathEscape(installer)),
		SourceRisk: "Fabric Meta Server Launcher 未提供 SHA-256；MineOps 下载后记录实际 SHA-256",
	}, nil
}

func (c *FabricCatalog) latestLoader(ctx context.Context, version string) (string, error) {
	var combinations []struct {
		Loader struct {
			Version string `json:"version"`
			Stable  bool   `json:"stable"`
		} `json:"loader"`
	}
	if err := c.client.GetJSON(ctx, c.metaBaseURL()+"/loader/"+url.PathEscape(version), &combinations); err != nil {
		return "", err
	}
	for _, combination := range combinations {
		if combination.Loader.Stable && combination.Loader.Version != "" {
			return combination.Loader.Version, nil
		}
	}
	return "", apperror.New(apperror.CodeIONotFound, "Fabric 版本没有稳定 Loader")
}

func (c *FabricCatalog) latestInstaller(ctx context.Context) (string, error) {
	var installers []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}
	if err := c.client.GetJSON(ctx, c.metaBaseURL()+"/installer", &installers); err != nil {
		return "", err
	}
	for _, installer := range installers {
		if installer.Stable && installer.Version != "" {
			return installer.Version, nil
		}
	}
	return "", apperror.New(apperror.CodeIONotFound, "Fabric Meta 没有稳定 Installer")
}

func (c *FabricCatalog) metaBaseURL() string {
	baseURL := fabricDefaultBaseURL
	if c.sources != nil {
		baseURL = c.sources.ResolveSource("fabric", baseURL)
	}
	return strings.TrimRight(baseURL, "/") + "/v2/versions"
}
