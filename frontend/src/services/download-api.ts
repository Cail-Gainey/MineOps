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

/** Returns non-secret proxy credential state. */
export async function getProxyCredentialStatus(): Promise<ProxyCredentialStatus> {
  const result = await GetProxyCredentialStatus()
  throwIfError(result.error)
  return result.status
}

/** Saves write-only proxy credentials to SQLCipher and returns non-secret state. */
export async function saveProxyCredential(
  username: string,
  password: string,
): Promise<ProxyCredentialStatus> {
  const result = await SaveProxyCredential({ username, password })
  throwIfError(result.error)
  return result.status
}

/** Removes the encrypted proxy credential and Settings reference. */
export async function clearProxyCredential(): Promise<void> {
  const result = await ClearProxyCredential()
  throwIfError(result.error)
}

/** Checks all enabled download sources through current proxy settings. */
export async function checkDownloadSources(): Promise<DownloadSourceStatus[]> {
  const result = await CheckSources()
  throwIfError(result.error)
  return result.sources
}

/** Returns local artifact cache usage. */
export async function getDownloadCacheStatus(): Promise<DownloadCacheStatus> {
  const result = await GetCacheStatus()
  throwIfError(result.error)
  return result.status
}

/** Clears local artifact cache children. */
export async function clearDownloadCache(): Promise<void> {
  const result = await ClearCache()
  throwIfError(result.error)
}
