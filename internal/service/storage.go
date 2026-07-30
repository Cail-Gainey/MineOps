package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/infrastructure/sqlcipher"
)

// StorageStatus describes the active encrypted database and staged offline maintenance.
type StorageStatus struct {
	DatabasePath       string                       `json:"databasePath"`
	DataDirectory      string                       `json:"dataDirectory"`
	DatabaseBytes      int64                        `json:"databaseBytes"`
	Encrypted          bool                         `json:"encrypted"`
	AutomaticKeyStore  bool                         `json:"automaticKeyStore"`
	AutomaticOpen      bool                         `json:"automaticOpen"`
	DatabaseID         string                       `json:"databaseID"`
	KeyVersion         int                          `json:"keyVersion"`
	PendingMaintenance sqlcipher.PendingMaintenance `json:"pendingMaintenance"`
}

// StorageManager exposes online backup and safe next-start restore or key-rotation workflows.
type StorageManager struct {
	backup        *sqlcipher.BackupManager
	databasePath  string
	dataDirectory string
	databaseID    string
	keyVersion    int
}

// NewStorageManager creates the storage and security boundary for one active encrypted database.
func NewStorageManager(backup *sqlcipher.BackupManager, databasePath, dataDirectory, databaseID string, keyVersion int) (*StorageManager, error) {
	if backup == nil || strings.TrimSpace(databasePath) == "" || strings.TrimSpace(dataDirectory) == "" || strings.TrimSpace(databaseID) == "" || keyVersion <= 0 {
		return nil, apperror.New(apperror.CodeValidationRequired, "StorageManager 依赖不能为空")
	}
	return &StorageManager{backup: backup, databasePath: databasePath, dataDirectory: dataDirectory, databaseID: databaseID, keyVersion: keyVersion}, nil
}

// Status returns database size, encryption/key state, and staged offline maintenance.
func (m *StorageManager) Status() (StorageStatus, error) {
	info, err := os.Stat(m.databasePath)
	if err != nil {
		return StorageStatus{}, apperror.Wrap(apperror.CodeIOReadFailed, "读取数据库文件信息失败", err)
	}
	pending, err := sqlcipher.ReadPendingMaintenance(m.dataDirectory)
	if err != nil {
		return StorageStatus{}, err
	}
	return StorageStatus{
		DatabasePath: m.databasePath, DataDirectory: m.dataDirectory, DatabaseBytes: info.Size(),
		Encrypted: true, AutomaticKeyStore: true, AutomaticOpen: true,
		DatabaseID: m.databaseID, KeyVersion: m.keyVersion, PendingMaintenance: pending,
	}, nil
}

// CreateBackup creates one consistent online portable backup.
func (m *StorageManager) CreateBackup(ctx context.Context, destination string) (sqlcipher.BackupInfo, error) {
	return m.backup.Create(ctx, destination)
}

// ScheduleRestore verifies and stages a portable backup for installation before the next database open.
func (m *StorageManager) ScheduleRestore(backupPath string) (sqlcipher.BackupInfo, error) {
	return sqlcipher.StageRestore(backupPath, m.dataDirectory)
}

// ScheduleKeyRotation requests a SQLCipher key rotation before the next database open.
func (m *StorageManager) ScheduleKeyRotation() error {
	return sqlcipher.StageKeyRotation(m.dataDirectory)
}

// ScheduleVacuum requests a VACUUM before the next database open to release SQLite free pages.
func (m *StorageManager) ScheduleVacuum() error {
	return sqlcipher.StageVacuum(m.dataDirectory)
}

// CancelPendingMaintenance removes all staged offline database work.
func (m *StorageManager) CancelPendingMaintenance() error {
	return sqlcipher.CancelPendingMaintenance(m.dataDirectory)
}

// OpenDataDirectory opens the controlled data directory with the platform file manager.
func (m *StorageManager) OpenDataDirectory(ctx context.Context) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.CommandContext(ctx, "open", m.dataDirectory)
	case "windows":
		command = exec.CommandContext(ctx, "explorer", filepath.Clean(m.dataDirectory))
	default:
		command = exec.CommandContext(ctx, "xdg-open", m.dataDirectory)
	}
	if err := command.Start(); err != nil {
		return apperror.Wrap(apperror.CodeIOReadFailed, "打开数据目录失败", err)
	}
	_ = command.Process.Release()
	return nil
}
