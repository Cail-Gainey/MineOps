import {
  LoggingService,
  type LogStatusResult,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services'
import type { LogStatus } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/service/models'
import { throwIfError } from './api-client'

function unwrapLogStatus(result: LogStatusResult): LogStatus {
  throwIfError(result.error)
  if (!result.status) throw new Error('LoggingService 未返回日志状态')
  return result.status
}

export async function getLogStatus(): Promise<LogStatus> {
  return unwrapLogStatus(await LoggingService.GetStatus())
}

export async function clearArchivedLogs(): Promise<LogStatus> {
  return unwrapLogStatus(await LoggingService.ClearArchived())
}

export async function openLogDirectory(): Promise<void> {
  throwIfError((await LoggingService.OpenDirectory()).error)
}
