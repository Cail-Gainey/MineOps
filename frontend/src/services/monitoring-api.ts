import type { MonitoringOverview } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/service/models'
import {
  ClearHistory,
  CollectNow,
  Overview,
  Pause,
  Resume,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/monitoringservice'
import { throwIfError } from './api-client'

/** Returns unified collection, Metric, Spark, Alert, and issue state. */
export async function getMonitoringOverview(serverID: string): Promise<MonitoringOverview> {
  const result = await Overview(serverID)
  throwIfError(result.error)
  if (!result.overview) throw new Error('MonitoringService 未返回 Overview')
  return result.overview
}

/** Triggers one immediate SSH collection pass for the Server. */
export async function collectNow(serverID: string): Promise<void> {
  const result = await CollectNow(serverID)
  throwIfError(result.error)
}

/** Pauses automatic local and remote monitoring collection for one Server. */
export async function pauseMonitoring(serverID: string): Promise<void> {
  const result = await Pause(serverID)
  throwIfError(result.error)
}

/** Resumes automatic monitoring collection for one Server. */
export async function resumeMonitoring(serverID: string): Promise<void> {
  const result = await Resume(serverID)
  throwIfError(result.error)
}

/** Deletes persisted Metric history, Spark snapshots, and pending remote Spark buffers. */
export async function clearMonitoringHistory(serverID: string): Promise<void> {
  const result = await ClearHistory(serverID)
  throwIfError(result.error)
}
