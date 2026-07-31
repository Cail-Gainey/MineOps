<script setup lang="ts">
import { BarChart, LineChart } from 'echarts/charts'
import type { BarSeriesOption, LineSeriesOption } from 'echarts/charts'
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
import { Browser } from '@wailsio/runtime'
import {
  NAlert,
  NButton,
  NCard,
  NDescriptions,
  NDescriptionsItem,
  NEmpty,
  NFlex,
  NGrid,
  NGridItem,
  NInputNumber,
  NSelect,
  NSpin,
  NStatistic,
  NTable,
  NTag,
  NText,
} from 'naive-ui'
import { storeToRefs } from 'pinia'
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import VChart from 'vue-echarts'

import type { SparkReport } from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import { ApplicationError } from '../../services/api-client'
import { deleteSparkReport } from '../../services/performance-api'
import {
  clearMonitoringHistory,
  pauseMonitoring,
  resumeMonitoring,
} from '../../services/monitoring-api'
import { hasMessage } from '../../locales/runtime'
import { useInteractionStore } from '../../stores/interactions'
import { useLocaleStore } from '../../stores/locale'
import { useNotificationStore } from '../../stores/notifications'
import { usePerformanceStore } from '../../stores/performance'
import { useSettingsStore } from '../../stores/settings'
import { useThemeStore } from '../../stores/theme'
import { accentColours } from '../../themes/tokens'
import { chartInitOptions } from '../../shared/charts/render-options'

use([
  CanvasRenderer,
  LineChart,
  BarChart,
  GridComponent,
  TooltipComponent,
  DataZoomComponent,
  LegendComponent,
])

type ChartOption = ComposeOption<
  | LineSeriesOption
  | BarSeriesOption
  | GridComponentOption
  | TooltipComponentOption
  | DataZoomComponentOption
  | LegendComponentOption
>

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

const performance = usePerformanceStore()
const interactions = useInteractionStore()
const locale = useLocaleStore()
const notifications = useNotificationStore()
const settings = useSettingsStore()
const theme = useThemeStore()
const route = useRoute()
const router = useRouter()
const chartInit = computed(() =>
  chartInitOptions(settings.committed?.general.hardwareAcceleration !== false),
)
const {
  capability,
  collectorPaused,
  error,
  installPlan,
  latestSnapshot,
  loading,
  partialMessage,
  profilerDurationSeconds,
  reportPrivacyConfirmation,
  reports,
  selectedServer,
  selectedServerID,
  servers,
  snapshots,
} = storeToRefs(performance)
const { accent, isDark } = storeToRefs(theme)
let refreshTimer: ReturnType<typeof setInterval> | undefined
const collectionActionLoading = ref(false)
const profilerDurationMinutes = computed({
  get: () => profilerDurationSeconds.value / 60,
  set: (value: number) => {
    profilerDurationSeconds.value = Math.round(value * 60)
  },
})

const serverOptions = computed(() =>
  servers.value.map((server) => ({
    label: `${server.name} · ${server.type} ${server.version}`,
    value: server.id,
  })),
)

const latestSnapshotSummary = computed(() => {
  const latest = latestSnapshot.value
  if (!latest) return ''
  const previous = snapshots.value[1]
  if (!previous) {
    return locale.t('performance.snapshotSummarySingle', {
      time: locale.formatDateTime(latest.collectedAt),
    })
  }
  const changed =
    latest.tps5Seconds !== previous.tps5Seconds ||
    latest.tps5SecondsCapped !== previous.tps5SecondsCapped ||
    latest.tps1Minute !== previous.tps1Minute ||
    latest.tps1MinuteCapped !== previous.tps1MinuteCapped ||
    latest.tps15Minutes !== previous.tps15Minutes ||
    latest.tps15MinutesCapped !== previous.tps15MinutesCapped ||
    latest.msptAvailable !== previous.msptAvailable ||
    latest.msptMedian !== previous.msptMedian ||
    latest.msptP95 !== previous.msptP95
  return locale.t('performance.snapshotSummary', {
    time: locale.formatDateTime(latest.collectedAt),
    count: snapshots.value.length,
    change: changed
      ? locale.t('performance.snapshotChanged')
      : locale.t('performance.snapshotUnchanged'),
  })
})

const latestTPSCapped = computed(
  () => latestSnapshot.value?.tps5SecondsCapped || latestSnapshot.value?.tps1MinuteCapped,
)
const sparkCollectionStatus = computed(() => {
  if (collectorPaused.value)
    return { type: 'warning' as const, label: locale.t('performance.collectionPaused') }
  if (selectedServer.value?.state === 'running')
    return { type: 'success' as const, label: locale.t('performance.collectionEnabled') }
  return { type: 'default' as const, label: locale.t('performance.collectionStopped') }
})

