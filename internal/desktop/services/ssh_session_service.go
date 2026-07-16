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

// SSHSessionDTO is the desktop-safe SSH Session representation without credential identifiers or secrets.
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
	CreatedAt           string `json:"createdAt"`
	UpdatedAt           string `json:"updatedAt"`
}

// SSHSessionInput contains editable metadata and write-only secret fields.
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

// SSHSessionResult contains one safe SSH Session DTO or a stable error.
type SSHSessionResult struct {
	Session *SSHSessionDTO `json:"session,omitempty"`
	Error   *apperror.DTO  `json:"error,omitempty"`
}

// SSHSessionListResult contains safe SSH Session DTOs or a stable error.
type SSHSessionListResult struct {
	Sessions []SSHSessionDTO `json:"sessions"`
	Error    *apperror.DTO   `json:"error,omitempty"`
}

// SSHPreflightDTO contains authenticated SSH request round-trip statistics through the configured route.
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

// SSHConfigDTO contains the resolved non-secret connection policy used by a preflight or handshake.
type SSHConfigDTO struct {
	Port                uint16 `json:"port"`
	ConnectTimeoutSec   int    `json:"connectTimeoutSec"`
	HandshakeTimeoutSec int    `json:"handshakeTimeoutSec"`
	KeepAliveSec        int    `json:"keepAliveSec"`
	Compression         bool   `json:"compression"`
	HostKeyPolicy       string `json:"hostKeyPolicy"`
}

// SSHConnectionTestDTO contains authenticated SSH handshake and command-channel evidence.
type SSHConnectionTestDTO struct {
	ServerVersion     string        `json:"serverVersion"`
	RemoteAddress     string        `json:"remoteAddress"`
	AuthType          string        `json:"authType"`
	ConnectDurationMs float64       `json:"connectDurationMs"`
	Error             *apperror.DTO `json:"error,omitempty"`
}

// durationMilliseconds converts elapsed durations to high-resolution milliseconds for UI DTOs.
func durationMilliseconds(duration time.Duration) float64 {
	if duration <= 0 {
		return 0
	}
	return math.Round(float64(duration)/float64(time.Millisecond)*1000) / 1000
}

// SSHSessionService exposes SSH Session CRUD while keeping secrets out of responses and logs.
type SSHSessionService struct {
	manager  *service.SSHSessionManager
	store    repository.Store
	settings *appsettings.Manager
	clients  *service.SSHClientFactory
	logger   *applog.Logger
}

// NewSSHSessionService creates the desktop SSH Session facade.
func NewSSHSessionService(manager *service.SSHSessionManager, store repository.Store, settings *appsettings.Manager, clients *service.SSHClientFactory, logger *applog.Logger) *SSHSessionService {
	return &SSHSessionService{manager: manager, store: store, settings: settings, clients: clients, logger: logger}
}

// List returns filtered SSH Sessions without secret material.
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

// Get returns one SSH Session without secret material.
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

// Create persists a new SSH Session and write-only credential input.
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

// Update persists metadata and optional replacement credential input.
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

// Delete removes an unreferenced SSH Session and its credential.
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

// Preflight authenticates the target through the configured direct or Jump Host route and measures SSH request RTT.
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

// TestConnection performs SSH handshake, host-key verification, authentication, and a no-op command.
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

// TestInput tests unsaved SSH Session input without persisting draft metadata or credential material.
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
		CreatedAt:        session.CreatedAt.UTC().Format(time.RFC3339Nano), UpdatedAt: session.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
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
