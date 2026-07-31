package services

import (
	"context"
	"math"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/global/appsettings"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
	"github.com/Cail-Gainey/MineOps/internal/service"
)

// SSHSessionDTO 是桌面侧安全的 SSH Session 表示,不含凭据标识与密文。
type SSHSessionDTO struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	Host                string `json:"host"`
	Port                uint16 `json:"port"`
	Username            string `json:"username"`
	AuthType            string `json:"authType"`
	HasCredential       bool   `json:"hasCredential"`
	HostKeyPolicy       string `json:"hostKeyPolicy"`
	Group               string `json:"group"`
	Favourite           bool   `json:"favourite"`
	Remark              string `json:"remark"`
	ConnectTimeoutSec   int    `json:"connectTimeoutSec"`
	HandshakeTimeoutSec int    `json:"handshakeTimeoutSec"`
	KeepAliveSec        int    `json:"keepAliveSec"`
	Compression         bool   `json:"compression"`
	OverrideSettings    bool   `json:"overrideSettings"`
	ServerCount         int64  `json:"serverCount"`
	CPUCount            int    `json:"cpuCount"`
	MemoryBytes         int64  `json:"memoryBytes"`
	DiskBytes           int64  `json:"diskBytes"`
	SpecsCollectedAt    string `json:"specsCollectedAt"`
	CreatedAt           string `json:"createdAt"`
	UpdatedAt           string `json:"updatedAt"`
}

// SSHSessionInput 承载可编辑元数据与只写的密文字段。
type SSHSessionInput struct {
	Name                string `json:"name"`
	Host                string `json:"host"`
	Port                uint16 `json:"port"`
	Username            string `json:"username"`
	AuthType            string `json:"authType"`
	Secret              string `json:"secret"`
	Passphrase          string `json:"passphrase"`
	HostKeyPolicy       string `json:"hostKeyPolicy"`
	Group               string `json:"group"`
	Favourite           bool   `json:"favourite"`
	Remark              string `json:"remark"`
	ConnectTimeoutSec   int    `json:"connectTimeoutSec"`
	HandshakeTimeoutSec int    `json:"handshakeTimeoutSec"`
	KeepAliveSec        int    `json:"keepAliveSec"`
	Compression         bool   `json:"compression"`
	OverrideSettings    bool   `json:"overrideSettings"`
}

// SSHSessionResult 承载一份安全的 SSH Session DTO 或稳定错误。
type SSHSessionResult struct {
	Session *SSHSessionDTO `json:"session,omitempty"`
	Error   *apperror.DTO  `json:"error,omitempty"`
}

// SSHSessionListResult 承载安全的 SSH Session DTO 列表或稳定错误。
type SSHSessionListResult struct {
	Sessions []SSHSessionDTO `json:"sessions"`
	Error    *apperror.DTO   `json:"error,omitempty"`
}

// SSHPreflightDTO 承载经配置路由完成认证后的 SSH 请求往返统计。
// 主机规格不在预检中返回:它随 SSH Session 持久化,由 List/Get 直接读取。
type SSHPreflightDTO struct {
	Addresses        []string      `json:"addresses"`
	ConnectedAddress string        `json:"connectedAddress"`
	DNSDurationMs    float64       `json:"dnsDurationMs"`
	LatencyMs        float64       `json:"latencyMs"`
	MinLatencyMs     float64       `json:"minLatencyMs"`
	AverageLatencyMs float64       `json:"averageLatencyMs"`
	MaxLatencyMs     float64       `json:"maxLatencyMs"`
	SampleCount      int           `json:"sampleCount"`
	Effective        SSHConfigDTO  `json:"effective"`
	Error            *apperror.DTO `json:"error,omitempty"`
}

// SSHConfigDTO 承载预检或握手所用、已解析的非机密连接策略。
type SSHConfigDTO struct {
	Port                uint16 `json:"port"`
	ConnectTimeoutSec   int    `json:"connectTimeoutSec"`
	HandshakeTimeoutSec int    `json:"handshakeTimeoutSec"`
	KeepAliveSec        int    `json:"keepAliveSec"`
	Compression         bool   `json:"compression"`
	HostKeyPolicy       string `json:"hostKeyPolicy"`
}

