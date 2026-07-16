import {
  BackgroundService,
  type BackgroundResourceResult,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services'
import type { BackgroundResource } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/service/models'
import { SettingsSnapshot } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import { throwIfError } from './api-client'

export interface BackgroundOperationResult {
  resource: BackgroundResource
  settings: SettingsSnapshot | null
}

function unwrapBackground(result: BackgroundResourceResult): BackgroundOperationResult {
  throwIfError(result.error)
  if (!result.resource) throw new Error('BackgroundService 未返回资源状态')
  return {
    resource: result.resource,
    settings: result.settings ? SettingsSnapshot.createFrom(result.settings) : null,
  }
}

/** 获取当前受控背景资源及可用于预览的 Data URL。 */
export async function getBackgroundResource(): Promise<BackgroundOperationResult> {
  return unwrapBackground(await BackgroundService.Get())
}

/** 将本地图片复制到受控数据目录并立即选为应用背景。 */
export async function importBackgroundResource(path: string): Promise<BackgroundOperationResult> {
  return unwrapBackground(await BackgroundService.Import(path))
}

/** 清除当前背景图片并恢复语义主题背景。 */
export async function resetBackgroundResource(): Promise<BackgroundOperationResult> {
  return unwrapBackground(await BackgroundService.Reset())
}
