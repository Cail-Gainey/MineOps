<script setup lang="ts">
import { LineChart } from 'echarts/charts'
import type { LineSeriesOption } from 'echarts/charts'
import {
  DataZoomComponent,
  GridComponent,
  LegendComponent,
  TooltipComponent,
  type DataZoomComponentOption,
  type GridComponentOption,
  type LegendComponentOption,
  type TooltipComponentOption,
} from 'echarts/components'
import type { ComposeOption } from 'echarts/core'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import {
  NAlert,
  NButton,
  NCard,
  NEmpty,
  NFlex,
  NGrid,
  NGridItem,
  NProgress,
  NSelect,
  NSpin,
  NStatistic,
  NTag,
  NText,
} from 'naive-ui'
import { storeToRefs } from 'pinia'
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import VChart from 'vue-echarts'

import type {
  MetricQueryResult,
  MetricSample,
} from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import {
  clearMonitoringHistory,
  pauseMonitoring,
  resumeMonitoring,
} from '../../services/monitoring-api'
import { useInteractionStore } from '../../stores/interactions'
import { useLocaleStore } from '../../stores/locale'
import { useMonitoringStore } from '../../stores/monitoring'
import { useNotificationStore } from '../../stores/notifications'
import { useSettingsStore } from '../../stores/settings'
import { useThemeStore } from '../../stores/theme'
import { accentColours } from '../../themes/tokens'
import { chartInitOptions } from '../../shared/charts/render-options'
import AlertRulesPanel from './AlertRulesPanel.vue'
import { byteDisplayScale, metricDisplayScale, roundToTwo } from './metric-units'

use([
  CanvasRenderer,
  LineChart,
  GridComponent,
  TooltipComponent,
  DataZoomComponent,
  LegendComponent,
])

const props = withDefaults(
  defineProps<{
    embedded?: boolean
    lockedServerID?: string
  }>(),
  {
    embedded: false,
    lockedServerID: '',
  },
)

type ChartOption = ComposeOption<
  | LineSeriesOption
  | GridComponentOption
  | TooltipComponentOption
  | DataZoomComponentOption
  | LegendComponentOption
>

const monitoring = useMonitoringStore()
const interactions = useInteractionStore()
const locale = useLocaleStore()
const notifications = useNotificationStore()
const settings = useSettingsStore()
const theme = useThemeStore()
const route = useRoute()
const router = useRouter()
const {
  error,
  granularity,
  history,
  latestByMetric,
  loading,
  overview,
  partialMessage,
  rangeHours,
  selectedMetric,
  selectedServer,
  selectedServerID,
  servers,
  storage,
  trendHistories,
  trendsLoading,
} = storeToRefs(monitoring)
const { accent, isDark } = storeToRefs(theme)
const chartFiltersReady = ref(false)
const monitoringActionLoading = ref(false)
const applyingRouteQuery = ref(false)
const metricHistorySection = ref<HTMLElement | null>(null)
const overviewChartsSection = ref<HTMLElement | null>(null)
// 六张常用指标小趋势图位于首屏之下，滚动到可见范围前不初始化 ECharts 实例：
// 一次性建七个 Canvas 加七个 ResizeObserver 是打开监控页时 CPU 冲高的主要前端成本。
const overviewChartsVisible = ref(false)
let overviewChartsObserver: IntersectionObserver | null = null
const chartInit = computed(() =>
  chartInitOptions(settings.committed?.general.hardwareAcceleration !== false),
)

