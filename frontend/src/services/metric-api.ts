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

export const metricRealtimeEventName = 'mineops:metric:realtime'

/** Returns latest cached or persisted values for one Server. */
export async function getLatestMetrics(serverID: string): Promise<MetricSample[]> {
  const result = await Latest(serverID)
  throwIfError(result.error)
  return result.samples
}

/** Queries raw, minute, or hour Metric series. */
export async function queryMetrics(query: MetricQuery): Promise<MetricQueryResult> {
  const result = await Query(query)
  throwIfError(result.error)
  if (!result.result) throw new Error('MetricService 未返回查询结果')
  return result.result
}

/** Returns Metric database capacity evidence. */
export async function getMetricStorageStatus(): Promise<MetricStorageStatus> {
  const result = await StorageStatus()
  throwIfError(result.error)
  if (!result.status) throw new Error('MetricService 未返回存储状态')
  return result.status
}

/** Runs one bounded downsample and retention pass. */
export async function runMetricMaintenance(): Promise<MetricMaintenanceResult> {
  const result = await RunMaintenance()
  throwIfError(result.error)
  if (!result.result) throw new Error('MetricService 未返回维护结果')
  return result.result
}

/** Subscribes to throttled latest-value events. */
export function subscribeMetricRealtime(
  listener: (event: MetricRealtimeEvent) => void,
): () => void {
  return Events.On(metricRealtimeEventName, (event) => listener(event.data))
}
