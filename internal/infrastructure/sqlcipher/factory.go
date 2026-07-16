package sqlcipher

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/constants"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ConnectionOptions contains the controlled SQLCipher connection and pool settings.
type ConnectionOptions struct {
	Path         string
	Key          []byte
	MaxOpenConns int
	MaxIdleConns int
}

// Connection owns the GORM database and its underlying SQL connection pool.
type Connection struct {
	database *gorm.DB
	pool     *sql.DB
}

// OpenConnection opens an encrypted GORM database with the MineOps SQLCipher baseline.
func OpenConnection(ctx context.Context, options ConnectionOptions) (*Connection, error) {
	if options.Path == "" || len(options.Key) != 32 {
		return nil, apperror.New(apperror.CodeValidationInvalidArgument, "数据库路径和 256-bit 密钥不能为空")
	}
	if err := os.MkdirAll(filepath.Dir(options.Path), 0o700); err != nil {
		return nil, apperror.Wrap(apperror.CodeIOWriteFailed, "创建数据库目录失败", err)
	}
	dsn := buildDSN(options.Path, options.Key)
	database, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeCryptoDecryptFailed, "打开加密数据库失败", err)
	}
	pool, err := database.DB()
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, "获取数据库连接池失败", err)
	}
	maxOpen := options.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 4
	}
	maxIdle := options.MaxIdleConns
	if maxIdle <= 0 || maxIdle > maxOpen {
		maxIdle = maxOpen
	}
	pool.SetMaxOpenConns(maxOpen)
	pool.SetMaxIdleConns(maxIdle)
	if err := pool.PingContext(ctx); err != nil {
		_ = pool.Close()
		return nil, apperror.Wrap(apperror.CodeCryptoDecryptFailed, "验证加密数据库连接失败", err)
	}
	return &Connection{database: database, pool: pool}, nil
}

func buildDSN(path string, key []byte) string {
	return fmt.Sprintf(
		"%s?_pragma_key=x'%s'&_pragma_cipher_page_size=4096&_foreign_keys=on&_journal_mode=WAL&_busy_timeout=%d",
		path,
		hex.EncodeToString(key),
		constants.DefaultBusyTimeoutMillis,
	)
}

// GORM returns the composition-root database handle used only to construct repositories and migrations.
func (c *Connection) GORM() *gorm.DB {
	if c == nil {
		return nil
	}
	return c.database
}

// Close closes the underlying SQL connection pool.
func (c *Connection) Close() error {
	if c == nil || c.pool == nil {
		return nil
	}
	return c.pool.Close()
}
