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

// ConnectionOptions 承载受控的 SQLCipher 连接与连接池设置。
type ConnectionOptions struct {
	Path         string
	Key          []byte
	MaxOpenConns int
	MaxIdleConns int
}

// Connection 持有 GORM 数据库句柄及其底层 SQL 连接池。
type Connection struct {
	database *gorm.DB
	pool     *sql.DB
}

// OpenConnection 按 MineOps 的 SQLCipher 基线打开加密 GORM 数据库。
func OpenConnection(ctx context.Context, options ConnectionOptions) (*Connection, error) {
	if options.Path == "" || len(options.Key) != 32 {
		return nil, apperror.New(apperror.CodeValidationInvalidArgument, "数据库路径和 256-bit 密钥不能为空")
	}
	return openDatabase(ctx, buildDSN(options.Path, options.Key), options, "打开加密数据库失败", "验证加密数据库连接失败")
}

// OpenPlainConnection 打开未加密的 GORM 数据库,用于不含凭据与隐私的批量数据。
// go.mod 把 mattn/go-sqlite3 替换成了 SQLCipher 驱动,不带 _pragma_key 的连接就是标准 SQLite:
// 监控时序是纯数值,放在这里可以完全省掉逐页 AES 加解密与 HMAC 校验。
func OpenPlainConnection(ctx context.Context, options ConnectionOptions) (*Connection, error) {
	if options.Path == "" {
		return nil, apperror.New(apperror.CodeValidationInvalidArgument, "数据库路径不能为空")
	}
	dsn := fmt.Sprintf("%s?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=%d", options.Path, constants.DefaultBusyTimeoutMillis)
	return openDatabase(ctx, dsn, options, "打开监控数据库失败", "验证监控数据库连接失败")
}

func openDatabase(ctx context.Context, dsn string, options ConnectionOptions, openMessage, pingMessage string) (*Connection, error) {
	if err := os.MkdirAll(filepath.Dir(options.Path), 0o700); err != nil {
		return nil, apperror.Wrap(apperror.CodeIOWriteFailed, "创建数据库目录失败", err)
	}
	database, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeCryptoDecryptFailed, openMessage, err)
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
		return nil, apperror.Wrap(apperror.CodeCryptoDecryptFailed, pingMessage, err)
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

// GORM 返回组装根的数据库句柄,仅用于构造 Repository 与迁移。
func (c *Connection) GORM() *gorm.DB {
	if c == nil {
		return nil
	}
	return c.database
}

// Close 关闭底层的 SQL 连接池。
func (c *Connection) Close() error {
	if c == nil || c.pool == nil {
		return nil
	}
	return c.pool.Close()
}
