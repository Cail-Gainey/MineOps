import {
  BackgroundService,
  type BackgroundResourceResult,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services'
import type { BackgroundResource } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/service/models'
import { SettingsSnapshot } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import { throwIfError } from './api-client'
import { translate } from '../locales/runtime'

export interface BackgroundOperationResult {
  resource: BackgroundResource
  settings: SettingsSnapshot | null
}

/**
 * 抛出背景资源接口返回的错误并取出资源本体。
 * @param result - BackgroundService 的原始返回值
 * @returns 背景资源操作结果
 */
function unwrapBackground(result: BackgroundResourceResult): BackgroundOperationResult {
  throwIfError(result.error)
  if (!result.resource)
    throw new Error(
      translate('service.missingField', {
        service: 'BackgroundService',
        field: translate('serviceField.backgroundResource'),
      }),
    )
  return {
    resource: result.resource,
    settings: result.settings ? SettingsSnapshot.createFrom(result.settings) : null,
  }
}

/**
 * 读取当前受控背景资源，含可用于预览的 Data URL。
 * @returns 背景资源操作结果
 */
export async function getBackgroundResource(): Promise<BackgroundOperationResult> {
  return unwrapBackground(await BackgroundService.Get())
}

/**
 * 把本地图片复制到受控数据目录并立即选为应用背景。
 * @param path - 本地图片路径
 * @returns 背景资源操作结果
 */
export async function importBackgroundResource(path: string): Promise<BackgroundOperationResult> {
  return unwrapBackground(await BackgroundService.Import(path))
}

/**
 * 清除当前背景图片并恢复语义主题背景。
 * @returns 背景资源操作结果
 */
export async function resetBackgroundResource(): Promise<BackgroundOperationResult> {
  return unwrapBackground(await BackgroundService.Reset())
}
