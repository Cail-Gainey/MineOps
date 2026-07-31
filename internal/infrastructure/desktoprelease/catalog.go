package desktoprelease

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/infrastructure/httpclient"
	"github.com/Cail-Gainey/MineOps/internal/infrastructure/releaseversion"
)

const (
	manifestAssetName          = "mineops-desktop-update.json"
	manifestSignatureAssetName = "mineops-desktop-update.sig.json"
	manifestSchemaVersion      = 1
	maximumGitHubPages         = 3
	maximumGitHubReleases      = 300
	maximumGitHubResponseSize  = 2 * 1024 * 1024
	maximumManifestSize        = 1024 * 1024
	maximumSignatureSize       = 16 * 1024
)

// PublicKeyBase64 是构建期注入的原始 Ed25519 更新公钥。
// 生产构建必须通过 -ldflags 设置;取值为空时按失败关闭处理。
var PublicKeyBase64 string

// Artifact 描述一份经认证的自更新负载。
type Artifact struct {
	URL           string `json:"url"`
	Platform      string `json:"platform"`
	Arch          string `json:"arch"`
	Filename      string `json:"filename"`
	Filetype      string `json:"filetype,omitempty"`
	Size          int64  `json:"size"`
	DigestAlgo    string `json:"digestAlgo"`
	Digest        string `json:"digest"`
	SignatureAlgo string `json:"signatureAlgo"`
	Signature     string `json:"signature"`
}

// Manifest 是经认证的 MineOps 桌面更新文档。
type Manifest struct {
	SchemaVersion int        `json:"schemaVersion"`
	Version       string     `json:"version"`
	Channel       string     `json:"channel"`
	Name          string     `json:"name,omitempty"`
	Notes         string     `json:"notes,omitempty"`
	PublishedAt   string     `json:"publishedAt"`
	Artifacts     []Artifact `json:"artifacts"`
}

// Release 描述一个经认证的 GitHub 桌面发行版及选中的构件。
type Release struct {
	Version     string
	PublishedAt string
	ReleaseURL  string
	Notes       string
	Channel     string
	TagName     string
	Prerelease  bool
	Artifact    Artifact
	ManifestURL string
}

type githubRelease struct {
	TagName     string        `json:"tag_name"`
	Draft       bool          `json:"draft"`
	Prerelease  bool          `json:"prerelease"`
	HTMLURL     string        `json:"html_url"`
	PublishedAt string        `json:"published_at"`
	Body        string        `json:"body"`
	Assets      []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type manifestSignature struct {
	Filename      string `json:"filename"`
	Size          int64  `json:"size"`
	DigestAlgo    string `json:"digestAlgo"`
	Digest        string `json:"digest"`
	SignatureAlgo string `json:"signatureAlgo"`
	Signature     string `json:"signature"`
}

// Catalog 拉取并认证 MineOps 的 GitHub 桌面发行版。
type Catalog struct {
	client    requestClient
	publicKey ed25519.PublicKey
}

type requestClient interface {
	Do(ctx context.Context, method, endpoint string, headers http.Header) (*http.Response, error)
}

// NewCatalog 用构建期注入的公钥创建 GitHub 发行版目录。
func NewCatalog(client *httpclient.Client) *Catalog {
	key := parsePublicKeyBase64(PublicKeyBase64)
	return NewCatalogWithPublicKey(client, key)
}

func parsePublicKeyBase64(value string) ed25519.PublicKey {
	keyBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return nil
	}
	if len(keyBytes) == ed25519.PublicKeySize {
		return ed25519.PublicKey(keyBytes)
	}
	if block, _ := pem.Decode(keyBytes); block != nil {
		keyBytes = block.Bytes
	}
	parsed, err := x509.ParsePKIXPublicKey(keyBytes)
	if err != nil {
		return nil
	}
	publicKey, _ := parsed.(ed25519.PublicKey)
	return publicKey
}

// NewCatalogWithPublicKey 用显式指定的 Ed25519 信任根创建 GitHub 发行版目录。
func NewCatalogWithPublicKey(client *httpclient.Client, publicKey []byte) *Catalog {
	return &Catalog{client: client, publicKey: append(ed25519.PublicKey(nil), publicKey...)}
}

// Resolve 返回当前运行平台上版本最高的已认证发行版。
func (c *Catalog) Resolve(ctx context.Context, sourceURL, channel string) (Release, error) {
	return c.ResolveFor(ctx, sourceURL, channel, "", runtime.GOOS, runtime.GOARCH)
}

