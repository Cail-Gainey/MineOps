package service

import (
	"context"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
)

// HostKeyDecision distinguishes first trust, unchanged keys, and dangerous fingerprint changes.
type HostKeyDecision string

const (
	// HostKeyFirstTrust requires an explicit first-use confirmation before persistence.
	HostKeyFirstTrust HostKeyDecision = "first_trust"
	// HostKeyTrusted indicates the observed fingerprint matches the active record.
	HostKeyTrusted HostKeyDecision = "trusted"
	// HostKeyChanged requires a distinct high-risk replacement confirmation and defaults to rejection.
	HostKeyChanged HostKeyDecision = "changed"
)

// HostKeyCheck contains the host-key verification outcome and existing record when available.
type HostKeyCheck struct {
	Decision HostKeyDecision
	Existing *model.KnownHost
}

// KnownHostManager coordinates strict host-key lookup, first trust, and replacement history.
type KnownHostManager struct {
	clock model.Clock
	store repository.Store
}

// NewKnownHostManager creates the Known Hosts application service.
func NewKnownHostManager(clock model.Clock, store repository.Store) (*KnownHostManager, error) {
	if clock == nil || store == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Known Host Manager 依赖不能为空")
	}
	return &KnownHostManager{clock: clock, store: store}, nil
}

// Check compares an observed fingerprint against the active encrypted SQLite record.
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

// TrustFirst persists a host key only when no active record already exists.
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

// Replace records a confirmed fingerprint change while preserving the previous key as history.
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

// Touch updates a trusted host's last-seen timestamp after successful strict verification.
func (m *KnownHostManager) Touch(ctx context.Context, knownHost *model.KnownHost, now time.Time) error {
	if knownHost == nil {
		return apperror.New(apperror.CodeValidationRequired, "Known Host 不能为空")
	}
	knownHost.LastSeenAt = now.UTC()
	return m.store.KnownHosts().Update(ctx, knownHost)
}
