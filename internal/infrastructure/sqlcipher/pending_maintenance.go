package sqlcipher

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/constants"
)

const (
	pendingRestoreFilename     = ".pending-restore.mineops-backup"
	pendingKeyRotationFilename = ".pending-key-rotation"
	pendingVacuumFilename      = ".pending-vacuum"
	pendingResetFilename       = ".pending-reset"
)

// PendingMaintenance describes offline database work staged for the next application start.
type PendingMaintenance struct {
	RestorePending     bool        `json:"restorePending"`
	KeyRotationPending bool        `json:"keyRotationPending"`
	VacuumPending      bool        `json:"vacuumPending"`
	ResetPending       bool        `json:"resetPending"`
	RestoreBackup      *BackupInfo `json:"restoreBackup,omitempty"`
}

// StageRestore verifies and atomically copies a portable backup for offline installation on next start.
func StageRestore(backupPath, dataDirectory string) (BackupInfo, error) {
	info, err := InspectPortableBackup(backupPath)
	if err != nil {
		return BackupInfo{}, err
	}
	if err := os.MkdirAll(dataDirectory, 0o700); err != nil {
		return BackupInfo{}, apperror.Wrap(apperror.CodeIOWriteFailed, "创建数据目录失败", err)
	}
	destination := filepath.Join(dataDirectory, pendingRestoreFilename)
	temporary, err := os.CreateTemp(dataDirectory, ".pending-restore-")
	if err != nil {
		return BackupInfo{}, apperror.Wrap(apperror.CodeIOWriteFailed, "创建恢复排队文件失败", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return BackupInfo{}, apperror.Wrap(apperror.CodeIOPermissionDenied, "设置恢复排队文件权限失败", err)
	}
	source, err := os.Open(backupPath)
	if err != nil {
		_ = temporary.Close()
		return BackupInfo{}, apperror.Wrap(apperror.CodeIOReadFailed, "打开恢复备份失败", err)
	}
	_, copyErr := io.Copy(temporary, source)
	closeSourceErr := source.Close()
	closeTemporaryErr := temporary.Close()
	if copyErr != nil || closeSourceErr != nil || closeTemporaryErr != nil {
		return BackupInfo{}, apperror.Wrap(apperror.CodeIOWriteFailed, "复制恢复备份失败", errors.Join(copyErr, closeSourceErr, closeTemporaryErr))
	}
	if err := os.Remove(destination); err != nil && !os.IsNotExist(err) {
		return BackupInfo{}, apperror.Wrap(apperror.CodeIOWriteFailed, "替换恢复排队文件失败", err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return BackupInfo{}, apperror.Wrap(apperror.CodeIOWriteFailed, "提交恢复排队文件失败", err)
	}
	info.Path = destination
	return info, nil
}

// StageKeyRotation requests an offline SQLCipher key rotation before the next database open.
func StageKeyRotation(dataDirectory string) error {
	if err := os.MkdirAll(dataDirectory, 0o700); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建数据目录失败", err)
	}
	marker := filepath.Join(dataDirectory, pendingKeyRotationFilename)
	file, err := os.OpenFile(marker, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建密钥轮换排队标记失败", err)
	}
	return file.Close()
}

// StageVacuum requests an offline VACUUM before the next database open.
// SQLite 的空闲页永远不会自动还给文件系统,删除历史数据只会让文件保持在历史高水位;
// VACUUM 需要独占锁并临时占用与库等大的磁盘空间,GB 级库会跑上几分钟,
// 因此和恢复、密钥轮换一样排队到下次启动、在数据库投入正常使用之前离线执行。
func StageVacuum(dataDirectory string) error {
	if err := os.MkdirAll(dataDirectory, 0o700); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建数据目录失败", err)
	}
	marker := filepath.Join(dataDirectory, pendingVacuumFilename)
	file, err := os.OpenFile(marker, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建存储整理排队标记失败", err)
	}
	return file.Close()
}

