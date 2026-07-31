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
	modrinthAPIBaseURL        = "https://api.modrinth.com/v2"
	sparkProjectID            = "l6YH9Als"
	fabricAPIProjectID        = "P7dR8mSH"
	quiltedFabricAPIProjectID = "qvIfYCYJ"
)

// modrinthDependencySpec 描述随 Spark 模组构建一并安装的一个加载器 API 依赖。
type modrinthDependencySpec struct {
	projectID    string
	name         string
	loader       string
	matchPattern string
}

// loaderDependency 返回某个加载器下 spark 模组构建所需的运行时 API 依赖。
// spark 的模组构建在运行期依赖对应加载器 API（fabric.mod.json 声明），但 Modrinth 元数据未声明，必须随装。
func loaderDependency(loader string) (modrinthDependencySpec, bool) {
	switch loader {
	case "fabric":
		return modrinthDependencySpec{projectID: fabricAPIProjectID, name: "fabric-api", loader: "fabric", matchPattern: "fabric-api-*.jar"}, true
	case "quilt":
		return modrinthDependencySpec{projectID: quiltedFabricAPIProjectID, name: "quilted-fabric-api", loader: "quilt", matchPattern: "qfapi-*.jar"}, true
	default:
		return modrinthDependencySpec{}, false
	}
}

// Catalog 解析动态的 Minecraft spark 发行版,同时为其他平台保留固定布局。
type Catalog struct {
	client  *httpclient.Client
	baseURL string
}

// NewCatalog 创建已核准的 Minecraft spark 发行版目录。
func NewCatalog(client *httpclient.Client) *Catalog {
	return &Catalog{client: client, baseURL: modrinthAPIBaseURL}
}

