package sqlcipher

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
)

// RotateDatabaseKeyOffline rotates the SQLCipher key with rollback when secure-store persistence fails.
func RotateDatabaseKeyOffline(ctx context.Context, databasePath string, keyStore KeyStore) (int, error) {
	current, err := keyStore.Load(ctx)
	if err != nil {
		return 0, err
	}
	if !current.DatabaseCreated {
		return 0, apperror.New(apperror.CodeValidationConflict, "数据库尚未完成初始化")
	}
	rotated, err := GenerateDatabaseKey()
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, "生成轮换密钥失败", err)
	}
	rotated.DatabaseID = current.DatabaseID
	rotated.Version = current.Version + 1
	rotated.DatabaseCreated = true
	if err := rekeyDatabase(ctx, databasePath, current.Key, rotated.Key); err != nil {
		return 0, err
	}
	connection, err := OpenConnection(ctx, ConnectionOptions{Path: databasePath, Key: rotated.Key, MaxOpenConns: 1, MaxIdleConns: 1})
	if err != nil {
		_ = rekeyDatabase(ctx, databasePath, rotated.Key, current.Key)
		return 0, err
	}
	updateResult := connection.GORM().WithContext(ctx).Model(&databaseMetadataRecord{}).
		Where("database_id = ?", current.DatabaseID).
		Update("key_version", rotated.Version)
	closeErr := connection.Close()
	if updateResult.Error != nil || updateResult.RowsAffected != 1 || closeErr != nil {
		_ = rekeyDatabase(ctx, databasePath, rotated.Key, current.Key)
		return 0, apperror.Wrap(apperror.CodeIOWriteFailed, "更新数据库密钥版本失败", updateResult.Error)
	}
	if err := keyStore.Save(ctx, rotated); err != nil {
		if rollbackErr := rekeyDatabase(ctx, databasePath, rotated.Key, current.Key); rollbackErr != nil {
			return 0, apperror.Wrap(apperror.CodeCryptoIntegrityFailed, "密钥托管失败且数据库回滚失败", rollbackErr)
		}
		rollbackConnection, openErr := OpenConnection(ctx, ConnectionOptions{Path: databasePath, Key: current.Key, MaxOpenConns: 1, MaxIdleConns: 1})
		if openErr == nil {
			_ = rollbackConnection.GORM().WithContext(ctx).Model(&databaseMetadataRecord{}).
				Where("database_id = ?", current.DatabaseID).
				Update("key_version", current.Version).Error
			_ = rollbackConnection.Close()
		}
		return 0, err
	}
	return rotated.Version, nil
}

func rekeyDatabase(ctx context.Context, databasePath string, oldKey []byte, newKey []byte) error {
	connection, err := OpenConnection(ctx, ConnectionOptions{Path: databasePath, Key: oldKey, MaxOpenConns: 1, MaxIdleConns: 1})
	if err != nil {
		return err
	}
	if _, err := connection.pool.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		_ = connection.Close()
		return apperror.Wrap(apperror.CodeIOWriteFailed, "密钥轮换前 WAL Checkpoint 失败", err)
	}
	statement := fmt.Sprintf("PRAGMA rekey = \"x'%s'\"", hex.EncodeToString(newKey))
	if _, err := connection.pool.ExecContext(ctx, statement); err != nil {
		_ = connection.Close()
		return apperror.Wrap(apperror.CodeCryptoDecryptFailed, "SQLCipher 密钥轮换失败", err)
	}
	if err := connection.Close(); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "关闭密钥轮换连接失败", err)
	}
	return nil
}
