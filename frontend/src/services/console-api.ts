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

export const consoleEventName = 'mineops:console:event'

/** Opens one live attachment to the running Server tmux session. */
export async function openServerConsole(serverID: string, offset = 0): Promise<ConsoleSession> {
  const result = await Open(serverID, offset)
  throwIfError(result.error)
  if (!result.session) throw new Error('ConsoleService 未返回 Console Session')
  return result.session
}

/** Sends command input to the attached Server tmux session. */
export async function writeServerConsole(sessionID: string, data: string): Promise<void> {
  const result = await Input(sessionID, data)
  throwIfError(result.error)
}

/** Detaches the view while keeping the remote Server running. */
export async function detachServerConsole(sessionID: string): Promise<void> {
  const result = await Detach(sessionID)
  throwIfError(result.error)
}

/** Closes the desktop Console attachment without stopping the Server. */
export async function closeServerConsole(sessionID: string): Promise<void> {
  const result = await Close(sessionID)
  throwIfError(result.error)
}

/** Subscribes to bounded versioned live Console output events. */
export function subscribeConsoleEvents(listener: (event: ConsoleEvent) => void): () => void {
  return Events.On(consoleEventName, (event) => listener(event.data))
}
