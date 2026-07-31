import type {
  DownloadCacheStatus,
  DownloadSourceStatus,
  ProxyCredentialStatus,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/service/models'
import {
  CheckSources,
  ClearCache,
  ClearProxyCredential,
  GetCacheStatus,
  GetProxyCredentialStatus,
  SaveProxyCredential,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/downloadservice'
import { throwIfError } from './api-client'

/**
 * 读取代理凭据的保存状态，不返回明文口令。
 * @returns 代理凭据状态
 */
export async function getProxyCredentialStatus(): Promise<ProxyCredentialStatus> {
  const result = await GetProxyCredentialStatus()
  throwIfError(result.error)
  return result.status
}

/**
 * 把代理用户名与口令保存到加密数据库。
 * @param username - 代理用户名
 * @param password - 代理口令
 * @returns 保存后的代理凭据状态
 */
export async function saveProxyCredential(
  username: string,
  password: string,
): Promise<ProxyCredentialStatus> {
  const result = await SaveProxyCredential({ username, password })
  throwIfError(result.error)
  return result.status
}

/**
 * 删除已保存的代理凭据。
 * @returns 删除完成后的 Promise
 */
export async function clearProxyCredential(): Promise<void> {
  const result = await ClearProxyCredential()
  throwIfError(result.error)
}

/**
 * 逐个探测下载源的可用性。
 * @returns 各下载源的探测结果
 */
export async function checkDownloadSources(): Promise<DownloadSourceStatus[]> {
  const result = await CheckSources()
  throwIfError(result.error)
  return result.sources
}

/**
 * 读取本地下载缓存的占用状态。
 * @returns 下载缓存状态
 */
export async function getDownloadCacheStatus(): Promise<DownloadCacheStatus> {
  const result = await GetCacheStatus()
  throwIfError(result.error)
  return result.status
}

/**
 * 清空本地下载缓存。
 * @returns 清理完成后的 Promise
 */
export async function clearDownloadCache(): Promise<void> {
  const result = await ClearCache()
  throwIfError(result.error)
}
