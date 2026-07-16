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
	keyringServiceName = "com.gainey.mineops"
	databaseKeyAccount = "database-key"
)

// DatabaseKey contains the database identity, key version, initialization state, and raw 256-bit key.
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

// KeyStore is the system-secure persistence boundary for the automatic SQLCipher key.
type KeyStore interface {
	Load(context.Context) (DatabaseKey, error)
	Save(context.Context, DatabaseKey) error
	Delete(context.Context) error
}

// SystemKeyStore stores the SQLCipher key in Keychain, Credential Manager, or Secret Service.
type SystemKeyStore struct{}

// Load reads and validates the automatic SQLCipher key from system secure storage.
func (SystemKeyStore) Load(ctx context.Context) (DatabaseKey, error) {
	if err := ctx.Err(); err != nil {
		return DatabaseKey{}, err
	}
	value, err := keyring.Get(keyringServiceName, databaseKeyAccount)
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

// Save atomically replaces the automatic SQLCipher key record in system secure storage.
func (SystemKeyStore) Save(ctx context.Context, value DatabaseKey) error {
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
	if err := keyring.Set(keyringServiceName, databaseKeyAccount, string(encoded)); err != nil {
		return apperror.Wrap(apperror.CodeCryptoKeyUnavailable, "写入系统数据库密钥失败", err)
	}
	return nil
}

// Delete removes the automatic SQLCipher key from system secure storage.
func (SystemKeyStore) Delete(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := keyring.Delete(keyringServiceName, databaseKeyAccount); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return apperror.Wrap(apperror.CodeCryptoKeyUnavailable, "删除系统数据库密钥失败", err)
	}
	return nil
}

// GenerateDatabaseKey creates a random database identity and 256-bit SQLCipher key in memory.
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
