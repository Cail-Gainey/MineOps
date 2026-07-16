package minecraftspark

import (
	"context"
	"encoding/hex"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/infrastructure/httpclient"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

const (
	modrinthAPIBaseURL = "https://api.modrinth.com/v2"
	sparkProjectID     = "l6YH9Als"
)

// Catalog resolves dynamic Minecraft spark releases while preserving fixed layouts for other platforms.
type Catalog struct {
	client  *httpclient.Client
	baseURL string
}

// NewCatalog creates the approved Minecraft spark release catalog.
func NewCatalog(client *httpclient.Client) *Catalog {
	return &Catalog{client: client, baseURL: modrinthAPIBaseURL}
}

// ResolveArtifact selects one exact Spark artifact for the Server platform and Minecraft version.
func (c *Catalog) ResolveArtifact(ctx context.Context, serverType enums.MinecraftServerType, minecraftVersion string, javaMajor int) (Artifact, error) {
	loader := modrinthLoader(serverType)
	if loader == "" {
		return ArtifactForServer(serverType, minecraftVersion, javaMajor)
	}
	if c == nil || c.client == nil {
		return Artifact{}, apperror.New(apperror.CodeValidationRequired, "Spark Catalog HTTP Client 不能为空")
	}
	minecraftVersion = strings.TrimSpace(minecraftVersion)
	if minecraftVersion == "" {
		return Artifact{}, apperror.New(apperror.CodeValidationRequired, "Minecraft 版本不能为空")
	}
	query := url.Values{}
	query.Set("loaders", `[`+strconv.Quote(loader)+`]`)
	query.Set("game_versions", `[`+strconv.Quote(minecraftVersion)+`]`)
	endpoint := strings.TrimRight(c.baseURL, "/") + "/project/" + sparkProjectID + "/version?" + query.Encode()
	var releases []modrinthVersion
	if err := c.client.GetJSON(ctx, endpoint, &releases); err != nil {
		return Artifact{}, err
	}
	sort.SliceStable(releases, func(left, right int) bool {
		return releases[left].DatePublished.After(releases[right].DatePublished)
	})
	for _, release := range releases {
		if release.VersionType != "release" || !containsExact(release.Loaders, loader) || !containsExact(release.GameVersions, minecraftVersion) {
			continue
		}
		requirement, requirementErr := model.JavaRequirementForMinecraft(serverType.String(), minecraftVersion)
		if requirementErr != nil {
			return Artifact{}, requirementErr
		}
		artifact, err := release.modArtifact(loader, requirement.MinimumMajor)
		if err != nil {
			return Artifact{}, err
		}
		if !SupportsPluginVersion(artifact.Version) {
			continue
		}
		if javaMajor > 0 && artifact.RequiredJavaMajor > javaMajor {
			continue
		}
		return artifact, nil
	}
	return Artifact{}, apperror.New(apperror.CodeIONotFound, "Modrinth 没有与当前 Minecraft 和 Java 版本兼容的稳定 Spark 发布").WithDetails(map[string]any{
		"minecraftVersion": minecraftVersion, "loader": loader, "javaMajor": javaMajor,
	})
}

type modrinthVersion struct {
	ID            string               `json:"id"`
	VersionNumber string               `json:"version_number"`
	VersionType   string               `json:"version_type"`
	DatePublished time.Time            `json:"date_published"`
	GameVersions  []string             `json:"game_versions"`
	Loaders       []string             `json:"loaders"`
	Files         []modrinthFile       `json:"files"`
	Dependencies  []modrinthDependency `json:"dependencies"`
}

type modrinthFile struct {
	URL      string            `json:"url"`
	Filename string            `json:"filename"`
	Primary  bool              `json:"primary"`
	Size     int64             `json:"size"`
	Hashes   map[string]string `json:"hashes"`
}

type modrinthDependency struct {
	DependencyType string `json:"dependency_type"`
	ProjectID      string `json:"project_id"`
	VersionID      string `json:"version_id"`
}

func (r modrinthVersion) modArtifact(loader string, requiredJavaMajor int) (Artifact, error) {
	if strings.TrimSpace(r.ID) == "" {
		return Artifact{}, apperror.New(apperror.CodeValidationInvalidArgument, "Modrinth Spark 发布缺少 Version ID")
	}
	for _, dependency := range r.Dependencies {
		if dependency.DependencyType == "required" {
			return Artifact{}, apperror.New(apperror.CodeSparkUnsupported, "Spark 发布声明了 MineOps 尚未支持的必需依赖").WithDetails(map[string]any{
				"releaseID": r.ID, "projectID": dependency.ProjectID, "versionID": dependency.VersionID,
			})
		}
	}
	primaryFiles := make([]modrinthFile, 0, 1)
	for _, file := range r.Files {
		if file.Primary {
			primaryFiles = append(primaryFiles, file)
		}
	}
	if len(primaryFiles) != 1 {
		return Artifact{}, apperror.New(apperror.CodeValidationConflict, "Modrinth Spark 发布必须且只能包含一个 primary 文件").WithDetails(map[string]any{
			"releaseID": r.ID, "primaryFiles": len(primaryFiles),
		})
	}
	file := primaryFiles[0]
	parsedURL, err := url.Parse(strings.TrimSpace(file.URL))
	if err != nil || parsedURL.Scheme != "https" || !strings.EqualFold(parsedURL.Hostname(), "cdn.modrinth.com") || parsedURL.User != nil {
		return Artifact{}, apperror.Wrap(apperror.CodeValidationInvalidArgument, "Modrinth Spark primary 文件 URL 无效", err)
	}
	if !strings.HasPrefix(parsedURL.Path, "/data/"+sparkProjectID+"/versions/"+r.ID+"/") || file.Filename == "" || path.Base(parsedURL.Path) != file.Filename || !strings.HasSuffix(strings.ToLower(file.Filename), ".jar") || file.Size <= 0 || file.Size > 64*1024*1024 {
		return Artifact{}, apperror.New(apperror.CodeValidationInvalidArgument, "Modrinth Spark primary 文件身份或大小无效")
	}
	sha512 := strings.ToLower(strings.TrimSpace(file.Hashes["sha512"]))
	if len(sha512) != 128 {
		return Artifact{}, apperror.New(apperror.CodeValidationInvalidArgument, "Modrinth Spark primary 文件缺少 SHA-512")
	}
	if _, err := hex.DecodeString(sha512); err != nil {
		return Artifact{}, apperror.Wrap(apperror.CodeValidationInvalidArgument, "Modrinth Spark SHA-512 无效", err)
	}
	version := strings.TrimSpace(r.VersionNumber)
	if separator := strings.IndexByte(version, '-'); separator > 0 {
		version = version[:separator]
	}
	return Artifact{
		Platform: loader, Provider: "spark-modrinth", ReleaseID: r.ID, Version: version,
		FileName: file.Filename, URL: parsedURL.String(), SHA512: sha512, Size: file.Size, TargetKind: "mods", RequiredJavaMajor: requiredJavaMajor,
	}, nil
}

func modrinthLoader(serverType enums.MinecraftServerType) string {
	switch serverType {
	case enums.ServerFabric:
		return "fabric"
	case enums.ServerForge:
		return "forge"
	case enums.ServerNeoForge:
		return "neoforge"
	case enums.ServerQuilt:
		return "quilt"
	default:
		return ""
	}
}

func containsExact(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
