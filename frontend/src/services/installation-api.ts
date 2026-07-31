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

/**
 * 列出可安装的服务端发行版。
 * @returns 服务端发行版数组
 */
export async function listServerDistributions(): Promise<ServerDistribution[]> {
  const result = await ListDistributions()
  throwIfError(result.error)
  return result.distributions
}

/**
 * 解析某个发行版可安装的版本列表。
 * @param distribution - 发行版标识
 * @returns 可安装版本数组
 */
export async function resolveServerVersions(distribution: string): Promise<ServerVersion[]> {
  const result = await ResolveVersions(distribution)
  throwIfError(result.error)
  return result.versions
}

/**
 * 为某台 Server 启动一次安装任务。
 * @param serverID - 目标 Server ID
 * @returns 安装任务的启动信息
 */
export async function startInstallation(serverID: string): Promise<InstallationStart> {
  const result = await Start(serverID)
  throwIfError(result.error)
  if (!result.taskID || !result.operationID) throw new Error('InstallationService 未返回任务标识')
  return { taskID: result.taskID, operationID: result.operationID }
}

/**
 * 读取一次安装任务的完整聚合状态。
 * @param taskID - 安装任务 ID
 * @returns 安装任务聚合
 */
export async function getInstallation(taskID: string): Promise<InstallationAggregate> {
  const result = await Get(taskID)
  throwIfError(result.error)
  if (!result.task) throw new Error('InstallationService 未返回 InstallationTask')
  return { task: result.task, steps: result.steps }
}

/**
 * 分页列出某台 Server 的安装历史。
 * @param serverID - 目标 Server ID
 * @param limit - 单页条数
 * @param offset - 偏移量
 * @returns 安装任务数组
 */
export async function listServerInstallations(
  serverID: string,
  limit = 50,
  offset = 0,
): Promise<InstallationTask[]> {
  const result = await ListByServer(serverID, limit, offset)
  throwIfError(result.error)
  return result.tasks
}

/**
 * 取消一次进行中的安装。
 * @param operationID - 关联的 Operation ID
 * @returns 取消完成后的 Promise
 */
export async function cancelInstallation(operationID: string): Promise<void> {
  const result = await Cancel(operationID)
  throwIfError(result.error)
}

/**
 * 重试一次失败的安装任务。
 * @param taskID - 安装任务 ID
 * @returns 新安装任务的启动信息
 */
export async function retryInstallation(taskID: string): Promise<InstallationStart> {
  const result = await Retry(taskID)
  throwIfError(result.error)
  if (!result.taskID || !result.operationID)
    throw new Error('InstallationService 未返回重试任务标识')
  return { taskID: result.taskID, operationID: result.operationID }
}

/**
 * 处理安装目录冲突：备份、改名或取消。
 * @param taskID - 安装任务 ID
 * @param action - 冲突处理方式
 * @param newName - 改名时的新目录名
 * @returns 安装任务的启动信息
 */
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
