package sqlcipher

import (
	"archive/zip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	sqlcipherdriver "github.com/WissCore/go-sqlcipher/v4"
)

const portableBackupFormatVersion = 1

type portableBackupManifest struct {
	FormatVersion  int       `json:"formatVersion"`
	DatabaseID     string    `json:"databaseID"`
	SchemaVersion  int       `json:"schemaVersion"`
	KeyVersion     int       `json:"keyVersion"`
	KeyBase64      string    `json:"keyBase64"`
	DatabaseSHA256 string    `json:"databaseSHA256"`
	CreatedAt      time.Time `json:"createdAt"`
	IntegrityHMAC  string    `json:"integrityHMAC"`
}

// BackupInfo 描述一份已完成的便携加密 SQLite 备份。
type BackupInfo struct {
	Path          string    `json:"path"`
	DatabaseID    string    `json:"databaseID"`
	SchemaVersion int       `json:"schemaVersion"`
	KeyVersion    int       `json:"keyVersion"`
	CreatedAt     time.Time `json:"createdAt"`
	Bytes         int64     `json:"bytes"`
}

// BackupManager 基于一条活动 SQLCipher 连接创建一致的在线快照。
type BackupManager struct {
	connection   *Connection
	databasePath string
	keyStore     KeyStore
}

// InspectPortableBackup 校验一份备份,不改动当前数据库与系统密钥存储。
func InspectPortableBackup(backupPath string) (BackupInfo, error) {
	if strings.TrimSpace(backupPath) == "" {
		return BackupInfo{}, apperror.New(apperror.CodeValidationRequired, "备份路径不能为空")
	}
	temporaryDirectory, err := os.MkdirTemp("", ".mineops-backup-inspect-")
	if err != nil {
		return BackupInfo{}, apperror.Wrap(apperror.CodeIOWriteFailed, "创建备份检查目录失败", err)
	}
	defer func() { _ = os.RemoveAll(temporaryDirectory) }()
	manifest, _, err := extractAndVerifyPortableBackup(backupPath, filepath.Join(temporaryDirectory, "database.db"))
	if err != nil {
		return BackupInfo{}, err
	}
	info, err := os.Stat(backupPath)
	if err != nil {
		return BackupInfo{}, apperror.Wrap(apperror.CodeIOReadFailed, "读取备份文件信息失败", err)
	}
	return BackupInfo{
		Path: backupPath, DatabaseID: manifest.DatabaseID, SchemaVersion: manifest.SchemaVersion,
		KeyVersion: manifest.KeyVersion, CreatedAt: manifest.CreatedAt, Bytes: info.Size(),
	}, nil
}

// NewBackupManager 为一个活动加密数据库创建便携备份边界。
func NewBackupManager(connection *Connection, databasePath string, keyStore KeyStore) (*BackupManager, error) {
	if connection == nil || databasePath == "" || keyStore == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "BackupManager 依赖不能为空")
	}
	return &BackupManager{connection: connection, databasePath: databasePath, keyStore: keyStore}, nil
}