// ResolveArtifact 为该 Server 平台与 Minecraft 版本选出一个确切的 Spark 构件。
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
	// Quilt 复用 Fabric 构建：spark 的 quilt 标记发布停留在 1.9 系列，超出锁定解析矩阵。
	searchLoader := loader
	if loader == "quilt" {
		searchLoader = "fabric"
	}
	query := url.Values{}
	query.Set("loaders", `[`+strconv.Quote(searchLoader)+`]`)
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
		if release.VersionType != "release" || !containsExact(release.Loaders, searchLoader) || !containsExact(release.GameVersions, minecraftVersion) {
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
		if dependencySpec, needed := loaderDependency(loader); needed {
			dependency, dependencyErr := c.resolveModrinthDependency(ctx, dependencySpec, minecraftVersion, requirement.MinimumMajor)
			if dependencyErr != nil {
				return Artifact{}, dependencyErr
			}
			artifact.Dependencies = []DependencyArtifact{dependency}
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
	for _, dependency := range r.Dependencies {
		// fabric-api / QFAPI 由 MineOps 主动解析并随装，不视为未支持的必需依赖。
		if dependency.DependencyType == "required" && dependency.ProjectID != fabricAPIProjectID && dependency.ProjectID != quiltedFabricAPIProjectID {
			return Artifact{}, apperror.New(apperror.CodeSparkUnsupported, "Spark 发布声明了 MineOps 尚未支持的必需依赖").WithDetails(map[string]any{
				"releaseID": r.ID, "projectID": dependency.ProjectID, "versionID": dependency.VersionID,
			})
		}
	}
	file, fileURL, sha512, err := r.primaryFile(sparkProjectID)
	if err != nil {
		return Artifact{}, err
	}
	version := strings.TrimSpace(r.VersionNumber)
	if separator := strings.IndexByte(version, '-'); separator > 0 {
		version = version[:separator]
	}
	return Artifact{
		Platform: loader, Provider: "spark-modrinth", ReleaseID: r.ID, Version: version,
		FileName: file.Filename, URL: fileURL, SHA512: sha512, Size: file.Size, TargetKind: "mods", RequiredJavaMajor: requiredJavaMajor,
	}, nil
}

// primaryFile 校验并返回某个 Modrinth 发行版唯一的主 CDN 文件。
func (r modrinthVersion) primaryFile(projectID string) (modrinthFile, string, string, error) {
	if strings.TrimSpace(r.ID) == "" {
		return modrinthFile{}, "", "", apperror.New(apperror.CodeValidationInvalidArgument, "Modrinth 发布缺少 Version ID")
	}
	primaryFiles := make([]modrinthFile, 0, 1)
	for _, file := range r.Files {
		if file.Primary {
			primaryFiles = append(primaryFiles, file)
		}
	}
	if len(primaryFiles) != 1 {
		return modrinthFile{}, "", "", apperror.New(apperror.CodeValidationConflict, "Modrinth 发布必须且只能包含一个 primary 文件").WithDetails(map[string]any{
			"releaseID": r.ID, "primaryFiles": len(primaryFiles),
		})
	}
	file := primaryFiles[0]
	parsedURL, err := url.Parse(strings.TrimSpace(file.URL))
	if err != nil || parsedURL.Scheme != "https" || !strings.EqualFold(parsedURL.Hostname(), "cdn.modrinth.com") || parsedURL.User != nil {
		return modrinthFile{}, "", "", apperror.Wrap(apperror.CodeValidationInvalidArgument, "Modrinth primary 文件 URL 无效", err)
	}
	if !strings.HasPrefix(parsedURL.Path, "/data/"+projectID+"/versions/"+r.ID+"/") || file.Filename == "" || path.Base(parsedURL.Path) != file.Filename || !strings.HasSuffix(strings.ToLower(file.Filename), ".jar") || file.Size <= 0 || file.Size > 64*1024*1024 {
		return modrinthFile{}, "", "", apperror.New(apperror.CodeValidationInvalidArgument, "Modrinth primary 文件身份或大小无效")
	}
	sha512 := strings.ToLower(strings.TrimSpace(file.Hashes["sha512"]))
	if len(sha512) != 128 {
		return modrinthFile{}, "", "", apperror.New(apperror.CodeValidationInvalidArgument, "Modrinth primary 文件缺少 SHA-512")
	}
	if _, err := hex.DecodeString(sha512); err != nil {
		return modrinthFile{}, "", "", apperror.Wrap(apperror.CodeValidationInvalidArgument, "Modrinth SHA-512 无效", err)
	}
	return file, parsedURL.String(), sha512, nil
}

// resolveModrinthDependency 为确切的 Minecraft 版本选出某个加载器 API 依赖的最新稳定发行版。
func (c *Catalog) resolveModrinthDependency(ctx context.Context, spec modrinthDependencySpec, minecraftVersion string, requiredJavaMajor int) (DependencyArtifact, error) {
	query := url.Values{}
	query.Set("loaders", `[`+strconv.Quote(spec.loader)+`]`)
	query.Set("game_versions", `[`+strconv.Quote(minecraftVersion)+`]`)
	endpoint := strings.TrimRight(c.baseURL, "/") + "/project/" + spec.projectID + "/version?" + query.Encode()
	var releases []modrinthVersion
	if err := c.client.GetJSON(ctx, endpoint, &releases); err != nil {
		return DependencyArtifact{}, err
	}
	sort.SliceStable(releases, func(left, right int) bool {
		return releases[left].DatePublished.After(releases[right].DatePublished)
	})
	for _, release := range releases {
		if release.VersionType != "release" || !containsExact(release.Loaders, spec.loader) || !containsExact(release.GameVersions, minecraftVersion) {
			continue
		}
		file, fileURL, sha512, err := release.primaryFile(spec.projectID)
		if err != nil {
			return DependencyArtifact{}, err
		}
		return DependencyArtifact{
			Name: spec.name, Version: strings.TrimSpace(release.VersionNumber), FileName: file.Filename,
			URL: fileURL, SHA512: sha512, Size: file.Size, TargetKind: "mods",
			MatchPattern: spec.matchPattern, RequiredJavaMajor: requiredJavaMajor,
		}, nil
	}
	return DependencyArtifact{}, apperror.New(apperror.CodeIONotFound, "Modrinth 没有与当前 Minecraft 版本兼容的稳定 "+spec.name+" 发布").WithDetails(map[string]any{
		"minecraftVersion": minecraftVersion, "projectID": spec.projectID, "dependency": spec.name,
	})
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
