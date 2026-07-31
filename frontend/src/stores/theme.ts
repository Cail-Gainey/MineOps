import { defineStore } from 'pinia'
import { darkTheme, type GlobalThemeOverrides } from 'naive-ui'
import { computed, onScopeDispose, ref, watchEffect } from 'vue'

import {
  createSemanticTokens,
  type AccentName,
  type SemanticThemeTokens,
  type ThemePresetName,
} from '../themes/tokens'

export type ThemeMode = 'light' | 'dark' | 'system'
export type BackgroundMode = 'theme' | 'color' | 'image'
export type BackgroundFit = 'cover' | 'contain' | 'center' | 'tile'

export const useThemeStore = defineStore('theme', () => {
  const mode = ref<ThemeMode>('system')
  const preset = ref<ThemePresetName>('mineops')
  const accent = ref<AccentName>('emerald')
  const backgroundMode = ref<BackgroundMode>('theme')
  const backgroundColor = ref('#0f172a')
  const backgroundFit = ref<BackgroundFit>('cover')
  const backgroundOpacity = ref(1)
  const backgroundImageDataURL = ref('')
  const overlayStrength = ref(0.1)
  const blurPixels = ref(0)
  const panelOpacity = ref(0.94)
  const highContrast = ref(false)
  const systemDark = ref(false)
  const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')

  const syncSystemTheme = (event: MediaQueryList | MediaQueryListEvent) => {
    systemDark.value = event.matches
  }

  syncSystemTheme(mediaQuery)
  mediaQuery.addEventListener('change', syncSystemTheme)
  onScopeDispose(() => mediaQuery.removeEventListener('change', syncSystemTheme))

  const isDark = computed(
    () => mode.value === 'dark' || (mode.value === 'system' && systemDark.value),
  )
  const variant = computed(() => (isDark.value ? 'dark' : 'light'))
  const tokens = computed<SemanticThemeTokens>(() =>
    createSemanticTokens(variant.value, accent.value, preset.value),
  )
  const naiveTheme = computed(() => (isDark.value ? darkTheme : null))
  const themeOverrides = computed<GlobalThemeOverrides>(() => {
    const value = tokens.value
    const opacity = clampOpacity(panelOpacity.value)
    const panelColour = withOpacity(value.surface.panel, opacity)
    const raisedColour = withOpacity(value.surface.raised, Math.min(1, opacity + 0.05))
    const borderColour = highContrast.value ? value.border.strong : value.border.default
    return {
      common: {
        bodyColor: 'transparent',
        cardColor: panelColour,
        modalColor: raisedColour,
        popoverColor: raisedColour,
        tableColor: panelColour,
        tableHeaderColor: raisedColour,
        tabColor: panelColour,
        actionColor: raisedColour,
        textColorBase: value.text.primary,
        textColor2: value.text.secondary,
        textColor3: value.text.muted,
        borderColor: borderColour,
        dividerColor: borderColour,
        primaryColor: value.accent.primary,
        primaryColorHover: value.accent.hover,
        primaryColorPressed: value.accent.pressed,
        primaryColorSuppl: value.accent.primary,
        successColor: value.status.success,
        warningColor: value.status.warning,
        errorColor: value.status.danger,
        infoColor: value.status.info,
      },
    }
  })

  watchEffect(() => {
    const root = document.documentElement
    const value = tokens.value
    const opacity = clampOpacity(panelOpacity.value)
    const borderColour = highContrast.value ? value.border.strong : value.border.default
    root.dataset.theme = variant.value
    root.dataset.themePreset = preset.value
    root.style.colorScheme = variant.value
    root.style.setProperty('--surface-canvas', value.surface.canvas)
    root.style.setProperty('--surface-panel', withOpacity(value.surface.panel, opacity))
    root.style.setProperty(
      '--surface-raised',
      withOpacity(value.surface.raised, Math.min(1, opacity + 0.05)),
    )
    root.style.setProperty('--surface-overlay', value.surface.overlay)
    root.style.setProperty('--text-primary', value.text.primary)
    root.style.setProperty('--text-secondary', value.text.secondary)
    root.style.setProperty('--text-muted', value.text.muted)
    root.style.setProperty('--text-inverse', value.text.inverse)
    root.style.setProperty('--border-default', borderColour)
    root.style.setProperty('--border-strong', value.border.strong)
    root.style.setProperty(
      '--border-focus',
      highContrast.value ? value.accent.primary : value.border.focus,
    )
    root.style.setProperty('--accent-primary', value.accent.primary)
    root.style.setProperty('--accent-subtle', value.accent.subtle)
    root.style.setProperty('--status-success', value.status.success)
    root.style.setProperty('--status-warning', value.status.warning)
    root.style.setProperty('--status-danger', value.status.danger)
    root.style.setProperty('--status-info', value.status.info)
    root.style.setProperty('--terminal-background', value.terminal.background)
    root.style.setProperty('--editor-background', value.editor.background)
    const chromeBackground = mixHexColours(value.surface.panel, value.surface.canvas, 0.42)
    const sidebarBackground = mixHexColours(value.surface.raised, value.surface.canvas, 0.68)
    root.style.setProperty(
      '--chrome-background',
      withOpacity(chromeBackground, Math.min(1, opacity + 0.05)),
    )
    root.style.setProperty(
      '--chrome-sidebar-background',
      withOpacity(sidebarBackground, Math.min(1, opacity + 0.08)),
    )
    root.style.setProperty('--chrome-text', value.text.primary)
    root.style.setProperty('--chrome-muted', value.text.muted)
    root.style.setProperty('--chrome-hover', value.accent.subtle)
    root.style.setProperty('--chrome-active', value.accent.primary)
    root.style.setProperty(
      '--chrome-border',
      highContrast.value
        ? value.border.strong
        : withOpacity(value.border.strong, isDark.value ? 0.62 : 0.7),
    )
    root.style.setProperty(
      '--app-background-color',
      backgroundMode.value === 'color' ? backgroundColor.value : value.surface.canvas,
    )
    root.style.setProperty(
      '--app-background-image',
      backgroundMode.value === 'image' && backgroundImageDataURL.value
        ? `url("${backgroundImageDataURL.value}")`
        : 'none',
    )
    root.style.setProperty(
      '--app-background-size',
      backgroundFit.value === 'tile' || backgroundFit.value === 'center'
        ? 'auto'
        : backgroundFit.value,
    )
    root.style.setProperty('--app-background-position', 'center')
    root.style.setProperty(
      '--app-background-repeat',
      backgroundFit.value === 'tile' ? 'repeat' : 'no-repeat',
    )
    root.style.setProperty(
      '--app-background-opacity',
      String(backgroundMode.value === 'theme' ? 1 : backgroundOpacity.value),
    )
    root.style.setProperty(
      '--app-background-overlay',
      backgroundMode.value === 'image'
        ? createBackgroundOverlay(overlayStrength.value)
        : 'transparent',
    )
    root.style.setProperty('--app-background-blur', `${blurPixels.value}px`)
    root.style.setProperty(
      '--app-background-scale',
      backgroundFit.value === 'tile' ? '1' : blurPixels.value > 0 ? '1.06' : '1.02',
    )
    root.dataset.contrast = highContrast.value ? 'high' : 'normal'
  })

  return {
    accent,
    backgroundColor,
    backgroundFit,
    backgroundImageDataURL,
    backgroundMode,
    backgroundOpacity,
    blurPixels,
    highContrast,
    isDark,
    mode,
    naiveTheme,
    overlayStrength,
    panelOpacity,
    preset,
    themeOverrides,
    tokens,
    variant,
  }
})