// Create 原子写入一个 .mineops-backup,内含在线快照、身份、schema、校验和与恢复密钥包。
func (m *BackupManager) Create(ctx context.Context, destination string) (BackupInfo, error) {
	if filepath.Ext(destination) != ".mineops-backup" {
		return BackupInfo{}, apperror.New(apperror.CodeValidationInvalidArgument, "备份文件必须使用 .mineops-backup 扩展名")
	}
	key, err := m.keyStore.Load(ctx)
	if err != nil {
		return BackupInfo{}, err
	}
	if !key.DatabaseCreated {
		return BackupInfo{}, apperror.New(apperror.CodeValidationConflict, "数据库尚未完成初始化")
	}
	runner, err := NewMigrationRunner(m.connection.GORM(), DefaultMigrations())
	if err != nil {
		return BackupInfo{}, err
	}
	schemaVersion, err := runner.CurrentVersion(ctx)
	if err != nil {
		return BackupInfo{}, err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return BackupInfo{}, apperror.Wrap(apperror.CodeIOWriteFailed, "创建备份目录失败", err)
	}
	temporaryDirectory, err := os.MkdirTemp(filepath.Dir(destination), ".mineops-backup-")
	if err != nil {
		return BackupInfo{}, apperror.Wrap(apperror.CodeIOWriteFailed, "创建备份临时目录失败", err)
	}
	defer func() { _ = os.RemoveAll(temporaryDirectory) }()
	snapshotPath := filepath.Join(temporaryDirectory, "database.db")
	if err := createOnlineSnapshot(ctx, m.connection, snapshotPath, key.Key); err != nil {
		return BackupInfo{}, err
	}
	digest, err := hashFile(snapshotPath)
	if err != nil {
		return BackupInfo{}, err
	}
	manifest := portableBackupManifest{
		FormatVersion: portableBackupFormatVersion, DatabaseID: key.DatabaseID, SchemaVersion: schemaVersion,
		KeyVersion: key.Version, KeyBase64: base64.StdEncoding.EncodeToString(key.Key),
		DatabaseSHA256: digest, CreatedAt: time.Now().UTC(),
	}
	manifest.IntegrityHMAC, err = portableBackupHMAC(manifest, snapshotPath, key.Key)
	if err != nil {
		return BackupInfo{}, err
	}
	temporaryBackup := filepath.Join(temporaryDirectory, "backup.mineops-backup")
	if err := writePortableBackup(temporaryBackup, manifest, snapshotPath); err != nil {
		return BackupInfo{}, err
	}
	if err := os.Chmod(temporaryBackup, 0o600); err != nil {
		return BackupInfo{}, apperror.Wrap(apperror.CodeIOPermissionDenied, "设置备份文件权限失败", err)
	}
	if err := os.Rename(temporaryBackup, destination); err != nil {
		return BackupInfo{}, apperror.Wrap(apperror.CodeIOWriteFailed, "提交备份文件失败", err)
	}
	info, err := os.Stat(destination)
	if err != nil {
		return BackupInfo{}, apperror.Wrap(apperror.CodeIOReadFailed, "读取备份文件信息失败", err)
	}
	return BackupInfo{
		Path: destination, DatabaseID: manifest.DatabaseID, SchemaVersion: manifest.SchemaVersion,
		KeyVersion: manifest.KeyVersion, CreatedAt: manifest.CreatedAt, Bytes: info.Size(),
	}, nil
}

