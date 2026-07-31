import type { MonitoringOverview } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/service/models'
import {
  ClearHistory,
  CollectNow,
  Overview,
  Pause,
  Resume,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/monitoringservice'
import { throwIfError } from './api-client'
import { translate } from '../locales/runtime'

/**
 * 读取某台 Server 的监控统一状态。
 * @param serverID - 目标 Server ID
 * @returns 监控总览
 */
export async function getMonitoringOverview(serverID: string): Promise<MonitoringOverview> {
  const result = await Overview(serverID)
  throwIfError(result.error)
  if (!result.overview)
    throw new Error(
      translate('service.missingField', { service: 'MonitoringService', field: 'Overview' }),
    )
  return result.overview
}

/**
 * 立即触发一次采集。
 * @param serverID - 目标 Server ID
 * @returns 采集完成后的 Promise
 */
export async function collectNow(serverID: string): Promise<void> {
  const result = await CollectNow(serverID)
  throwIfError(result.error)
}

/**
 * 暂停某台 Server 的自动采集并停止远端采集器。
 * @param serverID - 目标 Server ID
 * @returns 暂停完成后的 Promise
 */
export async function pauseMonitoring(serverID: string): Promise<void> {
  const result = await Pause(serverID)
  throwIfError(result.error)
}

/**
 * 恢复某台 Server 的自动采集并重新部署远端采集器。
 * @param serverID - 目标 Server ID
 * @returns 恢复完成后的 Promise
 */
export async function resumeMonitoring(serverID: string): Promise<void> {
  const result = await Resume(serverID)
  throwIfError(result.error)
}

/**
 * 清理某台 Server 的全部监控历史数据。
 * @param serverID - 目标 Server ID
 * @returns 清理完成后的 Promise
 */
export async function clearMonitoringHistory(serverID: string): Promise<void> {
  const result = await ClearHistory(serverID)
  throwIfError(result.error)
}