// SSHConnectionTestDTO 承载 SSH 握手与命令通道的认证证据。
type SSHConnectionTestDTO struct {
	ServerVersion     string        `json:"serverVersion"`
	RemoteAddress     string        `json:"remoteAddress"`
	AuthType          string        `json:"authType"`
	ConnectDurationMs float64       `json:"connectDurationMs"`
	Error             *apperror.DTO `json:"error,omitempty"`
}

// durationMilliseconds 把耗时转换成高精度毫秒,供界面 DTO 使用。
func durationMilliseconds(duration time.Duration) float64 {
	if duration <= 0 {
		return 0
	}
	return math.Round(float64(duration)/float64(time.Millisecond)*1000) / 1000
}

// SSHSessionService 对外暴露 SSH Session 增删改查,并确保密文不进入响应与日志。
type SSHSessionService struct {
	manager  *service.SSHSessionManager
	store    repository.Store
	settings *appsettings.Manager
	clients  *service.SSHClientFactory
	logger   *applog.Logger
}

// NewSSHSessionService 创建桌面侧的 SSH Session 门面。
func NewSSHSessionService(manager *service.SSHSessionManager, store repository.Store, settings *appsettings.Manager, clients *service.SSHClientFactory, logger *applog.Logger) *SSHSessionService {
	return &SSHSessionService{manager: manager, store: store, settings: settings, clients: clients, logger: logger}
}

// List 按过滤条件返回不含密文的 SSH Session。
func (s *SSHSessionService) List(ctx context.Context, search, group string, favouriteOnly bool, limit, offset int) (result SSHSessionListResult) {
	defer s.recoverList(ctx, "SSHSessionService.List", &result)
	sessions, err := s.store.SSHSessions().List(ctx, repository.SSHSessionQuery{
		Search: search, Group: group, FavouriteOnly: favouriteOnly, Limit: limit, Offset: offset,
	})
	if err != nil {
		dto := apperror.ToDTO(err)
		return SSHSessionListResult{Error: &dto}
	}
	result.Sessions = make([]SSHSessionDTO, 0, len(sessions))
	for index := range sessions {
		result.Sessions = append(result.Sessions, s.toDTO(ctx, &sessions[index]))
	}
	return result
}

// Get 返回一个不含密文的 SSH Session。
func (s *SSHSessionService) Get(ctx context.Context, id string) (result SSHSessionResult) {
	defer s.recoverOne(ctx, "SSHSessionService.Get", &result)
	session, err := s.store.SSHSessions().Get(ctx, model.ID(id))
	if err != nil {
		dto := apperror.ToDTO(err)
		return SSHSessionResult{Error: &dto}
	}
	dto := s.toDTO(ctx, session)
	return SSHSessionResult{Session: &dto}
}

// Create 持久化一个新的 SSH Session 及其只写凭据输入。
func (s *SSHSessionService) Create(ctx context.Context, input SSHSessionInput) (result SSHSessionResult) {
	defer s.recoverOne(ctx, "SSHSessionService.Create", &result)
	command := input.command()
	defer clear(command.Secret)
	defer clear(command.Passphrase)
	session, err := s.manager.Create(ctx, command)
	if err != nil {
		dto := apperror.ToDTO(err)
		return SSHSessionResult{Error: &dto}
	}
	dto := s.toDTO(ctx, session)
	return SSHSessionResult{Session: &dto}
}

// Update 持久化元数据与可选的替换凭据输入。
func (s *SSHSessionService) Update(ctx context.Context, id string, input SSHSessionInput) (result SSHSessionResult) {
	defer s.recoverOne(ctx, "SSHSessionService.Update", &result)
	command := input.command()
	defer clear(command.Secret)
	defer clear(command.Passphrase)
	session, err := s.manager.Update(ctx, model.ID(id), command)
	if err != nil {
		dto := apperror.ToDTO(err)
		return SSHSessionResult{Error: &dto}
	}
	dto := s.toDTO(ctx, session)
	return SSHSessionResult{Session: &dto}
}

