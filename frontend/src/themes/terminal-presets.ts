import type { ITheme } from '@xterm/xterm'

import type { TerminalSettings } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import type { SemanticThemeTokens } from './tokens'

const nord: ITheme = {
  foreground: '#d8dee9',
  background: '#2e3440',
  cursor: '#88c0d0',
  selectionBackground: '#4c566a',
  black: '#3b4252',
  red: '#bf616a',
  green: '#a3be8c',
  yellow: '#ebcb8b',
  blue: '#81a1c1',
  magenta: '#b48ead',
  cyan: '#88c0d0',
  white: '#e5e9f0',
  brightBlack: '#4c566a',
  brightRed: '#bf616a',
  brightGreen: '#a3be8c',
  brightYellow: '#ebcb8b',
  brightBlue: '#81a1c1',
  brightMagenta: '#b48ead',
  brightCyan: '#8fbcbb',
  brightWhite: '#eceff4',
}

const solarized: ITheme = {
  foreground: '#839496',
  background: '#002b36',
  cursor: '#93a1a1',
  selectionBackground: '#073642',
  black: '#073642',
  red: '#dc322f',
  green: '#859900',
  yellow: '#b58900',
  blue: '#268bd2',
  magenta: '#d33682',
  cyan: '#2aa198',
  white: '#eee8d5',
  brightBlack: '#002b36',
  brightRed: '#cb4b16',
  brightGreen: '#586e75',
  brightYellow: '#657b83',
  brightBlue: '#839496',
  brightMagenta: '#6c71c4',
  brightCyan: '#93a1a1',
  brightWhite: '#fdf6e3',
}

/** Resolves semantic, built-in, or custom Terminal colours into an xterm theme. */
export function resolveTerminalTheme(
  settings: TerminalSettings,
  semantic: SemanticThemeTokens,
): ITheme {
  if (settings.themePreset === 'nord') return nord
  if (settings.themePreset === 'solarized') return solarized
  if (settings.themePreset === 'semantic') {
    return {
      foreground: semantic.terminal.foreground,
      background: semantic.terminal.background,
      cursor: semantic.terminal.cursor,
      selectionBackground: semantic.terminal.selection,
    }
  }
  const colour = (index: number): string => settings.ansiColours[index] ?? ''
  return {
    foreground: settings.foreground,
    background: settings.background,
    cursor: settings.cursor,
    selectionBackground: settings.selection,
    black: colour(0),
    red: colour(1),
    green: colour(2),
    yellow: colour(3),
    blue: colour(4),
    magenta: colour(5),
    cyan: colour(6),
    white: colour(7),
    brightBlack: colour(8),
    brightRed: colour(9),
    brightGreen: colour(10),
    brightYellow: colour(11),
    brightBlue: colour(12),
    brightMagenta: colour(13),
    brightCyan: colour(14),
    brightWhite: colour(15),
  }
}
