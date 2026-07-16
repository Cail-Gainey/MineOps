import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import type { SettingsSnapshot } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import {
  cloneSettings,
  loadSettings,
  resetSettingsCategory,
  saveSettings,
  subscribeSettingsChanged,
} from '../services/settings-api'
import { getBackgroundResource } from '../services/background-api'
import type { AccentName, ThemePresetName } from '../themes/tokens'
import { type ThemeMode, useThemeStore } from './theme'
import { useLayoutStore } from './layout'
import { useLocaleStore } from './locale'

export interface ThemeSelectionUpdate {
  mode?: ThemeMode
  preset?: ThemePresetName
  accent?: AccentName
}

export const useSettingsStore = defineStore('settings', () => {
  const committed = ref<SettingsSnapshot | null>(null)
  const draft = ref<SettingsSnapshot | null>(null)
  const loading = ref(false)
  const saving = ref(false)
  const error = ref<unknown>(null)
  let unsubscribe: (() => void) | null = null
  let loadPromise: Promise<void> | null = null
  let pendingThemeSelection: ThemeSelectionUpdate | null = null
  let settingsMutationQueue: Promise<void> = Promise.resolve()
  const dirty = computed(
    () =>
      committed.value !== null &&
      draft.value !== null &&
      JSON.stringify(committed.value) !== JSON.stringify(draft.value),
  )

  function applyTheme(snapshot: SettingsSnapshot): void {
    const themeStore = useThemeStore()
    if (
      snapshot.theme.mode === 'light' ||
      snapshot.theme.mode === 'dark' ||
      snapshot.theme.mode === 'system'
    ) {
      themeStore.mode = snapshot.theme.mode
    }
    if (
      snapshot.theme.preset === 'mineops' ||
      snapshot.theme.preset === 'forest' ||
      snapshot.theme.preset === 'ocean' ||
      snapshot.theme.preset === 'amethyst' ||
      snapshot.theme.preset === 'graphite' ||
      snapshot.theme.preset === 'sunset'
    ) {
      themeStore.preset = snapshot.theme.preset
    }
    if (
      snapshot.theme.accent === 'emerald' ||
      snapshot.theme.accent === 'amber' ||
      snapshot.theme.accent === 'azure' ||
      snapshot.theme.accent === 'violet' ||
      snapshot.theme.accent === 'rose'
    ) {
      themeStore.accent = snapshot.theme.accent
    }
    if (
      snapshot.theme.backgroundMode === 'theme' ||
      snapshot.theme.backgroundMode === 'color' ||
      snapshot.theme.backgroundMode === 'image'
    )
      themeStore.backgroundMode = snapshot.theme.backgroundMode
    if (
      snapshot.theme.backgroundFit === 'cover' ||
      snapshot.theme.backgroundFit === 'contain' ||
      snapshot.theme.backgroundFit === 'center' ||
      snapshot.theme.backgroundFit === 'tile'
    )
      themeStore.backgroundFit = snapshot.theme.backgroundFit
    themeStore.backgroundColor = snapshot.theme.backgroundColor
    themeStore.backgroundOpacity = snapshot.theme.backgroundOpacity
    themeStore.overlayStrength = snapshot.theme.overlayStrength
    themeStore.blurPixels = snapshot.theme.blurPixels
    themeStore.panelOpacity = snapshot.theme.panelOpacity
    themeStore.highContrast = snapshot.theme.highContrast
  }

  async function applyBackgroundResource(): Promise<void> {
    const themeStore = useThemeStore()
    try {
      const result = await getBackgroundResource()
      themeStore.backgroundImageDataURL = result.resource.available
        ? (result.resource.dataURL ?? '')
        : ''
    } catch {
      themeStore.backgroundImageDataURL = ''
    }
  }

  function applyLayout(snapshot: SettingsSnapshot): void {
    const layoutStore = useLayoutStore()
    layoutStore.sidebarCollapsed = snapshot.layout.sidebarCollapsed
    layoutStore.sidebarVisible = snapshot.layout.sidebarVisible
    layoutStore.sidebarWidth = snapshot.layout.sidebarWidth
    layoutStore.topBarVisible = snapshot.layout.topBarVisible
    layoutStore.bottomBarVisible = snapshot.layout.bottomBarVisible
  }

  function applyLocale(snapshot: SettingsSnapshot): void {
    const localeStore = useLocaleStore()
    localeStore.setLocale(snapshot.general.language)
    localeStore.setTimeFormat(snapshot.general.timeFormat)
  }

  async function load(): Promise<void> {
    if (loadPromise) return loadPromise
    loadPromise = (async () => {
      loading.value = true
      error.value = null
      try {
        const snapshot = await loadSettings()
        committed.value = snapshot
        draft.value = cloneSettings(snapshot)
        applyTheme(snapshot)
        applyLayout(snapshot)
        applyLocale(snapshot)
        await applyBackgroundResource()
      } catch (reason) {
        error.value = reason
        throw reason
      } finally {
        loading.value = false
      }
    })()
    try {
      await loadPromise
    } finally {
      loadPromise = null
    }
  }

  function enqueueSettingsMutation(operation: () => Promise<void>): Promise<void> {
    const queued = settingsMutationQueue.catch(() => undefined).then(operation)
    settingsMutationQueue = queued
    return queued
  }

  function save(): Promise<void> {
    if (!draft.value) return Promise.resolve()
    return enqueueSettingsMutation(async () => {
      if (!draft.value) return
      saving.value = true
      error.value = null
      try {
        const snapshot = await saveSettings(draft.value)
        committed.value = snapshot
        draft.value = cloneSettings(snapshot)
        applyTheme(snapshot)
        applyLayout(snapshot)
        applyLocale(snapshot)
        await applyBackgroundResource()
      } catch (reason) {
        error.value = reason
        throw reason
      } finally {
        saving.value = false
      }
    })
  }

  function resetCategory(category: string): Promise<void> {
    return enqueueSettingsMutation(async () => {
      saving.value = true
      error.value = null
      try {
        const snapshot = await resetSettingsCategory(category)
        committed.value = snapshot
        draft.value = cloneSettings(snapshot)
        applyTheme(snapshot)
        applyLayout(snapshot)
        applyLocale(snapshot)
        await applyBackgroundResource()
      } catch (reason) {
        error.value = reason
        throw reason
      } finally {
        saving.value = false
      }
    })
  }

  /**
   * 保存顶栏选择的主题，同时保留设置页其他未提交内容。
   * @param selection - 本次变更的主题模式、套装或强调色
   * @returns 主题设置持久化完成后的 Promise
   */
  function saveThemeSelection(selection: ThemeSelectionUpdate): Promise<void> {
    const themeStore = useThemeStore()
    pendingThemeSelection = { ...pendingThemeSelection, ...selection }
    if (selection.mode !== undefined) themeStore.mode = selection.mode
    if (selection.preset !== undefined) themeStore.preset = selection.preset
    if (selection.accent !== undefined) themeStore.accent = selection.accent
    if (draft.value) {
      if (selection.mode !== undefined) draft.value.theme.mode = selection.mode
      if (selection.preset !== undefined) draft.value.theme.preset = selection.preset
      if (selection.accent !== undefined) draft.value.theme.accent = selection.accent
    }

    return enqueueSettingsMutation(async () => {
      saving.value = true
      error.value = null
      try {
        if (!committed.value) await load()
        if (!committed.value) throw new Error('Settings Snapshot 尚未加载')

        const currentSelection = { ...pendingThemeSelection }
        if (
          currentSelection.mode === undefined &&
          currentSelection.preset === undefined &&
          currentSelection.accent === undefined
        ) {
          return
        }
        const snapshot = cloneSettings(committed.value)
        if (currentSelection.mode !== undefined) {
          snapshot.theme.mode = currentSelection.mode
          themeStore.mode = currentSelection.mode
        }
        if (currentSelection.preset !== undefined) {
          snapshot.theme.preset = currentSelection.preset
          themeStore.preset = currentSelection.preset
        }
        if (currentSelection.accent !== undefined) {
          snapshot.theme.accent = currentSelection.accent
          themeStore.accent = currentSelection.accent
        }
        if (draft.value) {
          if (currentSelection.mode !== undefined) {
            draft.value.theme.mode = currentSelection.mode
          }
          if (currentSelection.preset !== undefined) {
            draft.value.theme.preset = currentSelection.preset
          }
          if (currentSelection.accent !== undefined) {
            draft.value.theme.accent = currentSelection.accent
          }
        }

        committed.value = await saveSettings(snapshot)
        if (pendingThemeSelection) {
          if (pendingThemeSelection.mode === currentSelection.mode) {
            delete pendingThemeSelection.mode
          }
          if (pendingThemeSelection.preset === currentSelection.preset) {
            delete pendingThemeSelection.preset
          }
          if (pendingThemeSelection.accent === currentSelection.accent) {
            delete pendingThemeSelection.accent
          }
          if (
            pendingThemeSelection.mode === undefined &&
            pendingThemeSelection.preset === undefined &&
            pendingThemeSelection.accent === undefined
          ) {
            pendingThemeSelection = null
          }
        }
      } catch (reason) {
        error.value = reason
        throw reason
      } finally {
        saving.value = false
      }
    })
  }

  function discard(): void {
    if (committed.value) {
      draft.value = cloneSettings(committed.value)
      applyTheme(committed.value)
      applyLayout(committed.value)
      applyLocale(committed.value)
    }
  }

  function connect(): void {
    if (unsubscribe) return
    unsubscribe = subscribeSettingsChanged(() => {
      if (!saving.value && !dirty.value) void load()
    })
  }

  return {
    committed,
    connect,
    dirty,
    discard,
    draft,
    error,
    load,
    loading,
    previewTheme: applyTheme,
    resetCategory,
    save,
    saveThemeSelection,
    saving,
  }
})