/**
 * 把数值四舍五入到两位小数。
 * @param value - 原始数值
 * @returns 保留两位小数的数值
 */
function roundToTwo(value: number): number {
  return Number(value.toFixed(2))
}

/**
 * 把可选数值四舍五入到两位小数，缺失时返回 null 以便图表断线。
 * @param value - 可选原始数值
 * @returns 保留两位小数的数值，输入缺失时为 null
 */
function roundOptionalToTwo(value: number | undefined): number | null {
  return value === undefined ? null : roundToTwo(value)
}

/**
 * 把 Tooltip 传入的值格式化成两位小数。
 * @param value - 单点数值，或 ECharts 传入的 [时间, 值] 数组
 * @returns 数值文本，非有限数返回破折号
 */
function formatChartValue(value: unknown): string {
  const candidate = Array.isArray(value) ? value[value.length - 1] : value
  const numeric = Number(candidate)
  return Number.isFinite(numeric) ? numeric.toFixed(2) : '—'
}

const trendOption = computed<ChartOption>(() => {
  const axisColour = isDark.value ? '#94a3b8' : '#475569'
  const splitColour = isDark.value ? '#334155' : '#e2e8f0'
  const ordered = [...snapshots.value].reverse()
  return {
    animation: false,
    grid: { left: 64, right: 64, top: 48, bottom: 72 },
    legend: { top: 8, textStyle: { color: axisColour } },
    tooltip: { trigger: 'axis', valueFormatter: formatChartValue },
    dataZoom: [
      { type: 'inside', filterMode: 'none' },
      { type: 'slider', filterMode: 'none', bottom: 16 },
    ],
    xAxis: { type: 'time', axisLabel: { color: axisColour } },
    yAxis: [
      {
        type: 'value',
        name: 'TPS',
        min: 0,
        max: 20,
        axisLabel: { color: axisColour, formatter: (value: number) => value.toFixed(2) },
        splitLine: { lineStyle: { color: splitColour } },
      },
      {
        type: 'value',
        name: 'MSPT',
        min: 0,
        axisLabel: { color: axisColour, formatter: (value: number) => value.toFixed(2) },
        splitLine: { show: false },
      },
    ],
    series: [
      {
        type: 'line',
        name: 'TPS 5s',
        showSymbol: false,
        data: ordered.map((snapshot) => [
          Date.parse(snapshot.collectedAt),
          roundToTwo(snapshot.tps5Seconds),
        ]),
        lineStyle: { color: accentColours[accent.value], width: 2 },
      },
      {
        type: 'line',
        name: 'TPS 1m',
        showSymbol: false,
        data: ordered.map((snapshot) => [
          Date.parse(snapshot.collectedAt),
          roundToTwo(snapshot.tps1Minute),
        ]),
      },
      {
        type: 'line',
        name: 'TPS 15m',
        showSymbol: false,
        data: ordered.map((snapshot) => [
          Date.parse(snapshot.collectedAt),
          roundToTwo(snapshot.tps15Minutes),
        ]),
      },
      {
        type: 'line',
        name: 'MSPT P95',
        yAxisIndex: 1,
        showSymbol: false,
        connectNulls: false,
        data: ordered.map((snapshot) => [
          Date.parse(snapshot.collectedAt),
          snapshot.msptAvailable ? roundOptionalToTwo(snapshot.msptP95) : null,
        ]),
      },
    ],
  }
})

const distributionOption = computed<ChartOption>(() => ({
  animation: false,
  grid: { left: 56, right: 24, top: 24, bottom: 40 },
  tooltip: { trigger: 'axis', valueFormatter: formatChartValue },
  xAxis: { type: 'category', data: ['Min', 'Median', 'P95', 'Max'] },
  yAxis: {
    type: 'value',
    name: 'ms',
    axisLabel: { formatter: (value: number) => value.toFixed(2) },
  },
  series: [
    {
      type: 'bar',
      name: 'Tick Duration',
      data: latestSnapshot.value?.msptAvailable
        ? [
            roundOptionalToTwo(latestSnapshot.value.msptMinimum),
            roundOptionalToTwo(latestSnapshot.value.msptMedian),
            roundOptionalToTwo(latestSnapshot.value.msptP95),
            roundOptionalToTwo(latestSnapshot.value.msptMaximum),
          ]
        : [],
      itemStyle: { color: accentColours[accent.value] },
    },
  ],
}))