const serverOptions = computed(() =>
  servers.value.map((server) => ({
    label: `${server.name} · ${server.type} ${server.version}`,
    value: server.id,
  })),
)
const metricOptions = [
  { label: 'CPU 使用率', value: 'host.cpu' },
  { label: '内存使用率', value: 'host.memory' },
  { label: '1 分钟负载', value: 'host.load.1m' },
  { label: '磁盘已用空间', value: 'host.disk.used' },
  { label: '磁盘读取速率', value: 'host.disk.read_bytes_per_second' },
  { label: '磁盘写入速率', value: 'host.disk.write_bytes_per_second' },
  { label: '网络接收速率', value: 'host.network.receive_bytes_per_second' },
  { label: '网络发送速率', value: 'host.network.transmit_bytes_per_second' },
  { label: 'Java CPU 使用率', value: 'process.cpu' },
  { label: 'Java 内存', value: 'process.rss' },
  { label: 'Java 线程数', value: 'process.threads' },
  { label: 'Minecraft TPS', value: 'minecraft.tps' },
  { label: 'Minecraft MSPT', value: 'minecraft.mspt' },
]
const rangeOptions = [
  { label: '最近 1 小时', value: 1 },
  { label: '最近 24 小时', value: 24 },
  { label: '最近 7 天', value: 168 },
  { label: '最近 30 天', value: 720 },
]
const granularityOptions = [
  { label: '原始', value: 'raw' },
  { label: '分钟', value: 'minute' },
  { label: '小时', value: 'hour' },
]
const kpis = [
  { label: 'CPU 使用率', metric: 'host.cpu' },
  { label: '内存使用率', metric: 'host.memory' },
  { label: '磁盘已用空间', metric: 'host.disk.used' },
  { label: '网络接收速率', metric: 'host.network.receive_bytes_per_second' },
  { label: 'Java CPU 使用率', metric: 'process.cpu' },
  { label: 'Java 内存', metric: 'process.rss' },
]

/**
 * 把 Tooltip 传入的值格式化成两位小数并附加单位。
 * @param value - 单点数值，或 ECharts 传入的 [时间, 值] 数组
 * @param unit - 换算后的显示单位，空串表示无单位
 * @returns 可直接展示的数值文本，非有限数返回破折号
 */
function formatChartValue(value: unknown, unit = ''): string {
  const candidate = Array.isArray(value) ? value[value.length - 1] : value
  const numeric = Number(candidate)
  return Number.isFinite(numeric) ? `${numeric.toFixed(2)}${unit ? ` ${unit}` : ''}` : '—'
}

/**
 * 由查询结果构建折线图配置，并按最大量级统一换算显示单位。
 * @param result - 一次 Metric 查询的序列结果，为空时生成空图
 * @param compact - 是否为 240px 高的小趋势图（省略图例并收窄留白）
 * @returns ECharts 配置对象
 */
function createChartOption(result: MetricQueryResult | null, compact = false): ChartOption {
  const axisColour = isDark.value ? '#94a3b8' : '#475569'
  const splitColour = isDark.value ? '#334155' : '#e2e8f0'
  const accentColour = accentColours[accent.value] ?? '#059669'
  // 单趟遍历：一次 Date.parse 同时算出量级，再按换算系数就地缩放。
  // 原实现先整体扫一遍求量级、再整体映射一遍，点数上万时白跑一趟全量遍历。
  let magnitude = 0
  const rawSeries = (result?.series ?? []).map((series) => {
    const data: [number, number | null][] = []
    for (const point of series.points) {
      const timestamp = Date.parse(point.timestamp)
      if (point.missing) {
        data.push([timestamp, null])
        continue
      }
      const absolute = point.value < 0 ? -point.value : point.value
      if (absolute > magnitude) magnitude = absolute
      data.push([timestamp, point.value])
    }
    return { series, data }
  })
  const scale = metricDisplayScale(result?.series[0]?.definition.unit, magnitude)
  for (const { data } of rawSeries) {
    for (const entry of data) {
      if (entry[1] !== null) entry[1] = roundToTwo(entry[1] / scale.factor)
    }
  }
  return {
    animation: false,
    grid: compact
      ? { left: 52, right: 16, top: 16, bottom: 36 }
      : { left: 64, right: 24, top: 48, bottom: 72 },
    ...(compact ? {} : { legend: { top: 8, textStyle: { color: axisColour } } }),
    tooltip: { trigger: 'axis', valueFormatter: (value) => formatChartValue(value, scale.unit) },
    dataZoom: compact
      ? [{ type: 'inside', filterMode: 'none' }]
      : [
          { type: 'inside', filterMode: 'none' },
          { type: 'slider', filterMode: 'none', bottom: 16 },
        ],
    xAxis: {
      type: 'time',
      axisLabel: { color: axisColour },
      axisLine: { lineStyle: { color: axisColour } },
    },
    yAxis: {
      type: 'value',
      name: scale.unit,
      axisLabel: { color: axisColour, formatter: (value: number) => value.toFixed(2) },
      splitLine: { lineStyle: { color: splitColour } },
    },
    series: rawSeries.map(({ series, data }, index) => ({
      type: 'line',
      name:
        series.tags && Object.keys(series.tags).length > 0
          ? Object.entries(series.tags)
              .map(([key, value]) => `${key}=${value}`)
              .join(', ')
          : `来源 ${series.sourceID.slice(0, 8)}`,
      data,
      showSymbol: false,
      connectNulls: false,
      sampling: 'lttb',
      lineStyle: { color: index === 0 ? accentColour : axisColour, width: 1.5 },
      itemStyle: { color: index === 0 ? accentColour : axisColour },
    })),
  }
}

