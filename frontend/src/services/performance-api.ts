import type {
  SparkCapability,
  SparkSnapshot,
  SparkReport,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import type {
  PerformanceOverview,
  SparkInstallPlan,
  SparkInstallResult,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/service/models'
import {
  CollectSnapshot,
  DeleteReport,
  Install,
  Overview,
  PlanInstall,
  Probe,
  Rollback,
  StartHealthReport,
  StartProfiler,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/performanceservice'
import { throwIfError } from './api-client'

/**
 * 读取某台 Server 的性能总览。
 * @param serverID - 目标 Server ID
 * @returns 性能总览
 */
export async function getPerformanceOverview(serverID: string): Promise<PerformanceOverview> {
  const result = await Overview(serverID)
  throwIfError(result.error)
  if (!result.overview) throw new Error('PerformanceService 未返回 Overview')
  return result.overview
}

/**
 * 探测远端 Spark 能力。
 * @param serverID - 目标 Server ID
 * @returns Spark 能力
 */
export async function probeSpark(serverID: string): Promise<SparkCapability> {
  const result = await Probe(serverID)
  throwIfError(result.error)
  if (!result.capability) throw new Error('PerformanceService 未返回 Spark Capability')
  return result.capability
}

/**
 * 计算 Spark 安装计划，不改动 Server。
 * @param serverID - 目标 Server ID
 * @returns Spark 安装计划
 */
export async function planSparkInstall(serverID: string): Promise<SparkInstallPlan> {
  const result = await PlanInstall(serverID)
  throwIfError(result.error)
  if (!result.plan) throw new Error('PerformanceService 未返回 Spark Install Plan')
  return result.plan
}

/**
 * 按已确认的计划安装 Spark。
 * @param serverID - 目标 Server ID
 * @param planDigest - 安装计划摘要，用于确认计划未变
 * @returns Spark 安装结果
 */
export async function installSpark(
  serverID: string,
  planDigest: string,
): Promise<SparkInstallResult> {
  const result = await Install(serverID, planDigest, true)
  throwIfError(result.error)
  if (!result.result) throw new Error('PerformanceService 未返回 Spark Install Result')
  return result.result
}

/**
 * 回滚到 Spark 安装前的备份。
 * @param serverID - 目标 Server ID
 * @returns 回滚后的 Spark 能力
 */
export async function rollbackSpark(serverID: string): Promise<SparkCapability> {
  const result = await Rollback(serverID, true)
  throwIfError(result.error)
  if (!result.capability) throw new Error('PerformanceService 未返回回滚后的 Spark Capability')
  return result.capability
}

/**
 * 立即采集一次 TPS/MSPT Snapshot。
 * @param serverID - 目标 Server ID
 * @returns Spark Snapshot
 */
export async function collectSparkSnapshot(serverID: string): Promise<SparkSnapshot> {
  const result = await CollectSnapshot(serverID)
  throwIfError(result.error)
  if (!result.snapshot) throw new Error('PerformanceService 未返回 Spark Snapshot')
  return result.snapshot
}

/**
 * 发起一次 Spark 健康报告上传。
 * @param serverID - 目标 Server ID
 * @param privacyAcknowledged - 是否已确认隐私风险
 * @returns Spark 报告
 */
export async function startSparkHealthReport(
  serverID: string,
  privacyAcknowledged: boolean,
): Promise<SparkReport> {
  const result = await StartHealthReport(serverID, privacyAcknowledged)
  throwIfError(result.error)
  if (!result.report) throw new Error('PerformanceService 未返回 Spark Health Report')
  return result.report
}

/**
 * 发起一次指定时长的 Spark 性能分析。
 * @param serverID - 目标 Server ID
 * @param durationSeconds - 分析时长秒数
 * @param privacyAcknowledged - 是否已确认隐私风险
 * @returns Spark 报告
 */
export async function startSparkProfiler(
  serverID: string,
  durationSeconds: number,
  privacyAcknowledged: boolean,
): Promise<SparkReport> {
  const result = await StartProfiler(serverID, durationSeconds, privacyAcknowledged)
  throwIfError(result.error)
  if (!result.report) throw new Error('PerformanceService 未返回 Spark Profiler Report')
  return result.report
}

/**
 * 删除一份 Spark 报告。
 * @param reportID - 报告 ID
 * @returns 删除完成后的 Promise
 */
export async function deleteSparkReport(reportID: string): Promise<void> {
  const result = await DeleteReport(reportID)
  throwIfError(result.error)
}
