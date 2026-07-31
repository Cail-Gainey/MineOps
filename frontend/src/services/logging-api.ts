import {
  LoggingService,
  type LogStatusResult,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services'
import type { LogStatus } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/service/models'
import { throwIfError } from './api-client'
import { translate } from '../locales/runtime'

/**
 * 抛出日志接口返回的错误并取出状态本体。
 * @param result - LoggingService 的原始返回值
 * @returns 日志状态
 */
function unwrapLogStatus(result: LogStatusResult): LogStatus {
  throwIfError(result.error)
  if (!result.status)
    throw new Error(
      translate('service.missingField', {
        service: 'LoggingService',
        field: translate('serviceField.logStatus'),
      }),
    )
  return result.status
}

/**
 * 读取日志目录容量与归档状态。
 * @returns 日志状态
 */
export async function getLogStatus(): Promise<LogStatus> {
  return unwrapLogStatus(await LoggingService.GetStatus())
}

/**
 * 清理已归档的日志文件。
 * @returns 清理后的日志状态
 */
export async function clearArchivedLogs(): Promise<LogStatus> {
  return unwrapLogStatus(await LoggingService.ClearArchived())
}

/**
 * 用系统文件管理器打开日志目录。
 * @returns 打开完成后的 Promise
 */
export async function openLogDirectory(): Promise<void> {
  throwIfError((await LoggingService.OpenDirectory()).error)
}
