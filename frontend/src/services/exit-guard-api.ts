import { Events } from '@wailsio/runtime'

import type { QuitBlockedEvent } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/models'
import type { UnsavedItem } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/service/models'
import {
  ConfirmQuit,
  SetDirty,
  State,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/exitguardservice'
import { throwIfError } from './api-client'

export const quitBlockedEventName = 'mineops:lifecycle:quit-blocked'

/**
 * 登记或清除某个模块的未保存状态，用于退出前拦截。
 * @param owner - 模块标识
 * @param label - 展示给用户的名称
 * @param dirty - 是否存在未保存修改
 * @returns 登记完成后的 Promise
 */
export async function setUnsavedState(owner: string, label: string, dirty: boolean): Promise<void> {
  const result = await SetDirty(owner, label, dirty)
  throwIfError(result.error)
}

/**
 * 确认放弃未保存内容并继续退出应用。
 * @returns 确认完成后的 Promise
 */
export async function confirmApplicationQuit(): Promise<void> {
  const result = await ConfirmQuit()
  throwIfError(result.error)
}

/**
 * 读取当前全部未保存项。
 * @returns 未保存项数组
 */
export async function getUnsavedItems(): Promise<UnsavedItem[]> {
  const result = await State()
  throwIfError(result.error)
  return result.items
}

/**
 * 订阅退出被未保存内容拦截的事件。
 * @param listener - 收到事件时的回调
 * @returns 取消订阅函数
 */
export function subscribeQuitBlocked(listener: (event: QuitBlockedEvent) => void): () => void {
  return Events.On(quitBlockedEventName, (event) => listener(event.data))
}
