// Package sqlcipher validates encrypted SQLite behavior before production persistence is built.
package sqlcipher

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var errRollbackSpike = errors.New("rollback spike")

type spikeRecord struct {
	ID        uint `gorm:"primaryKey"`
	Secret    string
	CreatedAt time.Time
}

type spikeMigration struct {
	Version   int `gorm:"primaryKey"`
	AppliedAt time.Time
}

type spikeDatabaseMetadata struct {
	DatabaseID string `gorm:"primaryKey"`
	KeyVersion int
}

// SpikeResult contains observable SQLCipher, GORM, transaction, and disk-encryption evidence.
type SpikeResult struct {
	CipherVersion         string `json:"cipherVersion"`
	JournalMode           string `json:"journalMode"`
	BusyTimeoutMillis     int    `json:"busyTimeoutMillis"`
	ForeignKeysEnabled    bool   `json:"foreignKeysEnabled"`
	MigrationVersion      int    `json:"migrationVersion"`
	TransactionCommitted  bool   `json:"transactionCommitted"`
	TransactionRolledBack bool   `json:"transactionRolledBack"`
	IntegrityCheckOK      bool   `json:"integrityCheckOK"`
	CipherIntegrityOK     bool   `json:"cipherIntegrityOK"`
	ForeignKeyCheckOK     bool   `json:"foreignKeyCheckOK"`
	EncryptedHeader       bool   `json:"encryptedHeader"`
	PlaintextAbsent       bool   `json:"plaintextAbsent"`
	WrongKeyRejected      bool   `json:"wrongKeyRejected"`
	CorrectKeyReopened    bool   `json:"correctKeyReopened"`
	DatabaseBytes         int64  `json:"databaseBytes"`
	DatabaseIdentity      string `json:"databaseIdentity"`
	AutomaticKeyRead      bool   `json:"automaticKeyRead"`
	KeyFileModeSecure     bool   `json:"keyFileModeSecure"`
	KeyRotationSucceeded  bool   `json:"keyRotationSucceeded"`
	OldKeyRejected        bool   `json:"oldKeyRejected"`
	BackupCreated         bool   `json:"backupCreated"`
	BackupRestored        bool   `json:"backupRestored"`
	TamperDetected        bool   `json:"tamperDetected"`
	BackupBytes           int64  `json:"backupBytes"`
	KeyVersion            int    `json:"keyVersion"`
}

// Passed reports whether every required SQLCipher and automatic-key gate succeeded.
func (r SpikeResult) Passed() bool {
	return strings.HasPrefix(r.CipherVersion, "4.15.0") &&
		r.JournalMode == "wal" &&
		r.BusyTimeoutMillis == 5_000 &&
		r.ForeignKeysEnabled &&
		r.MigrationVersion == 1 &&
		r.TransactionCommitted &&
		r.TransactionRolledBack &&
		r.IntegrityCheckOK &&
		r.CipherIntegrityOK &&
		r.ForeignKeyCheckOK &&
		r.EncryptedHeader &&
		r.PlaintextAbsent &&
		r.WrongKeyRejected &&
		r.CorrectKeyReopened &&
		r.DatabaseIdentity != "" &&
		r.AutomaticKeyRead &&
		r.KeyFileModeSecure &&
		r.KeyRotationSucceeded &&
		r.OldKeyRejected &&
		r.BackupCreated &&
		r.BackupRestored &&
		r.TamperDetected &&
		r.BackupBytes > 0 &&
		r.KeyVersion == 2
}

