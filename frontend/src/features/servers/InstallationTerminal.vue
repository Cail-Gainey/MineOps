<script setup lang="ts">
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { Terminal } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'
import { NText } from 'naive-ui'
import { storeToRefs } from 'pinia'
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'

import { useSettingsStore } from '../../stores/settings'
import { useThemeStore } from '../../stores/theme'
import { resolveTerminalTheme } from '../../themes/terminal-presets'

const props = defineProps<{
  taskId: string
  lines: string[]
  status: string
}>()

const settings = useSettingsStore()
const theme = useThemeStore()
const { tokens } = storeToRefs(theme)
const terminalElement = ref<HTMLElement | null>(null)
const terminalSettings = computed(
  () =>
    settings.draft?.terminal ??
    settings.committed?.terminal ?? {
      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
      fontSize: 13,
      letterSpacing: 0,
      lineHeight: 1.2,
      scrollback: 10_000,
      cursorStyle: 'block',
      cursorBlink: false,
      cursorWidth: 1,
      themePreset: 'semantic',
      foreground: '#e5e7eb',
      background: '#0d1014',
      cursor: '#34d399',
      selection: '#14532d',
      ansiColours: [],
      copyOnSelect: false,
      autoFocus: false,
      bellStyle: 'none',
    },
)

let terminal: Terminal | null = null
let fitAddon: FitAddon | null = null
let resizeObserver: ResizeObserver | null = null
let renderedLines: string[] = []

/**
 * 把安装日志行同步到终端，只追加新增部分。
 * @param lines - 完整的安装日志行
 * @returns 无返回值
 */
function syncLines(lines: string[]): void {
  if (!terminal) return
  const next = lines.map((line) => String(line))
  let commonLength = 0
  while (
    commonLength < renderedLines.length &&
    commonLength < next.length &&
    renderedLines[commonLength] === next[commonLength]
  ) {
    commonLength += 1
  }
  if (commonLength < renderedLines.length) {
    terminal.reset()
    renderedLines = []
    commonLength = 0
  }
  for (const line of next.slice(commonLength)) {
    terminal.writeln(line.replace(/\r?\n/g, '\r\n'))
  }
  renderedLines = next
  terminal.scrollToBottom()
}

/**
 * 重新适配终端尺寸。
 * @returns 无返回值
 */
function fitTerminal(): void {
  fitAddon?.fit()
}

onMounted(async () => {
  terminal = new Terminal({
    convertEol: true,
    cursorBlink: false,
    disableStdin: true,
    fontFamily: terminalSettings.value.fontFamily,
    fontSize: terminalSettings.value.fontSize,
    letterSpacing: terminalSettings.value.letterSpacing,
    lineHeight: terminalSettings.value.lineHeight,
    scrollback: terminalSettings.value.scrollback,
    theme: resolveTerminalTheme(terminalSettings.value, tokens.value),
  })
  fitAddon = new FitAddon()
  terminal.loadAddon(fitAddon)
  terminal.loadAddon(new WebLinksAddon())
  terminal.open(terminalElement.value!)
  resizeObserver = new ResizeObserver(fitTerminal)
  resizeObserver.observe(terminalElement.value!)
  await nextTick()
  fitTerminal()
  syncLines(props.lines)
})

watch(
  () => props.lines,
  (lines) => syncLines(lines),
  { deep: true },
)

watch(
  () => props.taskId,
  () => {
    renderedLines = []
    terminal?.reset()
    syncLines(props.lines)
  },
)

watch(
  [terminalSettings, tokens],
  () => {
    if (!terminal) return
    terminal.options.fontFamily = terminalSettings.value.fontFamily
    terminal.options.fontSize = terminalSettings.value.fontSize
    terminal.options.letterSpacing = terminalSettings.value.letterSpacing
    terminal.options.lineHeight = terminalSettings.value.lineHeight
    terminal.options.scrollback = terminalSettings.value.scrollback
    terminal.options.theme = resolveTerminalTheme(terminalSettings.value, tokens.value)
    fitTerminal()
  },
  { deep: true },
)

onUnmounted(() => {
  resizeObserver?.disconnect()
  terminal?.dispose()
})
</script>

<template>
  <section class="installation-terminal">
    <NText depth="3">{{ status }}</NText>
    <div ref="terminalElement" class="installation-terminal-host" />
  </section>
</template>

<style scoped>
.installation-terminal {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.installation-terminal-host {
  min-height: 260px;
  padding: 8px;
  overflow: hidden;
  border: 1px solid var(--border-default);
  border-radius: 8px;
  background: var(--terminal-background);
}
</style>
