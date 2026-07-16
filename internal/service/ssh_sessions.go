package service

import (
	"context"
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
)

// SSHSessionCommand contains connection metadata plus write-only credential material.
type SSHSessionCommand struct {
	Name                string
	Host                string
	Port                uint16
	Username            string
	AuthType            enums.SSHAuthType
	Secret              []byte
	Passphrase          []byte
	HostKeyPolicy       enums.SSHHostKeyPolicy
	Group               string
	Favourite           bool
	Remark              string
	ConnectTimeoutSec   int
	HandshakeTimeoutSec int
	KeepAliveSec        int
	Compression         bool
	OverrideSettings    bool
}

// SSHSessionManager coordinates SSH Session and Credential persistence as complete transactions.
type SSHSessionManager struct {
	clock model.Clock
	store repository.Store
}

// NewSSHSessionManager creates the SSH Session application service.
func NewSSHSessionManager(clock model.Clock, store repository.Store) (*SSHSessionManager, error) {
	if clock == nil || store == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "SSH Session Manager 依赖不能为空")
	}
	return &SSHSessionManager{clock: clock, store: store}, nil
}

// Create transactionally persists a credential and its SSH Session metadata.
func (m *SSHSessionManager) Create(ctx context.Context, command SSHSessionCommand) (*model.SSHSession, error) {
	var credential *model.SSHCredential
	var credentialID *model.ID
	var err error
	if command.AuthType != enums.SSHAuthAgent {
		credential, err = model.NewSSHCredential(m.clock, command.AuthType, command.Secret, command.Passphrase)
		if err != nil {
			return nil, err
		}
		defer credential.Clear()
		credentialID = &credential.ID
	}
	session, err := model.NewSSHSession(m.clock, command.toSession(credentialID))
	if err != nil {
		return nil, err
	}
	if err := m.store.Transaction(ctx, func(registry repository.Registry) error {
		if credential != nil {
			if err := registry.SSHCredentials().Create(ctx, credential); err != nil {
				return err
			}
		}
		return registry.SSHSessions().Create(ctx, session)
	}); err != nil {
		return nil, err
	}
	return session, nil
}

