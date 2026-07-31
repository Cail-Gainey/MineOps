package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/infrastructure/sqlcipher"
)

// SQLCipherSpikeService 对外暴露加密 SQLite 的基线验证。
type SQLCipherSpikeService struct{}

// Run 执行临时的 SQLCipher 与 GORM 验证,不持久化密钥。
func (s *SQLCipherSpikeService) Run(ctx context.Context) (sqlcipher.SpikeResult, error) {
	return sqlcipher.RunSpike(ctx)
}
