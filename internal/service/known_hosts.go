package service

import (
	"context"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
)

// HostKeyDecision 区分首次信任、密钥未变与危险的指纹变更。
type HostKeyDecision string

const (
	// HostKeyFirstTrust 表示持久化之前需要显式的首次使用确认。
	HostKeyFirstTrust HostKeyDecision = "first_trust"
	// HostKeyTrusted 表示观测到的指纹与当前生效记录一致。
	HostKeyTrusted HostKeyDecision = "trusted"
	// HostKeyChanged 表示需要单独的高风险替换确认,默认拒绝。
	HostKeyChanged HostKeyDecision = "changed"
)

// HostKeyCheck 承载主机密钥校验结论,以及存在时的既有记录。
type HostKeyCheck struct {
	Decision HostKeyDecision
	Existing *model.KnownHost
}

// KnownHostManager 统筹严格的主机密钥查找、首次信任与替换历史。
type KnownHostManager struct {
	clock model.Clock
	store repository.Store
}

// NewKnownHostManager 创建 Known Hosts 应用服务。
func NewKnownHostManager(clock model.Clock, store repository.Store) (*KnownHostManager, error) {
	if clock == nil || store == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Known Host Manager 依赖不能为空")
	}
	return &KnownHostManager{clock: clock, store: store}, nil
}

// Check 把观测到的指纹与加密 SQLite 中的生效记录做比对。
func (m *KnownHostManager) Check(ctx context.Context, hostIdentifier, fingerprint string) (HostKeyCheck, error) {
	existing, err := m.store.KnownHosts().FindActive(ctx, hostIdentifier)
	if err != nil {
		if dto := apperror.ToDTO(err); dto.Code == apperror.CodeIONotFound.String() {
			return HostKeyCheck{Decision: HostKeyFirstTrust}, nil
		}
		return HostKeyCheck{}, err
	}
	if existing.Fingerprint != fingerprint {
		return HostKeyCheck{Decision: HostKeyChanged, Existing: existing}, nil
	}
	existing.LastSeenAt = m.clock.Now().UTC()
	if err := m.store.KnownHosts().Update(ctx, existing); err != nil {
		return HostKeyCheck{}, err
	}
	return HostKeyCheck{Decision: HostKeyTrusted, Existing: existing}, nil
}

// TrustFirst 仅在不存在生效记录时持久化一条主机密钥。
func (m *KnownHostManager) TrustFirst(ctx context.Context, hostIdentifier, host string, port uint16, algorithm string, publicKey []byte, fingerprint string) (*model.KnownHost, error) {
	check, err := m.Check(ctx, hostIdentifier, fingerprint)
	if err != nil {
		return nil, err
	}
	if check.Decision != HostKeyFirstTrust {
		return nil, apperror.New(apperror.CodeValidationConflict, "Known Host 已存在，不能作为首次信任保存")
	}
	knownHost, err := model.NewKnownHost(m.clock, hostIdentifier, host, port, algorithm, publicKey, fingerprint)
	if err != nil {
		return nil, err
	}
	if err := m.store.KnownHosts().Create(ctx, knownHost); err != nil {
		return nil, err
	}
	return knownHost, nil
}

// Replace 记录一次已确认的指纹变更,并把旧密钥保留为历史。
func (m *KnownHostManager) Replace(ctx context.Context, hostIdentifier, host string, port uint16, algorithm string, publicKey []byte, fingerprint string) (*model.KnownHost, error) {
	check, err := m.Check(ctx, hostIdentifier, fingerprint)
	if err != nil {
		return nil, err
	}
	if check.Decision != HostKeyChanged || check.Existing == nil {
		return nil, apperror.New(apperror.CodeValidationConflict, "Known Host 指纹没有发生需要确认的变化")
	}
	replacement, err := model.NewKnownHost(m.clock, hostIdentifier, host, port, algorithm, publicKey, fingerprint)
	if err != nil {
		return nil, err
	}
	if err := m.store.KnownHosts().Replace(ctx, check.Existing, replacement); err != nil {
		return nil, err
	}
	return replacement, nil
}

// Touch 在严格校验成功后更新受信任主机的最近可见时间。
func (m *KnownHostManager) Touch(ctx context.Context, knownHost *model.KnownHost, now time.Time) error {
	if knownHost == nil {
		return apperror.New(apperror.CodeValidationRequired, "Known Host 不能为空")
	}
	knownHost.LastSeenAt = now.UTC()
	return m.store.KnownHosts().Update(ctx, knownHost)
}
