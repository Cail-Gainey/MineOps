import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'

import type { MinecraftServer } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import type {
  SparkCapability,
  SparkReport,
  SparkSnapshot,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import type {
  MonitoringOverview,
  PerformanceOverview,
  SparkInstallPlan,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/service/models'
import { listMinecraftServers } from '../services/minecraft-server-api'
import { getMonitoringOverview } from '../services/monitoring-api'
import {
  deleteSparkReport,
  collectSparkSnapshot,
  getPerformanceOverview,
  installSpark,
  planSparkInstall,
  probeSpark,
  rollbackSpark,
  startSparkHealthReport,
  startSparkProfiler,
} from '../services/performance-api'
import { useSettingsStore } from './settings'

export const usePerformanceStore = defineStore('performance', () => {
  const settings = useSettingsStore()
  const servers = ref<MinecraftServer[]>([])
  const selectedServerID = ref('')
  const overview = ref<PerformanceOverview | null>(null)
  const monitoringOverview = ref<MonitoringOverview | null>(null)
  const installPlan = ref<SparkInstallPlan | null>(null)
  const profilerDurationSeconds = ref(60)
  const loading = ref(false)
  const serversError = ref<unknown>(null)
  const overviewError = ref<unknown>(null)
  const monitoringOverviewError = ref<unknown>(null)
  const overviewServerID = ref('')
  watch(
    () => settings.committed?.monitoring.profilerDefaultSeconds,
    (value) => {
      if (value) profilerDurationSeconds.value = value
    },
    { immediate: true },
  )

  const selectedServer = computed(
    () => servers.value.find((server) => server.id === selectedServerID.value) ?? null,
  )
  const capability = computed<SparkCapability | null>(() => overview.value?.capability ?? null)
  const reportPrivacyConfirmation = computed(
    () => settings.committed?.monitoring.reportPrivacyConfirmation ?? true,
  )
  const latestSnapshot = computed<SparkSnapshot | null>(
    () => overview.value?.latestSnapshot ?? null,
  )
  const reports = computed<SparkReport[]>(() => overview.value?.reports ?? [])
  const snapshots = computed<SparkSnapshot[]>(() => overview.value?.snapshots ?? [])
  const collectorPaused = computed(() => monitoringOverview.value?.collector?.paused === true)
  const error = computed(() => {
    if (serversError.value && servers.value.length === 0) return serversError.value
    if (overviewError.value && !overview.value) return overviewError.value
    return null
  })
  const partialMessage = computed(() => {
    if (serversError.value && servers.value.length)
      return 'Server 列表刷新失败，继续使用已加载目标。'
    if (overviewError.value && overview.value)
      return 'Performance 数据刷新失败，当前显示上一次成功快照。'
    if (monitoringOverviewError.value) return 'Spark 采集控制状态刷新失败，当前显示上一次成功状态。'
    return ''
  })

  /**
   * 加载 Server 列表并选定目标。
   * @param preferredServerID - 优先选中的 Server ID
   * @returns 加载完成后的 Promise
   */
  async function loadServers(preferredServerID = ''): Promise<void> {
    serversError.value = null
    try {
      servers.value = await listMinecraftServers()
      const preferred = servers.value.find((server) => server.id === preferredServerID)
      if (preferred) selectedServerID.value = preferred.id
      if (!servers.value.some((server) => server.id === selectedServerID.value)) {
        selectedServerID.value = servers.value[0]?.id ?? ''
      }
    } catch (reason) {
      serversError.value = reason
      throw reason
    }
  }

  /**
   * 重新加载所选 Server 的性能总览、快照与报告。
   * @returns 刷新完成后的 Promise
   */
  async function refresh(): Promise<void> {
    loading.value = true
    overviewError.value = null
    monitoringOverviewError.value = null
    try {
      if (servers.value.length === 0) await loadServers()
      if (overviewServerID.value !== selectedServerID.value) overview.value = null
      if (!selectedServerID.value) {
        overview.value = null
        monitoringOverview.value = null
        return
      }
      const [performanceResult, monitoringResult] = await Promise.allSettled([
        getPerformanceOverview(selectedServerID.value),
        getMonitoringOverview(selectedServerID.value),
      ])
      if (performanceResult.status === 'rejected') throw performanceResult.reason
      overview.value = performanceResult.value
      if (monitoringResult.status === 'fulfilled') monitoringOverview.value = monitoringResult.value
      else monitoringOverviewError.value = monitoringResult.reason
      overviewServerID.value = selectedServerID.value
    } catch (reason) {
      overviewError.value = reason
      throw reason
    } finally {
      loading.value = false
    }
  }

  /**
   * 重新探测远端 Spark 能力。
   * @returns 探测后的 Spark 能力
   */
  async function probe(): Promise<SparkCapability> {
    loading.value = true
    try {
      const result = await probeSpark(selectedServerID.value)
      await refresh()
      return result
    } finally {
      loading.value = false
    }
  }

  /**
   * 生成 Spark 安装计划。
   * @returns Spark 安装计划
   */
  async function planInstall(): Promise<SparkInstallPlan> {
    loading.value = true
    try {
      installPlan.value = await planSparkInstall(selectedServerID.value)
      return installPlan.value
    } finally {
      loading.value = false
    }
  }

  /**
   * 按已生成的计划安装 Spark，计划缺失或过期时抛错。
   * @returns 安装完成后的 Promise
   */
  async function install(): Promise<void> {
    if (!installPlan.value?.planDigest) throw new Error('Spark 安装计划不存在或已过期')
    loading.value = true
    try {
      await installSpark(selectedServerID.value, installPlan.value.planDigest)
      installPlan.value = null
      await refresh()
    } finally {
      loading.value = false
    }
  }

  /**
   * 回滚到 Spark 安装前的备份。
   * @returns 回滚完成后的 Promise
   */
  async function rollback(): Promise<void> {
    loading.value = true
    try {
      await rollbackSpark(selectedServerID.value)
      await refresh()
    } finally {
      loading.value = false
    }
  }

  /**
   * 立即采集一次 TPS/MSPT Snapshot。
   * @returns 采集完成后的 Promise
   */
  async function collect(): Promise<void> {
    loading.value = true
    try {
      await collectSparkSnapshot(selectedServerID.value)
      await refresh()
    } finally {
      loading.value = false
    }
  }

  /**
   * 发起一次 Spark 健康报告上传。
   * @returns 新建的 Spark 报告
   */
  async function healthReport(): Promise<SparkReport> {
    const report = await startSparkHealthReport(
      selectedServerID.value,
      reportPrivacyConfirmation.value,
    )
    await refresh()
    return report
  }

  /**
   * 按当前设定时长发起一次 Spark 性能分析。
   * @returns 新建的 Spark 报告
   */
  async function profiler(): Promise<SparkReport> {
    const report = await startSparkProfiler(
      selectedServerID.value,
      profilerDurationSeconds.value,
      reportPrivacyConfirmation.value,
    )
    await refresh()
    return report
  }

  /**
   * 删除一份 Spark 报告并刷新列表。
   * @param reportID - 报告 ID
   * @returns 删除完成后的 Promise
   */
  async function deleteReport(reportID: string): Promise<void> {
    await deleteSparkReport(reportID)
    await refresh()
  }

  return {
    capability,
    collectorPaused,
    collect,
    error,
    deleteReport,
    healthReport,
    install,
    installPlan,
    latestSnapshot,
    loading,
    loadServers,
    overview,
    monitoringOverview,
    overviewError,
    partialMessage,
    planInstall,
    probe,
    profiler,
    profilerDurationSeconds,
    reportPrivacyConfirmation,
    refresh,
    reports,
    rollback,
    selectedServer,
    selectedServerID,
    serversError,
    servers,
    snapshots,
  }
})
