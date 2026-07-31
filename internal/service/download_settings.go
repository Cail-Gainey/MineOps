package service

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/global/appsettings"
	"github.com/Cail-Gainey/MineOps/internal/global/appthread"
	"github.com/Cail-Gainey/MineOps/internal/global/constants"
	"github.com/Cail-Gainey/MineOps/internal/infrastructure/httpclient"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
)

// ArtifactStreamProgress 汇报一路有界本地构件流的字节进度。
type ArtifactStreamProgress struct {
	Downloaded int64
	Total      int64
}

// DownloadSourceStatus 承载当前可达性、时延与稳定的失败细节。
type DownloadSourceStatus struct {
	Category     string        `json:"category"`
	Name         string        `json:"name"`
	URL          string        `json:"url"`
	Available    bool          `json:"available"`
	Latency      time.Duration `json:"latency"`
	CheckedAt    time.Time     `json:"checkedAt"`
	ErrorCode    string        `json:"errorCode,omitempty"`
	ErrorMessage string        `json:"errorMessage,omitempty"`
}

// DownloadCacheStatus 描述当前本地构件缓存的占用情况。
type DownloadCacheStatus struct {
	Directory     string `json:"directory"`
	Files         int    `json:"files"`
	Bytes         int64  `json:"bytes"`
	CapacityBytes int64  `json:"capacityBytes"`
}

// ProxyCredentialStatus 暴露非机密的代理凭据状态。
type ProxyCredentialStatus struct {
	CredentialID string `json:"credentialID,omitempty"`
	Username     string `json:"username,omitempty"`
	Configured   bool   `json:"configured"`
}

// DownloadManager 负责加密代理凭据、HTTP 热重载、源探测与缓存管理。
type DownloadManager struct {
	clock       model.Clock
	store       repository.Store
	settings    *appsettings.Manager
	client      *httpclient.Client
	pool        *appthread.Pool
	logger      *applog.Logger
	dataRoot    string
	unsubscribe func()
}

// NewDownloadManager 创建下载设置的所有者并载入初始 HTTP 传输层。
func NewDownloadManager(clock model.Clock, store repository.Store, settings *appsettings.Manager, client *httpclient.Client, pool *appthread.Pool, logger *applog.Logger, dataRoot string) (*DownloadManager, error) {
	if clock == nil || store == nil || settings == nil || client == nil || strings.TrimSpace(dataRoot) == "" {
		return nil, apperror.New(apperror.CodeValidationRequired, "DownloadManager 依赖不能为空")
	}
	if logger == nil {
		logger = applog.Default()
	}
	if pool == nil {
		pool = appthread.Default()
	}
	manager := &DownloadManager{clock: clock, store: store, settings: settings, client: client, pool: pool, logger: logger, dataRoot: dataRoot}
	if err := manager.ReloadHTTP(context.Background()); err != nil {
		return nil, err
	}
	manager.unsubscribe = settings.Subscribe(func(change appsettings.Change) {
		if change.Category == "" || change.Category.String() == "downloads" {
			if err := manager.ReloadHTTP(context.Background()); err != nil {
				manager.logger.Error(context.Background(), "重载 HTTP Client 配置失败", err, applog.Fields{"component": "downloads"})
			}
		}
	})
	return manager, nil
}

// Close 移除该 Manager 持有的设置订阅。
func (m *DownloadManager) Close() {
	if m != nil && m.unsubscribe != nil {
		m.unsubscribe()
		m.unsubscribe = nil
	}
}

// ReloadHTTP 原子应用超时、重试、代理、绕行与加密凭据。
func (m *DownloadManager) ReloadHTTP(ctx context.Context) error {
	snapshot := m.settings.Snapshot()
	proxy := snapshot.Downloads.Proxy
	username, password := "", ""
	if proxy.CredentialID != "" {
		credential, err := m.store.ProxyCredentials().Get(ctx, model.ID(proxy.CredentialID))
		if err != nil {
			return err
		}
		username, password = credential.Username, string(credential.Password)
		defer credential.Clear()
	}
	if err := m.client.Reload(httpclient.Config{
		UserAgent: constants.ApplicationUserAgent, RequestTimeout: time.Duration(snapshot.Downloads.TimeoutSeconds) * time.Second,
		OverallTimeout: time.Duration(snapshot.Downloads.OverallTimeoutSeconds) * time.Second,
		Retries:        snapshot.Downloads.Retries, RetryBackoff: time.Duration(snapshot.Downloads.RetryBackoffSeconds) * time.Second,
		MaximumResponseSize: int64(snapshot.Downloads.MaxArtifactMiB) * 1024 * 1024,
		ProxyMode:           proxy.Mode.String(), ProxyHost: proxy.Host, ProxyPort: proxy.Port,
		ProxyUsername: username, ProxyPassword: password, ProxyBypass: append([]string(nil), proxy.Bypass...),
		Concurrency: snapshot.Downloads.Concurrency, BandwidthLimitBytesPerSecond: int64(snapshot.Downloads.BandwidthLimitKiB) * 1024,
	}); err != nil {
		return err
	}
	if snapshot.Downloads.AutoCleanup {
		return m.enforceCacheCapacity()
	}
	return nil
}

