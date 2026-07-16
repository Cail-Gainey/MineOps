package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/infrastructure/sqlcipher"
)

// SQLCipherSpikeService exposes the encrypted SQLite stage 0 verification.
type SQLCipherSpikeService struct{}

// Run executes the temporary SQLCipher and GORM verification without persisting the key.
func (s *SQLCipherSpikeService) Run(ctx context.Context) (sqlcipher.SpikeResult, error) {
	return sqlcipher.RunSpike(ctx)
}
