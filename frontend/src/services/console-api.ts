import { Events } from '@wailsio/runtime'

import type { ConsoleEvent } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/models'
import type { ConsoleSession } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import {
  Close,
  Detach,
  Input,
  Open,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/consoleservice'
import { throwIfError } from './api-client'
import { translate } from '../locales/runtime'

export const consoleEventName = 'mineops:console:event'

/**
 * 打开某台 Server 的控制台会话并从指定偏移开始回放。
 * @param serverID - 目标 Server ID
 * @param offset - 日志回放起始偏移
 * @returns 控制台会话
 */
export async function openServerConsole(serverID: string, offset = 0): Promise<ConsoleSession> {
  const result = await Open(serverID, offset)
  throwIfError(result.error)
  if (!result.session)
    throw new Error(
      translate('service.missingField', {
        service: 'ConsoleService',
        field: 'Console Session',
      }),
    )
  return result.session
}

/**
 * 向控制台会话写入一段输入。
 * @param sessionID - 控制台会话 ID
 * @param data - 待写入的数据
 * @returns 写入完成后的 Promise
 */
export async function writeServerConsole(sessionID: string, data: string): Promise<void> {
  const result = await Input(sessionID, data)
  throwIfError(result.error)
}

/**
 * 从控制台会话分离，但保留远端会话继续运行。
 * @param sessionID - 控制台会话 ID
 * @returns 分离完成后的 Promise
 */
export async function detachServerConsole(sessionID: string): Promise<void> {
  const result = await Detach(sessionID)
  throwIfError(result.error)
}

/**
 * 关闭控制台会话。
 * @param sessionID - 控制台会话 ID
 * @returns 关闭完成后的 Promise
 */
export async function closeServerConsole(sessionID: string): Promise<void> {
  const result = await Close(sessionID)
  throwIfError(result.error)
}

/**
 * 订阅控制台输出事件。
 * @param listener - 收到事件时的回调
 * @returns 取消订阅函数
 */
export function subscribeConsoleEvents(listener: (event: ConsoleEvent) => void): () => void {
  return Events.On(consoleEventName, (event) => listener(event.data))
}