/**
 * 把 Spark 能力状态映射成标签配色。
 * @param status - Spark 能力状态字符串
 * @returns naive-ui 标签的语义类型
 */
function statusType(status: string | undefined): 'default' | 'success' | 'warning' | 'error' {
  if (status === 'available') return 'success'
  if (status === 'unknown' || status === 'collecting') return 'warning'
  if (status === 'unavailable' || status === 'unsupported' || status === 'failed') return 'error'
  return 'default'
}

/**
 * 把指标数值格式化成带单位的文本。
 * @param value - 指标数值，缺失时返回破折号
 * @param suffix - 单位后缀
 * @param digits - 保留小数位数
 * @returns 带单位的数值文本
 */
function formatMetric(value: number | undefined, suffix: string, digits = 2): string {
  return value === undefined ? '—' : `${value.toFixed(digits)} ${suffix}`
}

/**
 * 格式化 TPS，触顶时加前缀大于号表示实际值可能更高。
 * @param value - TPS 数值，缺失时返回破折号
 * @param capped - 该值是否被服务端截顶
 * @returns TPS 展示文本
 */
function formatTPS(value: number | undefined, capped: boolean | undefined): string {
  if (value === undefined) return '—'
  return `${capped ? '>' : ''}${value.toFixed(2)} TPS`
}

/**
 * 把校验算法名规范成带连字符的展示形式。
 * @param algorithm - 校验算法名，如 sha512
 * @returns 规范化后的算法标签
 */
function checksumLabel(algorithm: string): string {
  return algorithm.toUpperCase().replace('SHA', 'SHA-')
}

/**
 * 把构件字节数换算成 MiB 展示文本。
 * @param bytes - 构件字节数，非正数表示未声明
 * @returns 带单位的体积文本
 */
function formatArtifactSize(bytes: number): string {
  if (bytes <= 0) return locale.t('performance.sizeUndeclared')
  return `${(bytes / 1024 / 1024).toFixed(2)} MiB`
}

/**
 * 切换目标 Server 并重新加载性能数据，锁定模式下忽略。
 * @returns 切换完成后的 Promise
 */
async function changeServer(): Promise<void> {
  if (props.lockedServerID) return
  await router.replace({ query: { ...route.query, serverID: selectedServerID.value || undefined } })
  installPlan.value = null
  await refreshPerformance()
}

/**
 * 重新加载所选 Server 的 Spark 能力、快照与报告。
 * @returns 刷新完成后的 Promise
 */
async function refreshPerformance(): Promise<void> {
  try {
    await performance.refresh()
  } catch (reason) {
    notifications.push({
      kind: 'error',
      title: locale.t('performance.refreshFailed'),
      content: reason instanceof Error ? reason.message : String(reason),
      dedupeKey: 'performance:refresh:error',
    })
  }
}

/**
 * 重新探测远端 Spark 能力。
 * @returns 探测完成后的 Promise
 */
async function probeSparkCapability(): Promise<void> {
  await runAction(() => performance.probe(), locale.t('performance.probeSucceeded'))
}

/**
 * 立即采集一次 TPS/MSPT 快照。
 * @returns 采集完成后的 Promise
 */
async function collectSnapshot(): Promise<void> {
  await runAction(() => performance.collect(), locale.t('performance.collected'))
}

/**
 * 暂停或恢复该 Server 的 Spark 采集。
 * @returns 操作完成后的 Promise
 */
async function toggleSparkCollection(): Promise<void> {
  if (!selectedServerID.value || collectionActionLoading.value) return
  const paused = collectorPaused.value
  collectionActionLoading.value = true
  try {
    if (paused) await resumeMonitoring(selectedServerID.value)
    else await pauseMonitoring(selectedServerID.value)
    notifications.push({
      kind: 'success',
      title: paused
        ? locale.t('performance.collectionResumed')
        : locale.t('performance.collectionPausedNotice'),
      content: paused
        ? locale.t('performance.collectionResumedContent')
        : locale.t('performance.collectionPausedContent'),
      dedupeKey: `performance:collection:${paused ? 'resume' : 'pause'}`,
    })
    await performance.refresh()
  } catch (reason) {
    notifications.push({
      kind: 'error',
      title: paused ? locale.t('performance.resumeFailed') : locale.t('performance.pauseFailed'),
      content: reason instanceof Error ? reason.message : String(reason),
      dedupeKey: `performance:collection:${paused ? 'resume' : 'pause'}:error`,
    })
  } finally {
    collectionActionLoading.value = false
  }
}