// StageDatabaseReset requests deleting both databases and the stored key before the next database open.
// 恢复出厂只能离线做:应用运行期间两个库都被连接池持有,而且服务还在往里写。
// 排队到下次启动、在任何连接建立之前删文件并清掉系统密钥,Bootstrap 随后会重建空库并生成新密钥。
func StageDatabaseReset(dataDirectory string) error {
	if err := os.MkdirAll(dataDirectory, 0o700); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建数据目录失败", err)
	}
	marker := filepath.Join(dataDirectory, pendingResetFilename)
	file, err := os.OpenFile(marker, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建清空数据库排队标记失败", err)
	}
	return file.Close()
}

// ReadPendingMaintenance returns staged offline work without changing it.
func ReadPendingMaintenance(dataDirectory string) (PendingMaintenance, error) {
	result := PendingMaintenance{}
	restorePath := filepath.Join(dataDirectory, pendingRestoreFilename)
	if _, err := os.Stat(restorePath); err == nil {
		result.RestorePending = true
		info, inspectErr := InspectPortableBackup(restorePath)
		if inspectErr != nil {
			return PendingMaintenance{}, inspectErr
		}
		result.RestoreBackup = &info
	} else if !os.IsNotExist(err) {
		return PendingMaintenance{}, apperror.Wrap(apperror.CodeIOReadFailed, "读取恢复排队状态失败", err)
	}
	if _, err := os.Stat(filepath.Join(dataDirectory, pendingKeyRotationFilename)); err == nil {
		result.KeyRotationPending = true
	} else if !os.IsNotExist(err) {
		return PendingMaintenance{}, apperror.Wrap(apperror.CodeIOReadFailed, "读取密钥轮换排队状态失败", err)
	}
	if _, err := os.Stat(filepath.Join(dataDirectory, pendingVacuumFilename)); err == nil {
		result.VacuumPending = true
	} else if !os.IsNotExist(err) {
		return PendingMaintenance{}, apperror.Wrap(apperror.CodeIOReadFailed, "读取存储整理排队状态失败", err)
	}
	if _, err := os.Stat(filepath.Join(dataDirectory, pendingResetFilename)); err == nil {
		result.ResetPending = true
	} else if !os.IsNotExist(err) {
		return PendingMaintenance{}, apperror.Wrap(apperror.CodeIOReadFailed, "读取清空数据库排队状态失败", err)
	}
	return result, nil
}

// CancelPendingMaintenance removes staged restore, key-rotation, vacuum, and reset work.
func CancelPendingMaintenance(dataDirectory string) error {
	var firstError error
	for _, name := range []string{pendingRestoreFilename, pendingKeyRotationFilename, pendingVacuumFilename, pendingResetFilename} {
		if err := os.Remove(filepath.Join(dataDirectory, name)); err != nil && !os.IsNotExist(err) && firstError == nil {
			firstError = err
		}
	}
	return firstError
}

// ApplyPendingMaintenance performs staged reset, restore, key rotation, and vacuum before opening the active database.
func ApplyPendingMaintenance(ctx context.Context, dataDirectory, databasePath string, keyStore KeyStore) error {
	pending, err := ReadPendingMaintenance(dataDirectory)
	if err != nil {
		return err
	}
	if pending.ResetPending {
		// 清空数据库会把库文件整个删掉,排在它后面的恢复、密钥轮换、整理都失去意义,一并取消。
		// 标记先删再执行:重置失败时保留标记会让每次启动都重试同一个失败操作,把应用永久卡在启动阶段。
		if err := CancelPendingMaintenance(dataDirectory); err != nil {
			return apperror.Wrap(apperror.CodeIOWriteFailed, "清理清空数据库排队标记失败", err)
		}
		return resetDatabasesOffline(ctx, dataDirectory, databasePath, keyStore)
	}
	if pending.RestorePending {
		if _, err := RestorePortableBackup(ctx, filepath.Join(dataDirectory, pendingRestoreFilename), databasePath, keyStore); err != nil {
			return err
		}
		if err := os.Remove(filepath.Join(dataDirectory, pendingRestoreFilename)); err != nil && !os.IsNotExist(err) {
			return apperror.Wrap(apperror.CodeIOWriteFailed, "清理恢复排队文件失败", err)
		}
	}
	if pending.KeyRotationPending {
		if _, err := RotateDatabaseKeyOffline(ctx, databasePath, keyStore); err != nil {
			return err
		}
		if err := os.Remove(filepath.Join(dataDirectory, pendingKeyRotationFilename)); err != nil && !os.IsNotExist(err) {
			return apperror.Wrap(apperror.CodeIOWriteFailed, "清理密钥轮换排队标记失败", err)
		}
	}
	if pending.VacuumPending {
		// 排队标记先删再执行:VACUUM 失败(多为磁盘空间不足)时数据库本身完好无损,
		// 保留标记只会让每次启动都重试同一个必然失败的长操作,把应用永久卡在启动阶段。
		if err := os.Remove(filepath.Join(dataDirectory, pendingVacuumFilename)); err != nil && !os.IsNotExist(err) {
			return apperror.Wrap(apperror.CodeIOWriteFailed, "清理存储整理排队标记失败", err)
		}
		if err := vacuumDatabaseOffline(ctx, databasePath, keyStore); err != nil {
			return err
		}
		// 监控库通常是两个库里更大的那个,一并整理,否则缩短保留期释放的页永远还不回磁盘。
		if err := vacuumMetricsDatabaseOffline(ctx, filepath.Join(dataDirectory, constants.MetricsDatabaseFileName)); err != nil {
			return err
		}
	}
	return nil
}