const chartOption = computed<ChartOption>(() => createChartOption(history.value))
const overviewCharts = computed(() =>
  kpis.map((kpi) => {
    const result = trendHistories.value[kpi.metric] ?? null
    // 未滚动到可见范围时不构建 option，避免为看不见的图付出遍历与序列化成本。
    return {
      ...kpi,
      result,
      option: overviewChartsVisible.value ? createChartOption(result, true) : null,
    }
  }),
)

const storagePercentage = computed(() => {
  if (!storage.value || storage.value.capacityBytes <= 0) return 0
  return Math.min(
    100,
    roundToTwo((storage.value.databaseBytes / storage.value.capacityBytes) * 100),
  )
})
const latestTimestamp = computed(() => {
  const timestamps = [...latestByMetric.value.values()].map((sample) =>
    Date.parse(sample.timestamp),
  )
  if (timestamps.length === 0) return '尚无采集数据'
  return locale.formatDateTime(Math.max(...timestamps))
})
const offlineWindowMillis = computed(
  () => (settings.committed?.monitoring.offlineAfterSeconds ?? 60) * 1000,
)
const collectorStatus = computed<{
  type: 'default' | 'success' | 'warning'
  label: string
  loading: boolean
}>(() => {
  const collector = overview.value?.collector
  if (collector?.paused) return { type: 'warning', label: '已暂停', loading: false }
  if (collector?.collecting) return { type: 'default', label: '采集中', loading: true }
  if (collector?.lastError) return { type: 'warning', label: '采集错误', loading: false }
  if (collector?.collectedAt) {
    const age = Date.now() - Date.parse(collector.collectedAt)
    if (age <= offlineWindowMillis.value) return { type: 'success', label: '正常', loading: false }
    return { type: 'warning', label: '数据陈旧', loading: false }
  }
  return { type: 'default', label: '尚未采集', loading: false }
})

/**
 * 把字节数换算成合适的存储单位。
 * @param value - 字节数，负数或非有限数视为不可用
 * @returns 带单位的容量文本
 */
function formatBytes(value: number): string {
  if (!Number.isFinite(value) || value < 0) return '不可用'
  const scale = byteDisplayScale(value)
  return `${(value / scale.factor).toFixed(2)} ${scale.unit}`
}

/**
 * 把一个 Metric 样本换算成带单位的展示文本。
 * @param sample - 最新样本，缺失时返回破折号
 * @returns 带单位的数值文本，TPS 触顶时前缀大于号
 */
function formatSample(sample: MetricSample | undefined): string {
  if (!sample) return '—'
  const scale = metricDisplayScale(sample.unit, sample.value)
  const prefix = sample.unit === 'ticks_per_second' && sample.tags?.capped === 'true' ? '>' : ''
  return `${prefix}${(sample.value / scale.factor).toFixed(2)}${scale.unit ? ` ${scale.unit}` : ''}`
}

