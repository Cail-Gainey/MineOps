import {
  SettingsService,
  type SettingsResult,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services'
import type { SettingsChangedEvent } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/models'
import { SettingsSnapshot } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import { Events } from '@wailsio/runtime'
import { throwIfError } from './api-client'

function unwrapSettings(result: SettingsResult): SettingsSnapshot {
  throwIfError(result.error)
  if (!result.settings) {
    throw new Error('SettingsService 未返回 Settings Snapshot')
  }
  return SettingsSnapshot.createFrom(result.settings)
}

export async function loadSettings(): Promise<SettingsSnapshot> {
  return unwrapSettings(await SettingsService.Get())
}

export async function saveSettings(snapshot: SettingsSnapshot): Promise<SettingsSnapshot> {
  return unwrapSettings(await SettingsService.Save(snapshot))
}

export async function resetSettingsCategory(category: string): Promise<SettingsSnapshot> {
  return unwrapSettings(await SettingsService.ResetCategory(category))
}

export function cloneSettings(snapshot: SettingsSnapshot): SettingsSnapshot {
  return SettingsSnapshot.createFrom(JSON.parse(JSON.stringify(snapshot)) as unknown)
}

const settingsChangedEventName = 'mineops:settings:changed'

export function subscribeSettingsChanged(
  listener: (event: SettingsChangedEvent) => void,
): () => void {
  return Events.On(settingsChangedEventName, (event) => listener(event.data))
}