// resetDatabasesOffline deletes both database files and the stored key so bootstrap rebuilds an empty database.
func resetDatabasesOffline(ctx context.Context, dataDirectory, databasePath string, keyStore KeyStore) error {
	paths := []string{databasePath, filepath.Join(dataDirectory, constants.MetricsDatabaseFileName)}
	for _, path := range paths {
		// WAL 与 SHM 必须一起删:只删主库文件会让残留 WAL 在重建后被当成同一个库的日志重放。
		for _, suffix := range []string{"", "-wal", "-shm"} {
			if err := os.Remove(path + suffix); err != nil && !os.IsNotExist(err) {
				return apperror.Wrap(apperror.CodeIOWriteFailed, "删除数据库文件失败", err)
			}
		}
	}
	// 删掉系统密钥:Bootstrap 发现密钥缺失且库文件不存在时会重新生成密钥并建空库。
	// 保留旧密钥会让 Bootstrap 走「密钥存在但数据库缺失」分支并要求从备份恢复,应用将无法启动。
	if err := keyStore.Delete(ctx); err != nil {
		return err
	}
	return nil
}

func vacuumMetricsDatabaseOffline(ctx context.Context, databasePath string) error {
	if _, err := os.Stat(databasePath); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return apperror.Wrap(apperror.CodeIOReadFailed, "检查监控数据库文件失败", err)
	}
	connection, err := OpenPlainConnection(ctx, ConnectionOptions{Path: databasePath, MaxOpenConns: 1, MaxIdleConns: 1})
	if err != nil {
		return err
	}
	if _, err := connection.pool.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		_ = connection.Close()
		return apperror.Wrap(apperror.CodeIOWriteFailed, "监控库整理前 WAL Checkpoint 失败", err)
	}
	if _, err := connection.pool.ExecContext(ctx, "VACUUM"); err != nil {
		_ = connection.Close()
		return apperror.Wrap(apperror.CodeIOWriteFailed, "监控库存储整理 VACUUM 失败", err)
	}
	if err := connection.Close(); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "关闭监控库整理连接失败", err)
	}
	return nil
}

func vacuumDatabaseOffline(ctx context.Context, databasePath string, keyStore KeyStore) error {
	material, err := keyStore.Load(ctx)
	if err != nil {
		return err
	}
	if !material.DatabaseCreated {
		return apperror.New(apperror.CodeValidationConflict, "数据库尚未完成初始化")
	}
	connection, err := OpenConnection(ctx, ConnectionOptions{Path: databasePath, Key: material.Key, MaxOpenConns: 1, MaxIdleConns: 1})
	if err != nil {
		return err
	}
	if _, err := connection.pool.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		_ = connection.Close()
		return apperror.Wrap(apperror.CodeIOWriteFailed, "存储整理前 WAL Checkpoint 失败", err)
	}
	if _, err := connection.pool.ExecContext(ctx, "VACUUM"); err != nil {
		_ = connection.Close()
		return apperror.Wrap(apperror.CodeIOWriteFailed, "存储整理 VACUUM 失败", err)
	}
	if err := connection.Close(); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "关闭存储整理连接失败", err)
	}
	return nil
}
