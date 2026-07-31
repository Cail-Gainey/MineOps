import { Events } from '@wailsio/runtime'

import type {
  MetricMaintenanceResult,
  MetricQuery,
  MetricQueryResult,
  MetricRealtimeEvent,
  MetricSample,
  MetricStorageStatus,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import {
  Latest,
  Query,
  RunMaintenance,
  StorageStatus,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/metricservice'
import { throwIfError } from './api-client'
import { translate } from '../locales/runtime'

export const metricRealtimeEventName = 'mineops:metric:realtime'

/**
 * 读取某台 Server 每个指标的最新值。
 * @param serverID - 目标 Server ID
 * @returns 最新样本数组
 */
export async function getLatestMetrics(serverID: string): Promise<MetricSample[]> {
  const result = await Latest(serverID)
  throwIfError(result.error)
  return result.samples
}

/**
 * 按原始、分钟或小时粒度查询指标序列。
 * @param query - 指标查询条件
 * @returns 指标查询结果
 */
export async function queryMetrics(query: MetricQuery): Promise<MetricQueryResult> {
  const result = await Query(query)
  throwIfError(result.error)
  if (!result.result)
    throw new Error(
      translate('service.missingField', {
        service: 'MetricService',
        field: translate('serviceField.queryResult'),
      }),
    )
  return result.result
}

/**
 * 读取指标数据库的容量占用证据。
 * @returns 指标存储状态
 */
export async function getMetricStorageStatus(): Promise<MetricStorageStatus> {
  const result = await StorageStatus()
  throwIfError(result.error)
  if (!result.status)
    throw new Error(
      translate('service.missingField', {
        service: 'MetricService',
        field: translate('serviceField.storageStatus'),
      }),
    )
  return result.status
}

/**
 * 立即执行一轮有界的降采样与保留清理。
 * @returns 维护结果
 */
export async function runMetricMaintenance(): Promise<MetricMaintenanceResult> {
  const result = await RunMaintenance()
  throwIfError(result.error)
  if (!result.result)
    throw new Error(
      translate('service.missingField', {
        service: 'MetricService',
        field: translate('serviceField.maintenanceResult'),
      }),
    )
  return result.result
}

/**
 * 订阅经过节流的最新指标推送。
 * @param listener - 收到事件时的回调
 * @returns 取消订阅函数
 */
export function subscribeMetricRealtime(
  listener: (event: MetricRealtimeEvent) => void,
): () => void {
  return Events.On(metricRealtimeEventName, (event) => listener(event.data))
}
