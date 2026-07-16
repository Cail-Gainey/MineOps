package sqlcipher

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"gorm.io/gorm"
)

type databaseMetadataRecord struct {
	DatabaseID string `gorm:"primaryKey;size:32"`
	KeyVersion int
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// BootstrapResult contains the ready encrypted connection and immutable database identity metadata.
type BootstrapResult struct {
	Connection *Connection
	DatabaseID string
	KeyVersion int
}

// BootstrapDatabase initializes or opens the encrypted database without silently replacing missing state.
func BootstrapDatabase(ctx context.Context, path string, store KeyStore) (BootstrapResult, error) {
	if path == "" || store == nil {
		return BootstrapResult{}, apperror.New(apperror.CodeValidationRequired, "数据库路径和 KeyStore 不能为空")
	}
	_, statErr := os.Stat(path)
	databaseExists := statErr == nil
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return BootstrapResult{}, apperror.Wrap(apperror.CodeIOReadFailed, "检查数据库文件失败", statErr)
	}

	key, keyErr := store.Load(ctx)
	if keyErr != nil {
		var applicationError *apperror.Error
		if !errors.As(keyErr, &applicationError) || applicationError.Code != apperror.CodeCryptoKeyUnavailable {
			return BootstrapResult{}, keyErr
		}
		if databaseExists {
			return BootstrapResult{}, apperror.New(apperror.CodeCryptoKeyUnavailable, "数据库存在但系统密钥缺失，必须从备份恢复")
		}
		key, keyErr = GenerateDatabaseKey()
		if keyErr != nil {
			return BootstrapResult{}, apperror.Wrap(apperror.CodeInternal, "生成数据库密钥失败", keyErr)
		}
		if err := store.Save(ctx, key); err != nil {
			return BootstrapResult{}, err
		}
	} else if !databaseExists && key.DatabaseCreated {
		return BootstrapResult{}, apperror.New(apperror.CodeIONotFound, "系统密钥存在但数据库文件缺失，必须从备份恢复")
	}

	connection, err := OpenConnection(ctx, ConnectionOptions{Path: path, Key: key.Key})
	if err != nil {
		return BootstrapResult{}, err
	}
	runner, err := NewMigrationRunner(connection.GORM(), DefaultMigrations())
	if err != nil {
		_ = connection.Close()
		return BootstrapResult{}, err
	}
	if err := runner.Run(ctx); err != nil {
		_ = connection.Close()
		return BootstrapResult{}, err
	}

	var metadata databaseMetadataRecord
	result := connection.GORM().WithContext(ctx).First(&metadata)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		if databaseExists {
			_ = connection.Close()
			return BootstrapResult{}, apperror.New(apperror.CodeCryptoIntegrityFailed, "数据库缺少身份元数据")
		}
		metadata = databaseMetadataRecord{DatabaseID: key.DatabaseID, KeyVersion: key.Version, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
		if err := connection.GORM().WithContext(ctx).Create(&metadata).Error; err != nil {
			_ = connection.Close()
			return BootstrapResult{}, apperror.Wrap(apperror.CodeIOWriteFailed, "写入数据库身份失败", err)
		}
	} else if result.Error != nil {
		_ = connection.Close()
		return BootstrapResult{}, apperror.Wrap(apperror.CodeIOReadFailed, "读取数据库身份失败", result.Error)
	}
	if metadata.DatabaseID != key.DatabaseID || metadata.KeyVersion != key.Version {
		_ = connection.Close()
		return BootstrapResult{}, apperror.New(apperror.CodeCryptoIntegrityFailed, "数据库身份或密钥版本不匹配")
	}
	if !key.DatabaseCreated {
		key.DatabaseCreated = true
		if err := store.Save(ctx, key); err != nil {
			_ = connection.Close()
			return BootstrapResult{}, err
		}
	}
	return BootstrapResult{Connection: connection, DatabaseID: metadata.DatabaseID, KeyVersion: metadata.KeyVersion}, nil
}