// SaveProxyCredential 创建或替换仅存于 SQLCipher 的代理账号口令记录,并提交其引用。
func (m *DownloadManager) SaveProxyCredential(ctx context.Context, username string, password []byte) (ProxyCredentialStatus, error) {
	defer clearBytes(password)
	snapshot := m.settings.Snapshot()
	credentialID := model.ID(snapshot.Downloads.Proxy.CredentialID)
	if credentialID.Valid() {
		credential, err := m.store.ProxyCredentials().Get(ctx, credentialID)
		if err != nil {
			return ProxyCredentialStatus{}, err
		}
		defer credential.Clear()
		credential.Username = strings.TrimSpace(username)
		credential.Password = append(credential.Password[:0], password...)
		credential.UpdatedAt = m.clock.Now().UTC()
		if err := m.store.ProxyCredentials().Update(ctx, credential); err != nil {
			return ProxyCredentialStatus{}, err
		}
	} else {
		credential, err := model.NewProxyCredential(m.clock, username, password)
		if err != nil {
			return ProxyCredentialStatus{}, err
		}
		defer credential.Clear()
		if err := m.store.ProxyCredentials().Create(ctx, credential); err != nil {
			return ProxyCredentialStatus{}, err
		}
		credentialID = credential.ID
		snapshot.Downloads.Proxy.CredentialID = credentialID.String()
		if err := m.settings.Save(ctx, snapshot); err != nil {
			_ = m.store.ProxyCredentials().Delete(context.WithoutCancel(ctx), credentialID)
			return ProxyCredentialStatus{}, err
		}
	}
	if err := m.ReloadHTTP(ctx); err != nil {
		return ProxyCredentialStatus{}, err
	}
	return ProxyCredentialStatus{CredentialID: credentialID.String(), Username: strings.TrimSpace(username), Configured: true}, nil
}

// ProxyCredentialStatus 为设置界面返回非机密的凭据状态。
func (m *DownloadManager) ProxyCredentialStatus(ctx context.Context) (ProxyCredentialStatus, error) {
	id := model.ID(m.settings.Snapshot().Downloads.Proxy.CredentialID)
	if !id.Valid() {
		return ProxyCredentialStatus{}, nil
	}
	credential, err := m.store.ProxyCredentials().Get(ctx, id)
	if err != nil {
		return ProxyCredentialStatus{}, err
	}
	defer credential.Clear()
	return ProxyCredentialStatus{CredentialID: id.String(), Username: credential.Username, Configured: true}, nil
}

// ClearProxyCredential 先移除设置中的引用,再删除加密凭据记录。
func (m *DownloadManager) ClearProxyCredential(ctx context.Context) error {
	snapshot := m.settings.Snapshot()
	id := model.ID(snapshot.Downloads.Proxy.CredentialID)
	if !id.Valid() {
		return nil
	}
	snapshot.Downloads.Proxy.CredentialID = ""
	if err := m.settings.Save(ctx, snapshot); err != nil {
		return err
	}
	if err := m.store.ProxyCredentials().Delete(ctx, id); err != nil && apperror.ToDTO(err).Code != apperror.CodeIONotFound.String() {
		return err
	}
	return m.ReloadHTTP(ctx)
}

// CheckSources 通过当前代理并发探测每个启用的源,并按配置的优先级顺序返回结果。
// 每个源在全局线程池上独立测量,因此某个缓慢或超时的镜像不会拖住整轮测速。
func (m *DownloadManager) CheckSources(ctx context.Context) []DownloadSourceStatus {
	sources := append([]model.DownloadSourceSettings(nil), m.settings.Snapshot().Downloads.Sources...)
	sort.SliceStable(sources, func(left, right int) bool { return sources[left].Priority < sources[right].Priority })
	enabled := make([]model.DownloadSourceSettings, 0, len(sources))
	for _, source := range sources {
		if source.Enabled {
			enabled = append(enabled, source)
		}
	}
	return appthread.Map(ctx, m.pool, enabled, m.probeSource)
}

