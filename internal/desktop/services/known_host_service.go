package services

import (
	"context"
	"encoding/base64"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
	"github.com/Cail-Gainey/MineOps/internal/service"
)

// KnownHostDTO is the desktop-safe trusted-key representation without raw public-key bytes.
type KnownHostDTO struct {
	ID             string `json:"id"`
	HostIdentifier string `json:"hostIdentifier"`
	Host           string `json:"host"`
	Port           uint16 `json:"port"`
	Algorithm      string `json:"algorithm"`
	Fingerprint    string `json:"fingerprint"`
	FirstSeenAt    string `json:"firstSeenAt"`
	LastSeenAt     string `json:"lastSeenAt"`
	ReplacedAt     string `json:"replacedAt,omitempty"`
	Active         bool   `json:"active"`
}

// ObservedHostKeyInput contains public host-key material supplied by the SSH verification boundary.
type ObservedHostKeyInput struct {
	HostIdentifier  string `json:"hostIdentifier"`
	Host            string `json:"host"`
	Port            uint16 `json:"port"`
	Algorithm       string `json:"algorithm"`
	PublicKeyBase64 string `json:"publicKeyBase64"`
	Fingerprint     string `json:"fingerprint"`
}

// HostKeyCheckResult contains a strict verification decision and safe existing record.
type HostKeyCheckResult struct {
	Decision string        `json:"decision"`
	Existing *KnownHostDTO `json:"existing,omitempty"`
	Error    *apperror.DTO `json:"error,omitempty"`
}

// KnownHostResult contains one trusted host or a stable error.
type KnownHostResult struct {
	KnownHost *KnownHostDTO `json:"knownHost,omitempty"`
	Error     *apperror.DTO `json:"error,omitempty"`
}

// KnownHostListResult contains trusted host history or a stable error.
type KnownHostListResult struct {
	KnownHosts []KnownHostDTO `json:"knownHosts"`
	Error      *apperror.DTO  `json:"error,omitempty"`
}

// KnownHostService exposes first-trust, fingerprint-change, history, and deletion workflows.
type KnownHostService struct {
	manager *service.KnownHostManager
	store   repository.Store
	logger  *applog.Logger
}

// NewKnownHostService creates the desktop Known Hosts facade.
func NewKnownHostService(manager *service.KnownHostManager, store repository.Store, logger *applog.Logger) *KnownHostService {
	return &KnownHostService{manager: manager, store: store, logger: logger}
}

// Check returns first_trust, trusted, or changed; changed always requires a separate confirmation.
func (s *KnownHostService) Check(ctx context.Context, hostIdentifier, fingerprint string) (result HostKeyCheckResult) {
	defer s.recoverCheck(ctx, "KnownHostService.Check", &result)
	check, err := s.manager.Check(ctx, hostIdentifier, fingerprint)
	if err != nil {
		dto := apperror.ToDTO(err)
		return HostKeyCheckResult{Error: &dto}
	}
	result.Decision = string(check.Decision)
	if check.Existing != nil {
		dto := knownHostDTO(check.Existing)
		result.Existing = &dto
	}
	return result
}

// TrustFirst persists a user-confirmed first-seen host key.
func (s *KnownHostService) TrustFirst(ctx context.Context, input ObservedHostKeyInput) (result KnownHostResult) {
	defer s.recoverOne(ctx, "KnownHostService.TrustFirst", &result)
	publicKey, err := base64.StdEncoding.DecodeString(input.PublicKeyBase64)
	if err != nil {
		dto := apperror.ToDTO(apperror.Wrap(apperror.CodeValidationInvalidArgument, "Known Host 公钥编码无效", err))
		return KnownHostResult{Error: &dto}
	}
	knownHost, err := s.manager.TrustFirst(ctx, input.HostIdentifier, input.Host, input.Port, input.Algorithm, publicKey, input.Fingerprint)
	clear(publicKey)
	if err != nil {
		dto := apperror.ToDTO(err)
		return KnownHostResult{Error: &dto}
	}
	dto := knownHostDTO(knownHost)
	return KnownHostResult{KnownHost: &dto}
}

// Replace persists a separately confirmed fingerprint change and retains history.
func (s *KnownHostService) Replace(ctx context.Context, input ObservedHostKeyInput) (result KnownHostResult) {
	defer s.recoverOne(ctx, "KnownHostService.Replace", &result)
	publicKey, err := base64.StdEncoding.DecodeString(input.PublicKeyBase64)
	if err != nil {
		dto := apperror.ToDTO(apperror.Wrap(apperror.CodeValidationInvalidArgument, "Known Host 公钥编码无效", err))
		return KnownHostResult{Error: &dto}
	}
	knownHost, err := s.manager.Replace(ctx, input.HostIdentifier, input.Host, input.Port, input.Algorithm, publicKey, input.Fingerprint)
	clear(publicKey)
	if err != nil {
		dto := apperror.ToDTO(err)
		return KnownHostResult{Error: &dto}
	}
	dto := knownHostDTO(knownHost)
	return KnownHostResult{KnownHost: &dto}
}

// List returns active and historical host keys for Settings management.
func (s *KnownHostService) List(ctx context.Context, search string, limit, offset int) (result KnownHostListResult) {
	defer s.recoverList(ctx, "KnownHostService.List", &result)
	knownHosts, err := s.store.KnownHosts().List(ctx, search, limit, offset)
	if err != nil {
		dto := apperror.ToDTO(err)
		return KnownHostListResult{Error: &dto}
	}
	result.KnownHosts = make([]KnownHostDTO, len(knownHosts))
	for index := range knownHosts {
		result.KnownHosts[index] = knownHostDTO(&knownHosts[index])
	}
	return result
}

// Delete removes one trusted key record from encrypted SQLite.
func (s *KnownHostService) Delete(ctx context.Context, id string) (result ActionResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "KnownHostService.Delete", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	if err := s.store.KnownHosts().Delete(ctx, model.ID(id)); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

func knownHostDTO(knownHost *model.KnownHost) KnownHostDTO {
	replacedAt := ""
	if knownHost.ReplacedAt != nil {
		replacedAt = knownHost.ReplacedAt.UTC().Format(time.RFC3339Nano)
	}
	return KnownHostDTO{
		ID: knownHost.ID.String(), HostIdentifier: knownHost.HostIdentifier, Host: knownHost.Host,
		Port: knownHost.Port, Algorithm: knownHost.Algorithm, Fingerprint: knownHost.Fingerprint,
		FirstSeenAt: knownHost.FirstSeenAt.UTC().Format(time.RFC3339Nano),
		LastSeenAt:  knownHost.LastSeenAt.UTC().Format(time.RFC3339Nano),
		ReplacedAt:  replacedAt, Active: knownHost.ReplacedAt == nil,
	}
}

func (s *KnownHostService) recoverCheck(ctx context.Context, boundary string, result *HostKeyCheckResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

func (s *KnownHostService) recoverOne(ctx context.Context, boundary string, result *KnownHostResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

func (s *KnownHostService) recoverList(ctx context.Context, boundary string, result *KnownHostListResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}
