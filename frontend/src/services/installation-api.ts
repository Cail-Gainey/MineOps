import type {
  InstallationStep,
  InstallationTask,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import type {
  ServerDistribution,
  ServerVersion,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/port/models'
import {
  Cancel,
  Get,
  ListByServer,
  ListDistributions,
  ResolveVersions,
  ResolveDirectoryConflict,
  Retry,
  Start,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/installationservice'
import { throwIfError } from './api-client'

export interface InstallationStart {
  taskID: string
  operationID: string
}

export interface InstallationAggregate {
  task: InstallationTask
  steps: InstallationStep[]
}

/** Lists the backend-owned dynamic server distribution registry. */
export async function listServerDistributions(): Promise<ServerDistribution[]> {
  const result = await ListDistributions()
  throwIfError(result.error)
  return result.distributions
}

/** Resolves cached provider-neutral versions for one distribution. */
export async function resolveServerVersions(distribution: string): Promise<ServerVersion[]> {
  const result = await ResolveVersions(distribution)
  throwIfError(result.error)
  return result.versions
}

/** Starts a durable installation after backend capability preflight. */
export async function startInstallation(serverID: string): Promise<InstallationStart> {
  const result = await Start(serverID)
  throwIfError(result.error)
  if (!result.taskID || !result.operationID) throw new Error('InstallationService 未返回任务标识')
  return { taskID: result.taskID, operationID: result.operationID }
}

/** Loads one durable installation task and ordered checkpoint steps. */
export async function getInstallation(taskID: string): Promise<InstallationAggregate> {
  const result = await Get(taskID)
  throwIfError(result.error)
  if (!result.task) throw new Error('InstallationService 未返回 InstallationTask')
  return { task: result.task, steps: result.steps }
}

/** Lists recent durable installation tasks for one Minecraft Server. */
export async function listServerInstallations(
  serverID: string,
  limit = 50,
  offset = 0,
): Promise<InstallationTask[]> {
  const result = await ListByServer(serverID, limit, offset)
  throwIfError(result.error)
  return result.tasks
}

/** Cancels the active installation operation without coupling cancellation to UI lifetime. */
export async function cancelInstallation(operationID: string): Promise<void> {
  const result = await Cancel(operationID)
  throwIfError(result.error)
}

/** Retries only incomplete installation steps and returns the replacement Operation ID. */
export async function retryInstallation(taskID: string): Promise<InstallationStart> {
  const result = await Retry(taskID)
  throwIfError(result.error)
  if (!result.taskID || !result.operationID)
    throw new Error('InstallationService 未返回重试任务标识')
  return { taskID: result.taskID, operationID: result.operationID }
}

/** Resolves a failed server-directory decision without deleting the existing directory. */
export async function resolveInstallationDirectoryConflict(
  taskID: string,
  action: 'backup' | 'rename' | 'cancel',
  newName = '',
): Promise<InstallationStart> {
  const result = await ResolveDirectoryConflict(taskID, action, newName)
  throwIfError(result.error)
  if (!result.taskID) throw new Error('InstallationService 未返回目录冲突任务标识')
  return { taskID: result.taskID, operationID: result.operationID ?? '' }
}
