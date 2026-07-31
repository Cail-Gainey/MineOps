import type {
  KnownHostDTO,
  ObservedHostKeyInput,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/models'
import {
  Delete,
  List,
  Replace,
  TrustFirst,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/knownhostservice'
import { throwIfError } from './api-client'

/**
 * 按关键字列出已信任的主机密钥。
 * @param search - 搜索关键字，空串表示全部
 * @returns Known Host 数组
 */
export async function listKnownHosts(search = ''): Promise<KnownHostDTO[]> {
  const result = await List(search, 500, 0)
  throwIfError(result.error)
  return result.knownHosts
}

/**
 * 删除一条已信任的主机密钥。
 * @param id - Known Host ID
 * @returns 删除完成后的 Promise
 */
export async function deleteKnownHost(id: string): Promise<void> {
  const result = await Delete(id)
  throwIfError(result.error)
}

/**
 * 首次连接时信任并记录观测到的主机密钥。
 * @param input - 观测到的主机密钥信息
 * @returns 记录后的 Known Host
 */
export async function trustFirstKnownHost(input: ObservedHostKeyInput): Promise<KnownHostDTO> {
  const result = await TrustFirst(input)
  throwIfError(result.error)
  if (!result.knownHost) throw new Error('Known Host 首次信任响应为空')
  return result.knownHost
}

/**
 * 主机密钥变更时替换已记录的指纹。
 * @param input - 观测到的主机密钥信息
 * @returns 替换后的 Known Host
 */
export async function replaceKnownHost(input: ObservedHostKeyInput): Promise<KnownHostDTO> {
  const result = await Replace(input)
  throwIfError(result.error)
  if (!result.knownHost) throw new Error('Known Host 指纹替换响应为空')
  return result.knownHost
}