/**
 * 锁定嵌入模式的目标 Server 后刷新监控数据，失败时推送通知。
 * @returns 刷新完成后的 Promise
 */
async function refresh(): Promise<void> {
  try {
    lockSelectedServer()
    await monitoring.refresh()
  } catch (reason) {
    notifications.push({
      kind: 'error',
      title: '刷新 Monitoring 失败',
      content: reason instanceof Error ? reason.message : String(reason),
      dedupeKey: 'monitoring:refresh:error',
    })
  }
}

/**
 * 嵌入模式下把选中 Server 固定为外部传入的目标。
 * @returns 无返回值
 */
function lockSelectedServer(): void {
  if (!props.embedded || !props.lockedServerID) return
  if (servers.value.some((server) => server.id === props.lockedServerID)) {
    selectedServerID.value = props.lockedServerID
  }
}

/**
 * 按路由参数或嵌入属性选定 Server 并加载其监控数据。
 * @returns 加载完成后的 Promise
 */
async function loadSelectedServer(): Promise<void> {
  const preferredServerID = props.embedded
    ? props.lockedServerID
    : String(route.query.serverID ?? '')
  await monitoring.loadServers(preferredServerID)
  lockSelectedServer()
  await monitoring.refresh()
}

/**
 * 平滑滚动到历史趋势区块。
 * @returns 无返回值
 */
