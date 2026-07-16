export const accentColours = {
  emerald: '#059669',
  amber: '#d97706',
  azure: '#0284c7',
  violet: '#7c3aed',
  rose: '#e11d48',
} as const

export const themePresetNames = [
  'mineops',
  'forest',
  'ocean',
  'amethyst',
  'graphite',
  'sunset',
] as const

export type AccentName = keyof typeof accentColours
export type ThemePresetName = (typeof themePresetNames)[number]
export type ThemeVariant = 'light' | 'dark'

export const themePresetAccents: Record<ThemePresetName, AccentName> = {
  mineops: 'emerald',
  forest: 'emerald',
  ocean: 'azure',
  amethyst: 'violet',
  graphite: 'azure',
  sunset: 'rose',
}

export interface SemanticThemeTokens {
  surface: {
    canvas: string
    panel: string
    raised: string
    overlay: string
  }
  text: {
    primary: string
    secondary: string
    muted: string
    inverse: string
  }
  border: {
    default: string
    strong: string
    focus: string
  }
  accent: {
    primary: string
    hover: string
    pressed: string
    subtle: string
  }
  status: {
    success: string
    warning: string
    danger: string
    info: string
  }
  terminal: {
    background: string
    foreground: string
    cursor: string
    selection: string
  }
  editor: {
    background: string
    foreground: string
    lineHighlight: string
    selection: string
  }
}

type ThemeBaseTokens = Omit<SemanticThemeTokens, 'accent'>

const themePresets: Record<ThemePresetName, Record<ThemeVariant, ThemeBaseTokens>> = {
  mineops: {
    light: createBaseTokens({
      canvas: '#f4f6f8',
      panel: '#ffffff',
      raised: '#ffffff',
      primary: '#172033',
      secondary: '#475569',
      muted: '#64748b',
      border: '#dce2e8',
      borderStrong: '#aeb8c5',
      terminal: '#f8fafc',
      editor: '#ffffff',
      lineHighlight: '#f1f5f9',
    }),
    dark: createBaseTokens({
      canvas: '#111418',
      panel: '#181c21',
      raised: '#20252b',
      primary: '#edf1f5',
      secondary: '#bdc6d0',
      muted: '#8f9aa7',
      border: '#303740',
      borderStrong: '#4b5563',
      terminal: '#0d1014',
      editor: '#14181d',
      lineHighlight: '#20262d',
    }),
  },
  forest: {
    light: createBaseTokens({
      canvas: '#e4f2e5',
      panel: '#f7fff7',
      raised: '#ffffff',
      primary: '#123c2a',
      secondary: '#37624a',
      muted: '#5f8069',
      border: '#b9d5bd',
      borderStrong: '#75a17e',
      terminal: '#eff9ef',
      editor: '#f7fff7',
      lineHighlight: '#d9eedb',
    }),
    dark: createBaseTokens({
      canvas: '#071c12',
      panel: '#0e2f1c',
      raised: '#174d2c',
      primary: '#edfff0',
      secondary: '#b6d9bd',
      muted: '#7fa889',
      border: '#28603a',
      borderStrong: '#438653',
      terminal: '#04140b',
      editor: '#0b2415',
      lineHighlight: '#174d2c',
    }),
  },
  ocean: {
    light: createBaseTokens({
      canvas: '#e3f2ff',
      panel: '#f7fbff',
      raised: '#ffffff',
      primary: '#0b3154',
      secondary: '#35627d',
      muted: '#5d829c',
      border: '#b8d5ea',
      borderStrong: '#6fa1c7',
      terminal: '#edf8ff',
      editor: '#f7fbff',
      lineHighlight: '#d7ecfb',
    }),
    dark: createBaseTokens({
      canvas: '#06182b',
      panel: '#0b2a4a',
      raised: '#0f3b61',
      primary: '#e7f5ff',
      secondary: '#afd3ec',
      muted: '#72a5ca',
      border: '#1e5984',
      borderStrong: '#3680b5',
      terminal: '#03101f',
      editor: '#08233e',
      lineHighlight: '#0f3b61',
    }),
  },
  amethyst: {
    light: createBaseTokens({
      canvas: '#f1e6ff',
      panel: '#fcf8ff',
      raised: '#ffffff',
      primary: '#3a155e',
      secondary: '#6d438d',
      muted: '#8b66a5',
      border: '#d8bde9',
      borderStrong: '#ab7ac7',
      terminal: '#f8f0ff',
      editor: '#fcf8ff',
      lineHighlight: '#ead8f7',
    }),
    dark: createBaseTokens({
      canvas: '#18082b',
      panel: '#271040',
      raised: '#3b1761',
      primary: '#faedff',
      secondary: '#d9b9f0',
      muted: '#aa7ac7',
      border: '#633092',
      borderStrong: '#8a4fc0',
      terminal: '#10051c',
      editor: '#210b36',
      lineHighlight: '#3b1761',
    }),
  },
  graphite: {
    light: createBaseTokens({
      canvas: '#dff8f5',
      panel: '#f4fffd',
      raised: '#ffffff',
      primary: '#073b3a',
      secondary: '#236b68',
      muted: '#4d8d89',
      border: '#a7ddd8',
      borderStrong: '#57aaa3',
      terminal: '#eafffc',
      editor: '#f4fffd',
      lineHighlight: '#ccefeb',
    }),
    dark: createBaseTokens({
      canvas: '#031f24',
      panel: '#07343a',
      raised: '#0b4b50',
      primary: '#e5fffd',
      secondary: '#a9ddd9',
      muted: '#70aaa7',
      border: '#17666a',
      borderStrong: '#279094',
      terminal: '#02161a',
      editor: '#052a2f',
      lineHighlight: '#0b4b50',
    }),
  },
  sunset: {
    light: createBaseTokens({
      canvas: '#fff0e4',
      panel: '#fffaf5',
      raised: '#ffffff',
      primary: '#4a2118',
      secondary: '#865044',
      muted: '#a76e5b',
      border: '#efc8b8',
      borderStrong: '#d9947b',
      terminal: '#fff5ed',
      editor: '#fffaf5',
      lineHighlight: '#ffe2d1',
    }),
    dark: createBaseTokens({
      canvas: '#28100d',
      panel: '#431a14',
      raised: '#63271b',
      primary: '#fff0e8',
      secondary: '#f0b7a0',
      muted: '#c78169',
      border: '#803727',
      borderStrong: '#b85235',
      terminal: '#1d0908',
      editor: '#35130f',
      lineHighlight: '#63271b',
    }),
  },
}

