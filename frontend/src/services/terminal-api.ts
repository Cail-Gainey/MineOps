import { Events } from '@wailsio/runtime'

import type { TerminalEvent } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/models'
import type { TerminalSession } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import {
  Close,
  Input,
  Open,
  Resize,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/terminalservice'
import { throwIfError } from './api-client'
import { translate } from '../locales/runtime'

export const terminalEventName = 'mineops:terminal:event'

/**
 * 按指定终端尺寸打开一个 SSH 终端会话。
 * @param sshSessionID - SSH Session ID
 * @param columns - 终端列数
 * @param rows - 终端行数
 * @returns 终端会话
 */
export async function openTerminal(
  sshSessionID: string,
  columns: number,
  rows: number,
): Promise<TerminalSession> {
  const result = await Open(sshSessionID, columns, rows)
  throwIfError(result.error)
  if (!result.session)
    throw new Error(translate('service.emptyResponse', { subject: 'Terminal Session' }))
  return result.session
}

/**
 * 向终端会话写入一段输入。
 * @param sessionID - 终端会话 ID
 * @param data - 待写入的数据
 * @returns 写入完成后的 Promise
 */
export async function writeTerminal(sessionID: string, data: string): Promise<void> {
  const result = await Input(sessionID, data)
  throwIfError(result.error)
}

/**
 * 调整终端会话的尺寸。
 * @param sessionID - 终端会话 ID
 * @param columns - 终端列数
 * @param rows - 终端行数
 * @returns 调整完成后的 Promise
 */
export async function resizeTerminal(
  sessionID: string,
  columns: number,
  rows: number,
): Promise<void> {
  const result = await Resize(sessionID, columns, rows)
  throwIfError(result.error)
}

/**
 * 关闭终端会话。
 * @param sessionID - 终端会话 ID
 * @returns 关闭完成后的 Promise
 */
export async function closeTerminal(sessionID: string): Promise<void> {
  const result = await Close(sessionID)
  throwIfError(result.error)
}

/**
 * 订阅终端输出事件。
 * @param listener - 收到事件时的回调
 * @returns 取消订阅函数
 */
export function subscribeTerminalEvents(listener: (event: TerminalEvent) => void): () => void {
  return Events.On(terminalEventName, (event) => listener(event.data))
}
