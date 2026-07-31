package servercatalog

import (
	"context"
	"encoding/xml"
	"fmt"
	"sort"
	"strings"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/infrastructure/httpclient"
	"github.com/Cail-Gainey/MineOps/internal/port"
)

const forgeDefaultBaseURL = "https://maven.minecraftforge.net"
const neoForgeDefaultBaseURL = "https://maven.neoforged.net/releases"

// ForgeCatalog 从官方 Maven 元数据解析 Forge 或 NeoForge 安装器构件。
type ForgeCatalog struct {
	client       *httpclient.Client
	sources      SourceResolver
	distribution enums.MinecraftServerType
}

// NewForgeCatalog 创建官方 Forge 或 NeoForge 的 Maven 适配器。
func NewForgeCatalog(client *httpclient.Client, sources SourceResolver, distribution enums.MinecraftServerType) *ForgeCatalog {
	return &ForgeCatalog{client: client, sources: sources, distribution: distribution}
}

// Distribution 返回 Forge 或 NeoForge。
func (c *ForgeCatalog) Distribution() enums.MinecraftServerType { return c.distribution }

// ResolveVersions 为每个受支持的 Minecraft 版本返回最新可用的 Loader 构建。
func (c *ForgeCatalog) ResolveVersions(ctx context.Context) ([]port.ServerVersion, error) {
	if c == nil || c.client == nil || c.distribution != enums.ServerForge && c.distribution != enums.ServerNeoForge {
		return nil, apperror.New(apperror.CodeValidationInvalidArgument, "Forge Catalog 配置无效")
	}
	if c.distribution == enums.ServerForge {
		var metadata struct {
			Versioning struct {
				Versions []string `xml:"versions>version"`
			} `xml:"versioning"`
		}
		payload, err := c.client.GetText(ctx, c.baseURL()+"/net/minecraftforge/forge/maven-metadata.xml")
		if err != nil {
			return nil, err
		}
		if err := xml.Unmarshal([]byte(payload), &metadata); err != nil {
			return nil, apperror.Wrap(apperror.CodeIOReadFailed, "解析 Forge Maven Metadata 失败", err)
		}
		latest := make(map[string]string)
		for _, combined := range metadata.Versioning.Versions {
			gameVersion, build, found := strings.Cut(combined, "-")
			if !found || javaMajorForMinecraft(gameVersion) < 1 {
				continue
			}
			latest[gameVersion] = build
		}
		result := make([]port.ServerVersion, 0, len(latest))
		for gameVersion, build := range latest {
			result = append(result, port.ServerVersion{
				Distribution: enums.ServerForge, Version: gameVersion, Build: build,
				Stable: true, JavaMajor: javaMajorForMinecraft(gameVersion),
			})
		}
		sort.Slice(result, func(left, right int) bool { return result[left].Version > result[right].Version })
		return result, nil
	}
	var metadata struct {
		Versioning struct {
			Versions []string `xml:"versions>version"`
		} `xml:"versioning"`
	}
	payload, err := c.client.GetText(ctx, c.baseURL()+"/net/neoforged/neoforge/maven-metadata.xml")
	if err != nil {
		return nil, err
	}
	if err := xml.Unmarshal([]byte(payload), &metadata); err != nil {
		return nil, apperror.Wrap(apperror.CodeIOReadFailed, "解析 NeoForge Maven Metadata 失败", err)
	}
	latest := make(map[string]string)
	for _, build := range metadata.Versioning.Versions {
		gameVersion := neoForgeGameVersion(build)
		if gameVersion != "" && build > latest[gameVersion] {
			latest[gameVersion] = build
		}
	}
	result := make([]port.ServerVersion, 0, len(latest))
	for gameVersion, build := range latest {
		result = append(result, port.ServerVersion{
			Distribution: enums.ServerNeoForge, Version: gameVersion, Build: build,
			Stable: !strings.Contains(strings.ToLower(build), "beta"), JavaMajor: javaMajorForMinecraft(gameVersion),
		})
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Version > result[right].Version })
	return result, nil
}

// ResolveArtifact 返回一个确切的 Maven 安装器 Jar 及其公布的 SHA-256。
func (c *ForgeCatalog) ResolveArtifact(ctx context.Context, version, build string) (port.ServerArtifact, error) {
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
		return port.ServerArtifact{}, apperror.New(apperror.CodeIONotFound, "Forge 版本没有可用 Installer")
	}
	var artifactURL, fileName string
	if c.distribution == enums.ServerForge {
		combined := build
		if !strings.HasPrefix(combined, version+"-") {
			combined = version + "-" + build
		}
		fileName = "forge-" + combined + "-installer.jar"
		artifactURL = fmt.Sprintf("%s/net/minecraftforge/forge/%s/%s", c.baseURL(), combined, fileName)
	} else {
		fileName = "neoforge-" + build + "-installer.jar"
		artifactURL = fmt.Sprintf("%s/net/neoforged/neoforge/%s/%s", c.baseURL(), build, fileName)
	}
	checksum, err := c.client.GetText(ctx, artifactURL+".sha256")
	if err != nil {
		return port.ServerArtifact{}, err
	}
	checksumFields := strings.Fields(checksum)
	if len(checksumFields) == 0 {
		return port.ServerArtifact{}, apperror.New(apperror.CodeArtifactChecksumMismatch, "Forge Maven SHA-256 响应为空")
	}
	checksum = strings.ToLower(checksumFields[0])
	if len(checksum) != 64 {
		return port.ServerArtifact{}, apperror.New(apperror.CodeArtifactChecksumMismatch, "Forge Maven SHA-256 元数据无效")
	}
	return port.ServerArtifact{
		Distribution: c.distribution, GameVersion: version, Build: build,
		FileName: fileName, URL: artifactURL, SHA256: checksum,
	}, nil
}

func (c *ForgeCatalog) baseURL() string {
	provider, fallback := "forge", forgeDefaultBaseURL
	if c.distribution == enums.ServerNeoForge {
		provider, fallback = "neoforge", neoForgeDefaultBaseURL
	}
	if c.sources != nil {
		fallback = c.sources.ResolveSource(provider, fallback)
	}
	return strings.TrimRight(fallback, "/")
}

func neoForgeGameVersion(build string) string {
	parts := strings.Split(build, ".")
	if len(parts) < 2 {
		return ""
	}
	major := strings.TrimLeft(parts[0], "0")
	patch := strings.TrimLeft(parts[1], "0")
	if major == "" || patch == "" {
		return ""
	}
	return "1." + major + "." + patch
}