const accentVariants: Record<AccentName, Record<ThemeVariant, SemanticThemeTokens['accent']>> = {
  emerald: {
    light: { primary: '#059669', hover: '#047857', pressed: '#065f46', subtle: '#d1fae5' },
    dark: { primary: '#34d399', hover: '#6ee7b7', pressed: '#10b981', subtle: '#133b31' },
  },
  amber: {
    light: { primary: '#d97706', hover: '#b45309', pressed: '#92400e', subtle: '#fef3c7' },
    dark: { primary: '#fbbf24', hover: '#fcd34d', pressed: '#f59e0b', subtle: '#453316' },
  },
  azure: {
    light: { primary: '#0284c7', hover: '#0369a1', pressed: '#075985', subtle: '#e0f2fe' },
    dark: { primary: '#38bdf8', hover: '#7dd3fc', pressed: '#0ea5e9', subtle: '#12394a' },
  },
  violet: {
    light: { primary: '#7c3aed', hover: '#6d28d9', pressed: '#5b21b6', subtle: '#ede9fe' },
    dark: { primary: '#a78bfa', hover: '#c4b5fd', pressed: '#8b5cf6', subtle: '#312451' },
  },
  rose: {
    light: { primary: '#e11d48', hover: '#be123c', pressed: '#9f1239', subtle: '#ffe4e6' },
    dark: { primary: '#fb7185', hover: '#fda4af', pressed: '#f43f5e', subtle: '#4a1d2a' },
  },
}

/**
 * 创建当前主题使用的完整语义 Token。
 * @param variant - 浅色或深色主题变体
 * @param accent - 强调色名称
 * @param preset - 完整界面主题套装名称
 * @returns 可同时供 CSS、Naive UI、Monaco 与 xterm 使用的语义 Token
 */
export function createSemanticTokens(
  variant: ThemeVariant,
  accent: AccentName,
  preset: ThemePresetName = 'mineops',
): SemanticThemeTokens {
  const base = themePresets[preset][variant]
  const accentTokens = accentVariants[accent][variant]
  return {
    ...base,
    border: { ...base.border, focus: accentTokens.primary },
    terminal: { ...base.terminal, cursor: accentTokens.primary, selection: accentTokens.subtle },
    editor: { ...base.editor, selection: accentTokens.subtle },
    accent: accentTokens,
  }
}

export const terminalThemes = {
  light: createSemanticTokens('light', 'emerald').terminal,
  dark: createSemanticTokens('dark', 'emerald').terminal,
} as const

interface BaseTokenColours {
  canvas: string
  panel: string
  raised: string
  primary: string
  secondary: string
  muted: string
  border: string
  borderStrong: string
  terminal: string
  editor: string
  lineHighlight: string
}

function createBaseTokens(colours: BaseTokenColours): ThemeBaseTokens {
  const dark = isDarkColour(colours.canvas)
  return {
    surface: {
      canvas: colours.canvas,
      panel: colours.panel,
      raised: colours.raised,
      overlay: dark ? 'rgba(0, 0, 0, 0.68)' : 'rgba(15, 23, 42, 0.48)',
    },
    text: {
      primary: colours.primary,
      secondary: colours.secondary,
      muted: colours.muted,
      inverse: dark ? '#111418' : '#f8fafc',
    },
    border: { default: colours.border, strong: colours.borderStrong, focus: '#059669' },
    status: dark
      ? { success: '#4ade80', warning: '#fbbf24', danger: '#f87171', info: '#38bdf8' }
      : { success: '#15803d', warning: '#b45309', danger: '#b91c1c', info: '#0369a1' },
    terminal: {
      background: colours.terminal,
      foreground: colours.primary,
      cursor: '#059669',
      selection: dark ? '#14532d' : '#d1fae5',
    },
    editor: {
      background: colours.editor,
      foreground: colours.primary,
      lineHighlight: colours.lineHighlight,
      selection: dark ? '#164e3b' : '#d1fae5',
    },
  }
}

function isDarkColour(colour: string): boolean {
  const value = colour.replace('#', '')
  const red = Number.parseInt(value.slice(0, 2), 16)
  const green = Number.parseInt(value.slice(2, 4), 16)
  const blue = Number.parseInt(value.slice(4, 6), 16)
  return red * 0.299 + green * 0.587 + blue * 0.114 < 128
}