// RunSpike creates a temporary encrypted database and verifies the stage 0 SQLCipher baseline.
func RunSpike(ctx context.Context) (SpikeResult, error) {
	directory, err := os.MkdirTemp("", "mineops-sqlcipher-spike-")
	if err != nil {
		return SpikeResult{}, fmt.Errorf("create SQLCipher spike directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(directory) }()

	databasePath := filepath.Join(directory, "spike.db")
	databaseID, err := randomHex(16)
	if err != nil {
		return SpikeResult{}, fmt.Errorf("generate database identity: %w", err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return SpikeResult{}, fmt.Errorf("generate SQLCipher key: %w", err)
	}

	db, sqlDB, err := open(databasePath, key)
	if err != nil {
		return SpikeResult{}, err
	}
	closed := false
	defer func() {
		if !closed {
			_ = sqlDB.Close()
		}
	}()

	db = db.WithContext(ctx)
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Migrator().AutoMigrate(&spikeRecord{}, &spikeMigration{}, &spikeDatabaseMetadata{}); err != nil {
			return err
		}
		if err := tx.Create(&spikeMigration{Version: 1, AppliedAt: time.Now().UTC()}).Error; err != nil {
			return err
		}
		return tx.Create(&spikeDatabaseMetadata{DatabaseID: databaseID, KeyVersion: 1}).Error
	}); err != nil {
		return SpikeResult{}, fmt.Errorf("run GORM migration: %w", err)
	}

	const plaintextMarker = "MINEOPS_SQLCIPHER_PLAINTEXT_MARKER"
	if err := db.Transaction(func(tx *gorm.DB) error {
		return tx.Create(&spikeRecord{Secret: plaintextMarker, CreatedAt: time.Now().UTC()}).Error
	}); err != nil {
		return SpikeResult{}, fmt.Errorf("commit transaction: %w", err)
	}

	rollbackErr := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&spikeRecord{Secret: "ROLLBACK_MARKER", CreatedAt: time.Now().UTC()}).Error; err != nil {
			return err
		}
		return errRollbackSpike
	})
	if !errors.Is(rollbackErr, errRollbackSpike) {
		return SpikeResult{}, fmt.Errorf("rollback transaction: %w", rollbackErr)
	}

	var committedCount int64
	if err := db.Model(&spikeRecord{}).Where("secret = ?", plaintextMarker).Count(&committedCount).Error; err != nil {
		return SpikeResult{}, fmt.Errorf("query committed record: %w", err)
	}
	var rolledBackCount int64
	if err := db.Model(&spikeRecord{}).Where("secret = ?", "ROLLBACK_MARKER").Count(&rolledBackCount).Error; err != nil {
		return SpikeResult{}, fmt.Errorf("query rolled back record: %w", err)
	}

	cipherVersion, err := queryString(ctx, sqlDB, "PRAGMA cipher_version")
	if err != nil {
		return SpikeResult{}, fmt.Errorf("query cipher version: %w", err)
	}
	journalMode, err := queryString(ctx, sqlDB, "PRAGMA journal_mode")
	if err != nil {
		return SpikeResult{}, fmt.Errorf("query journal mode: %w", err)
	}
	busyTimeout, err := queryInt(ctx, sqlDB, "PRAGMA busy_timeout")
	if err != nil {
		return SpikeResult{}, fmt.Errorf("query busy timeout: %w", err)
	}
	foreignKeys, err := queryInt(ctx, sqlDB, "PRAGMA foreign_keys")
	if err != nil {
		return SpikeResult{}, fmt.Errorf("query foreign keys: %w", err)
	}
	integrityOK, err := pragmaCheck(ctx, sqlDB, "PRAGMA integrity_check", false)
	if err != nil {
		return SpikeResult{}, fmt.Errorf("run integrity check: %w", err)
	}
	cipherIntegrityOK, err := pragmaCheck(ctx, sqlDB, "PRAGMA cipher_integrity_check", true)
	if err != nil {
		return SpikeResult{}, fmt.Errorf("run cipher integrity check: %w", err)
	}
	foreignKeyCheckOK, err := pragmaCheck(ctx, sqlDB, "PRAGMA foreign_key_check", true)
	if err != nil {
		return SpikeResult{}, fmt.Errorf("run foreign key check: %w", err)
	}

	if err := sqlDB.Close(); err != nil {
		return SpikeResult{}, fmt.Errorf("close encrypted database: %w", err)
	}
	closed = true

	databaseData, err := os.ReadFile(databasePath)
	if err != nil {
		return SpikeResult{}, fmt.Errorf("read encrypted database: %w", err)
	}
	plaintextAbsent := !bytes.Contains(databaseData, []byte(plaintextMarker))
	for _, suffix := range []string{"-wal", "-shm"} {
		data, readErr := os.ReadFile(databasePath + suffix)
		if readErr == nil && bytes.Contains(data, []byte(plaintextMarker)) {
			plaintextAbsent = false
		}
	}

	wrongKey := make([]byte, 32)
	if _, err := rand.Read(wrongKey); err != nil {
		return SpikeResult{}, fmt.Errorf("generate wrong SQLCipher key: %w", err)
	}
	wrongKeyRejected := false
	wrongDB, wrongSQLDB, wrongOpenErr := open(databasePath, wrongKey)
	if wrongOpenErr != nil {
		wrongKeyRejected = true
	} else {
		var wrongKeyCount int64
		wrongKeyErr := wrongDB.WithContext(ctx).Model(&spikeRecord{}).Count(&wrongKeyCount).Error
		wrongKeyRejected = wrongKeyErr != nil
		_ = wrongSQLDB.Close()
	}

	reopenedDB, reopenedSQLDB, err := open(databasePath, key)
	if err != nil {
		return SpikeResult{}, fmt.Errorf("reopen database with correct key: %w", err)
	}
	var reopenedCount int64
	reopenErr := reopenedDB.WithContext(ctx).Model(&spikeRecord{}).Count(&reopenedCount).Error

	var migration spikeMigration
	if err := reopenedDB.WithContext(ctx).Order("version desc").First(&migration).Error; err != nil {
		_ = reopenedSQLDB.Close()
		return SpikeResult{}, fmt.Errorf("query migration version: %w", err)
	}
	_ = reopenedSQLDB.Close()

	automaticResult, err := runAutomaticKeySpike(ctx, directory, databasePath, databaseID, migration.Version, key)
	if err != nil {
		return SpikeResult{}, err
	}

	return SpikeResult{
		CipherVersion:         cipherVersion,
		JournalMode:           strings.ToLower(journalMode),
		BusyTimeoutMillis:     busyTimeout,
		ForeignKeysEnabled:    foreignKeys == 1,
		MigrationVersion:      migration.Version,
		TransactionCommitted:  committedCount == 1,
		TransactionRolledBack: rolledBackCount == 0,
		IntegrityCheckOK:      integrityOK,
		CipherIntegrityOK:     cipherIntegrityOK,
		ForeignKeyCheckOK:     foreignKeyCheckOK,
		EncryptedHeader:       !bytes.HasPrefix(databaseData, []byte("SQLite format 3")),
		PlaintextAbsent:       plaintextAbsent,
		WrongKeyRejected:      wrongKeyRejected,
		CorrectKeyReopened:    reopenErr == nil && reopenedCount == 1,
		DatabaseBytes:         int64(len(databaseData)),
		DatabaseIdentity:      databaseID,
		AutomaticKeyRead:      automaticResult.AutomaticKeyRead,
		KeyFileModeSecure:     automaticResult.KeyFileModeSecure,
		KeyRotationSucceeded:  automaticResult.KeyRotationSucceeded,
		OldKeyRejected:        automaticResult.OldKeyRejected,
		BackupCreated:         automaticResult.BackupCreated,
		BackupRestored:        automaticResult.BackupRestored,
		TamperDetected:        automaticResult.TamperDetected,
		BackupBytes:           automaticResult.BackupBytes,
		KeyVersion:            automaticResult.KeyVersion,
	}, nil
}

func open(databasePath string, key []byte) (*gorm.DB, *sql.DB, error) {
	dsn := fmt.Sprintf(
		"%s?_pragma_key=x'%s'&_pragma_cipher_page_size=4096&_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000",
		databasePath,
		hex.EncodeToString(key),
	)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("open encrypted GORM database: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("get SQLCipher connection pool: %w", err)
	}
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(4)
	return db, sqlDB, nil
}

func queryString(ctx context.Context, db *sql.DB, query string) (string, error) {
	var value string
	err := db.QueryRowContext(ctx, query).Scan(&value)
	return value, err
}

func queryInt(ctx context.Context, db *sql.DB, query string) (int, error) {
	var value int
	err := db.QueryRowContext(ctx, query).Scan(&value)
	return value, err
}

func pragmaCheck(ctx context.Context, db *sql.DB, query string, emptyIsOK bool) (bool, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()

	hasRows := false
	for rows.Next() {
		hasRows = true
		var result string
		if err := rows.Scan(&result); err != nil {
			return false, err
		}
		if !strings.EqualFold(result, "ok") {
			return false, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return hasRows || emptyIsOK, nil
}