/**
 * 二次确认后清理该 Server 的 Spark 历史数据。
 * @returns 清理完成后的 Promise
 */
async function clearSparkHistory(): Promise<void> {
  if (!selectedServer.value || collectionActionLoading.value) return
  const confirmed = await interactions.confirm({
    title: locale.t('performance.clearTitle'),
    content: locale.t('performance.clearContent'),
    objectLabel: selectedServer.value.name,
    impact: collectorPaused.value
      ? locale.t('performance.clearImpactPaused')
      : locale.t('performance.clearImpactRunning'),
    positiveText: locale.t('performance.clearConfirm'),
    danger: true,
  })
  if (!confirmed) return
  collectionActionLoading.value = true
  try {
    await clearMonitoringHistory(selectedServer.value.id)
    notifications.push({
      kind: 'success',
      title: locale.t('performance.cleared'),
      content: selectedServer.value.name,
      dedupeKey: `performance:clear:${selectedServer.value.id}`,
    })
    await performance.refresh()
  } catch (reason) {
    notifications.push({
      kind: 'error',
      title: locale.t('performance.clearFailed'),
      content: reason instanceof Error ? reason.message : String(reason),
      dedupeKey: 'performance:clear:error',
    })
  } finally {
    collectionActionLoading.value = false
  }
}

/**
 * 执行一次性能页动作并统一处理加载态、成功通知与错误通知。
 * @param action - 待执行的异步动作
 * @param title - 成功时的通知标题
 * @returns 动作完成后的 Promise
 */
async function runAction(action: () => Promise<unknown>, title: string): Promise<void> {
  try {
    await action()
    notifications.push({ kind: 'success', title, dedupeKey: `performance:${title}` })
  } catch (reason) {
    const diagnostics: string[] = []
    if (reason instanceof ApplicationError) {
      if (typeof reason.details.stderr === 'string' && reason.details.stderr.trim()) {
        diagnostics.push(reason.details.stderr.trim())
      }
      if (Array.isArray(reason.details.sources)) {
        diagnostics.push(
          locale.t('performance.diagnosticSources', {
            items: reason.details.sources.join(', '),
          }),
        )
      }
      if (
        typeof reason.details.expectedChecksumAlgorithm === 'string' &&
        typeof reason.details.expectedChecksum === 'string'
      ) {
        diagnostics.push(
          locale.t('performance.diagnosticExpectedChecksum', {
            algorithm: checksumLabel(reason.details.expectedChecksumAlgorithm),
            checksum: reason.details.expectedChecksum,
          }),
        )
      } else if (typeof reason.details.expectedSHA256 === 'string') {
        diagnostics.push(
          locale.t('performance.diagnosticExpectedSHA256', {
            checksum: reason.details.expectedSHA256,
          }),
        )
      }
      if (Array.isArray(reason.details.downloadErrors) && reason.details.downloadErrors.length) {
        diagnostics.push(
          locale.t('performance.diagnosticDownloadErrors', {
            items: reason.details.downloadErrors.join('; '),
          }),
        )
      }
      if (Array.isArray(reason.details.rollbackErrors) && reason.details.rollbackErrors.length) {
        diagnostics.push(
          locale.t('performance.diagnosticRollbackErrors', {
            items: reason.details.rollbackErrors.join('; '),
          }),
        )
      }
      if (typeof reason.details.rawResponse === 'string' && reason.details.rawResponse.trim()) {
        diagnostics.push(
          locale.t('performance.diagnosticRawResponse', {
            response: reason.details.rawResponse.trim(),
          }),
        )
      }
    }
    notifications.push({
      kind: 'error',
      title: locale.t('performance.actionFailed', { title }),
      content: `${reason instanceof Error ? reason.message : String(reason)}${diagnostics.length ? `\n${diagnostics.join('\n')}` : ''}`,
      dedupeKey: `performance:${title}:error`,
    })
  }
}

/**
 * 生成 Spark 安装计划供用户确认。
 * @returns 生成完成后的 Promise
 */
async function prepareInstall(): Promise<void> {
  await runAction(() => performance.planInstall(), locale.t('performance.planGenerated'))
}

/**
 * 二次确认后按计划安装 Spark。
 * @returns 安装完成后的 Promise
 */
