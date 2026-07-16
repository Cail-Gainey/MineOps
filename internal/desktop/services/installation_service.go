package services

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/port"
	"github.com/Cail-Gainey/MineOps/internal/service"
)

// InstallationStartResult contains durable task/operation identities or a stable error.
type InstallationStartResult struct {
	TaskID      string        `json:"taskID,omitempty"`
	OperationID string        `json:"operationID,omitempty"`
	Error       *apperror.DTO `json:"error,omitempty"`
}

// InstallationResult contains one durable installation aggregate or a stable error.
type InstallationResult struct {
	Task  *model.InstallationTask  `json:"task,omitempty"`
	Steps []model.InstallationStep `json:"steps"`
	Error *apperror.DTO            `json:"error,omitempty"`
}

// InstallationListResult contains recent durable installation tasks or a stable error.
type InstallationListResult struct {
	Tasks []model.InstallationTask `json:"tasks"`
	Error *apperror.DTO            `json:"error,omitempty"`
}

// ServerDistributionListResult contains the dynamic server type registry or a stable error.
type ServerDistributionListResult struct {
	Distributions []port.ServerDistribution `json:"distributions"`
	Error         *apperror.DTO             `json:"error,omitempty"`
}

// ServerVersionListResult contains cached provider-neutral catalog versions or a stable error.
type ServerVersionListResult struct {
	Versions []port.ServerVersion `json:"versions"`
	Error    *apperror.DTO        `json:"error,omitempty"`
}

type cachedServerVersions struct {
	versions  []port.ServerVersion
	expiresAt time.Time
}

type serverVersionRefresh struct {
	done chan struct{}
	err  error
}

type persistedServerVersionCache struct {
	Schema  int                                    `json:"schema"`
	Entries map[string]persistedServerVersionEntry `json:"entries"`
}

type persistedServerVersionEntry struct {
	Versions  []port.ServerVersion `json:"versions"`
	ExpiresAt time.Time            `json:"expiresAt"`
}

const (
	serverVersionCacheSchema  = 1
	serverVersionCacheTTL     = 6 * time.Hour
	serverVersionRetryDelay   = 5 * time.Minute
	serverVersionRefreshLimit = 45 * time.Second
)

// InstallationService exposes catalog, start, cancel, get, and retry workflows to Wails.
type InstallationService struct {
	manager *service.InstallationManager
	logger  *applog.Logger

	cacheMu  sync.Mutex
	cache    map[enums.MinecraftServerType]cachedServerVersions
	inflight map[enums.MinecraftServerType]*serverVersionRefresh

	persistMu sync.Mutex
	cachePath string
}

// NewInstallationService creates the desktop installation facade with persistent stale-while-refresh version caching.
func NewInstallationService(manager *service.InstallationManager, logger *applog.Logger, dataDirectory string) *InstallationService {
	result := &InstallationService{
		manager: manager, logger: logger,
		cache:     make(map[enums.MinecraftServerType]cachedServerVersions),
		inflight:  make(map[enums.MinecraftServerType]*serverVersionRefresh),
		cachePath: filepath.Join(dataDirectory, "cache", "server-versions-v1.json"),
	}
	if err := result.loadVersionCache(); err != nil && !os.IsNotExist(err) && logger != nil {
		logger.Warn(context.Background(), "读取 Minecraft 版本缓存失败", applog.Fields{"error": err.Error()})
	}
	return result
}

// ListDistributions returns all dynamically registered first-release server types.
func (s *InstallationService) ListDistributions(ctx context.Context) (result ServerDistributionListResult) {
	defer s.recoverDistributions(ctx, &result)
	return ServerDistributionListResult{Distributions: s.manager.ListDistributions()}
}

