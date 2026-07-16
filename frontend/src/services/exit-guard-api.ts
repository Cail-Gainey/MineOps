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

/** Registers or clears one editor/form unsaved-content state. */
export async function setUnsavedState(owner: string, label: string, dirty: boolean): Promise<void> {
  const result = await SetDirty(owner, label, dirty)
  throwIfError(result.error)
}

/** Authorizes exactly one native quit request after the user confirms data loss. */
export async function confirmApplicationQuit(): Promise<void> {
  const result = await ConfirmQuit()
  throwIfError(result.error)
}

/** Returns the current unsaved-content registry without starting a quit request. */
export async function getUnsavedItems(): Promise<UnsavedItem[]> {
  const result = await State()
  throwIfError(result.error)
  return result.items
}

/** Subscribes to native close attempts blocked by unsaved content. */
export function subscribeQuitBlocked(listener: (event: QuitBlockedEvent) => void): () => void {
  return Events.On(quitBlockedEventName, (event) => listener(event.data))
}
