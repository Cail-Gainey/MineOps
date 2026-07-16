// Package jdkcatalog implements approved external JDK catalog adapters.
package jdkcatalog

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/infrastructure/httpclient"
	"github.com/Cail-Gainey/MineOps/internal/port"
)

const adoptiumDefaultBaseURL = "https://api.adoptium.net"

type sourceResolver interface {
	ResolveSource(string, string) string
}

// AdoptiumCatalog resolves Eclipse Temurin Linux JDK artifacts from the Adoptium v3 API.
type AdoptiumCatalog struct {
	client  *httpclient.Client
	sources sourceResolver
}

// NewAdoptiumCatalog creates a catalog with bounded connection and response timeouts.
func NewAdoptiumCatalog(client *httpclient.Client, sources sourceResolver) *AdoptiumCatalog {
	return &AdoptiumCatalog{client: client, sources: sources}
}

// List returns approved latest HotSpot JDK artifacts for one Java major and Linux architecture.
func (c *AdoptiumCatalog) List(ctx context.Context, majorVersion int, architecture, operatingSystem string) ([]port.JDKArtifact, error) {
	if majorVersion != 8 && majorVersion != 11 && majorVersion != 17 && majorVersion != 21 && majorVersion != 24 && majorVersion != 25 {
		return nil, apperror.New(apperror.CodeValidationInvalidArgument, "不支持的 Java Catalog 主版本")
	}
	architecture = normalizeAdoptiumArchitecture(architecture)
	operatingSystem = strings.ToLower(strings.TrimSpace(operatingSystem))
	if operatingSystem != "linux" || architecture == "" {
		return nil, apperror.New(apperror.CodeValidationInvalidArgument, "Java Catalog 当前仅支持 Linux x64/aarch64")
	}
	query := url.Values{
		"architecture": {architecture}, "image_type": {"jdk"}, "jvm_impl": {"hotspot"},
		"os": {operatingSystem}, "vendor": {"eclipse"},
	}
	baseURL := adoptiumDefaultBaseURL
	if c.sources != nil {
		baseURL = c.sources.ResolveSource("adoptium", baseURL)
	}
	endpoint := fmt.Sprintf("%s/v3/assets/latest/%d/hotspot?%s", strings.TrimRight(baseURL, "/"), majorVersion, query.Encode())
	var assets []adoptiumAsset
	if c == nil || c.client == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Adoptium HTTP Client 不能为空")
	}
	if err := c.client.GetJSON(ctx, endpoint, &assets); err != nil {
		return nil, err
	}
	result := make([]port.JDKArtifact, 0, len(assets))
	for _, asset := range assets {
		binary := asset.Binary.Package
		if binary.Link == "" || binary.Checksum == "" || binary.Size <= 0 {
			continue
		}
		version := asset.ReleaseName
		if asset.VersionData.Semver != "" {
			version = asset.VersionData.Semver
		}
		result = append(result, port.JDKArtifact{
			Version: version, MajorVersion: majorVersion, Vendor: "Eclipse Adoptium",
			Architecture: architecture, OperatingSystem: operatingSystem,
			ArchiveType: archiveType(binary.Name), URL: binary.Link, Size: binary.Size,
			SHA256: strings.ToLower(binary.Checksum),
		})
	}
	if len(result) == 0 {
		return nil, apperror.New(apperror.CodeIONotFound, "Adoptium Catalog 没有可用 JDK Artifact")
	}
	return result, nil
}

// Resolve returns the preferred first artifact from the approved catalog result.
func (c *AdoptiumCatalog) Resolve(ctx context.Context, majorVersion int, architecture, operatingSystem string) (port.JDKArtifact, error) {
	artifacts, err := c.List(ctx, majorVersion, architecture, operatingSystem)
	if err != nil {
		return port.JDKArtifact{}, err
	}
	return artifacts[0], nil
}

type adoptiumAsset struct {
	ReleaseName string `json:"release_name"`
	VersionData struct {
		Semver string `json:"semver"`
	} `json:"version_data"`
	Binary struct {
		Package struct {
			Checksum string `json:"checksum"`
			Link     string `json:"link"`
			Name     string `json:"name"`
			Size     int64  `json:"size"`
		} `json:"package"`
	} `json:"binary"`
}

func normalizeAdoptiumArchitecture(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "x64", "x86_64", "amd64":
		return "x64"
	case "aarch64", "arm64":
		return "aarch64"
	default:
		return ""
	}
}

func archiveType(name string) string {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".tar.gz") {
		return "tar.gz"
	}
	if strings.HasSuffix(lower, ".zip") {
		return "zip"
	}
	return "unknown"
}