// probeSource 通过当前 HTTP 传输层对单个源执行一次有界的连通性与时延探测,
// 并把任何失败转换成稳定的错误元数据。
func (m *DownloadManager) probeSource(ctx context.Context, source model.DownloadSourceSettings) DownloadSourceStatus {
	startedAt := time.Now()
	response, err := m.client.Do(ctx, http.MethodGet, source.ProbeURL, http.Header{"Accept": []string{"application/json, text/plain, */*"}})
	status := DownloadSourceStatus{Category: source.Category, Name: source.Name, URL: source.BaseURL, CheckedAt: m.clock.Now().UTC(), Latency: time.Since(startedAt)}
	if err == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		_ = response.Body.Close()
		status.Available = true
	} else {
		dto := apperror.ToDTO(err)
		status.ErrorCode, status.ErrorMessage = dto.Code, dto.Message
	}
	return status
}

// ResolveSource 返回某个稳定 provider 键下优先级最高的启用端点。
func (m *DownloadManager) ResolveSource(provider, fallback string) string {
	snapshot := m.settings.Snapshot()
	sources := append([]model.DownloadSourceSettings(nil), snapshot.Downloads.Sources...)
	sort.SliceStable(sources, func(left, right int) bool { return sources[left].Priority < sources[right].Priority })
	for _, source := range sources {
		if source.Enabled && !source.Official && source.Provider == provider && strings.TrimSpace(source.BaseURL) != "" {
			return strings.TrimRight(source.BaseURL, "/")
		}
	}
	if mirror := categoryMirror(snapshot.Mirrors, provider); mirror != "" {
		return mirror
	}
	for _, source := range sources {
		if source.Enabled && source.Official && source.Provider == provider && strings.TrimSpace(source.BaseURL) != "" {
			return strings.TrimRight(source.BaseURL, "/")
		}
	}
	return strings.TrimRight(fallback, "/")
}

