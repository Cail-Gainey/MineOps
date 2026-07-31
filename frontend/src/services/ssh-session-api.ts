import type {
  SSHSessionDTO,
  SSHSessionInput,
  SSHPreflightDTO,
  SSHConnectionTestDTO,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/models'
import {
  Create,
  Delete,
  EnsureHostSpecs,
  Get,
  List,
  Preflight,
  TestConnection,
  TestInput,
  Update,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/sshsessionservice'
import { throwIfError } from './api-client'
import { runWithHostKeyTrustConfirmation } from './host-key-trust'
import { translate } from '../locales/runtime'

/**
 * 按关键字、分组与收藏状态列出 SSH Session。
 * @param search - 搜索关键字
 * @param group - 分组名，空串表示不过滤
 * @param favouriteOnly - 是否只列出收藏项
 * @returns SSH Session 数组
 */
export async function listSSHSessions(
  search = '',
  group = '',
  favouriteOnly = false,
): Promise<SSHSessionDTO[]> {
  const result = await List(search, group, favouriteOnly, 200, 0)
  throwIfError(result.error)
  return result.sessions
}

/**
 * 按 ID 读取一个 SSH Session。
 * @param id - SSH Session ID
 * @returns SSH Session 详情
 */
export async function getSSHSession(id: string): Promise<SSHSessionDTO> {
  const result = await Get(id)
  throwIfError(result.error)
  if (!result.session)
    throw new Error(translate('service.emptyResponse', { subject: 'SSH Session' }))
  return result.session
}

/**
 * 创建一个 SSH Session 及其凭据。
 * @param input - SSH Session 输入内容
 * @returns 创建后的 SSH Session
 */
export async function createSSHSession(input: SSHSessionInput): Promise<SSHSessionDTO> {
  const result = await Create(input)
  throwIfError(result.error)
  if (!result.session)
    throw new Error(translate('service.emptyResponse', { subject: 'SSH Session' }))
  return result.session
}

/**
 * 更新一个 SSH Session 及其凭据。
 * @param id - SSH Session ID
 * @param input - SSH Session 输入内容
 * @returns 更新后的 SSH Session
 */
export async function updateSSHSession(id: string, input: SSHSessionInput): Promise<SSHSessionDTO> {
  const result = await Update(id, input)
  throwIfError(result.error)
  if (!result.session)
    throw new Error(translate('service.emptyResponse', { subject: 'SSH Session' }))
  return result.session
}

/**
 * 删除一个 SSH Session 及其关联凭据。
 * @param id - SSH Session ID
 * @returns 删除完成后的 Promise
 */
export async function deleteSSHSession(id: string): Promise<void> {
  const result = await Delete(id)
  throwIfError(result.error)
}

/**
 * 为尚未采集过主机规格的 Session 补采一次并写回该行。
 * 迁移前创建的历史 Session 用它补采；规格已存在时后端不会再连接 SSH。
 * @param id - SSH Session ID
 * @returns 补采后的 SSH Session
 */
export async function ensureSSHSessionHostSpecs(id: string): Promise<SSHSessionDTO> {
  const result = await EnsureHostSpecs(id)
  throwIfError(result.error)
  if (!result.session)
    throw new Error(translate('service.emptyResponse', { subject: 'SSH host specs' }))
  return result.session
}

/**
 * 按正式 SSH 路由执行连通性预检。
 * @param id - SSH Session ID
 * @returns 预检结果
 */
export async function preflightSSHSession(id: string): Promise<SSHPreflightDTO> {
  const result = await Preflight(id)
  throwIfError(result.error)
  return result
}

/**
 * 测试 SSH 连接，遇到未信任主机密钥时先走信任确认。
 * @param id - SSH Session ID
 * @returns 连接测试结果
 */
export async function testSSHSessionConnection(id: string): Promise<SSHConnectionTestDTO> {
  return runWithHostKeyTrustConfirmation(async () => {
    const result = await TestConnection(id)
    throwIfError(result.error)
    return result
  })
}

/**
 * @param id - Existing SSH Session ID, or an empty string for a new draft.
 * @param input - Unsaved metadata and write-only credential input.
 * @returns Authenticated SSH handshake and command-channel evidence.
 */
export async function testSSHSessionInput(
  id: string,
  input: SSHSessionInput,
): Promise<SSHConnectionTestDTO> {
  return runWithHostKeyTrustConfirmation(async () => {
    const result = await TestInput(id, input)
    throwIfError(result.error)
    return result
  })
}
