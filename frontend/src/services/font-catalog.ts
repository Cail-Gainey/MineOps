import { translate } from '../locales/runtime'

const candidates = [
  'SFMono-Regular',
  'Menlo',
  'Monaco',
  'JetBrains Mono',
  'Fira Code',
  'Cascadia Code',
  'Cascadia Mono',
  'Consolas',
  'Source Code Pro',
  'Ubuntu Mono',
  'DejaVu Sans Mono',
  'Liberation Mono',
  'Noto Sans Mono',
]

export interface FontOption {
  label: string
  value: string
}

/**
 * 枚举当前系统实际可用的等宽字体候选。
 * @returns 可用字体选项数组
 */
export function enumerateMonospaceFonts(): FontOption[] {
  const available = candidates.filter((font) => document.fonts.check(`13px "${font}"`))
  return [
    {
      label: translate('font.systemMonospace'),
      value: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
    },
    ...available.map((font) => ({ label: font, value: `"${font}", monospace` })),
  ]
}
