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
    return `最近采集 ${locale.formatDateTime(latest.collectedAt)} · 已加载 1 条 Snapshot`
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
  return `最近采集 ${locale.formatDateTime(latest.collectedAt)} · 已加载 ${snapshots.value.length} 条 Snapshot · ${changed ? '本次指标已变化' : '本次与上一采样数值相同'}`
})

const latestTPSCapped = computed(
  () => latestSnapshot.value?.tps5SecondsCapped || latestSnapshot.value?.tps1MinuteCapped,
)
const sparkCollectionStatus = computed(() => {
  if (collectorPaused.value) return { type: 'warning' as const, label: '已暂停' }
  if (selectedServer.value?.state === 'running')
    return { type: 'success' as const, label: '采集已启用' }
  return { type: 'default' as const, label: '服务器停止，采集已停止' }
})

function roundToTwo(value: number): number {
  return Number(value.toFixed(2))
}

function roundOptionalToTwo(value: number | undefined): number | null {
  return value === undefined ? null : roundToTwo(value)
}

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

function statusType(status: string | undefined): 'default' | 'success' | 'warning' | 'error' {
  if (status === 'available') return 'success'
  if (status === 'unknown' || status === 'collecting') return 'warning'
  if (status === 'unavailable' || status === 'unsupported' || status === 'failed') return 'error'
  return 'default'
}

function formatMetric(value: number | undefined, suffix: string, digits = 2): string {
  return value === undefined ? '—' : `${value.toFixed(digits)} ${suffix}`
}

function formatTPS(value: number | undefined, capped: boolean | undefined): string {
  if (value === undefined) return '—'
  return `${capped ? '>' : ''}${value.toFixed(2)} TPS`
}

function checksumLabel(algorithm: string): string {
  return algorithm.toUpperCase().replace('SHA', 'SHA-')
}

function formatArtifactSize(bytes: number): string {
  if (bytes <= 0) return '未声明'
  return `${(bytes / 1024 / 1024).toFixed(2)} MiB`
}

async function changeServer(): Promise<void> {
  if (props.lockedServerID) return
  await router.replace({ query: { ...route.query, serverID: selectedServerID.value || undefined } })
  installPlan.value = null
  await refreshPerformance()
}

async function refreshPerformance(): Promise<void> {
  try {
    await performance.refresh()
  } catch (reason) {
    notifications.push({
      kind: 'error',
      title: '刷新 Performance 数据失败',
      content: reason instanceof Error ? reason.message : String(reason),
      dedupeKey: 'performance:refresh:error',
    })
  }
}

async function probeSparkCapability(): Promise<void> {
  await runAction(() => performance.probe(), 'Spark 探测完成')
}

async function collectSnapshot(): Promise<void> {
  await runAction(() => performance.collect(), 'Spark TPS/MSPT 已采集')
}

async function toggleSparkCollection(): Promise<void> {
  if (!selectedServerID.value || collectionActionLoading.value) return
  const paused = collectorPaused.value
  collectionActionLoading.value = true
  try {
    if (paused) await resumeMonitoring(selectedServerID.value)
    else await pauseMonitoring(selectedServerID.value)
    notifications.push({
      kind: 'success',
      title: paused ? 'Spark 自动采集已恢复' : 'Spark 自动采集已暂停',
      content: paused
        ? '运行中的服务器将在下一个周期重新部署远端采集守护进程。'
        : '远端 Spark 与主机指标采集守护进程均已停止。',
      dedupeKey: `performance:collection:${paused ? 'resume' : 'pause'}`,
    })
    await performance.refresh()
  } catch (reason) {
    notifications.push({
      kind: 'error',
      title: paused ? '恢复 Spark 自动采集失败' : '暂停 Spark 自动采集失败',
      content: reason instanceof Error ? reason.message : String(reason),
      dedupeKey: `performance:collection:${paused ? 'resume' : 'pause'}:error`,
    })
  } finally {
    collectionActionLoading.value = false
  }
}