async function confirmInstall(): Promise<void> {
  if (!installPlan.value) return
  const dependencyImpact = installPlan.value.dependencies
    .map(
      (dependency) =>
        `${dependency.name} ${dependency.version}: ${dependency.targetPath}\n${checksumLabel(dependency.checksumAlgorithm)}: ${dependency.checksum}`,
    )
    .join('\n')
  const confirmed = await interactions.confirm({
    title: capability.value?.installed
      ? locale.t('performance.upgradeTitle')
      : locale.t('performance.installTitle'),
    content: installPlan.value.impact,
    objectLabel: `${installPlan.value.serverName} · ${installPlan.value.targetVersion}`,
    impact: `${installPlan.value.targetPath}\n${checksumLabel(installPlan.value.checksumAlgorithm)}: ${installPlan.value.checksum}${dependencyImpact ? `\n${dependencyImpact}` : ''}`,
    positiveText: capability.value?.installed
      ? locale.t('performance.upgradeConfirm')
      : locale.t('performance.installConfirm'),
    danger: installPlan.value.restartRequired,
  })
  if (confirmed) await runAction(() => performance.install(), locale.t('performance.installed'))
}

/**
 * 二次确认后回滚到安装前的备份。
 * @returns 回滚完成后的 Promise
 */
async function confirmRollback(): Promise<void> {
  if (!capability.value?.backupPath || !selectedServer.value) return
  const confirmed = await interactions.confirm({
    title: locale.t('performance.rollbackTitle'),
    content: locale.t('performance.rollbackContent'),
    objectLabel: selectedServer.value.name,
    impact: capability.value.backupPath,
    positiveText: locale.t('performance.rollbackConfirm'),
    danger: true,
  })
  if (confirmed) await runAction(() => performance.rollback(), locale.t('performance.rolledBack'))
}

/**
 * 确认隐私风险后生成 Spark 健康报告。
 * @returns 生成完成后的 Promise
 */
async function createHealthReport(): Promise<void> {
  if (!reportPrivacyConfirmation.value) {
    await runAction(() => performance.healthReport(), locale.t('performance.healthStarted'))
    return
  }
  const confirmed = await interactions.confirm({
    title: locale.t('performance.healthTitle'),
    content: locale.t('performance.healthContent'),
    objectLabel: selectedServer.value?.name ?? 'Minecraft Server',
    positiveText: locale.t('performance.healthConfirm'),
  })
  if (confirmed)
    await runAction(() => performance.healthReport(), locale.t('performance.healthStarted'))
}

/**
 * 确认隐私风险后发起一次 Spark 性能分析。
 * @returns 发起完成后的 Promise
 */
async function createProfiler(): Promise<void> {
  if (!reportPrivacyConfirmation.value) {
    await runAction(() => performance.profiler(), locale.t('performance.profilerStarted'))
    return
  }
  const confirmed = await interactions.confirm({
    title: locale.t('performance.profilerTitle'),
    content: locale.t('performance.profilerContent', {
      minutes: profilerDurationMinutes.value,
    }),
    objectLabel: selectedServer.value?.name ?? 'Minecraft Server',
    impact: locale.t('performance.profilerImpact'),
    positiveText: locale.t('performance.profilerConfirm'),
  })
  if (confirmed)
    await runAction(() => performance.profiler(), locale.t('performance.profilerStarted'))
}

/**
 * 确认隐私风险后打开报告的外部链接。
 * @param report - 目标 Spark 报告
 * @returns 打开完成后的 Promise
 */
async function openReport(report: SparkReport): Promise<void> {
  if (!report.reportURL) return
  if (!reportPrivacyConfirmation.value) {
    await openExternalReport(report.reportURL)
    return
  }
  const confirmed = await interactions.confirm({
    title: locale.t('performance.openReportTitle'),
    content: locale.t('performance.openReportContent'),
    objectLabel: report.kind,
    impact: report.reportURL,
    positiveText: locale.t('performance.openReportConfirm'),
  })
  if (confirmed) await openExternalReport(report.reportURL)
}

/**
 * 用系统浏览器打开报告链接。
 * @param reportURL - 报告的外部地址
 * @returns 打开完成后的 Promise
 */
async function openExternalReport(reportURL: string): Promise<void> {
  try {
    await Browser.OpenURL(reportURL)
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: locale.t('performance.openReportFailed'),
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: `spark-report:open:${reportURL}`,
    })
  }
}

/**
 * 把报告类型映射成本地化标签。
 * @param kind - 报告类型
 * @returns 本地化标签，未知类型原样返回
 */
function reportKindLabel(kind: string): string {
  const key = `performance.reportKind.${kind}`
  return hasMessage(key) ? locale.t(key) : kind
}

/**
 * 把报告状态映射成本地化标签。
 * @param state - 报告状态
 * @returns 本地化标签，未知状态原样返回
 */
function reportStateLabel(state: string): string {
  const key = `performance.reportState.${state}`
  return hasMessage(key) ? locale.t(key) : state
}

