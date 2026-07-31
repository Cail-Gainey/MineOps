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

// StorageStatus 描述当前加密数据库与已排队的离线维护任务。
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

// StorageManager 对外暴露在线备份,以及安全的下次启动恢复与密钥轮换流程。
type StorageManager struct {
	backup        *sqlcipher.BackupManager
	databasePath  string
	dataDirectory string
	databaseID    string
	keyVersion    int
}

// NewStorageManager 为当前加密数据库创建存储与安全边界。
func NewStorageManager(backup *sqlcipher.BackupManager, databasePath, dataDirectory, databaseID string, keyVersion int) (*StorageManager, error) {
	if backup == nil || strings.TrimSpace(databasePath) == "" || strings.TrimSpace(dataDirectory) == "" || strings.TrimSpace(databaseID) == "" || keyVersion <= 0 {
		return nil, apperror.New(apperror.CodeValidationRequired, "StorageManager 依赖不能为空")
	}
	return &StorageManager{backup: backup, databasePath: databasePath, dataDirectory: dataDirectory, databaseID: databaseID, keyVersion: keyVersion}, nil
}

// Status 返回数据库体积、加密与密钥状态,以及已排队的离线维护任务。
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

// CreateBackup 在线创建一份一致的便携备份。
func (m *StorageManager) CreateBackup(ctx context.Context, destination string) (sqlcipher.BackupInfo, error) {
	return m.backup.Create(ctx, destination)
}

// ScheduleRestore 校验便携备份并排队到下次打开数据库之前安装。
func (m *StorageManager) ScheduleRestore(backupPath string) (sqlcipher.BackupInfo, error) {
	return sqlcipher.StageRestore(backupPath, m.dataDirectory)
}

// ScheduleKeyRotation 排队一次在下次打开数据库之前执行的 SQLCipher 密钥轮换。
func (m *StorageManager) ScheduleKeyRotation() error {
	return sqlcipher.StageKeyRotation(m.dataDirectory)
}

// ScheduleVacuum 排队一次在下次打开数据库前执行的 VACUUM,释放 SQLite 空闲页。
func (m *StorageManager) ScheduleVacuum() error {
	return sqlcipher.StageVacuum(m.dataDirectory)
}

// ScheduleDatabaseReset 排队在下次打开数据库前删除两个库文件与系统密钥。
func (m *StorageManager) ScheduleDatabaseReset() error {
	return sqlcipher.StageDatabaseReset(m.dataDirectory)
}

// CancelPendingMaintenance 清除全部已排队的离线数据库任务。
func (m *StorageManager) CancelPendingMaintenance() error {
	return sqlcipher.CancelPendingMaintenance(m.dataDirectory)
}

// OpenDataDirectory 用系统文件管理器打开受控数据目录。
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