async function clearSparkHistory(): Promise<void> {
  if (!selectedServer.value || collectionActionLoading.value) return
  const confirmed = await interactions.confirm({
    title: '清空 Spark 与监控历史？',
    content: '将永久删除该服务器的 Spark TPS/MSPT Snapshot、原始指标和聚合历史。',
    objectLabel: selectedServer.value.name,
    impact: collectorPaused.value
      ? '采集当前已暂停，清空后不会立即产生新数据。'
      : '运行中的服务器会在下一个远端采集周期重新产生数据。',
    positiveText: '确认清空',
    danger: true,
  })
  if (!confirmed) return
  collectionActionLoading.value = true
  try {
    await clearMonitoringHistory(selectedServer.value.id)
    notifications.push({
      kind: 'success',
      title: 'Spark 与监控历史已清空',
      content: selectedServer.value.name,
      dedupeKey: `performance:clear:${selectedServer.value.id}`,
    })
    await performance.refresh()
  } catch (reason) {
    notifications.push({
      kind: 'error',
      title: '清空 Spark 历史失败',
      content: reason instanceof Error ? reason.message : String(reason),
      dedupeKey: 'performance:clear:error',
    })
  } finally {
    collectionActionLoading.value = false
  }
}

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
        diagnostics.push(`下载源：${reason.details.sources.join(', ')}`)
      }
      if (
        typeof reason.details.expectedChecksumAlgorithm === 'string' &&
        typeof reason.details.expectedChecksum === 'string'
      ) {
        diagnostics.push(
          `预期 ${checksumLabel(reason.details.expectedChecksumAlgorithm)}：${reason.details.expectedChecksum}`,
        )
      } else if (typeof reason.details.expectedSHA256 === 'string') {
        diagnostics.push(`预期 SHA-256：${reason.details.expectedSHA256}`)
      }
      if (Array.isArray(reason.details.downloadErrors) && reason.details.downloadErrors.length) {
        diagnostics.push(`下载异常：${reason.details.downloadErrors.join('; ')}`)
      }
      if (Array.isArray(reason.details.rollbackErrors) && reason.details.rollbackErrors.length) {
        diagnostics.push(`回滚异常：${reason.details.rollbackErrors.join('; ')}`)
      }
      if (typeof reason.details.rawResponse === 'string' && reason.details.rawResponse.trim()) {
        diagnostics.push(`Spark 原始响应：\n${reason.details.rawResponse.trim()}`)
      }
    }
    notifications.push({
      kind: 'error',
      title: `${title}失败`,
      content: `${reason instanceof Error ? reason.message : String(reason)}${diagnostics.length ? `\n${diagnostics.join('\n')}` : ''}`,
      dedupeKey: `performance:${title}:error`,
    })
  }
}

async function prepareInstall(): Promise<void> {
  await runAction(() => performance.planInstall(), 'Spark 安装计划已生成')
}

async function confirmInstall(): Promise<void> {
  if (!installPlan.value) return
  const dependencyImpact = installPlan.value.dependencies
    .map(
      (dependency) =>
        `${dependency.name} ${dependency.version}: ${dependency.targetPath}\n${checksumLabel(dependency.checksumAlgorithm)}: ${dependency.checksum}`,
    )
    .join('\n')
  const confirmed = await interactions.confirm({
    title: capability.value?.installed ? '升级 Minecraft spark？' : '安装 Minecraft spark？',
    content: installPlan.value.impact,
    objectLabel: `${installPlan.value.serverName} · ${installPlan.value.targetVersion}`,
    impact: `${installPlan.value.targetPath}\n${checksumLabel(installPlan.value.checksumAlgorithm)}: ${installPlan.value.checksum}${dependencyImpact ? `\n${dependencyImpact}` : ''}`,
    positiveText: capability.value?.installed ? '确认升级' : '确认安装',
    danger: installPlan.value.restartRequired,
  })
  if (confirmed) await runAction(() => performance.install(), 'Spark Artifact 已安装')
}

async function confirmRollback(): Promise<void> {
  if (!capability.value?.backupPath || !selectedServer.value) return
  const confirmed = await interactions.confirm({
    title: '回滚 Minecraft spark？',
    content:
      '当前 Artifact 会保留为失败副本，最近一次备份将恢复到原路径。Server 运行中时仍需重启。',
    objectLabel: selectedServer.value.name,
    impact: capability.value.backupPath,
    positiveText: '确认回滚',
    danger: true,
  })
  if (confirmed) await runAction(() => performance.rollback(), 'Spark Artifact 已回滚')
}

async function createHealthReport(): Promise<void> {
  if (!reportPrivacyConfirmation.value) {
    await runAction(() => performance.healthReport(), 'Health Report Operation 已启动')
    return
  }
  const confirmed = await interactions.confirm({
    title: '生成 Spark Health Report？',
    content: '报告将上传到 spark.lucko.me，可能包含服务器版本、插件、主机性能和运行信息。',
    objectLabel: selectedServer.value?.name ?? 'Minecraft Server',
    positiveText: '确认并生成',
  })
  if (confirmed) await runAction(() => performance.healthReport(), 'Health Report Operation 已启动')
}

