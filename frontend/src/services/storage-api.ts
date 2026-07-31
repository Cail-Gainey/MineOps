import {
  StorageService,
  type StorageResult,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services'
import type { BackupInfo } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/infrastructure/sqlcipher/models'
import type { StorageStatus } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/service/models'
import { throwIfError } from './api-client'

/**
 * 抛出 StorageService 返回的错误，并把状态与备份信息统一成可空结果。
 * @param result - StorageService 的原始返回值
 * @returns 归一化后的存储状态与备份信息
 */
function unwrapStorage(result: StorageResult): {
  status: StorageStatus | null
  backup: BackupInfo | null
} {
  throwIfError(result.error)
  return { status: result.status ?? null, backup: result.backup ?? null }
}

/**
 * 读取当前数据库路径、体积、加密状态与已排队的离线维护任务。
 * @returns 当前存储状态
 */
export async function getStorageStatus(): Promise<StorageStatus> {
  const result = unwrapStorage(await StorageService.GetStatus())
  if (!result.status) throw new Error('StorageService 未返回存储状态')
  return result.status
}

/**
 * 在线创建一份一致的加密便携备份。
 * @param destination - 备份文件的保存路径
 * @returns 新建备份的元信息
 */
export async function createPortableBackup(destination: string): Promise<BackupInfo> {
  const result = unwrapStorage(await StorageService.CreateBackup(destination))
  if (!result.backup) throw new Error('StorageService 未返回备份信息')
  return result.backup
}

/**
 * 校验便携备份并排队到下次启动安装。
 * @param path - 待恢复的备份文件路径
 * @returns 排队后的存储状态
 */
export async function schedulePortableRestore(path: string): Promise<StorageStatus> {
  const result = unwrapStorage(await StorageService.ScheduleRestore(path))
  if (!result.status) throw new Error('StorageService 未返回恢复排队状态')
  return result.status
}

/**
 * 排队一次下次启动执行的 SQLCipher 密钥轮换。
 * @returns 排队后的存储状态
 */
export async function scheduleDatabaseKeyRotation(): Promise<StorageStatus> {
  const result = unwrapStorage(await StorageService.ScheduleKeyRotation())
  if (!result.status) throw new Error('StorageService 未返回密钥轮换状态')
  return result.status
}

/**
 * 取消全部已排队的离线维护任务。
 * @returns 取消后的存储状态
 */
export async function cancelPendingDatabaseMaintenance(): Promise<StorageStatus> {
  const result = unwrapStorage(await StorageService.CancelPendingMaintenance())
  if (!result.status) throw new Error('StorageService 未返回维护状态')
  return result.status
}

/**
 * 排队一次下次启动执行的 VACUUM，释放 SQLite 空闲页占用的文件空间。
 * @returns 排队后的存储状态
 */
export async function scheduleDatabaseVacuum(): Promise<StorageStatus> {
  const result = unwrapStorage(await StorageService.ScheduleVacuum())
  if (!result.status) throw new Error('StorageService 未返回存储整理状态')
  return result.status
}

/**
 * 排队一次下次启动执行的清空数据库：删除两个库文件与系统密钥，重建空库。
 * @returns 排队后的存储状态
 */
export async function scheduleDatabaseReset(): Promise<StorageStatus> {
  const result = unwrapStorage(await StorageService.ScheduleDatabaseReset())
  if (!result.status) throw new Error('StorageService 未返回清空数据库状态')
  return result.status
}

/**
 * 用系统文件管理器打开受控数据目录。
 * @returns 打开动作完成后的 Promise
 */
export async function openDataDirectory(): Promise<void> {
  throwIfError((await StorageService.OpenDataDirectory()).error)
}
