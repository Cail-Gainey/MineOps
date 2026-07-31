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

// KnownHostDTO 是桌面侧安全的受信任密钥表示,不含公钥原始字节。
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

// ObservedHostKeyInput 承载 SSH 校验边界提供的主机公钥材料。
type ObservedHostKeyInput struct {
	HostIdentifier  string `json:"hostIdentifier"`
	Host            string `json:"host"`
	Port            uint16 `json:"port"`
	Algorithm       string `json:"algorithm"`
	PublicKeyBase64 string `json:"publicKeyBase64"`
	Fingerprint     string `json:"fingerprint"`
}

// HostKeyCheckResult 承载严格校验结论与安全的既有记录。
type HostKeyCheckResult struct {
	Decision string        `json:"decision"`
	Existing *KnownHostDTO `json:"existing,omitempty"`
	Error    *apperror.DTO `json:"error,omitempty"`
}

// KnownHostResult 承载一条受信任主机或稳定错误。
type KnownHostResult struct {
	KnownHost *KnownHostDTO `json:"knownHost,omitempty"`
	Error     *apperror.DTO `json:"error,omitempty"`
}

// KnownHostListResult 承载受信任主机历史或稳定错误。
type KnownHostListResult struct {
	KnownHosts []KnownHostDTO `json:"knownHosts"`
	Error      *apperror.DTO  `json:"error,omitempty"`
}

// KnownHostService 对外暴露首次信任、指纹变更、历史与删除流程。
type KnownHostService struct {
	manager *service.KnownHostManager
	store   repository.Store
	logger  *applog.Logger
}

// NewKnownHostService 创建桌面侧的 Known Hosts 门面。
func NewKnownHostService(manager *service.KnownHostManager, store repository.Store, logger *applog.Logger) *KnownHostService {
	return &KnownHostService{manager: manager, store: store, logger: logger}
}

// Check 返回 first_trust、trusted 或 changed;changed 一律需要单独确认。
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

// TrustFirst 持久化一条用户确认过的首见主机密钥。
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

// Replace 持久化一次单独确认过的指纹变更并保留历史。
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

// List 返回生效与历史主机密钥,供设置页管理。
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

// Delete 从加密 SQLite 中删除一条受信任密钥记录。
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