async function createProfiler(): Promise<void> {
  if (!reportPrivacyConfirmation.value) {
    await runAction(() => performance.profiler(), 'Profiler Operation 已启动')
    return
  }
  const confirmed = await interactions.confirm({
    title: '启动 Spark Profiler？',
    content: `Profiler 将运行 ${profilerDurationMinutes.value} 分钟，完成后上传到 spark.lucko.me。`,
    objectLabel: selectedServer.value?.name ?? 'Minecraft Server',
    impact: '报告可能包含线程、插件和服务器运行信息；可在 Operations 中取消。',
    positiveText: '确认并启动',
  })
  if (confirmed) await runAction(() => performance.profiler(), 'Profiler Operation 已启动')
}

async function openReport(report: SparkReport): Promise<void> {
  if (!report.reportURL) return
  if (!reportPrivacyConfirmation.value) {
    await openExternalReport(report.reportURL)
    return
  }
  const confirmed = await interactions.confirm({
    title: '打开外部 Spark Report？',
    content: '该链接由 spark.lucko.me 托管，打开或分享可能暴露服务器运行信息。',
    objectLabel: report.kind,
    impact: report.reportURL,
    positiveText: '确认打开',
  })
  if (confirmed) await openExternalReport(report.reportURL)
}

async function openExternalReport(reportURL: string): Promise<void> {
  try {
    await Browser.OpenURL(reportURL)
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: 'Spark 报告打开失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: `spark-report:open:${reportURL}`,
    })
  }
}

function reportKindLabel(kind: string): string {
  return kind === 'health' ? '健康报告' : kind === 'profiler' ? '性能分析' : kind
}

function reportStateLabel(state: string): string {
  const labels: Record<string, string> = {
    pending: '等待中',
    running: '运行中',
    completed: '已完成',
    failed: '失败',
    cancelled: '已取消',
  }
  return labels[state] ?? state
}

async function deleteReport(report: SparkReport): Promise<void> {
  const confirmed = await interactions.confirm({
    title: '删除 Spark 报告记录？',
    content: '删除后将无法从 MineOps 报告历史中恢复，但不会删除 spark.lucko.me 上的外部数据。',
    objectLabel: `${reportKindLabel(report.kind)} · ${locale.formatDateTime(report.createdAt)}`,
    positiveText: '确认删除',
    danger: true,
  })
  if (!confirmed) return
  await runAction(async () => {
    await deleteSparkReport(report.id)
    await performance.refresh()
  }, '报告记录已删除')
}

