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

export const terminalEventName = 'mineops:terminal:event'

/** Opens a remote xterm-256color PTY through generated Wails bindings. */
export async function openTerminal(
  sshSessionID: string,
  columns: number,
  rows: number,
): Promise<TerminalSession> {
  const result = await Open(sshSessionID, columns, rows)
  throwIfError(result.error)
  if (!result.session) throw new Error('Terminal Session 响应为空')
  return result.session
}

/** Writes ordered terminal input. */
export async function writeTerminal(sessionID: string, data: string): Promise<void> {
  const result = await Input(sessionID, data)
  throwIfError(result.error)
}

/** Applies a remote PTY window-change. */
export async function resizeTerminal(
  sessionID: string,
  columns: number,
  rows: number,
): Promise<void> {
  const result = await Resize(sessionID, columns, rows)
  throwIfError(result.error)
}

/** Closes one remote PTY idempotently. */
export async function closeTerminal(sessionID: string): Promise<void> {
  const result = await Close(sessionID)
  throwIfError(result.error)
}

/** Subscribes to versioned terminal events and returns an unsubscribe function. */
export function subscribeTerminalEvents(listener: (event: TerminalEvent) => void): () => void {
  return Events.On(terminalEventName, (event) => listener(event.data))
}