// Update persists metadata and optionally replaces credential contents without returning secrets.
func (m *SSHSessionManager) Update(ctx context.Context, id model.ID, command SSHSessionCommand) (*model.SSHSession, error) {
	existing, err := m.store.SSHSessions().Get(ctx, id)
	if err != nil {
		return nil, err
	}
	var credential *model.SSHCredential
	var deleteCredentialID *model.ID
	credentialID := existing.CredentialID
	if command.AuthType == enums.SSHAuthAgent {
		deleteCredentialID = existing.CredentialID
		credentialID = nil
	} else if len(command.Secret) > 0 {
		credential, err = model.NewSSHCredential(m.clock, command.AuthType, command.Secret, command.Passphrase)
		if err != nil {
			return nil, err
		}
		defer credential.Clear()
		deleteCredentialID = existing.CredentialID
		credentialID = &credential.ID
	} else if credentialID == nil || existing.AuthType != command.AuthType {
		return nil, apperror.New(apperror.CodeValidationRequired, "切换 SSH 认证方式时必须提供新凭据")
	}

	updated := command.toSession(credentialID)
	updated.ID = existing.ID
	updated.CreatedAt = existing.CreatedAt
	updated.UpdatedAt = m.clock.Now().UTC()
	if err := updated.Validate(); err != nil {
		return nil, err
	}
	if err := m.store.Transaction(ctx, func(registry repository.Registry) error {
		if credential != nil {
			if err := registry.SSHCredentials().Create(ctx, credential); err != nil {
				return err
			}
		}
		if err := registry.SSHSessions().Update(ctx, &updated); err != nil {
			return err
		}
		if deleteCredentialID != nil && (credentialID == nil || *deleteCredentialID != *credentialID) {
			return registry.SSHCredentials().Delete(ctx, *deleteCredentialID)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return &updated, nil
}

// Delete removes an unreferenced SSH Session and its credential in one transaction.
func (m *SSHSessionManager) Delete(ctx context.Context, id model.ID) error {
	session, err := m.store.SSHSessions().Get(ctx, id)
	if err != nil {
		return err
	}
	references, err := m.store.SSHSessions().CountServerReferences(ctx, id)
	if err != nil {
		return err
	}
	if references > 0 {
		return apperror.New(apperror.CodeValidationConflict, "SSH Session 已被 Minecraft Server 引用").WithDetails(map[string]any{
			"serverCount": references,
		})
	}
	return m.store.Transaction(ctx, func(registry repository.Registry) error {
		if err := registry.SSHSessions().Delete(ctx, id); err != nil {
			return err
		}
		if session.CredentialID != nil {
			return registry.SSHCredentials().Delete(ctx, *session.CredentialID)
		}
		return nil
	})
}

// Get returns one SSH Session without exposing its encrypted credential material.
func (m *SSHSessionManager) Get(ctx context.Context, id model.ID) (*model.SSHSession, error) {
	if !id.Valid() {
		return nil, apperror.New(apperror.CodeValidationInvalidArgument, "SSH Session ID 无效")
	}
	return m.store.SSHSessions().Get(ctx, id)
}

// LoadCredential retrieves secret material for one authentication attempt; the caller must defer Clear.
func (m *SSHSessionManager) LoadCredential(ctx context.Context, session *model.SSHSession) (*model.SSHCredential, error) {
	if session == nil || session.AuthType == enums.SSHAuthAgent || session.CredentialID == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "SSH Session 没有可加载的数据库凭据")
	}
	credential, err := m.store.SSHCredentials().Get(ctx, *session.CredentialID)
	if err != nil {
		return nil, err
	}
	if credential.AuthType != session.AuthType {
		credential.Clear()
		return nil, apperror.New(apperror.CodeValidationConflict, "SSH Session 与 Credential 认证类型不一致")
	}
	return credential, nil
}

// PrepareConnectionTest builds a validated in-memory Session and Credential without persisting the draft.
func (m *SSHSessionManager) PrepareConnectionTest(ctx context.Context, id model.ID, command SSHSessionCommand) (*model.SSHSession, *model.SSHCredential, error) {
	var existing *model.SSHSession
	var err error
	if id != "" {
		if !id.Valid() {
			return nil, nil, apperror.New(apperror.CodeValidationInvalidArgument, "SSH Session ID 无效")
		}
		existing, err = m.store.SSHSessions().Get(ctx, id)
		if err != nil {
			return nil, nil, err
		}
	}

	var credential *model.SSHCredential
	var credentialID *model.ID
	if command.AuthType != enums.SSHAuthAgent {
		if len(command.Secret) > 0 {
			credential, err = model.NewSSHCredential(m.clock, command.AuthType, command.Secret, command.Passphrase)
		} else if existing != nil && existing.AuthType == command.AuthType && existing.CredentialID != nil {
			credential, err = m.LoadCredential(ctx, existing)
		} else {
			return nil, nil, apperror.New(apperror.CodeValidationRequired, "测试密码或私钥认证时必须提供凭据")
		}
		if err != nil {
			return nil, nil, err
		}
		credentialID = &credential.ID
	}

	draft := command.toSession(credentialID)
	if strings.TrimSpace(draft.Name) == "" {
		draft.Name = "SSH 连接测试"
	}
	if existing == nil {
		session, createErr := model.NewSSHSession(m.clock, draft)
		if createErr != nil {
			credential.Clear()
			return nil, nil, createErr
		}
		return session, credential, nil
	}
	draft.ID = existing.ID
	draft.CreatedAt = existing.CreatedAt
	draft.UpdatedAt = m.clock.Now().UTC()
	if err := draft.Validate(); err != nil {
		credential.Clear()
		return nil, nil, err
	}
	return &draft, credential, nil
}

func (c SSHSessionCommand) toSession(credentialID *model.ID) model.SSHSession {
	return model.SSHSession{
		Name: c.Name, Host: c.Host, Port: c.Port, Username: c.Username, AuthType: c.AuthType,
		CredentialID: credentialID, HostKeyPolicy: c.HostKeyPolicy, Group: c.Group,
		Favourite: c.Favourite, Remark: c.Remark, ConnectTimeoutSec: c.ConnectTimeoutSec,
		HandshakeTimeoutSec: c.HandshakeTimeoutSec, KeepAliveSec: c.KeepAliveSec, Compression: c.Compression,
		OverrideSettings: c.OverrideSettings,
		CreatedAt:        time.Time{}, UpdatedAt: time.Time{},
	}
}