/**
 * 二次确认后删除一份 Spark 报告。
 * @param report - 待删除的报告
 * @returns 删除完成后的 Promise
 */
async function deleteReport(report: SparkReport): Promise<void> {
  const confirmed = await interactions.confirm({
    title: locale.t('performance.deleteReportTitle'),
    content: locale.t('performance.deleteReportContent'),
    objectLabel: `${reportKindLabel(report.kind)} · ${locale.formatDateTime(report.createdAt)}`,
    positiveText: locale.t('performance.deleteReportConfirm'),
    danger: true,
  })
  if (!confirmed) return
  await runAction(async () => {
    await deleteSparkReport(report.id)
    await performance.refresh()
  }, locale.t('performance.reportDeleted'))
}

/**
 * 加载 Server 列表并选定目标后拉取性能数据。
 * @param preferredServerID - 优先选中的 Server ID
 * @returns 加载完成后的 Promise
 */
async function loadPerformance(preferredServerID: string): Promise<void> {
  try {
    await performance.loadServers(preferredServerID)
    await performance.refresh()
  } catch (reason) {
    notifications.push({
      kind: 'error',
      title: locale.t('performance.loadFailed'),
      content: reason instanceof Error ? reason.message : String(reason),
      dedupeKey: 'performance:load:error',
    })
  }
}

onMounted(async () => {
  await loadPerformance(props.lockedServerID || String(route.query.serverID ?? ''))
  refreshTimer = setInterval(() => {
    if (document.visibilityState !== 'visible' || loading.value || !selectedServerID.value) return
    void performance.refresh().catch(() => undefined)
  }, 10_000)
})

onUnmounted(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})

watch(
  () => props.lockedServerID,
  async (serverID, previousServerID) => {
    if (!serverID || serverID === previousServerID) return
    installPlan.value = null
    await loadPerformance(serverID)
  },
)
</script>