async function loadPerformance(preferredServerID: string): Promise<void> {
  try {
    await performance.loadServers(preferredServerID)
    await performance.refresh()
  } catch (reason) {
    notifications.push({
      kind: 'error',
      title: '加载 Performance Center 失败',
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
              placeholder="选择 Server"
              style="width: 280px"
              @update:value="changeServer"
            />
            <NButton @click="refreshPerformance">刷新</NButton>
            <NButton type="primary" @click="probeSparkCapability">
              {{ capability ? '重新探测 Spark' : '探测 Spark' }}
            </NButton>
          </NFlex>
        </NFlex>
      </NCard>

      <NAlert v-if="error" type="error" title="Performance Center 加载失败">
        {{ error instanceof Error ? error.message : String(error) }}
      </NAlert>
      <NAlert v-if="partialMessage" type="warning" title="部分 Performance 数据不可用">
        {{ partialMessage }}
      </NAlert>
      <NEmpty v-if="servers.length === 0" description="请先创建 Minecraft Server" />

      <template v-if="selectedServer">
        <NCard title="Spark Capability">
          <NFlex vertical :size="12">
            <NFlex align="center" justify="space-between" wrap>
              <NFlex align="center">
                <NTag :type="statusType(capability?.status)">{{
                  capability?.status ?? 'unknown'
                }}</NTag>
                <NText>{{ capability?.pluginVersion || '版本未确认' }}</NText>
                <NText depth="3">{{ capability?.platform || selectedServer.type }}</NText>
                <NText v-if="capability?.detectedAt" depth="3">
                  最近探测 · {{ locale.formatDateTime(capability.detectedAt) }}
                </NText>
              </NFlex>
              <NFlex wrap>
                <NTag :type="sparkCollectionStatus.type">{{ sparkCollectionStatus.label }}</NTag>
                <NButton :loading="loading" @click="prepareInstall">生成安装/升级计划</NButton>
                <NButton v-if="capability?.backupPath" type="warning" @click="confirmRollback">
                  回滚
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
                  采集 TPS/MSPT
                </NButton>
                <NButton
                  :type="collectorPaused ? 'primary' : 'warning'"
                  :loading="collectionActionLoading"
                  @click="toggleSparkCollection"
                >
                  {{ collectorPaused ? '恢复采集' : '暂停采集' }}
                </NButton>
                <NButton
                  type="error"
                  secondary
                  :loading="collectionActionLoading"
                  @click="clearSparkHistory"
                >
                  清空历史
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
                capability?.tpsSupported ? '支持' : '不可用'
              }}</NDescriptionsItem>
              <NDescriptionsItem label="MSPT">{{
                capability?.msptSupported ? '支持' : '不可用'
              }}</NDescriptionsItem>
              <NDescriptionsItem label="Artifact">{{
                capability?.artifactPath || '内置或未检测'
              }}</NDescriptionsItem>
              <NDescriptionsItem label="Permission">{{
                capability?.permissionGranted ? '已验证' : '待运行验证'
              }}</NDescriptionsItem>
            </NDescriptions>
            <NAlert
              v-if="capability?.lastError"
              :type="capability.status === 'unsupported' ? 'error' : 'warning'"
              :title="capability.lastErrorCode || 'Spark 状态'"
            >
              {{ capability.lastError }}
            </NAlert>
          </NFlex>
        </NCard>

        <NCard v-if="installPlan" title="官方安装计划">
          <NDescriptions bordered :columns="2" label-placement="left">
            <NDescriptionsItem label="来源">
              {{ installPlan.source }}
              {{
                installPlan.releaseID
                  ? `release ${installPlan.releaseID}`
                  : `build ${installPlan.build}`
              }}
            </NDescriptionsItem>
            <NDescriptionsItem label="版本">{{ installPlan.targetVersion }}</NDescriptionsItem>
            <NDescriptionsItem label="文件大小">{{
              formatArtifactSize(installPlan.artifactSize)
            }}</NDescriptionsItem>
            <NDescriptionsItem label="目标">{{ installPlan.targetPath }}</NDescriptionsItem>
            <NDescriptionsItem label="备份">{{ installPlan.backupPath }}</NDescriptionsItem>
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
          <NAlert :type="installPlan.restartRequired ? 'warning' : 'info'" title="变更影响">
            {{ installPlan.impact }}
          </NAlert>
          <NFlex justify="end">
            <NButton type="primary" @click="confirmInstall">确认执行</NButton>
          </NFlex>
        </NCard>

        <NAlert v-if="latestSnapshot" type="info" title="Spark Snapshot 持续采集中">
          {{ latestSnapshotSummary }}。
          {{
            latestTPSCapped
              ? 'Spark 官方已将高于目标值的 TPS 封顶显示为 20，动态负载请结合 MSPT 查看。'
              : ''
          }}
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
                    : '不可用'
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
                    : '不可用'
                "
            /></NCard>
          </NGridItem>
        </NGrid>

        <NGrid cols="1 900:2" :x-gap="12" :y-gap="12">
          <NGridItem>
            <NCard title="TPS / MSPT 趋势">
              <VChart
                v-if="snapshots.length"
                class="trend-chart"
                :option="trendOption"
                :init-options="chartInit"
                autoresize
              />
              <NEmpty v-else description="尚无 Spark Snapshot" />
            </NCard>
          </NGridItem>
          <NGridItem>
            <NCard title="最新 Tick 分布">
              <VChart
                v-if="latestSnapshot?.msptAvailable"
                class="trend-chart"
                :option="distributionOption"
                :init-options="chartInit"
                autoresize
              />
              <NEmpty v-else description="当前平台或响应没有 MSPT 分布" />
            </NCard>
          </NGridItem>
        </NGrid>

        <NCard title="健康报告与性能分析">
          <NFlex align="center" justify="space-between" wrap>
            <NText depth="3">生成和打开外部报告链接时均需要确认隐私风险。</NText>
            <NFlex align="center" wrap>
              <NButton :disabled="capability?.status !== 'available'" @click="createHealthReport"
                >生成健康报告</NButton
              >
              <NInputNumber
                v-model:value="profilerDurationMinutes"
                :min="1"
                :max="10"
                :step="1"
                :precision="0"
                style="width: 130px"
              />
              <NText depth="3">分钟</NText>
              <NButton
                type="warning"
                :disabled="capability?.status !== 'available'"
                @click="createProfiler"
                >启动性能分析</NButton
              >
            </NFlex>
          </NFlex>
          <NTable v-if="reports.length" size="small" striped>
            <thead>
              <tr>
                <th>类型</th>
                <th>状态</th>
                <th>时间</th>
                <th>任务 ID</th>
                <th>报告</th>
                <th>错误</th>
                <th>操作</th>
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
                  <NButton v-if="report.reportURL" text type="primary" @click="openReport(report)"
                    >打开</NButton
                  ><span v-else>—</span>
                </td>
                <td>{{ report.errorMessage || '—' }}</td>
                <td>
                  <NButton
                    text
                    type="error"
                    :disabled="report.state === 'pending' || report.state === 'running'"
                    @click="deleteReport(report)"
                    >删除</NButton
                  >
                </td>
              </tr>
            </tbody>
          </NTable>
          <NEmpty v-else description="尚无 Spark 报告历史" />
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
