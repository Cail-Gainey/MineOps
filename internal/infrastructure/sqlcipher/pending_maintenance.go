package sqlcipher

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
)

const (
	pendingRestoreFilename     = ".pending-restore.mineops-backup"
	pendingKeyRotationFilename = ".pending-key-rotation"
)

// PendingMaintenance describes offline database work staged for the next application start.
type PendingMaintenance struct {
	RestorePending     bool        `json:"restorePending"`
	KeyRotationPending bool        `json:"keyRotationPending"`
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
	return result, nil
}

// CancelPendingMaintenance removes staged restore and key-rotation work.
func CancelPendingMaintenance(dataDirectory string) error {
	var firstError error
	for _, name := range []string{pendingRestoreFilename, pendingKeyRotationFilename} {
		if err := os.Remove(filepath.Join(dataDirectory, name)); err != nil && !os.IsNotExist(err) && firstError == nil {
			firstError = err
		}
	}
	return firstError
}

// ApplyPendingMaintenance performs staged restore and key rotation before opening the active database.
func ApplyPendingMaintenance(ctx context.Context, dataDirectory, databasePath string, keyStore KeyStore) error {
	pending, err := ReadPendingMaintenance(dataDirectory)
	if err != nil {
		return err
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
	return nil
}
