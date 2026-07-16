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

/** Returns the bounded Performance Center state for one Server. */
export async function getPerformanceOverview(serverID: string): Promise<PerformanceOverview> {
  const result = await Overview(serverID)
  throwIfError(result.error)
  if (!result.overview) throw new Error('PerformanceService 未返回 Overview')
  return result.overview
}

/** Detects Spark installation, version, permissions, and collection method. */
export async function probeSpark(serverID: string): Promise<SparkCapability> {
  const result = await Probe(serverID)
  throwIfError(result.error)
  if (!result.capability) throw new Error('PerformanceService 未返回 Spark Capability')
  return result.capability
}

/** Computes the official artifact, checksum, backup, and restart impact. */
export async function planSparkInstall(serverID: string): Promise<SparkInstallPlan> {
  const result = await PlanInstall(serverID)
  throwIfError(result.error)
  if (!result.plan) throw new Error('PerformanceService 未返回 Spark Install Plan')
  return result.plan
}

/** Executes one explicitly confirmed Spark install or upgrade. */
export async function installSpark(
  serverID: string,
  planDigest: string,
): Promise<SparkInstallResult> {
  const result = await Install(serverID, planDigest, true)
  throwIfError(result.error)
  if (!result.result) throw new Error('PerformanceService 未返回 Spark Install Result')
  return result.result
}

/** Restores the latest recorded Spark backup after explicit confirmation. */
export async function rollbackSpark(serverID: string): Promise<SparkCapability> {
  const result = await Rollback(serverID, true)
  throwIfError(result.error)
  if (!result.capability) throw new Error('PerformanceService 未返回回滚后的 Spark Capability')
  return result.capability
}

/** Collects and persists one versioned TPS/MSPT snapshot. */
export async function collectSparkSnapshot(serverID: string): Promise<SparkSnapshot> {
  const result = await CollectSnapshot(serverID)
  throwIfError(result.error)
  if (!result.snapshot) throw new Error('PerformanceService 未返回 Spark Snapshot')
  return result.snapshot
}

/** Starts one privacy-confirmed Spark health report Operation. */
export async function startSparkHealthReport(
  serverID: string,
  privacyAcknowledged: boolean,
): Promise<SparkReport> {
  const result = await StartHealthReport(serverID, privacyAcknowledged)
  throwIfError(result.error)
  if (!result.report) throw new Error('PerformanceService 未返回 Spark Health Report')
  return result.report
}

/** Starts one explicit-duration privacy-confirmed Spark profiler Operation. */
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

/** Deletes one completed, failed, or cancelled Spark report record. */
export async function deleteSparkReport(reportID: string): Promise<void> {
  const result = await DeleteReport(reportID)
  throwIfError(result.error)
}