// RestorePortableBackup 校验并原子安装一份备份,同时导入其系统密钥记录。
func RestorePortableBackup(ctx context.Context, backupPath string, databasePath string, keyStore KeyStore) (BackupInfo, error) {
	if backupPath == "" || databasePath == "" || keyStore == nil {
		return BackupInfo{}, apperror.New(apperror.CodeValidationRequired, "恢复路径和 KeyStore 不能为空")
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		return BackupInfo{}, apperror.Wrap(apperror.CodeIOWriteFailed, "创建数据库目录失败", err)
	}
	temporaryDirectory, err := os.MkdirTemp(filepath.Dir(databasePath), ".mineops-restore-")
	if err != nil {
		return BackupInfo{}, apperror.Wrap(apperror.CodeIOWriteFailed, "创建恢复临时目录失败", err)
	}
	defer func() { _ = os.RemoveAll(temporaryDirectory) }()
	restoredPath := filepath.Join(temporaryDirectory, "database.db")
	manifest, key, err := extractAndVerifyPortableBackup(backupPath, restoredPath)
	if err != nil {
		return BackupInfo{}, err
	}
	connection, err := OpenConnection(ctx, ConnectionOptions{Path: restoredPath, Key: key})
	if err != nil {
		return BackupInfo{}, err
	}
	var metadata databaseMetadataRecord
	queryErr := connection.GORM().WithContext(ctx).First(&metadata).Error
	closeErr := connection.Close()
	if queryErr != nil {
		return BackupInfo{}, apperror.Wrap(apperror.CodeCryptoIntegrityFailed, "备份数据库身份不可读取", queryErr)
	}
	if closeErr != nil {
		return BackupInfo{}, apperror.Wrap(apperror.CodeIOWriteFailed, "关闭恢复验证连接失败", closeErr)
	}
	if metadata.DatabaseID != manifest.DatabaseID || metadata.KeyVersion != manifest.KeyVersion {
		return BackupInfo{}, apperror.New(apperror.CodeCryptoIntegrityFailed, "备份数据库身份与恢复密钥不匹配")
	}

	previousPath := databasePath + ".restore-previous"
	_ = os.Remove(previousPath)
	databaseExisted := false
	if _, err := os.Stat(databasePath); err == nil {
		databaseExisted = true
		if err := os.Rename(databasePath, previousPath); err != nil {
			return BackupInfo{}, apperror.Wrap(apperror.CodeIOWriteFailed, "暂存当前数据库失败", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return BackupInfo{}, apperror.Wrap(apperror.CodeIOReadFailed, "检查当前数据库失败", err)
	}
	if err := os.Rename(restoredPath, databasePath); err != nil {
		if databaseExisted {
			_ = os.Rename(previousPath, databasePath)
		}
		return BackupInfo{}, apperror.Wrap(apperror.CodeIOWriteFailed, "安装恢复数据库失败", err)
	}
	restoredKey := DatabaseKey{DatabaseID: manifest.DatabaseID, Version: manifest.KeyVersion, DatabaseCreated: true, Key: key}
	if err := keyStore.Save(ctx, restoredKey); err != nil {
		_ = os.Remove(databasePath)
		if databaseExisted {
			_ = os.Rename(previousPath, databasePath)
		}
		return BackupInfo{}, err
	}
	_ = os.Remove(previousPath)
	info, err := os.Stat(backupPath)
	if err != nil {
		return BackupInfo{}, apperror.Wrap(apperror.CodeIOReadFailed, "读取备份文件信息失败", err)
	}
	return BackupInfo{
		Path: backupPath, DatabaseID: manifest.DatabaseID, SchemaVersion: manifest.SchemaVersion,
		KeyVersion: manifest.KeyVersion, CreatedAt: manifest.CreatedAt, Bytes: info.Size(),
	}, nil
}

func createOnlineSnapshot(ctx context.Context, source *Connection, destinationPath string, key []byte) error {
	destinationDatabase, err := sql.Open("sqlite3", buildDSN(destinationPath, key))
	if err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "打开备份快照数据库失败", err)
	}
	defer func() { _ = destinationDatabase.Close() }()
	if err := destinationDatabase.PingContext(ctx); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "初始化备份快照数据库失败", err)
	}
	sourceConnection, err := source.pool.Conn(ctx)
	if err != nil {
		return apperror.Wrap(apperror.CodeIOReadFailed, "获取源数据库连接失败", err)
	}
	defer func() { _ = sourceConnection.Close() }()
	destinationConnection, err := destinationDatabase.Conn(ctx)
	if err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "获取快照数据库连接失败", err)
	}
	defer func() { _ = destinationConnection.Close() }()
	if err := sourceConnection.Raw(func(sourceDriver any) error {
		sourceSQLite, ok := sourceDriver.(*sqlcipherdriver.SQLiteConn)
		if !ok {
			return errors.New("source connection is not SQLCipher SQLite")
		}
		return destinationConnection.Raw(func(destinationDriver any) error {
			destinationSQLite, ok := destinationDriver.(*sqlcipherdriver.SQLiteConn)
			if !ok {
				return errors.New("destination connection is not SQLCipher SQLite")
			}
			backup, err := destinationSQLite.Backup("main", sourceSQLite, "main")
			if err != nil {
				return err
			}
			if _, err := backup.Step(-1); err != nil {
				_ = backup.Close()
				return err
			}
			return backup.Finish()
		})
	}); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建 SQLite Online Backup 失败", err)
	}
	return nil
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", apperror.Wrap(apperror.CodeIOReadFailed, "读取备份快照失败", err)
	}
	defer func() { _ = file.Close() }()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", apperror.Wrap(apperror.CodeIOReadFailed, "计算备份摘要失败", err)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func portableBackupHMAC(manifest portableBackupManifest, databasePath string, key []byte) (string, error) {
	manifest.IntegrityHMAC = ""
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	file, err := os.Open(databasePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(manifestData)
	if _, err := io.Copy(mac, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func writePortableBackup(path string, manifest portableBackupManifest, databasePath string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建备份包失败", err)
	}
	writer := zip.NewWriter(file)
	manifestEntry, err := writer.CreateHeader(&zip.FileHeader{Name: "manifest.json", Method: zip.Deflate})
	if err == nil {
		var encoded []byte
		encoded, err = json.Marshal(manifest)
		if err == nil {
			_, err = manifestEntry.Write(encoded)
		}
	}
	if err == nil {
		var databaseFile *os.File
		databaseFile, err = os.Open(databasePath)
		if err == nil {
			defer func() { _ = databaseFile.Close() }()
			var databaseEntry io.Writer
			databaseEntry, err = writer.CreateHeader(&zip.FileHeader{Name: "database.db", Method: zip.Store})
			if err == nil {
				_, err = io.Copy(databaseEntry, databaseFile)
			}
		}
	}
	writerCloseErr := writer.Close()
	fileCloseErr := file.Close()
	if err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "写入备份包失败", err)
	}
	if writerCloseErr != nil {
		return writerCloseErr
	}
	return fileCloseErr
}

