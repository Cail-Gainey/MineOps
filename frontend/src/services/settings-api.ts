import {
  SettingsService,
  type SettingsResult,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services'
import type { SettingsChangedEvent } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/models'
import { SettingsSnapshot } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import { Events } from '@wailsio/runtime'
import { throwIfError } from './api-client'

/**
 * 抛出设置接口返回的错误并取出快照本体。
 * @param result - SettingsService 的原始返回值
 * @returns 设置快照
 */
function unwrapSettings(result: SettingsResult): SettingsSnapshot {
  throwIfError(result.error)
  if (!result.settings) {
    throw new Error('SettingsService 未返回 Settings Snapshot')
  }
  return SettingsSnapshot.createFrom(result.settings)
}

/**
 * 加载完整的设置快照。
 * @returns 设置快照
 */
export async function loadSettings(): Promise<SettingsSnapshot> {
  return unwrapSettings(await SettingsService.Get())
}

/**
 * 保存完整的设置快照。
 * @param snapshot - 待保存的设置快照
 * @returns 保存后的设置快照
 */
export async function saveSettings(snapshot: SettingsSnapshot): Promise<SettingsSnapshot> {
  return unwrapSettings(await SettingsService.Save(snapshot))
}

/**
 * 把某个设置分类恢复为内置默认值。
 * @param category - 设置分类标识
 * @returns 重置后的设置快照
 */
export async function resetSettingsCategory(category: string): Promise<SettingsSnapshot> {
  return unwrapSettings(await SettingsService.ResetCategory(category))
}

/**
 * 深拷贝一份设置快照，用于草稿与已提交值的隔离。
 * @param snapshot - 源设置快照
 * @returns 与源无共享引用的副本
 */
export function cloneSettings(snapshot: SettingsSnapshot): SettingsSnapshot {
  return SettingsSnapshot.createFrom(JSON.parse(JSON.stringify(snapshot)) as unknown)
}

const settingsChangedEventName = 'mineops:settings:changed'

/**
 * 订阅设置变更事件。
 * @param listener - 收到事件时的回调
 * @returns 取消订阅函数
 */
export function subscribeSettingsChanged(
  listener: (event: SettingsChangedEvent) => void,
): () => void {
  return Events.On(settingsChangedEventName, (event) => listener(event.data))
}