<template>
  <NSpin :show="loading">
    <NFlex vertical :size="16">
      <NCard :bordered="!embedded" :size="embedded ? 'small' : 'medium'">
        <NFlex align="center" justify="end" wrap>
          <NFlex wrap>
            <NSelect
              v-if="!embedded && !lockedServerID"
              v-model:value="selectedServerID"
              :options="serverOptions"
              :placeholder="locale.t('performance.selectServer')"
              style="width: 280px"
              @update:value="changeServer"
            />
            <NButton @click="refreshPerformance">{{ locale.t('common.refresh') }}</NButton>
            <NButton type="primary" @click="probeSparkCapability">
              {{ capability ? locale.t('performance.reprobe') : locale.t('performance.probe') }}
            </NButton>
          </NFlex>
        </NFlex>
      </NCard>

      <NAlert v-if="error" type="error" :title="locale.t('performance.loadFailedTitle')">
        {{ error instanceof Error ? error.message : String(error) }}
      </NAlert>
      <NAlert v-if="partialMessage" type="warning" :title="locale.t('performance.partialTitle')">
        {{ partialMessage }}
      </NAlert>
      <NEmpty v-if="servers.length === 0" :description="locale.t('performance.noServers')" />

      <template v-if="selectedServer">
        <NCard title="Spark Capability">
          <NFlex vertical :size="12">
            <NFlex align="center" justify="space-between" wrap>
              <NFlex align="center">
                <NTag :type="statusType(capability?.status)">{{
                  capability?.status ?? 'unknown'
                }}</NTag>
                <NText>{{
                  capability?.pluginVersion || locale.t('performance.versionUnknown')
                }}</NText>
                <NText depth="3">{{ capability?.platform || selectedServer.type }}</NText>
                <NText v-if="capability?.detectedAt" depth="3">
                  {{
                    locale.t('performance.lastProbe', {
                      time: locale.formatDateTime(capability.detectedAt),
                    })
                  }}
                </NText>
              </NFlex>
              <NFlex wrap>
                <NTag :type="sparkCollectionStatus.type">{{ sparkCollectionStatus.label }}</NTag>
                <NButton :loading="loading" @click="prepareInstall">
                  {{ locale.t('performance.planButton') }}
                </NButton>
                <NButton v-if="capability?.backupPath" type="warning" @click="confirmRollback">
                  {{ locale.t('performance.rollback') }}
                </NButton>
                <NButton
                  type="primary"
                  :disabled="
                    capability?.status !== 'available' ||
                    !capability?.tpsSupported ||
                    selectedServer.state !== 'running' ||
                    collectorPaused
                  "
                  @click="collectSnapshot"
                >
                  {{ locale.t('performance.collectSnapshot') }}
                </NButton>
                <NButton
                  :type="collectorPaused ? 'primary' : 'warning'"
                  :loading="collectionActionLoading"
                  @click="toggleSparkCollection"
                >
                  {{
                    collectorPaused
                      ? locale.t('performance.resumeCollection')
                      : locale.t('performance.pauseCollection')
                  }}
                </NButton>
                <NButton
                  type="error"
                  secondary
                  :loading="collectionActionLoading"
                  @click="clearSparkHistory"
                >
                  {{ locale.t('performance.clearHistory') }}
                </NButton>
              </NFlex>
            </NFlex>
            <NDescriptions bordered :columns="2" label-placement="left">
              <NDescriptionsItem label="Parser">{{
                capability?.parserVersion || '—'
              }}</NDescriptionsItem>
              <NDescriptionsItem label="Collection">{{
                capability?.collectionMethod || '—'
              }}</NDescriptionsItem>
              <NDescriptionsItem label="TPS">{{
                capability?.tpsSupported
                  ? locale.t('performance.supported')
                  : locale.t('performance.unavailable')
              }}</NDescriptionsItem>
              <NDescriptionsItem label="MSPT">{{
                capability?.msptSupported
                  ? locale.t('performance.supported')
                  : locale.t('performance.unavailable')
              }}</NDescriptionsItem>
              <NDescriptionsItem label="Artifact">{{
                capability?.artifactPath || locale.t('performance.artifactBuiltIn')
              }}</NDescriptionsItem>
              <NDescriptionsItem label="Permission">{{
                capability?.permissionGranted
                  ? locale.t('performance.permissionVerified')
                  : locale.t('performance.permissionPending')
              }}</NDescriptionsItem>
            </NDescriptions>
            <NAlert
              v-if="capability?.lastError"
              :type="capability.status === 'unsupported' ? 'error' : 'warning'"
              :title="capability.lastErrorCode || locale.t('performance.sparkStatus')"
            >
              {{ capability.lastError }}
            </NAlert>
          </NFlex>
        </NCard>

        <NCard v-if="installPlan" :title="locale.t('performance.planCard')">
          <NDescriptions bordered :columns="2" label-placement="left">
            <NDescriptionsItem :label="locale.t('performance.planSource')">
              {{ installPlan.source }}
              {{
                installPlan.releaseID
                  ? `release ${installPlan.releaseID}`
                  : `build ${installPlan.build}`
              }}
            </NDescriptionsItem>
            <NDescriptionsItem :label="locale.t('performance.planVersion')">{{
              installPlan.targetVersion
            }}</NDescriptionsItem>
            <NDescriptionsItem :label="locale.t('performance.planSize')">{{
              formatArtifactSize(installPlan.artifactSize)
            }}</NDescriptionsItem>
            <NDescriptionsItem :label="locale.t('performance.planTarget')">{{
              installPlan.targetPath
            }}</NDescriptionsItem>
            <NDescriptionsItem :label="locale.t('performance.planBackup')">{{
              installPlan.backupPath
            }}</NDescriptionsItem>
            <NDescriptionsItem :label="checksumLabel(installPlan.checksumAlgorithm)" :span="2">{{
              installPlan.checksum
            }}</NDescriptionsItem>
            <NDescriptionsItem
              v-for="dependency in installPlan.dependencies"
              :key="dependency.targetPath"
              :label="dependency.name"
              :span="2"
            >
              {{ dependency.version }} · {{ dependency.targetPath }} ·
              {{ checksumLabel(dependency.checksumAlgorithm) }} {{ dependency.checksum }}
            </NDescriptionsItem>
          </NDescriptions>
          <NAlert
            :type="installPlan.restartRequired ? 'warning' : 'info'"
            :title="locale.t('performance.planImpact')"
          >
            {{ installPlan.impact }}
          </NAlert>
          <NFlex justify="end">
            <NButton type="primary" @click="confirmInstall">
              {{ locale.t('performance.planConfirm') }}
            </NButton>
          </NFlex>
        </NCard>

        <NAlert
          v-if="latestSnapshot"
          type="info"
          :title="locale.t('performance.snapshotAlertTitle')"
        >
          {{ latestSnapshotSummary }}
          {{ latestTPSCapped ? locale.t('performance.tpsCappedHint') : '' }}
        </NAlert>

        <NGrid cols="1 620:2 1000:4" :x-gap="12" :y-gap="12">
          <NGridItem>
            <NCard size="small"
              ><NStatistic
                label="TPS 5s"
                :value="formatTPS(latestSnapshot?.tps5Seconds, latestSnapshot?.tps5SecondsCapped)"
            /></NCard>
          </NGridItem>
          <NGridItem>
            <NCard size="small"
              ><NStatistic
                label="TPS 1m"
                :value="formatTPS(latestSnapshot?.tps1Minute, latestSnapshot?.tps1MinuteCapped)"
            /></NCard>
          </NGridItem>
          <NGridItem>
            <NCard size="small"
              ><NStatistic
                label="MSPT Median"
                :value="
                  latestSnapshot?.msptAvailable
                    ? formatMetric(latestSnapshot.msptMedian, 'ms')
                    : locale.t('performance.unavailable')
                "
            /></NCard>
          </NGridItem>
          <NGridItem>
            <NCard size="small"
              ><NStatistic
                label="MSPT P95"
                :value="
                  latestSnapshot?.msptAvailable
                    ? formatMetric(latestSnapshot.msptP95, 'ms')
                    : locale.t('performance.unavailable')
                "
            /></NCard>
          </NGridItem>
        </NGrid>

        <NGrid cols="1 900:2" :x-gap="12" :y-gap="12">
          <NGridItem>
            <NCard :title="locale.t('performance.trendCard')">
              <VChart
                v-if="snapshots.length"
                class="trend-chart"
                :option="trendOption"
                :init-options="chartInit"
                autoresize
              />
              <NEmpty v-else :description="locale.t('performance.noSnapshots')" />
            </NCard>
          </NGridItem>
          <NGridItem>
            <NCard :title="locale.t('performance.distributionCard')">
              <VChart
                v-if="latestSnapshot?.msptAvailable"
                class="trend-chart"
                :option="distributionOption"
                :init-options="chartInit"
                autoresize
              />
              <NEmpty v-else :description="locale.t('performance.noDistribution')" />
            </NCard>
          </NGridItem>
        </NGrid>

        <NCard :title="locale.t('performance.reportsCard')">
          <NFlex align="center" justify="space-between" wrap>
            <NText depth="3">{{ locale.t('performance.privacyHint') }}</NText>
            <NFlex align="center" wrap>
              <NButton :disabled="capability?.status !== 'available'" @click="createHealthReport">
                {{ locale.t('performance.createHealthReport') }}
              </NButton>
              <NInputNumber
                v-model:value="profilerDurationMinutes"
                :min="1"
                :max="10"
                :step="1"
                :precision="0"
                style="width: 130px"
              />
              <NText depth="3">{{ locale.t('performance.minutes') }}</NText>
              <NButton
                type="warning"
                :disabled="capability?.status !== 'available'"
                @click="createProfiler"
              >
                {{ locale.t('performance.startProfiler') }}
              </NButton>
            </NFlex>
          </NFlex>
          <NTable v-if="reports.length" size="small" striped>
            <thead>
              <tr>
                <th>{{ locale.t('performance.column.kind') }}</th>
                <th>{{ locale.t('performance.column.state') }}</th>
                <th>{{ locale.t('performance.column.time') }}</th>
                <th>{{ locale.t('performance.column.operation') }}</th>
                <th>{{ locale.t('performance.column.report') }}</th>
                <th>{{ locale.t('performance.column.error') }}</th>
                <th>{{ locale.t('common.actions') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="report in reports" :key="report.id">
                <td>{{ reportKindLabel(report.kind) }}</td>
                <td>
                  <NTag
                    :type="
                      report.state === 'completed'
                        ? 'success'
                        : report.state === 'failed'
                          ? 'error'
                          : 'warning'
                    "
                    >{{ reportStateLabel(report.state) }}</NTag
                  >
                </td>
                <td>{{ locale.formatDateTime(report.createdAt) }}</td>
                <td>{{ report.operationID || '—' }}</td>
                <td>
                  <NButton v-if="report.reportURL" text type="primary" @click="openReport(report)">
                    {{ locale.t('performance.openReport') }}
                  </NButton>
                  <span v-else>—</span>
                </td>
                <td>{{ report.errorMessage || '—' }}</td>
                <td>
                  <NButton
                    text
                    type="error"
                    :disabled="report.state === 'pending' || report.state === 'running'"
                    @click="deleteReport(report)"
                  >
                    {{ locale.t('common.delete') }}
                  </NButton>
                </td>
              </tr>
            </tbody>
          </NTable>
          <NEmpty v-else :description="locale.t('performance.noReports')" />
        </NCard>
      </template>
    </NFlex>
  </NSpin>
</template>

<style scoped>
.trend-chart {
  width: 100%;
  height: 360px;
}
</style>
