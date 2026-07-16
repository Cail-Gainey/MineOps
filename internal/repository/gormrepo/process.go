package gormrepo

import (
	"context"
	"errors"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"gorm.io/gorm"
)

// ProcessIdentityRecord is the encrypted SQLite remote PID identity representation.
type ProcessIdentityRecord struct {
	ID                 string `gorm:"primaryKey;size:36"`
	ServerID           string `gorm:"uniqueIndex;size:36"`
	SSHSessionID       string `gorm:"index;size:36"`
	PID                int
	ProcessGroupID     int
	LinuxStartTicks    uint64
	CommandFingerprint string `gorm:"size:128"`
	WorkingDirectory   string
	TmuxSession        string `gorm:"size:128"`
	ConsoleFIFO        string
	ConsoleLog         string
	State              string `gorm:"index;size:24"`
	StartedAt          time.Time
	LastProbedAt       time.Time `gorm:"index"`
	ExitedAt           *time.Time
	ExitCode           *int
	LastOutput         string
	CreatedAt          time.Time
	UpdatedAt          time.Time `gorm:"index"`
}

type processIdentityRepository struct{ database *gorm.DB }

func (r *processIdentityRepository) Save(ctx context.Context, identity *model.RemoteProcessIdentity) error {
	if identity == nil {
		return apperror.New(apperror.CodeValidationRequired, "Remote Process Identity 不能为空")
	}
	if err := identity.Validate(); err != nil {
		return err
	}
	record := processIdentityToRecord(identity)
	var existing ProcessIdentityRecord
	if err := r.database.WithContext(ctx).Select("id").First(&existing, "server_id = ?", record.ServerID).Error; err == nil {
		record.ID = existing.ID
		identity.ID = model.ID(existing.ID)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return apperror.Wrap(apperror.CodeIOReadFailed, "查询现有 Remote Process Identity 失败", err)
	}
	if err := r.database.WithContext(ctx).Save(&record).Error; err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "保存 Remote Process Identity 失败", err)
	}
	return nil
}

func (r *processIdentityRepository) GetByServer(ctx context.Context, serverID model.ID) (*model.RemoteProcessIdentity, error) {
	var record ProcessIdentityRecord
	if err := r.database.WithContext(ctx).First(&record, "server_id = ?", serverID.String()).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.New(apperror.CodeIONotFound, "Remote Process Identity 不存在")
		}
		return nil, apperror.Wrap(apperror.CodeIOReadFailed, "查询 Remote Process Identity 失败", err)
	}
	identity := recordToProcessIdentity(record)
	return &identity, nil
}

func (r *processIdentityRepository) ListActive(ctx context.Context) ([]model.RemoteProcessIdentity, error) {
	var records []ProcessIdentityRecord
	if err := r.database.WithContext(ctx).Where("state = ?", enums.RemoteProcessRunning.String()).Order("updated_at asc").Find(&records).Error; err != nil {
		return nil, apperror.Wrap(apperror.CodeIOReadFailed, "查询活动 Remote Process Identities 失败", err)
	}
	result := make([]model.RemoteProcessIdentity, len(records))
	for index, record := range records {
		result[index] = recordToProcessIdentity(record)
	}
	return result, nil
}

func (r *processIdentityRepository) DeleteByServer(ctx context.Context, serverID model.ID) error {
	result := r.database.WithContext(ctx).Delete(&ProcessIdentityRecord{}, "server_id = ?", serverID.String())
	if result.Error != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "删除 Remote Process Identity 失败", result.Error)
	}
	return nil
}

func processIdentityToRecord(identity *model.RemoteProcessIdentity) ProcessIdentityRecord {
	return ProcessIdentityRecord{
		ID: identity.ID.String(), ServerID: identity.ServerID.String(), SSHSessionID: identity.SSHSessionID.String(),
		PID: identity.PID, ProcessGroupID: identity.ProcessGroupID, LinuxStartTicks: identity.LinuxStartTicks,
		CommandFingerprint: identity.CommandFingerprint, WorkingDirectory: identity.WorkingDirectory,
		TmuxSession: identity.TmuxSession, ConsoleFIFO: identity.ConsoleFIFO, ConsoleLog: identity.ConsoleLog, State: identity.State.String(),
		StartedAt: identity.StartedAt, LastProbedAt: identity.LastProbedAt, ExitedAt: identity.ExitedAt,
		ExitCode: identity.ExitCode, LastOutput: identity.LastOutput, CreatedAt: identity.CreatedAt, UpdatedAt: identity.UpdatedAt,
	}
}

func recordToProcessIdentity(record ProcessIdentityRecord) model.RemoteProcessIdentity {
	return model.RemoteProcessIdentity{
		ID: model.ID(record.ID), ServerID: model.ID(record.ServerID), SSHSessionID: model.ID(record.SSHSessionID),
		PID: record.PID, ProcessGroupID: record.ProcessGroupID, LinuxStartTicks: record.LinuxStartTicks,
		CommandFingerprint: record.CommandFingerprint, WorkingDirectory: record.WorkingDirectory,
		TmuxSession: record.TmuxSession, ConsoleFIFO: record.ConsoleFIFO, ConsoleLog: record.ConsoleLog, State: enums.RemoteProcessState(record.State),
		StartedAt: record.StartedAt, LastProbedAt: record.LastProbedAt, ExitedAt: record.ExitedAt,
		ExitCode: record.ExitCode, LastOutput: record.LastOutput, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}