function scrollToMetricHistory(): void {
  metricHistorySection.value?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

/**
 * 监听常用指标趋势区块进入视口，首次可见后一次性建图并停止观察。
 * 没有 IntersectionObserver 时直接建图，保证功能不依赖该能力。
 * @returns 无返回值
 */
function observeOverviewCharts(): void {
  if (typeof IntersectionObserver === 'undefined') {
    overviewChartsVisible.value = true
    return
  }
  overviewChartsObserver = new IntersectionObserver(
    (entries) => {
      if (!entries.some((entry) => entry.isIntersecting)) return
      overviewChartsVisible.value = true
      overviewChartsObserver?.disconnect()
      overviewChartsObserver = null
    },
    { rootMargin: '200px' },
  )
  watch(
    overviewChartsSection,
    (element) => {
      if (element && overviewChartsObserver) overviewChartsObserver.observe(element)
    },
    { immediate: true, flush: 'post' },
  )
}

/**
 * 切换 Server 时同步路由查询参数并重新加载数据。
 * @returns 切换完成后的 Promise
 */
async function changeServer(): Promise<void> {
  await router.replace({ query: { ...route.query, serverID: selectedServerID.value || undefined } })
  await refresh()
}

/**
 * 暂停或恢复所选 Server 的自动采集，并刷新页面状态。
 * @returns 操作完成后的 Promise
 */
async function toggleMonitoring(): Promise<void> {
  if (!selectedServerID.value || monitoringActionLoading.value) return
  const paused = overview.value?.collector?.paused === true
  monitoringActionLoading.value = true
  try {
    if (paused) await resumeMonitoring(selectedServerID.value)
    else await pauseMonitoring(selectedServerID.value)
    notifications.push({
      kind: 'success',
      title: paused ? '监控采集已恢复' : '监控采集已暂停',
      content: paused
        ? '后台自动采集将在下一个周期继续。'
        : '远端 Spark 与主机指标采集守护进程均已停止；恢复后会重新部署。',
      dedupeKey: `monitoring:${paused ? 'resume' : 'pause'}:success`,
    })
    await refresh()
  } catch (reason) {
    notifications.push({
      kind: 'error',
      title: paused ? '恢复监控失败' : '暂停监控失败',
      content: reason instanceof Error ? reason.message : String(reason),
      dedupeKey: `monitoring:${paused ? 'resume' : 'pause'}:error`,
    })
  } finally {
    monitoringActionLoading.value = false
  }
}

/**
 * 二次确认后清理所选 Server 的全部监控历史数据。
 * @returns 清理完成后的 Promise
 */
async function clearHistory(): Promise<void> {
  if (!selectedServer.value || monitoringActionLoading.value) return
  const confirmed = await interactions.confirm({
    title: '清理监控历史数据？',
    content:
      '将永久删除该服务器的原始、分钟和小时指标、Spark TPS/MSPT Snapshot、当前实时缓存和待同步的远端 Spark 缓冲。',
    objectLabel: selectedServer.value.name,
    impact: overview.value?.collector?.paused
      ? '监控当前已暂停，清理后不会立即产生新数据。'
      : '监控仍在运行，清理后下一个采集周期会重新产生数据。',
    positiveText: '确认清理',
    danger: true,
  })
  if (!confirmed) return
  monitoringActionLoading.value = true
  try {
    await clearMonitoringHistory(selectedServer.value.id)
    notifications.push({
      kind: 'success',
      title: '监控历史数据已清理',
      content: selectedServer.value.name,
      dedupeKey: `monitoring:clear:${selectedServer.value.id}`,
    })
    await refresh()
  } catch (reason) {
    notifications.push({
      kind: 'error',
      title: '清理监控历史数据失败',
      content: reason instanceof Error ? reason.message : String(reason),
      dedupeKey: 'monitoring:clear:error',
    })
  } finally {
    monitoringActionLoading.value = false
  }
}

onMounted(async () => {
  monitoring.startSubscription()
  observeOverviewCharts()
  try {
    if (!props.embedded) {
      const requestedMetric = String(route.query.metric ?? '')
      if (metricOptions.some((option) => option.value === requestedMetric)) {
        selectedMetric.value = requestedMetric
      }
      const requestedRange = Number(route.query.rangeHours)
      if (rangeOptions.some((option) => option.value === requestedRange)) {
        rangeHours.value = requestedRange
      }
      const requestedGranularity = String(route.query.granularity ?? '')
      if (granularityOptions.some((option) => option.value === requestedGranularity)) {
        granularity.value = requestedGranularity
      }
    }
    await loadSelectedServer()
    if (!props.embedded && route.query.focus) {
      await nextTick()
      scrollToMetricHistory()
    }
  } catch (reason) {
    notifications.push({
      kind: 'error',
      title: '加载 Monitoring 失败',
      content: reason instanceof Error ? reason.message : String(reason),
      dedupeKey: 'monitoring:load:error',
    })
  } finally {
    chartFiltersReady.value = true
  }
})
watch(
  () => [props.embedded, props.lockedServerID] as const,
  async ([embedded, serverID], [previousEmbedded, previousServerID]) => {
    if (!embedded || !serverID || (embedded === previousEmbedded && serverID === previousServerID))
      return
    try {
      await loadSelectedServer()
    } catch (reason) {
      notifications.push({
        kind: 'error',
        title: '切换 Monitoring Server 失败',
        content: reason instanceof Error ? reason.message : String(reason),
        dedupeKey: 'monitoring:load:error',
      })
    }
  },
)
watch(
  () =>
    [
      route.query.serverID,
      route.query.metric,
      route.query.rangeHours,
      route.query.granularity,
      route.query.focus,
    ] as const,
  async ([serverIDValue, metricValue, rangeValue, granularityValue, focusValue]) => {
    if (props.embedded || !chartFiltersReady.value) return
    applyingRouteQuery.value = true
    try {
      let changed = false
      const requestedServerID = String(serverIDValue ?? '')
      if (
        requestedServerID &&
        requestedServerID !== selectedServerID.value &&
        servers.value.some((server) => server.id === requestedServerID)
      ) {
        selectedServerID.value = requestedServerID
        changed = true
      }
      const requestedMetric = String(metricValue ?? '')
      if (
        requestedMetric !== selectedMetric.value &&
        metricOptions.some((option) => option.value === requestedMetric)
      ) {
        selectedMetric.value = requestedMetric
        changed = true
      }
      const requestedRange = Number(rangeValue)
      if (
        requestedRange !== rangeHours.value &&
        rangeOptions.some((option) => option.value === requestedRange)
      ) {
        rangeHours.value = requestedRange
        changed = true
      }
      const requestedGranularity = String(granularityValue ?? '')
      if (
        requestedGranularity !== granularity.value &&
        granularityOptions.some((option) => option.value === requestedGranularity)
      ) {
        granularity.value = requestedGranularity
        changed = true
      }
      if (changed) await refresh()
      if (focusValue) {
        await nextTick()
        scrollToMetricHistory()
      }
    } finally {
      applyingRouteQuery.value = false
    }
  },
)
watch([selectedMetric, rangeHours, granularity], async () => {
  if (!chartFiltersReady.value || applyingRouteQuery.value) return
  if (!props.embedded) {
    await router.replace({
      query: {
        ...route.query,
        metric: selectedMetric.value,
        rangeHours: rangeHours.value,
        granularity: granularity.value,
      },
    })
  }
  await refresh()
})
onUnmounted(() => {
  monitoring.stopSubscription()
  overviewChartsObserver?.disconnect()
  overviewChartsObserver = null
})
</script>

<template>
  <NSpin :show="loading">
    <NFlex vertical :size="16">
      <section class="monitoring-header">
        <NFlex align="center" justify="end" wrap>
          <NFlex wrap>
            <NSelect
              v-if="!embedded"
              v-model:value="selectedServerID"
              :options="serverOptions"
              placeholder="选择 Server"
              style="width: 260px"
              @update:value="changeServer"
            />
            <NButton :loading="loading" @click="refresh">刷新</NButton>
            <NButton
              :type="overview?.collector?.paused ? 'primary' : 'warning'"
              :loading="monitoringActionLoading"
              :disabled="!selectedServerID"
              @click="toggleMonitoring"
            >
              {{ overview?.collector?.paused ? '恢复监控' : '暂停监控' }}
            </NButton>
            <NButton
              type="error"
              secondary
              :loading="monitoringActionLoading"
              :disabled="!selectedServerID"
              @click="clearHistory"
            >
              清理历史数据
            </NButton>
          </NFlex>
        </NFlex>
      </section>

      <NAlert v-if="error" type="error" title="Metric 查询失败">
        {{ error instanceof Error ? error.message : String(error) }}
      </NAlert>
      <NAlert v-if="partialMessage" type="warning" title="部分 Monitoring 数据不可用">
        {{ partialMessage }}
      </NAlert>
      <NEmpty v-if="servers.length === 0" description="请先创建 Minecraft Server" />

      <template v-if="selectedServer">
        <NGrid cols="1 640:3 1000:6" :x-gap="12" :y-gap="12">
          <NGridItem v-for="kpi in kpis" :key="kpi.metric">
            <NCard size="small">
              <NStatistic
                :label="kpi.label"
                :value="formatSample(latestByMetric.get(kpi.metric))"
              />
            </NCard>
          </NGridItem>
        </NGrid>

        <section
          id="monitoring-metric-history"
          ref="metricHistorySection"
          class="metric-history-section"
        >
          <NCard>
            <template #header>
              <NFlex align="center" justify="space-between" wrap>
                <div>
                  <NText tag="h3">历史趋势 · {{ selectedServer.name }}</NText>
                  <div>
                    <NText depth="3">最近采集：{{ latestTimestamp }}</NText>
                  </div>
                </div>
                <NFlex wrap>
                  <NSelect
                    v-model:value="selectedMetric"
                    :options="metricOptions"
                    style="width: 220px"
                  />
                  <NSelect
                    v-model:value="rangeHours"
                    :options="rangeOptions"
                    style="width: 150px"
                  />
                  <NSelect
                    v-model:value="granularity"
                    :options="granularityOptions"
                    style="width: 110px"
                  />
                </NFlex>
              </NFlex>
            </template>
            <VChart
              v-if="history?.series.length"
              class="metric-chart"
              :option="chartOption"
              :init-options="chartInit"
              autoresize
            />
            <NEmpty v-else description="当前时间范围没有该指标数据" />
          </NCard>
        </section>

        <section ref="overviewChartsSection">
          <NCard title="常用指标趋势">
            <NGrid cols="1 760:2" :x-gap="12" :y-gap="12">
              <NGridItem v-for="chart in overviewCharts" :key="chart.metric">
                <NCard :title="chart.label" size="small">
                  <VChart
                    v-if="chart.option && chart.result?.series.length"
                    class="metric-overview-chart"
                    :option="chart.option"
                    :init-options="chartInit"
                    autoresize
                  />
                  <NSpin
                    v-else-if="overviewChartsVisible && trendsLoading"
                    class="metric-overview-chart"
                    size="small"
                  />
                  <div v-else-if="!overviewChartsVisible" class="metric-overview-chart" />
                  <NEmpty v-else size="small" description="暂无趋势数据" />
                </NCard>
              </NGridItem>
            </NGrid>
          </NCard>
        </section>

        <NCard title="指标存储">
          <NFlex vertical>
            <NProgress
              type="line"
              :percentage="storagePercentage"
              :status="storage?.pressure ? 'warning' : 'success'"
            />
            <NText depth="3">
              数据库已用 {{ formatBytes(storage?.databaseBytes ?? 0) }} /
              {{ formatBytes(storage?.capacityBytes ?? 0) }} · 已分配
              {{ formatBytes(storage?.allocatedDatabaseBytes ?? 0) }} · 磁盘可用
              {{ formatBytes(storage?.availableDiskBytes ?? -1) }}
            </NText>
          </NFlex>
        </NCard>

        <NCard title="监控状态与自动采集">
          <NFlex vertical :size="12">
            <NFlex wrap>
              <NTag :type="collectorStatus.type"> 采集 {{ collectorStatus.label }} </NTag>
              <NTag
                :type="
                  overview?.spark?.status === 'available'
                    ? 'success'
                    : overview?.spark?.status === 'unsupported'
                      ? 'error'
                      : 'warning'
                "
              >
                Spark {{ overview?.spark?.status ?? 'unavailable' }}
              </NTag>
              <NTag :type="overview?.activeAlerts.length ? 'error' : 'success'">
                活跃告警 {{ overview?.activeAlerts.length ?? 0 }}
              </NTag>
            </NFlex>
            <NAlert
              v-for="issue in overview?.issues ?? []"
              :key="`${issue.code}:${issue.timestamp}`"
              :type="issue.severity === 'error' ? 'error' : 'warning'"
              :title="issue.code"
            >
              {{ issue.message }} · {{ locale.formatDateTime(issue.timestamp) }}
            </NAlert>
            <NText v-if="!overview?.issues.length" depth="3"
              >当前没有离线、采集、Spark 或阈值异常。</NText
            >
            <NText depth="3">
              最近采集：{{
                overview?.collector?.collectedAt
                  ? locale.formatDateTime(overview.collector.collectedAt)
                  : '尚未采集'
              }}
            </NText>
            <NText v-if="overview?.collector?.lastError" type="error" depth="3">
              最近错误：{{ overview.collector.lastError
              }}{{
                overview.collector.lastErrorAt
                  ? ` · ${locale.formatDateTime(overview.collector.lastErrorAt)}`
                  : ''
              }}
            </NText>
          </NFlex>
        </NCard>

        <AlertRulesPanel :server-i-d="selectedServerID" />
      </template>
    </NFlex>
  </NSpin>
</template>

<style scoped>
.metric-chart {
  width: 100%;
  height: 420px;
}

.metric-history-section {
  scroll-margin-top: 16px;
}

.metric-overview-chart {
  width: 100%;
  height: 240px;
}
</style>