// ResolveVersions returns cached catalog versions for one distribution.
func (s *InstallationService) ResolveVersions(ctx context.Context, distribution string) (result ServerVersionListResult) {
	defer s.recoverVersions(ctx, &result)
	typeValue := enums.MinecraftServerType(distribution)
	now := time.Now()
	s.cacheMu.Lock()
	cached, found := s.cache[typeValue]
	if found && now.Before(cached.expiresAt) {
		versions := append([]port.ServerVersion(nil), cached.versions...)
		s.cacheMu.Unlock()
		return ServerVersionListResult{Versions: versions}
	}
	if active := s.inflight[typeValue]; active != nil {
		if found {
			versions := append([]port.ServerVersion(nil), cached.versions...)
			s.cacheMu.Unlock()
			return ServerVersionListResult{Versions: versions}
		}
		done := active.done
		s.cacheMu.Unlock()
		select {
		case <-ctx.Done():
			dto := apperror.ToDTO(apperror.Wrap(apperror.CodeProcessCancelled, "等待 Minecraft 版本目录时已取消", ctx.Err()))
			return ServerVersionListResult{Error: &dto}
		case <-done:
		}
		if active.err != nil {
			dto := apperror.ToDTO(active.err)
			return ServerVersionListResult{Error: &dto}
		}
		s.cacheMu.Lock()
		cached = s.cache[typeValue]
		versions := append([]port.ServerVersion(nil), cached.versions...)
		s.cacheMu.Unlock()
		return ServerVersionListResult{Versions: versions}
	}
	refresh := &serverVersionRefresh{done: make(chan struct{})}
	s.inflight[typeValue] = refresh
	if found {
		versions := append([]port.ServerVersion(nil), cached.versions...)
		s.cacheMu.Unlock()
		go s.refreshStaleVersions(typeValue, refresh)
		return ServerVersionListResult{Versions: versions}
	}
	s.cacheMu.Unlock()
	versions, err := s.refreshVersions(ctx, typeValue, refresh)
	if err != nil {
		dto := apperror.ToDTO(err)
		return ServerVersionListResult{Error: &dto}
	}
	return ServerVersionListResult{Versions: versions}
}

func (s *InstallationService) refreshStaleVersions(distribution enums.MinecraftServerType, refresh *serverVersionRefresh) {
	ctx, cancel := context.WithTimeout(context.Background(), serverVersionRefreshLimit)
	defer cancel()
	_, _ = s.refreshVersions(ctx, distribution, refresh)
}

func (s *InstallationService) refreshVersions(ctx context.Context, distribution enums.MinecraftServerType, refresh *serverVersionRefresh) ([]port.ServerVersion, error) {
	versions, err := s.manager.ResolveVersions(ctx, distribution)
	s.cacheMu.Lock()
	if err == nil {
		s.cache[distribution] = cachedServerVersions{
			versions: append([]port.ServerVersion(nil), versions...), expiresAt: time.Now().Add(serverVersionCacheTTL),
		}
	} else if cached, found := s.cache[distribution]; found {
		cached.expiresAt = time.Now().Add(serverVersionRetryDelay)
		s.cache[distribution] = cached
	}
	refresh.err = err
	delete(s.inflight, distribution)
	close(refresh.done)
	s.cacheMu.Unlock()
	if err == nil {
		if persistErr := s.persistVersionCache(); persistErr != nil && s.logger != nil {
			s.logger.Warn(context.WithoutCancel(ctx), "保存 Minecraft 版本缓存失败", applog.Fields{"error": persistErr.Error()})
		}
	} else if s.logger != nil {
		s.logger.Warn(context.WithoutCancel(ctx), "刷新 Minecraft 版本目录失败", applog.Fields{
			"distribution": distribution.String(), "error": err.Error(),
		})
	}
	return versions, err
}

func (s *InstallationService) loadVersionCache() error {
	payload, err := os.ReadFile(s.cachePath)
	if err != nil {
		return err
	}
	var stored persistedServerVersionCache
	if err := json.Unmarshal(payload, &stored); err != nil {
		return err
	}
	if stored.Schema != serverVersionCacheSchema {
		return nil
	}
	for key, entry := range stored.Entries {
		distribution := enums.MinecraftServerType(key)
		if distribution.Valid() && len(entry.Versions) > 0 {
			s.cache[distribution] = cachedServerVersions{
				versions: append([]port.ServerVersion(nil), entry.Versions...), expiresAt: entry.ExpiresAt,
			}
		}
	}
	return nil
}

func (s *InstallationService) persistVersionCache() error {
	s.persistMu.Lock()
	defer s.persistMu.Unlock()
	s.cacheMu.Lock()
	stored := persistedServerVersionCache{
		Schema: serverVersionCacheSchema, Entries: make(map[string]persistedServerVersionEntry, len(s.cache)),
	}
	for distribution, cached := range s.cache {
		stored.Entries[distribution.String()] = persistedServerVersionEntry{
			Versions: append([]port.ServerVersion(nil), cached.versions...), ExpiresAt: cached.expiresAt,
		}
	}
	s.cacheMu.Unlock()
	payload, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	directory := filepath.Dir(s.cachePath)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".server-versions-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(payload); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Remove(s.cachePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(temporaryPath, s.cachePath)
}