// EnsureHostSpecs 为尚无主机规格的 Session 采集并持久化容量信息。
// 这是主机规格唯一的采集入口:新建 Session、连接目标变更清空规格、以及迁移前的历史 Session 都走它,
// 每个 Session 最多采集一次。它与 Create/Update 分离,保证保存操作不会阻塞在一次完整 SSH 往返上。
func (s *SSHSessionService) EnsureHostSpecs(ctx context.Context, id string) (result SSHSessionResult) {
	defer s.recoverOne(ctx, "SSHSessionService.EnsureHostSpecs", &result)
	session, err := s.store.SSHSessions().Get(ctx, model.ID(id))
	if err != nil {
		dto := apperror.ToDTO(err)
		return SSHSessionResult{Error: &dto}
	}
	if _, err := s.manager.EnsureHostSpecs(ctx, s.clients, session, s.settings.Snapshot().SSH); err != nil {
		s.logger.Warn(ctx, "采集 SSH 主机规格失败", applog.Fields{
			"ssh_session_id": session.ID.String(), "error": err.Error(),
		})
		dto := apperror.ToDTO(err)
		return SSHSessionResult{Error: &dto}
	}
	dto := s.toDTO(ctx, session)
	return SSHSessionResult{Session: &dto}
}

// Delete 删除一个无引用的 SSH Session 及其凭据。
func (s *SSHSessionService) Delete(ctx context.Context, id string) (result ActionResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "SSHSessionService.Delete", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	sessionID := model.ID(id)
	if err := validateJumpHostDeletion(sessionID, s.settings.Snapshot().SSH); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	if err := s.manager.Delete(ctx, sessionID); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

func validateJumpHostDeletion(id model.ID, settings model.SSHSettings) error {
	if settings.DefaultJumpHostID != "" && model.ID(settings.DefaultJumpHostID) == id {
		return apperror.New(apperror.CodeValidationConflict, "SSH Session 正被用作默认 Jump Host").WithDetails(map[string]any{
			"usage": "default_jump_host",
		})
	}
	return nil
}

// Preflight 按配置的直连或 Jump Host 路由完成认证,并测量 SSH 请求往返时延。
func (s *SSHSessionService) Preflight(ctx context.Context, id string) (result SSHPreflightDTO) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "SSHSessionService.Preflight", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	session, err := s.store.SSHSessions().Get(ctx, model.ID(id))
	if err != nil {
		dto := apperror.ToDTO(err)
		return SSHPreflightDTO{Error: &dto}
	}
	preflight, err := s.clients.Preflight(ctx, session, s.settings.Snapshot().SSH)
	if err != nil {
		dto := apperror.ToDTO(err)
		return SSHPreflightDTO{Error: &dto}
	}
	return SSHPreflightDTO{
		Addresses: preflight.Addresses, ConnectedAddress: preflight.Address,
		DNSDurationMs: durationMilliseconds(preflight.DNSDuration),
		LatencyMs:     durationMilliseconds(preflight.Latency.Median), MinLatencyMs: durationMilliseconds(preflight.Latency.Minimum),
		AverageLatencyMs: durationMilliseconds(preflight.Latency.Average), MaxLatencyMs: durationMilliseconds(preflight.Latency.Maximum),
		SampleCount: preflight.Latency.Samples,
		Effective: SSHConfigDTO{
			Port: preflight.Config.Port, ConnectTimeoutSec: preflight.Config.ConnectTimeoutSec,
			HandshakeTimeoutSec: preflight.Config.HandshakeTimeoutSec, KeepAliveSec: preflight.Config.KeepAliveSec,
			Compression: preflight.Config.Compression, HostKeyPolicy: preflight.Config.HostKeyPolicy.String(),
		},
	}
}