func extractAndVerifyPortableBackup(path string, databasePath string) (portableBackupManifest, []byte, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return portableBackupManifest{}, nil, apperror.Wrap(apperror.CodeCryptoIntegrityFailed, "打开备份包失败", err)
	}
	defer func() { _ = reader.Close() }()
	var manifest portableBackupManifest
	foundDatabase := false
	for _, entry := range reader.File {
		switch entry.Name {
		case "manifest.json":
			if entry.UncompressedSize64 > 1024*1024 {
				return portableBackupManifest{}, nil, apperror.New(apperror.CodeCryptoIntegrityFailed, "备份 Manifest 超出限制")
			}
			entryReader, err := entry.Open()
			if err != nil {
				return portableBackupManifest{}, nil, err
			}
			encoded, readErr := io.ReadAll(io.LimitReader(entryReader, 1024*1024+1))
			_ = entryReader.Close()
			if readErr != nil {
				return portableBackupManifest{}, nil, readErr
			}
			if err := json.Unmarshal(encoded, &manifest); err != nil {
				return portableBackupManifest{}, nil, apperror.Wrap(apperror.CodeCryptoIntegrityFailed, "备份 Manifest 无效", err)
			}
		case "database.db":
			entryReader, err := entry.Open()
			if err != nil {
				return portableBackupManifest{}, nil, err
			}
			destination, err := os.OpenFile(databasePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if err != nil {
				_ = entryReader.Close()
				return portableBackupManifest{}, nil, err
			}
			_, copyErr := io.Copy(destination, entryReader)
			closeDestinationErr := destination.Close()
			_ = entryReader.Close()
			if copyErr != nil {
				return portableBackupManifest{}, nil, copyErr
			}
			if closeDestinationErr != nil {
				return portableBackupManifest{}, nil, closeDestinationErr
			}
			foundDatabase = true
		default:
			return portableBackupManifest{}, nil, apperror.New(apperror.CodeCryptoIntegrityFailed, "备份包包含未知条目").WithDetails(map[string]any{"entry": entry.Name})
		}
	}
	if manifest.FormatVersion != portableBackupFormatVersion || manifest.DatabaseID == "" || !foundDatabase {
		return portableBackupManifest{}, nil, apperror.New(apperror.CodeCryptoIntegrityFailed, "备份包缺少必要数据")
	}
	key, err := base64.StdEncoding.DecodeString(manifest.KeyBase64)
	if err != nil || len(key) != 32 {
		return portableBackupManifest{}, nil, apperror.New(apperror.CodeCryptoIntegrityFailed, "备份恢复密钥无效")
	}
	digest, err := hashFile(databasePath)
	if err != nil {
		return portableBackupManifest{}, nil, err
	}
	if !strings.EqualFold(digest, manifest.DatabaseSHA256) {
		return portableBackupManifest{}, nil, apperror.New(apperror.CodeCryptoIntegrityFailed, "备份数据库摘要不匹配")
	}
	expectedHMAC, err := portableBackupHMAC(manifest, databasePath, key)
	if err != nil {
		return portableBackupManifest{}, nil, err
	}
	provided, err := hex.DecodeString(manifest.IntegrityHMAC)
	expected, expectedErr := hex.DecodeString(expectedHMAC)
	if err != nil || expectedErr != nil || !hmac.Equal(provided, expected) {
		return portableBackupManifest{}, nil, apperror.New(apperror.CodeCryptoIntegrityFailed, "备份完整性 HMAC 不匹配")
	}
	return manifest, key, nil
}
