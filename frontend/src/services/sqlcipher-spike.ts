import { SQLCipherSpikeService } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services'

/**
 * @returns SQLCipher 与 GORM 临时数据库验证结果
 */
export function runSQLCipherSpike() {
  return SQLCipherSpikeService.Run()
}