// Start performs preflight and immediately returns durable task and operation IDs.
func (s *InstallationService) Start(ctx context.Context, serverID string) (result InstallationStartResult) {
	defer s.recoverStart(ctx, &result)
	started, err := s.manager.Start(ctx, model.ID(serverID))
	if err != nil {
		dto := apperror.ToDTO(err)
		return InstallationStartResult{Error: &dto}
	}
	return InstallationStartResult{TaskID: started.TaskID.String(), OperationID: started.OperationID.String()}
}

// Cancel requests cancellation of the active installation Operation.
func (s *InstallationService) Cancel(ctx context.Context, operationID string) (result ActionResult) {
	defer s.recoverAction(ctx, "InstallationService.Cancel", &result)
	if err := s.manager.Cancel(ctx, model.ID(operationID)); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

// Get returns one task and its ordered durable step checkpoints.
func (s *InstallationService) Get(ctx context.Context, taskID string) (result InstallationResult) {
	defer s.recoverInstallation(ctx, &result)
	task, steps, err := s.manager.Get(ctx, model.ID(taskID))
	if err != nil {
		dto := apperror.ToDTO(err)
		return InstallationResult{Error: &dto}
	}
	return InstallationResult{Task: task, Steps: steps}
}

// ListByServer returns paginated installation history for one Minecraft Server.
func (s *InstallationService) ListByServer(ctx context.Context, serverID string, limit, offset int) (result InstallationListResult) {
	defer s.recoverInstallationList(ctx, &result)
	tasks, err := s.manager.ListByServer(ctx, model.ID(serverID), limit, offset)
	if err != nil {
		dto := apperror.ToDTO(err)
		return InstallationListResult{Error: &dto}
	}
	return InstallationListResult{Tasks: tasks}
}

// Retry starts a new Operation and preserves every successful step checkpoint.
func (s *InstallationService) Retry(ctx context.Context, taskID string) (result InstallationStartResult) {
	defer s.recoverStart(ctx, &result)
	started, err := s.manager.Retry(ctx, model.ID(taskID))
	if err != nil {
		dto := apperror.ToDTO(err)
		return InstallationStartResult{Error: &dto}
	}
	return InstallationStartResult{TaskID: started.TaskID.String(), OperationID: started.OperationID.String()}
}

// ResolveDirectoryConflict chooses backup, rename, or cancel for a failed server-directory step.
func (s *InstallationService) ResolveDirectoryConflict(ctx context.Context, taskID, action, newName string) (result InstallationStartResult) {
	defer s.recoverStart(ctx, &result)
	started, err := s.manager.ResolveDirectoryConflict(ctx, model.ID(taskID), action, newName)
	if err != nil {
		dto := apperror.ToDTO(err)
		return InstallationStartResult{Error: &dto}
	}
	return InstallationStartResult{TaskID: started.TaskID.String(), OperationID: started.OperationID.String()}
}

func (s *InstallationService) recoverStart(ctx context.Context, result *InstallationStartResult) {
	var err error
	apperror.Recover(ctx, s.logger, "InstallationService", &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

func (s *InstallationService) recoverInstallation(ctx context.Context, result *InstallationResult) {
	var err error
	apperror.Recover(ctx, s.logger, "InstallationService.Get", &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

func (s *InstallationService) recoverInstallationList(ctx context.Context, result *InstallationListResult) {
	var err error
	apperror.Recover(ctx, s.logger, "InstallationService.ListByServer", &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

func (s *InstallationService) recoverVersions(ctx context.Context, result *ServerVersionListResult) {
	var err error
	apperror.Recover(ctx, s.logger, "InstallationService.ResolveVersions", &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

func (s *InstallationService) recoverDistributions(ctx context.Context, result *ServerDistributionListResult) {
	var err error
	apperror.Recover(ctx, s.logger, "InstallationService.ListDistributions", &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

func (s *InstallationService) recoverAction(ctx context.Context, boundary string, result *ActionResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}