// ResolveFor 返回某个目标平台上高于 currentVersion 的最高已认证发行版。
func (c *Catalog) ResolveFor(ctx context.Context, sourceURL, channel, currentVersion, platform, architecture string) (Release, error) {
	if c == nil || c.client == nil {
		return Release{}, apperror.New(apperror.CodeValidationRequired, "Desktop Release Catalog HTTP Client 不能为空")
	}
	if len(c.publicKey) != ed25519.PublicKeySize {
		return Release{}, apperror.New(apperror.CodeDesktopUpdateMetadataUnauthenticated, "Desktop Update Public Key 未配置或无效")
	}
	channel = strings.ToLower(strings.TrimSpace(channel))
	if channel != "stable" && channel != "beta" {
		return Release{}, apperror.New(apperror.CodeValidationInvalidArgument, "Desktop Update Channel 仅支持 stable/beta")
	}
	endpoint, err := githubReleasesEndpoint(sourceURL)
	if err != nil {
		return Release{}, err
	}
	releases, err := c.listReleases(ctx, endpoint)
	if err != nil {
		return Release{}, err
	}
	candidates, err := selectCandidates(releases, channel, currentVersion)
	if err != nil {
		return Release{}, err
	}
	if len(candidates) > 0 {
		return c.resolveCandidate(ctx, candidates[0], channel, platform, architecture)
	}
	return Release{}, apperror.New(apperror.CodeIONotFound, "Desktop Update Channel 当前没有更高版本").WithDetails(map[string]any{"channel": channel})
}

func (c *Catalog) listReleases(ctx context.Context, endpoint string) ([]githubRelease, error) {
	all := make([]githubRelease, 0, maximumGitHubReleases)
	for page := 1; page <= maximumGitHubPages; page++ {
		pageURL := fmt.Sprintf("%s%cper_page=100&page=%d", endpoint, querySeparator(endpoint), page)
		var releases []githubRelease
		if err := c.fetchGitHubJSON(ctx, pageURL, &releases); err != nil {
			return nil, err
		}
		if len(releases) > 100 || len(all)+len(releases) > maximumGitHubReleases {
			return nil, apperror.New(apperror.CodeHTTPResponseTooLarge, "GitHub Releases 列表超过分页限制")
		}
		all = append(all, releases...)
		if len(releases) < 100 {
			return all, nil
		}
	}
	return nil, apperror.New(apperror.CodeHTTPResponseTooLarge, "GitHub Releases 分页超过 3 页限制")
}