// ResolveArtifactURL 把已核准构件的源站替换成配置的 provider 源,同时保留路径与查询串。
func (m *DownloadManager) ResolveArtifactURL(provider, original string) (string, error) {
	originalURL, err := url.Parse(strings.TrimSpace(original))
	if err != nil || originalURL.Scheme != "https" || originalURL.Host == "" || originalURL.User != nil {
		return "", apperror.Wrap(apperror.CodeValidationInvalidArgument, "原始 Artifact URL 必须是无凭据 HTTPS", err)
	}
	resolvedBase := m.ResolveSource(provider, originalURL.Scheme+"://"+originalURL.Host)
	if provider == "spark" && strings.EqualFold(strings.TrimRight(resolvedBase, "/"), "https://spark.lucko.me") {
		resolvedBase = originalURL.Scheme + "://" + originalURL.Host
	}
	baseURL, err := url.Parse(resolvedBase)
	if err != nil || baseURL.Scheme != "https" || baseURL.Host == "" || baseURL.User != nil {
		return "", apperror.Wrap(apperror.CodeValidationInvalidArgument, "Artifact 镜像 Base URL 必须是无凭据 HTTPS", err)
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + "/" + strings.TrimLeft(originalURL.Path, "/")
	baseURL.RawQuery = originalURL.RawQuery
	baseURL.Fragment = ""
	return baseURL.String(), nil
}

// StreamArtifact 把一个经认证的 HTTPS 构件下载到 dst,并强制校验其声明大小。
func (m *DownloadManager) StreamArtifact(ctx context.Context, artifactURL string, expectedSize int64, dst io.Writer, onProgress func(ArtifactStreamProgress)) error {
	parsed, err := url.Parse(strings.TrimSpace(artifactURL))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || dst == nil || expectedSize <= 0 {
		return apperror.Wrap(apperror.CodeValidationInvalidArgument, "Desktop Update Artifact URL、大小或目标无效", err)
	}
	maximumSize := int64(m.settings.Snapshot().Downloads.MaxArtifactMiB) * 1024 * 1024
	if expectedSize > maximumSize {
		return apperror.New(apperror.CodeArtifactSizeExceeded, "Desktop Update Artifact 超过下载大小限制")
	}
	response, err := m.client.Do(ctx, http.MethodGet, parsed.String(), http.Header{"Accept": []string{"application/octet-stream"}})
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.ContentLength > 0 && response.ContentLength != expectedSize {
		return apperror.New(apperror.CodeArtifactSizeExceeded, "Desktop Update Artifact Content-Length 与 Manifest 不一致")
	}
	buffer := make([]byte, 128*1024)
	var downloaded int64
	for {
		read, readErr := response.Body.Read(buffer)
		if read > 0 {
			downloaded += int64(read)
			if downloaded > expectedSize || downloaded > maximumSize {
				return apperror.New(apperror.CodeArtifactSizeExceeded, "Desktop Update Artifact 下载超过大小限制")
			}
			if _, writeErr := dst.Write(buffer[:read]); writeErr != nil {
				return apperror.Wrap(apperror.CodeIOWriteFailed, "写入 Desktop Update Artifact 失败", writeErr)
			}
			if onProgress != nil {
				onProgress(ArtifactStreamProgress{Downloaded: downloaded, Total: expectedSize})
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return apperror.Wrap(apperror.CodeIOReadFailed, "读取 Desktop Update Artifact 失败", readErr).WithRetryable(true)
		}
	}
	if downloaded != expectedSize {
		return apperror.New(apperror.CodeArtifactSizeExceeded, "Desktop Update Artifact 实际大小与 Manifest 不一致")
	}
	return nil
}

func categoryMirror(settings model.MirrorSettings, provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	var mirror string
	switch provider {
	case "adoptium":
		mirror = settings.Java
	case "spark", "spark-modrinth":
		mirror = settings.Spark
	case "mojang", "papermc", "purpur", "fabric", "quilt", "spigot", "bungeecord", "forge", "neoforge":
		mirror = settings.Minecraft
		if strings.TrimSpace(mirror) != "" {
			return strings.TrimRight(strings.TrimSpace(mirror), "/") + "/" + provider
		}
	}
	return strings.TrimRight(strings.TrimSpace(mirror), "/")
}

// CacheStatus 计算当前本地下载缓存的占用。
func (m *DownloadManager) CacheStatus() (DownloadCacheStatus, error) {
	directory := m.cacheDirectory()
	status := DownloadCacheStatus{Directory: directory, CapacityBytes: int64(m.settings.Snapshot().Downloads.CacheCapacityMiB) * 1024 * 1024}
	err := filepath.WalkDir(directory, func(_ string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		status.Files++
		status.Bytes += info.Size()
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return DownloadCacheStatus{}, apperror.Wrap(apperror.CodeIOReadFailed, "统计下载缓存失败", err)
	}
	return status, nil
}

// ClearCache 删除缓存目录下的内容,保留已配置的根目录本身。
func (m *DownloadManager) ClearCache() error {
	directory := m.cacheDirectory()
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return apperror.Wrap(apperror.CodeIOReadFailed, "读取下载缓存失败", err)
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(directory, entry.Name())); err != nil {
			return apperror.Wrap(apperror.CodeIOWriteFailed, "清理下载缓存失败", err)
		}
	}
	return nil
}

func (m *DownloadManager) cacheDirectory() string {
	return filepath.Join(m.dataRoot, filepath.Clean(m.settings.Snapshot().Paths.DownloadsDirectory))
}

func (m *DownloadManager) enforceCacheCapacity() error {
	directory := m.cacheDirectory()
	type cacheFile struct {
		path    string
		size    int64
		modTime time.Time
	}
	files := make([]cacheFile, 0)
	var total int64
	err := filepath.WalkDir(directory, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		files = append(files, cacheFile{path: filePath, size: info.Size(), modTime: info.ModTime()})
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return apperror.Wrap(apperror.CodeIOReadFailed, "扫描下载缓存失败", err)
	}
	capacity := int64(m.settings.Snapshot().Downloads.CacheCapacityMiB) * 1024 * 1024
	if total <= capacity {
		return nil
	}
	sort.Slice(files, func(left, right int) bool { return files[left].modTime.Before(files[right].modTime) })
	for _, file := range files {
		if total <= capacity {
			break
		}
		if err := os.Remove(file.path); err != nil && !os.IsNotExist(err) {
			return apperror.Wrap(apperror.CodeIOWriteFailed, "自动清理下载缓存失败", err)
		}
		total -= file.size
	}
	return nil
}

func clearBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
