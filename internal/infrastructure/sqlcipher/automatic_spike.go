package sqlcipher

import (
	"archive/zip"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const maximumBackupEntryBytes = 64 * 1024 * 1024

type automaticSpikeResult struct {
	AutomaticKeyRead     bool
	KeyFileModeSecure    bool
	KeyRotationSucceeded bool
	OldKeyRejected       bool
	BackupCreated        bool
	BackupRestored       bool
	TamperDetected       bool
	BackupBytes          int64
	KeyVersion           int
}

type keyRecord struct {
	DatabaseID string `json:"databaseID"`
	KeyVersion int    `json:"keyVersion"`
	KeyBase64  string `json:"keyBase64"`
}

type backupManifest struct {
	FormatVersion  int    `json:"formatVersion"`
	DatabaseID     string `json:"databaseID"`
	SchemaVersion  int    `json:"schemaVersion"`
	KeyVersion     int    `json:"keyVersion"`
	KeyBase64      string `json:"keyBase64"`
	DatabaseSHA256 string `json:"databaseSHA256"`
	IntegrityHMAC  string `json:"integrityHMAC"`
}

func runAutomaticKeySpike(
	ctx context.Context,
	directory string,
	databasePath string,
	databaseID string,
	schemaVersion int,
	initialKey []byte,
) (automaticSpikeResult, error) {
	keyPath := filepath.Join(directory, "secure-store-key.json")
	initialRecord := keyRecord{
		DatabaseID: databaseID,
		KeyVersion: 1,
		KeyBase64:  base64.StdEncoding.EncodeToString(initialKey),
	}
	if err := writeKeyRecord(keyPath, initialRecord); err != nil {
		return automaticSpikeResult{}, fmt.Errorf("store initial automatic key: %w", err)
	}
	keyInfo, err := os.Stat(keyPath)
	if err != nil {
		return automaticSpikeResult{}, fmt.Errorf("inspect automatic key file: %w", err)
	}
	loadedRecord, loadedKey, err := readKeyRecord(keyPath)
	if err != nil {
		return automaticSpikeResult{}, fmt.Errorf("read automatic key: %w", err)
	}
	automaticKeyRead := loadedRecord.DatabaseID == databaseID && databaseContainsRecord(ctx, databasePath, loadedKey)

	rotatedKey := make([]byte, 32)
	if _, err := rand.Read(rotatedKey); err != nil {
		return automaticSpikeResult{}, fmt.Errorf("generate rotated SQLCipher key: %w", err)
	}
	if err := rotateKey(ctx, databasePath, databaseID, initialKey, rotatedKey); err != nil {
		return automaticSpikeResult{}, err
	}
	rotatedRecord := keyRecord{
		DatabaseID: databaseID,
		KeyVersion: 2,
		KeyBase64:  base64.StdEncoding.EncodeToString(rotatedKey),
	}
	if err := writeKeyRecord(keyPath, rotatedRecord); err != nil {
		return automaticSpikeResult{}, fmt.Errorf("store rotated automatic key: %w", err)
	}
	reloadedRecord, reloadedKey, err := readKeyRecord(keyPath)
	if err != nil {
		return automaticSpikeResult{}, fmt.Errorf("reload rotated automatic key: %w", err)
	}
	rotationSucceeded := reloadedRecord.KeyVersion == 2 && databaseContainsRecord(ctx, databasePath, reloadedKey)
	oldKeyRejected := !databaseContainsRecord(ctx, databasePath, initialKey)

	backupPath := filepath.Join(directory, "spike.mineops-backup")
	if err := createBackup(backupPath, databasePath, rotatedRecord, schemaVersion); err != nil {
		return automaticSpikeResult{}, fmt.Errorf("create portable backup: %w", err)
	}
	backupInfo, err := os.Stat(backupPath)
	if err != nil {
		return automaticSpikeResult{}, fmt.Errorf("inspect portable backup: %w", err)
	}
	restoreDirectory := filepath.Join(directory, "restore")
	restoredDatabasePath, restoredKeyPath, err := restoreBackup(backupPath, restoreDirectory)
	if err != nil {
		return automaticSpikeResult{}, fmt.Errorf("restore portable backup: %w", err)
	}
	restoredRecord, restoredKey, err := readKeyRecord(restoredKeyPath)
	if err != nil {
		return automaticSpikeResult{}, fmt.Errorf("read restored key package: %w", err)
	}
	backupRestored := restoredRecord.DatabaseID == databaseID && databaseContainsRecord(ctx, restoredDatabasePath, restoredKey)

	tamperedPath := filepath.Join(directory, "tampered.mineops-backup")
	if err := createTamperedBackup(backupPath, tamperedPath); err != nil {
		return automaticSpikeResult{}, fmt.Errorf("create tampered backup: %w", err)
	}
	_, _, tamperErr := restoreBackup(tamperedPath, filepath.Join(directory, "tampered-restore"))

	return automaticSpikeResult{
		AutomaticKeyRead:     automaticKeyRead,
		KeyFileModeSecure:    keyInfo.Mode().Perm()&0o077 == 0,
		KeyRotationSucceeded: rotationSucceeded,
		OldKeyRejected:       oldKeyRejected,
		BackupCreated:        backupInfo.Size() > 0,
		BackupRestored:       backupRestored,
		TamperDetected:       tamperErr != nil,
		BackupBytes:          backupInfo.Size(),
		KeyVersion:           reloadedRecord.KeyVersion,
	}, nil
}

func randomHex(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func writeKeyRecord(path string, record keyRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func readKeyRecord(path string) (keyRecord, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return keyRecord{}, nil, err
	}
	var record keyRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return keyRecord{}, nil, err
	}
	key, err := base64.StdEncoding.DecodeString(record.KeyBase64)
	if err != nil || len(key) != 32 {
		return keyRecord{}, nil, errors.New("automatic key record does not contain a 256-bit key")
	}
	return record, key, nil
}

func rotateKey(ctx context.Context, databasePath string, databaseID string, oldKey []byte, newKey []byte) error {
	_, sqlDB, err := open(databasePath, oldKey)
	if err != nil {
		return fmt.Errorf("open database for key rotation: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	if _, err := sqlDB.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		_ = sqlDB.Close()
		return fmt.Errorf("checkpoint database before key rotation: %w", err)
	}
	if _, err := sqlDB.ExecContext(ctx, fmt.Sprintf("PRAGMA rekey = \"x'%s'\"", hex.EncodeToString(newKey))); err != nil {
		_ = sqlDB.Close()
		return fmt.Errorf("rotate SQLCipher key: %w", err)
	}
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("close database after key rotation: %w", err)
	}

	rotatedDB, rotatedSQLDB, err := open(databasePath, newKey)
	if err != nil {
		return fmt.Errorf("open database after key rotation: %w", err)
	}
	defer func() { _ = rotatedSQLDB.Close() }()
	result := rotatedDB.WithContext(ctx).
		Model(&spikeDatabaseMetadata{}).
		Where("database_id = ?", databaseID).
		Update("key_version", 2)
	if result.Error != nil {
		return fmt.Errorf("update database key metadata: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return errors.New("database identity was not found after key rotation")
	}
	return nil
}

func databaseContainsRecord(ctx context.Context, databasePath string, key []byte) bool {
	db, sqlDB, err := open(databasePath, key)
	if err != nil {
		return false
	}
	defer func() { _ = sqlDB.Close() }()
	var count int64
	return db.WithContext(ctx).Model(&spikeRecord{}).Count(&count).Error == nil && count == 1
}

func createBackup(path string, databasePath string, record keyRecord, schemaVersion int) error {
	database, err := os.ReadFile(databasePath)
	if err != nil {
		return err
	}
	key, err := base64.StdEncoding.DecodeString(record.KeyBase64)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(database)
	manifest := backupManifest{
		FormatVersion:  1,
		DatabaseID:     record.DatabaseID,
		SchemaVersion:  schemaVersion,
		KeyVersion:     record.KeyVersion,
		KeyBase64:      record.KeyBase64,
		DatabaseSHA256: hex.EncodeToString(digest[:]),
	}
	manifest.IntegrityHMAC, err = backupHMAC(manifest, database, key)
	if err != nil {
		return err
	}
	return writeBackup(path, manifest, database)
}

func backupHMAC(manifest backupManifest, database []byte, key []byte) (string, error) {
	manifest.IntegrityHMAC = ""
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(manifestData)
	_, _ = mac.Write(database)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func writeBackup(path string, manifest backupManifest, database []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	writer := zip.NewWriter(file)
	manifestEntry, err := writer.Create("manifest.json")
	if err == nil {
		var manifestData []byte
		manifestData, err = json.Marshal(manifest)
		if err == nil {
			_, err = manifestEntry.Write(manifestData)
		}
	}
	if err == nil {
		var databaseEntry io.Writer
		databaseEntry, err = writer.Create("database.db")
		if err == nil {
			_, err = databaseEntry.Write(database)
		}
	}
	closeWriterErr := writer.Close()
	closeFileErr := file.Close()
	if err != nil {
		return err
	}
	if closeWriterErr != nil {
		return closeWriterErr
	}
	return closeFileErr
}

func readBackup(path string) (backupManifest, []byte, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return backupManifest{}, nil, err
	}
	defer func() { _ = reader.Close() }()

	var manifest backupManifest
	var database []byte
	for _, entry := range reader.File {
		if entry.UncompressedSize64 > maximumBackupEntryBytes {
			return backupManifest{}, nil, errors.New("backup entry exceeds size limit")
		}
		entryReader, openErr := entry.Open()
		if openErr != nil {
			return backupManifest{}, nil, openErr
		}
		data, readErr := io.ReadAll(io.LimitReader(entryReader, maximumBackupEntryBytes+1))
		_ = entryReader.Close()
		if readErr != nil {
			return backupManifest{}, nil, readErr
		}
		switch entry.Name {
		case "manifest.json":
			if err := json.Unmarshal(data, &manifest); err != nil {
				return backupManifest{}, nil, err
			}
		case "database.db":
			database = data
		default:
			return backupManifest{}, nil, fmt.Errorf("unexpected backup entry %q", entry.Name)
		}
	}
	if manifest.FormatVersion != 1 || manifest.DatabaseID == "" || len(database) == 0 {
		return backupManifest{}, nil, errors.New("backup is missing required data")
	}
	return manifest, database, nil
}

func restoreBackup(path string, directory string) (string, string, error) {
	manifest, database, err := readBackup(path)
	if err != nil {
		return "", "", err
	}
	key, err := base64.StdEncoding.DecodeString(manifest.KeyBase64)
	if err != nil || len(key) != 32 {
		return "", "", errors.New("backup key package is invalid")
	}
	digest := sha256.Sum256(database)
	if !hmac.Equal([]byte(manifest.DatabaseSHA256), []byte(hex.EncodeToString(digest[:]))) {
		return "", "", errors.New("backup database checksum mismatch")
	}
	expectedHMAC, err := backupHMAC(manifest, database, key)
	if err != nil {
		return "", "", err
	}
	providedHMAC, err := hex.DecodeString(manifest.IntegrityHMAC)
	if err != nil || !hmac.Equal(providedHMAC, mustDecodeHex(expectedHMAC)) {
		return "", "", errors.New("backup integrity HMAC mismatch")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", "", err
	}
	databasePath := filepath.Join(directory, "restored.db")
	if err := os.WriteFile(databasePath, database, 0o600); err != nil {
		return "", "", err
	}
	keyPath := filepath.Join(directory, "restored-key.json")
	if err := writeKeyRecord(keyPath, keyRecord{
		DatabaseID: manifest.DatabaseID,
		KeyVersion: manifest.KeyVersion,
		KeyBase64:  manifest.KeyBase64,
	}); err != nil {
		return "", "", err
	}
	return databasePath, keyPath, nil
}

func createTamperedBackup(sourcePath string, targetPath string) error {
	manifest, database, err := readBackup(sourcePath)
	if err != nil {
		return err
	}
	database[len(database)/2] ^= 0xff
	return writeBackup(targetPath, manifest, database)
}

func mustDecodeHex(value string) []byte {
	decoded, _ := hex.DecodeString(value)
	return decoded
}