func (c *Catalog) fetchGitHubJSON(ctx context.Context, endpoint string, target any) error {
	payload, err := c.fetchBytes(ctx, endpoint, maximumGitHubResponseSize, "GitHub Releases API")
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := decoder.Decode(target); err != nil {
		return apperror.Wrap(apperror.CodeIOReadFailed, "解析 GitHub Releases API 失败", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return apperror.New(apperror.CodeIOReadFailed, "GitHub Releases API 必须只包含一个 JSON 文档")
	}
	return nil
}

func selectCandidates(releases []githubRelease, channel, currentVersion string) ([]githubRelease, error) {
	selected := make([]githubRelease, 0, len(releases))
	for _, release := range releases {
		if release.Draft || channel == "stable" && release.Prerelease {
			continue
		}
		if _, err := releaseversion.Normalize(release.TagName); err != nil {
			return nil, apperror.Wrap(apperror.CodeValidationInvalidArgument, "GitHub Release Tag 必须是有效 SemVer", err)
		}
		isPrerelease, err := releaseversion.IsPrerelease(release.TagName)
		if err != nil {
			return nil, err
		}
		if isPrerelease != release.Prerelease {
			return nil, apperror.New(apperror.CodeValidationConflict, "GitHub Release prerelease 标记与 Tag 不一致")
		}
		if currentVersion != "" {
			comparison, err := releaseversion.Compare(currentVersion, release.TagName)
			if err != nil {
				return nil, err
			}
			if comparison >= 0 {
				continue
			}
		}
		selected = append(selected, release)
	}
	for i := 0; i < len(selected); i++ {
		for j := i + 1; j < len(selected); j++ {
			comparison, err := releaseversion.Compare(selected[i].TagName, selected[j].TagName)
			if err != nil {
				return nil, err
			}
			if comparison < 0 {
				selected[i], selected[j] = selected[j], selected[i]
			}
		}
	}
	return selected, nil
}

func (c *Catalog) resolveCandidate(ctx context.Context, release githubRelease, requestedChannel, platform, architecture string) (Release, error) {
	manifestAsset, signatureAsset, err := requiredManifestAssets(release.Assets)
	if err != nil {
		return Release{}, err
	}
	manifestURL, err := secureURL(manifestAsset.URL, "Manifest")
	if err != nil {
		return Release{}, err
	}
	signatureURL, err := secureURL(signatureAsset.URL, "Manifest Signature")
	if err != nil {
		return Release{}, err
	}
	manifestBytes, err := c.fetchBytes(ctx, manifestURL, maximumManifestSize, "Desktop Update Manifest")
	if err != nil {
		return Release{}, err
	}
	var sidecar manifestSignature
	if err := c.fetchStrictJSON(ctx, signatureURL, maximumSignatureSize, &sidecar); err != nil {
		return Release{}, err
	}
	if err := verifyManifest(c.publicKey, manifestBytes, sidecar); err != nil {
		return Release{}, err
	}
	var manifest Manifest
	if err := decodeStrictJSON(manifestBytes, &manifest, "Desktop Update Manifest"); err != nil {
		return Release{}, err
	}
	derivedChannel := "stable"
	if release.Prerelease {
		derivedChannel = "beta"
	}
	if err := validateManifest(manifest, release, derivedChannel, requestedChannel); err != nil {
		return Release{}, err
	}
	artifact, err := selectArtifact(manifest.Artifacts, platform, architecture)
	if err != nil {
		return Release{}, err
	}
	return Release{
		Version: manifest.Version, PublishedAt: manifest.PublishedAt, ReleaseURL: release.HTMLURL,
		Notes: manifest.Notes, Channel: manifest.Channel, TagName: release.TagName,
		Prerelease: release.Prerelease, Artifact: artifact, ManifestURL: manifestURL,
	}, nil
}

func requiredManifestAssets(assets []githubAsset) (githubAsset, githubAsset, error) {
	var manifests, signatures []githubAsset
	for _, asset := range assets {
		switch asset.Name {
		case manifestAssetName:
			manifests = append(manifests, asset)
		case manifestSignatureAssetName:
			signatures = append(signatures, asset)
		}
	}
	if len(manifests) != 1 || len(signatures) != 1 {
		return githubAsset{}, githubAsset{}, apperror.New(apperror.CodeValidationConflict, "GitHub Release 必须包含唯一 Manifest 及签名 Sidecar")
	}
	return manifests[0], signatures[0], nil
}

func verifyManifest(publicKey ed25519.PublicKey, payload []byte, sidecar manifestSignature) error {
	if sidecar.Filename != manifestAssetName || sidecar.Size != int64(len(payload)) || sidecar.DigestAlgo != "sha512" || sidecar.SignatureAlgo != "ed25519ph" {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Manifest Signature Algorithm 仅支持 ed25519ph")
	}
	digest := sha512.Sum512(payload)
	expectedDigest, err := base64.StdEncoding.DecodeString(sidecar.Digest)
	if err != nil || !bytes.Equal(expectedDigest, digest[:]) {
		return apperror.Wrap(apperror.CodeDesktopUpdateMetadataUnauthenticated, "Desktop Update Manifest Digest 验证失败", err)
	}
	signature, err := base64.StdEncoding.DecodeString(sidecar.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return apperror.Wrap(apperror.CodeValidationInvalidArgument, "Manifest Signature 必须是有效 Base64 Ed25519 签名", err)
	}
	if err := ed25519.VerifyWithOptions(publicKey, digest[:], signature, &ed25519.Options{Hash: crypto.SHA512}); err != nil {
		return apperror.Wrap(apperror.CodeDesktopUpdateMetadataUnauthenticated, "Desktop Update Manifest 签名验证失败", err)
	}
	return nil
}

func validateManifest(manifest Manifest, release githubRelease, derivedChannel, requestedChannel string) error {
	if manifest.SchemaVersion != manifestSchemaVersion {
		return apperror.New(apperror.CodeAgentProtocolUnsupported, "Desktop Update Manifest Schema 版本不受支持")
	}
	manifestVersion, err := releaseversion.Normalize(manifest.Version)
	if err != nil {
		return err
	}
	tagVersion, err := releaseversion.Normalize(release.TagName)
	if err != nil {
		return err
	}
	if manifestVersion != tagVersion || manifest.Channel != derivedChannel {
		return apperror.New(apperror.CodeValidationConflict, "Git Tag、Release 类型与 Manifest 版本通道不一致")
	}
	if requestedChannel == "stable" && manifest.Channel != "stable" {
		return apperror.New(apperror.CodeValidationConflict, "Stable Channel 禁止 Beta Manifest")
	}
	if _, err := time.Parse(time.RFC3339, manifest.PublishedAt); err != nil {
		return apperror.Wrap(apperror.CodeValidationInvalidArgument, "Manifest PublishedAt 必须是 RFC3339 时间", err)
	}
	if len(manifest.Notes) > 4000 || strings.ContainsRune(manifest.Notes, '\x00') {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Manifest Notes 超过限制")
	}
	if len(manifest.Artifacts) == 0 || len(manifest.Artifacts) > 32 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Manifest Artifact 数量必须为 1-32")
	}
	if _, err := secureURL(release.HTMLURL, "GitHub Release Page"); err != nil {
		return err
	}
	return nil
}