// TestConnection 执行 SSH 握手、主机密钥校验、认证与一次空操作命令。
func (s *SSHSessionService) TestConnection(ctx context.Context, id string) (result SSHConnectionTestDTO) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "SSHSessionService.TestConnection", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	session, err := s.store.SSHSessions().Get(ctx, model.ID(id))
	if err != nil {
		dto := apperror.ToDTO(err)
		return SSHConnectionTestDTO{Error: &dto}
	}
	testResult, err := s.clients.TestConnection(ctx, session, s.settings.Snapshot().SSH)
	if err != nil {
		dto := apperror.ToDTO(err)
		return SSHConnectionTestDTO{Error: &dto}
	}
	return SSHConnectionTestDTO{
		ServerVersion: testResult.ServerVersion, RemoteAddress: testResult.RemoteAddress,
		AuthType: testResult.AuthType.String(), ConnectDurationMs: durationMilliseconds(testResult.ConnectDuration),
	}
}

// TestInput 测试未保存的 SSH Session 输入,不持久化草稿元数据与凭据。
func (s *SSHSessionService) TestInput(ctx context.Context, id string, input SSHSessionInput) (result SSHConnectionTestDTO) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "SSHSessionService.TestInput", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	command := input.command()
	defer clear(command.Secret)
	defer clear(command.Passphrase)
	session, credential, err := s.manager.PrepareConnectionTest(ctx, model.ID(id), command)
	if credential != nil {
		defer credential.Clear()
	}
	if err != nil {
		dto := apperror.ToDTO(err)
		return SSHConnectionTestDTO{Error: &dto}
	}
	testResult, err := s.clients.TestConnectionWithCredential(ctx, session, credential, s.settings.Snapshot().SSH)
	if err != nil {
		dto := apperror.ToDTO(err)
		return SSHConnectionTestDTO{Error: &dto}
	}
	return SSHConnectionTestDTO{
		ServerVersion: testResult.ServerVersion, RemoteAddress: testResult.RemoteAddress,
		AuthType: testResult.AuthType.String(), ConnectDurationMs: durationMilliseconds(testResult.ConnectDuration),
	}
}

func (s *SSHSessionService) toDTO(ctx context.Context, session *model.SSHSession) SSHSessionDTO {
	serverCount, _ := s.store.SSHSessions().CountServerReferences(ctx, session.ID)
	return SSHSessionDTO{
		ID: session.ID.String(), Name: session.Name, Host: session.Host, Port: session.Port,
		Username: session.Username, AuthType: session.AuthType.String(), HasCredential: session.CredentialID != nil,
		HostKeyPolicy: session.HostKeyPolicy.String(), Group: session.Group, Favourite: session.Favourite,
		Remark: session.Remark, ConnectTimeoutSec: session.ConnectTimeoutSec,
		HandshakeTimeoutSec: session.HandshakeTimeoutSec, KeepAliveSec: session.KeepAliveSec,
		Compression: session.Compression, ServerCount: serverCount,
		OverrideSettings: session.OverrideSettings,
		CPUCount:         session.HostSpecs.CPUCount, MemoryBytes: session.HostSpecs.MemoryBytes,
		DiskBytes: session.HostSpecs.DiskBytes, SpecsCollectedAt: formatOptionalTime(session.HostSpecs.CollectedAt),
		CreatedAt: session.CreatedAt.UTC().Format(time.RFC3339Nano), UpdatedAt: session.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

// formatOptionalTime 把缺失的时间戳渲染为空串,便于界面识别尚未采集的规格。
func formatOptionalTime(value *time.Time) string {
	if value == nil || value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func (i SSHSessionInput) command() service.SSHSessionCommand {
	return service.SSHSessionCommand{
		Name: i.Name, Host: i.Host, Port: i.Port, Username: i.Username,
		AuthType: enums.SSHAuthType(i.AuthType), Secret: []byte(i.Secret), Passphrase: []byte(i.Passphrase),
		HostKeyPolicy: enums.SSHHostKeyPolicy(i.HostKeyPolicy), Group: i.Group, Favourite: i.Favourite,
		Remark: i.Remark, ConnectTimeoutSec: i.ConnectTimeoutSec, HandshakeTimeoutSec: i.HandshakeTimeoutSec,
		KeepAliveSec: i.KeepAliveSec, Compression: i.Compression,
		OverrideSettings: i.OverrideSettings,
	}
}

func (s *SSHSessionService) recoverOne(ctx context.Context, boundary string, result *SSHSessionResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

func (s *SSHSessionService) recoverList(ctx context.Context, boundary string, result *SSHSessionListResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}
