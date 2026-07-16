import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import { MetricQuery } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import type {
  MetricQueryResult,
  MetricRealtimeEvent,
  MetricSample,
  MetricStorageStatus,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import type { MinecraftServer } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import type { MonitoringOverview } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/service/models'
import { listMinecraftServers } from '../services/minecraft-server-api'
import { getMonitoringOverview } from '../services/monitoring-api'
import {
  getLatestMetrics,
  getMetricStorageStatus,
  queryMetrics,
  subscribeMetricRealtime,
} from '../services/metric-api'

export const useMonitoringStore = defineStore('monitoring', () => {
  const trendMetrics = [
    'host.cpu',
    'host.memory',
    'host.disk.used',
    'host.network.receive_bytes_per_second',
    'process.cpu',
    'process.rss',
  ]
  const servers = ref<MinecraftServer[]>([])
  const selectedServerID = ref('')
  const selectedMetric = ref('host.cpu')
  const rangeHours = ref(24)
  const granularity = ref('minute')
  const latest = ref<MetricSample[]>([])
  const history = ref<MetricQueryResult | null>(null)
  const trendHistories = ref<Record<string, MetricQueryResult>>({})
  const storage = ref<MetricStorageStatus | null>(null)
  const overview = ref<MonitoringOverview | null>(null)
  const loading = ref(false)
  const serversError = ref<unknown>(null)
  const storageError = ref<unknown>(null)
  const latestError = ref<unknown>(null)
  const historyError = ref<unknown>(null)
  const trendsError = ref<unknown>(null)
  const overviewError = ref<unknown>(null)
  const dataServerID = ref('')
  const error = computed(() =>
    serversError.value && servers.value.length === 0 ? serversError.value : null,
  )
  const partialMessage = computed(() => {
    const missing: string[] = []
    if (storageError.value) missing.push('存储容量')
    if (latestError.value) missing.push('实时值')
    if (historyError.value) missing.push('历史趋势')
    if (trendsError.value) missing.push('指标概览趋势')
    if (overviewError.value) missing.push('统一状态')
    return missing.length ? `${missing.join('、')} 暂时不可用；其余已成功数据继续显示。` : ''
  })
  let unsubscribe: (() => void) | null = null
  let refreshSequence = 0

  const selectedServer = computed(
    () => servers.value.find((server) => server.id === selectedServerID.value) ?? null,
  )
  const latestByMetric = computed(() => {
    const values = new Map<string, MetricSample>()
    for (const sample of latest.value) {
      const current = values.get(sample.metric)
      if (!current || Date.parse(sample.timestamp) >= Date.parse(current.timestamp)) {
        values.set(sample.metric, sample)
      }
    }
    return values
  })

  /** Loads Servers and selects the first available target. */
  async function loadServers(preferredServerID = ''): Promise<void> {
    serversError.value = null
    try {
      servers.value = await listMinecraftServers()
    } catch (reason) {
      serversError.value = reason
      throw reason
    }
    const preferred = servers.value.find((server) => server.id === preferredServerID)
    if (preferred) selectedServerID.value = preferred.id
    if (
      !selectedServerID.value ||
      !servers.value.some((server) => server.id === selectedServerID.value)
    ) {
      selectedServerID.value = servers.value[0]?.id ?? ''
    }
  }

  /** Refreshes latest values, history, and storage status for the selected Server. */
  async function refresh(): Promise<void> {
    const sequence = ++refreshSequence
    loading.value = true
    storageError.value = null
    latestError.value = null
    historyError.value = null
    trendsError.value = null
    overviewError.value = null
    try {
      if (servers.value.length === 0) {
        try {
          await loadServers()
        } catch {
          /* page-level error is retained */
        }
      }
      const storageResult = await Promise.allSettled([getMetricStorageStatus()])
      if (storageResult[0].status === 'fulfilled') storage.value = storageResult[0].value
      else storageError.value = storageResult[0].reason
      if (!selectedServerID.value) {
        latest.value = []
        history.value = null
        trendHistories.value = {}
        overview.value = null
        dataServerID.value = ''
        return
      }
      const targetServerID = selectedServerID.value
      if (dataServerID.value !== targetServerID) {
        latest.value = []
        history.value = null
        trendHistories.value = {}
        overview.value = null
      }
      const end = new Date()
      const start = new Date(end.getTime() - rangeHours.value * 60 * 60 * 1000)
      const timeZone = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
      const metricNames = [...new Set([selectedMetric.value, ...trendMetrics])]
      const latestRequest = getLatestMetrics(targetServerID)
      const overviewRequest = getMonitoringOverview(targetServerID)
      const metricRequests = metricNames.map((metric) =>
        queryMetrics(
          new MetricQuery({
            serverID: targetServerID,
            metric,
            start: start.toISOString(),
            end: end.toISOString(),
            granularity: granularity.value,
            timeZone,
            limit: 100_000,
            offset: 0,
          }),
        ),
      )
      const [latestResult, overviewResult] = await Promise.allSettled([
        latestRequest,
        overviewRequest,
      ])
      const metricResults = await Promise.allSettled(metricRequests)
      if (sequence !== refreshSequence || selectedServerID.value !== targetServerID) return
      dataServerID.value = targetServerID
      if (latestResult.status === 'fulfilled') latest.value = latestResult.value
      else latestError.value = latestResult.reason
      if (overviewResult.status === 'fulfilled') overview.value = overviewResult.value
      else overviewError.value = overviewResult.reason
      const selectedHistoryResult = metricResults[metricNames.indexOf(selectedMetric.value)]
      if (selectedHistoryResult?.status === 'fulfilled') history.value = selectedHistoryResult.value
      else historyError.value = selectedHistoryResult?.reason ?? new Error('未返回所选指标趋势')
      const nextTrendHistories: Record<string, MetricQueryResult> = {}
      for (const metric of trendMetrics) {
        const result = metricResults[metricNames.indexOf(metric)]
        if (result?.status === 'fulfilled') nextTrendHistories[metric] = result.value
        else trendsError.value = result?.reason ?? new Error(`未返回 ${metric} 趋势`)
      }
      trendHistories.value = nextTrendHistories
    } finally {
      if (sequence === refreshSequence) loading.value = false
    }
  }

  function mergeRealtime(event: MetricRealtimeEvent): void {
    if (event.serverID !== selectedServerID.value) return
    const merged = new Map(
      latest.value.map((sample) => [`${sample.sourceID}\0${sample.metric}`, sample]),
    )
    for (const sample of event.samples) merged.set(`${sample.sourceID}\0${sample.metric}`, sample)
    latest.value = [...merged.values()]
  }

  /** Starts the throttled Wails Metric event subscription. */
  function startSubscription(): void {
    unsubscribe?.()
    unsubscribe = subscribeMetricRealtime(mergeRealtime)
  }

  /** Stops the Wails Metric event subscription. */
  function stopSubscription(): void {
    unsubscribe?.()
    unsubscribe = null
  }

  return {
    error,
    granularity,
    history,
    latest,
    latestByMetric,
    loading,
    loadServers,
    overview,
    overviewError,
    partialMessage,
    rangeHours,
    refresh,
    selectedMetric,
    selectedServer,
    selectedServerID,
    servers,
    startSubscription,
    stopSubscription,
    storage,
    storageError,
    trendHistories,
  }
})