func selectArtifact(artifacts []Artifact, platform, architecture string) (Artifact, error) {
	matches := make([]Artifact, 0, 1)
	for _, artifact := range artifacts {
		if artifact.Platform == platform && artifact.Arch == architecture {
			matches = append(matches, artifact)
		}
	}
	if len(matches) != 1 {
		return Artifact{}, apperror.New(apperror.CodeValidationConflict, "Manifest 必须包含唯一的当前平台架构 Artifact").WithDetails(map[string]any{"platform": platform, "architecture": architecture, "matches": len(matches)})
	}
	artifact := matches[0]
	artifactURL, err := secureURL(artifact.URL, "Artifact")
	if err != nil {
		return Artifact{}, err
	}
	parsed, _ := url.Parse(artifactURL)
	if artifact.Filename == "" {
		artifact.Filename = path.Base(parsed.Path)
	}
	if path.Base(artifact.Filename) != artifact.Filename || strings.ContainsAny(artifact.Filename, "\\\x00") {
		return Artifact{}, apperror.New(apperror.CodeValidationInvalidArgument, "Artifact Filename 无效")
	}
	if path.Base(parsed.Path) != artifact.Filename {
		return Artifact{}, apperror.New(apperror.CodeValidationConflict, "Artifact URL 与 Filename 不一致")
	}
	if artifact.Size <= 0 || artifact.Size > 2*1024*1024*1024 {
		return Artifact{}, apperror.New(apperror.CodeValidationInvalidArgument, "Artifact Size 超出限制")
	}
	if artifact.DigestAlgo != "sha512" || artifact.SignatureAlgo != "ed25519ph" {
		return Artifact{}, apperror.New(apperror.CodeValidationInvalidArgument, "Artifact 仅支持 sha512 与 ed25519ph")
	}
	digest, digestErr := base64.StdEncoding.DecodeString(artifact.Digest)
	signature, signatureErr := base64.StdEncoding.DecodeString(artifact.Signature)
	if digestErr != nil || len(digest) != sha512.Size || signatureErr != nil || len(signature) != ed25519.SignatureSize {
		return Artifact{}, apperror.New(apperror.CodeValidationInvalidArgument, "Artifact Digest 或 Signature 编码无效")
	}
	return artifact, nil
}

func (c *Catalog) fetchStrictJSON(ctx context.Context, endpoint string, maximum int64, target any) error {
	payload, err := c.fetchBytes(ctx, endpoint, maximum, "Desktop Release JSON")
	if err != nil {
		return err
	}
	return decodeStrictJSON(payload, target, "Desktop Release JSON")
}

func (c *Catalog) fetchBytes(ctx context.Context, endpoint string, maximum int64, label string) ([]byte, error) {
	if _, err := secureURL(endpoint, label); err != nil {
		return nil, err
	}
	response, err := c.client.Do(ctx, http.MethodGet, endpoint, http.Header{"Accept": []string{"application/json"}})
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.ContentLength > maximum {
		return nil, apperror.New(apperror.CodeHTTPResponseTooLarge, label+" 超过大小限制")
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, maximum+1))
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeIOReadFailed, "读取 "+label+" 失败", err)
	}
	if int64(len(payload)) > maximum {
		return nil, apperror.New(apperror.CodeHTTPResponseTooLarge, label+" 超过大小限制")
	}
	return payload, nil
}

func decodeStrictJSON(payload []byte, target any, label string) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return apperror.Wrap(apperror.CodeIOReadFailed, "解析 "+label+" 失败", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("包含多余 JSON 值")
		}
		return apperror.Wrap(apperror.CodeIOReadFailed, label+" 必须只包含一个 JSON 文档", err)
	}
	return nil
}

func githubReleasesEndpoint(sourceURL string) (string, error) {
	endpoint, err := secureURL(strings.TrimSpace(sourceURL), "GitHub Releases API")
	if err != nil {
		return "", err
	}
	parsed, _ := url.Parse(endpoint)
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", apperror.New(apperror.CodeValidationInvalidArgument, "GitHub Releases API URL 禁止 Query 或 Fragment")
	}
	return strings.TrimRight(endpoint, "/"), nil
}

func secureURL(value, label string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return "", apperror.Wrap(apperror.CodeValidationInvalidArgument, label+" 必须使用无凭据 HTTPS URL", err)
	}
	return parsed.String(), nil
}

func querySeparator(endpoint string) byte {
	if strings.Contains(endpoint, "?") {
		return '&'
	}
	return '?'
}
