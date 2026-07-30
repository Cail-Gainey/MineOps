import {
  StorageService,
  type StorageResult,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services'
import type { BackupInfo } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/infrastructure/sqlcipher/models'
import type { StorageStatus } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/service/models'
import { throwIfError } from './api-client'

function unwrapStorage(result: StorageResult): {
  status: StorageStatus | null
  backup: BackupInfo | null
} {
  throwIfError(result.error)
  return { status: result.status ?? null, backup: result.backup ?? null }
}

export async function getStorageStatus(): Promise<StorageStatus> {
  const result = unwrapStorage(await StorageService.GetStatus())
  if (!result.status) throw new Error('StorageService 未返回存储状态')
  return result.status
}

export async function createPortableBackup(destination: string): Promise<BackupInfo> {
  const result = unwrapStorage(await StorageService.CreateBackup(destination))
  if (!result.backup) throw new Error('StorageService 未返回备份信息')
  return result.backup
}

export async function schedulePortableRestore(path: string): Promise<StorageStatus> {
  const result = unwrapStorage(await StorageService.ScheduleRestore(path))
  if (!result.status) throw new Error('StorageService 未返回恢复排队状态')
  return result.status
}

export async function scheduleDatabaseKeyRotation(): Promise<StorageStatus> {
  const result = unwrapStorage(await StorageService.ScheduleKeyRotation())
  if (!result.status) throw new Error('StorageService 未返回密钥轮换状态')
  return result.status
}

export async function cancelPendingDatabaseMaintenance(): Promise<StorageStatus> {
  const result = unwrapStorage(await StorageService.CancelPendingMaintenance())
  if (!result.status) throw new Error('StorageService 未返回维护状态')
  return result.status
}

/** 排队一次下次启动执行的 VACUUM，释放 SQLite 空闲页占用的文件空间。 */
export async function scheduleDatabaseVacuum(): Promise<StorageStatus> {
  const result = unwrapStorage(await StorageService.ScheduleVacuum())
  if (!result.status) throw new Error('StorageService 未返回存储整理状态')
  return result.status
}

export async function openDataDirectory(): Promise<void> {
  throwIfError((await StorageService.OpenDataDirectory()).error)
}
