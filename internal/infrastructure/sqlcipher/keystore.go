package sqlcipher

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/zalando/go-keyring"
)

const (
	keyringServiceName            = "com.gainey.mineops"
	developmentKeyringServiceName = "com.gainey.mineops.development"
	databaseKeyAccount            = "database-key"
)

// DatabaseKey 承载数据库身份、密钥版本、初始化状态与 256 位原始密钥。
type DatabaseKey struct {
	DatabaseID      string `json:"databaseID"`
	Version         int    `json:"version"`
	DatabaseCreated bool   `json:"databaseCreated"`
	Key             []byte `json:"-"`
}

type storedDatabaseKey struct {
	DatabaseID      string `json:"databaseID"`
	Version         int    `json:"version"`
	DatabaseCreated bool   `json:"databaseCreated"`
	KeyBase64       string `json:"keyBase64"`
}

// KeyStore 是自动 SQLCipher 密钥在系统安全存储上的持久化边界。
type KeyStore interface {
	Load(context.Context) (DatabaseKey, error)
	Save(context.Context, DatabaseKey) error
	Delete(context.Context) error
}

// SystemKeyStore 把 SQLCipher 密钥存入 Keychain、凭据管理器或 Secret Service。
type SystemKeyStore struct {
	serviceName string
	accountName string
}

// NewDevelopmentSystemKeyStore 返回与打包版数据库密钥相互隔离的安全存储。
func NewDevelopmentSystemKeyStore() SystemKeyStore {
	return SystemKeyStore{serviceName: developmentKeyringServiceName, accountName: databaseKeyAccount}
}

func (s SystemKeyStore) keyringCoordinates() (string, string) {
	serviceName := s.serviceName
	if serviceName == "" {
		serviceName = keyringServiceName
	}
	accountName := s.accountName
	if accountName == "" {
		accountName = databaseKeyAccount
	}
	return serviceName, accountName
}

// Load 从系统安全存储读取并校验自动 SQLCipher 密钥。
func (s SystemKeyStore) Load(ctx context.Context) (DatabaseKey, error) {
	if err := ctx.Err(); err != nil {
		return DatabaseKey{}, err
	}
	serviceName, accountName := s.keyringCoordinates()
	value, err := keyring.Get(serviceName, accountName)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return DatabaseKey{}, apperror.New(apperror.CodeCryptoKeyUnavailable, "数据库密钥尚未初始化")
		}
		return DatabaseKey{}, apperror.Wrap(apperror.CodeCryptoKeyUnavailable, "读取系统数据库密钥失败", err)
	}
	var stored storedDatabaseKey
	if err := json.Unmarshal([]byte(value), &stored); err != nil {
		return DatabaseKey{}, apperror.Wrap(apperror.CodeCryptoIntegrityFailed, "系统数据库密钥记录损坏", err)
	}
	key, err := base64.StdEncoding.DecodeString(stored.KeyBase64)
	if err != nil || len(key) != 32 || stored.DatabaseID == "" || stored.Version < 1 {
		return DatabaseKey{}, apperror.New(apperror.CodeCryptoIntegrityFailed, "系统数据库密钥记录无效")
	}
	return DatabaseKey{
		DatabaseID: stored.DatabaseID, Version: stored.Version,
		DatabaseCreated: stored.DatabaseCreated, Key: key,
	}, nil
}

// Save 在系统安全存储中原子替换自动 SQLCipher 密钥记录。
func (s SystemKeyStore) Save(ctx context.Context, value DatabaseKey) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if value.DatabaseID == "" || value.Version < 1 || len(value.Key) != 32 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "数据库密钥记录无效")
	}
	encoded, err := json.Marshal(storedDatabaseKey{
		DatabaseID: value.DatabaseID, Version: value.Version, DatabaseCreated: value.DatabaseCreated,
		KeyBase64: base64.StdEncoding.EncodeToString(value.Key),
	})
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, "编码数据库密钥失败", err)
	}
	serviceName, accountName := s.keyringCoordinates()
	if err := keyring.Set(serviceName, accountName, string(encoded)); err != nil {
		return apperror.Wrap(apperror.CodeCryptoKeyUnavailable, "写入系统数据库密钥失败", err)
	}
	return nil
}

// Delete 从系统安全存储中删除自动 SQLCipher 密钥。
func (s SystemKeyStore) Delete(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	serviceName, accountName := s.keyringCoordinates()
	if err := keyring.Delete(serviceName, accountName); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return apperror.Wrap(apperror.CodeCryptoKeyUnavailable, "删除系统数据库密钥失败", err)
	}
	return nil
}

// GenerateDatabaseKey 在内存中生成随机的数据库身份与 256 位 SQLCipher 密钥。
func GenerateDatabaseKey() (DatabaseKey, error) {
	identity := make([]byte, 16)
	key := make([]byte, 32)
	if _, err := rand.Read(identity); err != nil {
		return DatabaseKey{}, err
	}
	if _, err := rand.Read(key); err != nil {
		return DatabaseKey{}, err
	}
	return DatabaseKey{DatabaseID: hex.EncodeToString(identity), Version: 1, Key: key}, nil
}
