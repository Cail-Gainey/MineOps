import type { RemoteProcessIdentity } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import {
  GetState,
  Recover,
  Restart,
  Start,
  Stop,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/lifecycleservice'
import { throwIfError } from './api-client'

export interface LifecycleStateSnapshot {
  state: string
  identity?: RemoteProcessIdentity
}

/**
 * 读取某台 Server 的生命周期状态快照。
 * @param serverID - 目标 Server ID
 * @returns 生命周期状态快照
 */
export async function getLifecycleState(serverID: string): Promise<LifecycleStateSnapshot> {
  const result = await GetState(serverID)
  throwIfError(result.error)
  const snapshot: LifecycleStateSnapshot = { state: result.state }
  if (result.identity) snapshot.identity = result.identity
  return snapshot
}

/**
 * 启动 Server，必要时携带防火墙与 tmux 安装的确认结果。
 * @param serverID - 目标 Server ID
 * @param firewallConfirmed - 是否已确认放行防火墙端口
 * @param tmuxInstallConfirmed - 是否已确认安装 tmux
 * @returns 关联的 Operation ID
 */
export async function startServer(
  serverID: string,
  firewallConfirmed = false,
  tmuxInstallConfirmed = false,
): Promise<string> {
  const result = await Start(serverID, firewallConfirmed, tmuxInstallConfirmed)
  throwIfError(result.error)
  return result.operationID ?? ''
}

/**
 * 停止 Server，可选择强制结束。
 * @param serverID - 目标 Server ID
 * @param force - 是否强制结束进程
 * @returns 关联的 Operation ID
 */
export async function stopServer(serverID: string, force = false): Promise<string> {
  const result = await Stop(serverID, force)
  throwIfError(result.error)
  return result.operationID ?? ''
}

/**
 * 重启 Server。
 * @param serverID - 目标 Server ID
 * @param tmuxInstallConfirmed - 是否已确认安装 tmux
 * @returns 关联的 Operation ID
 */
export async function restartServer(
  serverID: string,
  tmuxInstallConfirmed = false,
): Promise<string> {
  const result = await Restart(serverID, tmuxInstallConfirmed)
  throwIfError(result.error)
  return result.operationID ?? ''
}

/**
 * 应用启动后校正各 Server 的生命周期状态。
 * @returns 恢复完成后的 Promise
 */
export async function recoverServerLifecycles(): Promise<void> {
  const result = await Recover()
  throwIfError(result.error)
}
