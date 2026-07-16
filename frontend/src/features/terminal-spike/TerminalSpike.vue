<script setup lang="ts">
import { FitAddon } from '@xterm/addon-fit'
import { SearchAddon } from '@xterm/addon-search'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { Terminal } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'
import { NButton, NFlex, NInput, NText } from 'naive-ui'
import { storeToRefs } from 'pinia'
import { nextTick, onMounted, onUnmounted, ref, watch } from 'vue'

import { useThemeStore } from '../../stores/theme'
import { accentColours, terminalThemes } from '../../themes/tokens'
import { TerminalOutputBuffer, type TerminalOutputSnapshot } from './terminal-output-buffer'

const terminalElement = ref<HTMLElement | null>(null)
const searchText = ref('MineOps')
const outputRunning = ref(false)
const status = ref('等待初始化')
const stressRunning = ref(false)
const stressProducedBytes = ref(0)
const outputSnapshot = ref<TerminalOutputSnapshot>({
  queuedBytes: 0,
  droppedBytes: 0,
  writtenBytes: 0,
})

const themeStore = useThemeStore()
const { accent, isDark } = storeToRefs(themeStore)

let terminal: Terminal | null = null
let fitAddon: FitAddon | null = null
let searchAddon: SearchAddon | null = null
let resizeObserver: ResizeObserver | null = null
let outputTimer: number | null = null
let stressTimer: number | null = null
let metricsTimer: number | null = null
let lineNumber = 0
let outputBuffer: TerminalOutputBuffer | null = null

const stressTargetBytes = 100 * 1024 * 1024
const stressChunk = `${'MineOps-backpressure-'.repeat(12_000)}\r\n`
const stressChunkBytes = new TextEncoder().encode(stressChunk).byteLength

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MiB`
}

function currentTerminalTheme() {
  return {
    ...(isDark.value ? terminalThemes.dark : terminalThemes.light),
    cursor: accentColours[accent.value],
  }
}

function writeBaselineOutput(): void {
  terminal?.writeln('\x1b[1;32mMineOps xterm.js Spike\x1b[0m')
  terminal?.writeln('ANSI: \x1b[31mred\x1b[0m \x1b[33myellow\x1b[0m \x1b[36mcyan\x1b[0m')
  terminal?.writeln('中文输入验证：请直接在终端中输入“你好，MineOps”。')
  terminal?.writeln('链接验证：https://xtermjs.org')
  terminal?.write('\r\n$ ')
}

function toggleContinuousOutput(): void {
  if (outputTimer !== null) {
    window.clearInterval(outputTimer)
    outputTimer = null
    outputRunning.value = false
    terminal?.write('\r\n\x1b[33m持续输出已停止\x1b[0m\r\n$ ')
    return
  }

  outputRunning.value = true
  outputTimer = window.setInterval(() => {
    lineNumber++
    outputBuffer?.enqueue(
      `\x1b[90m${new Date().toISOString()}\x1b[0m 连续输出 ${lineNumber} — 中文/ANSI/resize\r\n`,
    )
  }, 50)
}

function runBackpressureStress(): void {
  if (stressTimer !== null) {
    window.clearInterval(stressTimer)
    stressTimer = null
    stressRunning.value = false
    return
  }

  stressProducedBytes.value = 0
  stressRunning.value = true
  stressTimer = window.setInterval(() => {
    for (let index = 0; index < 4 && stressProducedBytes.value < stressTargetBytes; index++) {
      outputBuffer?.enqueue(stressChunk)
      stressProducedBytes.value += stressChunkBytes
    }

    if (stressProducedBytes.value >= stressTargetBytes && stressTimer !== null) {
      window.clearInterval(stressTimer)
      stressTimer = null
      stressRunning.value = false
      status.value = '100MB 已快速入队，等待有界队列排空'
    }
  }, 0)
}

function findNext(): void {
  const found = searchAddon?.findNext(searchText.value, { caseSensitive: false }) ?? false
  status.value = found ? `已定位：${searchText.value}` : `未找到：${searchText.value}`
}

async function copySelection(): Promise<void> {
  const selection = terminal?.getSelection() ?? ''
  if (!selection) {
    status.value = '请先在终端中选择文本'
    return
  }

  await navigator.clipboard.writeText(selection)
  status.value = `已复制 ${selection.length} 个字符`
}

onMounted(async () => {
  terminal = new Terminal({
    allowProposedApi: false,
    convertEol: true,
    cursorBlink: true,
    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
    fontSize: 13,
    scrollback: 10_000,
    theme: currentTerminalTheme(),
  })
  fitAddon = new FitAddon()
  searchAddon = new SearchAddon()
  terminal.loadAddon(fitAddon)
  terminal.loadAddon(searchAddon)
  terminal.loadAddon(new WebLinksAddon())
  terminal.open(terminalElement.value!)
  terminal.onData((data) => terminal?.write(data))
  outputBuffer = new TerminalOutputBuffer(terminal)

  resizeObserver = new ResizeObserver(() => fitAddon?.fit())
  resizeObserver.observe(terminalElement.value!)
  await nextTick()
  fitAddon.fit()
  writeBaselineOutput()
  status.value = `${terminal.cols} × ${terminal.rows}`
  metricsTimer = window.setInterval(() => {
    outputSnapshot.value = outputBuffer?.snapshot() ?? outputSnapshot.value
  }, 100)
})

watch([isDark, accent], () => {
  if (terminal) {
    terminal.options.theme = currentTerminalTheme()
  }
})

onUnmounted(() => {
  if (outputTimer !== null) {
    window.clearInterval(outputTimer)
  }
  if (stressTimer !== null) {
    window.clearInterval(stressTimer)
  }
  if (metricsTimer !== null) {
    window.clearInterval(metricsTimer)
  }
  resizeObserver?.disconnect()
  outputBuffer?.dispose()
  terminal?.dispose()
})
</script>

<template>
  <NFlex vertical>
    <NFlex>
      <NInput v-model:value="searchText" placeholder="搜索终端内容" @keyup.enter="findNext" />
      <NButton @click="findNext">查找下一个</NButton>
      <NButton @click="copySelection">复制选择</NButton>
      <NButton :type="outputRunning ? 'warning' : 'primary'" @click="toggleContinuousOutput">
        {{ outputRunning ? '停止持续输出' : '启动持续输出' }}
      </NButton>
      <NButton :type="stressRunning ? 'warning' : 'error'" @click="runBackpressureStress">
        {{ stressRunning ? '停止 100MB 压力' : '运行 100MB 压力' }}
      </NButton>
    </NFlex>
    <div ref="terminalElement" class="terminal-host" />
    <NText depth="3">{{ status }}</NText>
    <NText depth="3">
      生产 {{ formatBytes(stressProducedBytes) }} / 队列
      {{ formatBytes(outputSnapshot.queuedBytes) }} / 已写入
      {{ formatBytes(outputSnapshot.writtenBytes) }} / 丢弃
      {{ formatBytes(outputSnapshot.droppedBytes) }}
    </NText>
  </NFlex>
</template>

<style scoped>
.terminal-host {
  width: 100%;
  height: 320px;
  overflow: hidden;
  border: 1px solid var(--n-border-color);
  border-radius: 6px;
  padding: 8px;
}
</style>
