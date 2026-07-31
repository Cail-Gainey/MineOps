import { defineStore } from 'pinia'
import { computed, ref, shallowRef } from 'vue'

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

/** 一次刷新的目标 Server 与时间窗口，用于把小趋势图加载推迟到页面骨架渲染之后。 */
interface TrendContext {
  serverID: string
  start: Date
  end: Date
  timeZone: string
}

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
  // Metric 查询结果整块替换、从不就地修改，用 shallowRef 跳过 Vue 深层代理：
  // 24 小时分钟粒度下七条序列合计上万个点，深层响应式会为每个点生成 Proxy，
  // 打开监控页时这部分开销与图表初始化叠加，正是 CPU 短暂冲高的前端来源。
  const latest = shallowRef<MetricSample[]>([])
  const history = shallowRef<MetricQueryResult | null>(null)
  const trendHistories = shallowRef<Record<string, MetricQueryResult>>({})
  const storage = shallowRef<MetricStorageStatus | null>(null)
  const overview = shallowRef<MonitoringOverview | null>(null)
  const loading = ref(false)
  const trendsLoading = ref(false)
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

  /**
   * 加载 Server 列表并选定目标,优先使用传入的偏好 Server。
   * @param preferredServerID - 优先选中的 Server ID,为空时选第一个可用项
   * @returns 加载完成后的 Promise
   */
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

  /**
   * 构造一次 Metric 区间查询。
   * @param context - 目标 Server 与时间窗口
   * @param metric - 指标名称
   * @param queryGranularity - 查询粒度
   * @returns 可直接提交给 MetricService 的查询对象
   */
  function buildQuery(
    context: TrendContext,
    metric: string,
    queryGranularity: string,
  ): MetricQuery {
    return new MetricQuery({
      serverID: context.serverID,
      metric,
      start: context.start.toISOString(),
      end: context.end.toISOString(),
      granularity: queryGranularity,
      timeZone: context.timeZone,
      limit: 100_000,
      offset: 0,
    })
  }

  /**
   * 逐个加载六张常用指标小趋势图，不阻塞页面骨架。
   * @param sequence - 发起刷新时的序号，用于丢弃过期结果
   * @param context - 目标 Server 与时间窗口
   * @returns 全部趋势加载结束后的 Promise
   */
  async function loadTrendHistories(sequence: number, context: TrendContext): Promise<void> {
    trendsLoading.value = true
    // 小趋势图只有 240px 高，原始粒度在 30 天窗口下单图可达数十万点，统一压到分钟粒度。
    const trendGranularity = granularity.value === 'raw' ? 'minute' : granularity.value
    const collected: Record<string, MetricQueryResult> = {}
    try {
      for (const metric of trendMetrics) {
        if (sequence !== refreshSequence || selectedServerID.value !== context.serverID) return
        if (metric === selectedMetric.value && trendGranularity === granularity.value) {
          // 所选指标的曲线已在关键路径查过，直接复用，避免同一区间重复查询。
          if (history.value) collected[metric] = history.value
        } else {
          try {
            collected[metric] = await queryMetrics(buildQuery(context, metric, trendGranularity))
          } catch (reason) {
            trendsError.value = reason
          }
        }
        if (sequence !== refreshSequence || selectedServerID.value !== context.serverID) return
        trendHistories.value = { ...collected }
      }
    } finally {
      if (sequence === refreshSequence) trendsLoading.value = false
    }
  }

  /**
   * 刷新所选 Server 的存储容量、统一状态与所选指标历史，随后后台补齐小趋势图。
   * @returns 关键路径加载完成后的 Promise
   */
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
          /* 页面级错误已保留,此处不再向上抛出 */
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
      const context: TrendContext = {
        serverID: targetServerID,
        start: new Date(end.getTime() - rangeHours.value * 60 * 60 * 1000),
        end,
        timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC',
      }
      const [overviewResult, historyResult] = await Promise.allSettled([
        getMonitoringOverview(targetServerID),
        queryMetrics(buildQuery(context, selectedMetric.value, granularity.value)),
      ])
      if (sequence !== refreshSequence || selectedServerID.value !== targetServerID) return
      dataServerID.value = targetServerID
      if (overviewResult.status === 'fulfilled') {
        overview.value = overviewResult.value
        // MonitoringOverview 内部已经取过同一份最新值，不再单独调用 MetricService.Latest：
        latest.value = overviewResult.value.latest
      } else {
        overviewError.value = overviewResult.reason
        const fallback = await Promise.allSettled([getLatestMetrics(targetServerID)])
        if (sequence !== refreshSequence || selectedServerID.value !== targetServerID) return
        if (fallback[0].status === 'fulfilled') latest.value = fallback[0].value
        else latestError.value = fallback[0].reason
      }
      if (historyResult.status === 'fulfilled') history.value = historyResult.value
      else historyError.value = historyResult.reason
      // 关键路径到此结束，先放掉页面骨架的加载态，再补六张小趋势图：
      loading.value = false
      await loadTrendHistories(sequence, context)
    } finally {
      if (sequence === refreshSequence) loading.value = false
    }
  }

  /**
   * 把实时推送的样本按序列合并进最新值缓存。
   * @param event - 实时指标事件
   * @returns 无返回值
   */
  function mergeRealtime(event: MetricRealtimeEvent): void {
    if (event.serverID !== selectedServerID.value) return
    const merged = new Map(
      latest.value.map((sample) => [`${sample.sourceID}\0${sample.metric}`, sample]),
    )
    for (const sample of event.samples) merged.set(`${sample.sourceID}\0${sample.metric}`, sample)
    latest.value = [...merged.values()]
  }

  /**
   * 订阅经过节流的 Wails Metric 实时事件。
   * @returns 无返回值
   */
  function startSubscription(): void {
    unsubscribe?.()
    unsubscribe = subscribeMetricRealtime(mergeRealtime)
  }

  /**
   * 取消 Wails Metric 实时事件订阅。
   * @returns 无返回值
   */
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
    trendsLoading,
  }
})