/**
 * 把十六进制颜色转换成带透明度的 rgba 字符串。
 * @param colour - 十六进制颜色
 * @param opacity - 不透明度 0 到 1
 * @returns rgba 颜色字符串
 */
function withOpacity(colour: string, opacity: number): string {
  const value = colour.replace('#', '')
  if (value.length !== 6) return colour
  const red = Number.parseInt(value.slice(0, 2), 16)
  const green = Number.parseInt(value.slice(2, 4), 16)
  const blue = Number.parseInt(value.slice(4, 6), 16)
  return `rgba(${red}, ${green}, ${blue}, ${opacity})`
}

/**
 * 把不透明度约束到 0 到 1 之间。
 * @param opacity - 原始不透明度
 * @returns 约束后的不透明度
 */
function clampOpacity(opacity: number): number {
  return Math.min(1, Math.max(0, opacity))
}

/**
 * 按遮罩强度生成背景蒙层的渐变值。
 * @param strength - 遮罩强度 0 到 1
 * @returns CSS 渐变字符串
 */
function createBackgroundOverlay(strength: number): string {
  const strong = Math.min(1, strength + 0.12)
  const soft = Math.max(0, strength * 0.72)
  return `linear-gradient(135deg, ${withOpacity('#000000', strong)}, ${withOpacity('#000000', soft)})`
}

/**
 * 按权重混合两个十六进制颜色。
 * @param first - 第一个颜色
 * @param second - 第二个颜色
 * @param secondWeight - 第二个颜色的权重 0 到 1
 * @returns 混合后的十六进制颜色
 */
function mixHexColours(first: string, second: string, secondWeight: number): string {
  const firstValue = first.replace('#', '')
  const secondValue = second.replace('#', '')
  if (firstValue.length !== 6 || secondValue.length !== 6) return first
  const weight = Math.min(1, Math.max(0, secondWeight))
  const channels = [0, 2, 4].map((offset) => {
    const firstChannel = Number.parseInt(firstValue.slice(offset, offset + 2), 16)
    const secondChannel = Number.parseInt(secondValue.slice(offset, offset + 2), 16)
    return Math.round(firstChannel * (1 - weight) + secondChannel * weight)
      .toString(16)
      .padStart(2, '0')
  })
  return `#${channels.join('')}`
}
