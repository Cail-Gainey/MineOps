<script setup lang="ts">
import { SearchAddon } from '@xterm/addon-search'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { Terminal } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'
import { NButton, NEmpty, NFlex, NInput, NText } from 'naive-ui'
import { storeToRefs } from 'pinia'
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'

import type { ConsoleEvent } from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/models'
import { ApplicationError } from '../../services/api-client'
import {
  detachServerConsole,
  openServerConsole,
  subscribeConsoleEvents,
  writeServerConsole,
} from '../../services/console-api'
import { useNotificationStore } from '../../stores/notifications'
import { useSettingsStore } from '../../stores/settings'
import { useThemeStore } from '../../stores/theme'
import { resolveTerminalTheme } from '../../themes/terminal-presets'

const props = defineProps<{ serverId: string; serverState: string; active: boolean }>()
const notifications = useNotificationStore()
const settings = useSettingsStore()
const theme = useThemeStore()
const { tokens } = storeToRefs(theme)
const terminalElement = ref<HTMLElement | null>(null)
const searchText = ref('')
const commandText = ref('')
const filterSpark = ref(false)
const consoleReady = ref(false)
const status = ref('服务器未运行')
const observableStates = new Set(['starting', 'running', 'stopping'])
const canShowConsole = computed(() => observableStates.has(props.serverState))
const shouldAttach = computed(() => props.active && canShowConsole.value)
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
      cursorBlink: true,
      cursorWidth: 1,
      themePreset: 'semantic',
      foreground: '#e5e7eb',
      background: '#0d1014',
      cursor: '#34d399',
      selection: '#14532d',
      ansiColours: [],
      copyOnSelect: false,
      autoFocus: true,
      bellStyle: 'none',
    },
)

let terminal: Terminal | null = null
let searchAddon: SearchAddon | null = null
let unsubscribe: (() => void) | null = null
let sessionID = ''
let disposed = false
let opening = false
let attachmentGeneration = 0
let retryTimer: number | null = null
const pendingEvents: ConsoleEvent[] = []

function decodeBase64(value: string): Uint8Array {
  const binary = window.atob(value)
  const bytes = new Uint8Array(binary.length)
  for (let index = 0; index < binary.length; index++) bytes[index] = binary.charCodeAt(index)
  return bytes
}

function showError(error: unknown): void {
  notifications.push({
    kind: 'error',
    title: '服务器控制台附加失败',
    content: error instanceof Error ? error.message : String(error),
    dedupeKey: `console:error:${props.serverId}`,
  })
}

function clearRetry(): void {
  if (retryTimer === null) return
  window.clearTimeout(retryTimer)
  retryTimer = null
}

function scheduleRetry(): void {
  if (!shouldAttach.value || disposed || retryTimer !== null) return
  retryTimer = window.setTimeout(() => {
    retryTimer = null
    void attach()
  }, 1_000)
}

function handleEvent(event: ConsoleEvent): void {
  if (!sessionID && opening) {
    pendingEvents.push(event)
    return
  }
  if (event.sessionID !== sessionID) return
  if (event.type === 'data' && event.data) {
    const data = decodeBase64(event.data)
    if (!filterSpark.value) {
      terminal?.write(data)
    } else {
      const text = new TextDecoder().decode(data)
      const filtered = text
        .split(/(?<=\n)/)
        .filter((line) => !/spark|\[⚡\]/i.test(line))
        .join('')
      if (filtered) terminal?.write(filtered)
    }
  }
  if (event.type === 'dropped') status.value = event.message || '控制台输出过快，部分内容已丢弃'
  if (event.type === 'closed') {
    sessionID = ''
    consoleReady.value = false
    status.value = canShowConsole.value ? '实时日志连接已断开，正在重新附加' : '服务器未运行'
    scheduleRetry()
  }
  if (event.type === 'error') {
    sessionID = ''
    consoleReady.value = false
    status.value = event.error?.message ?? '控制台附加失败'
    showError(new Error(status.value))
    scheduleRetry()
  }
}

async function attach(): Promise<void> {
  if (sessionID || opening || disposed || !shouldAttach.value) return
  clearRetry()
  opening = true
  const generation = attachmentGeneration
  status.value = props.serverState === 'starting' ? '等待服务器实时日志' : '正在附加实时日志'
  try {
    const session = await openServerConsole(props.serverId)
    if (disposed || generation !== attachmentGeneration || !shouldAttach.value) {
      await detachServerConsole(session.id).catch(() => undefined)
      return
    }
    sessionID = session.id
    consoleReady.value = true
    for (const event of pendingEvents.splice(0)) handleEvent(event)
    status.value = '已附加实时日志，可输入命令'
    if (props.active && terminalSettings.value.autoFocus) terminal?.focus()
  } catch (error) {
    pendingEvents.length = 0
    if (error instanceof ApplicationError && error.code === 'validation.conflict') {
      status.value =
        props.serverState === 'starting' ? '等待服务器实时日志' : '正在重新附加实时日志'
    } else {
      status.value = error instanceof Error ? error.message : String(error)
      showError(error)
    }
    scheduleRetry()
  } finally {
    opening = false
  }
}

async function detach(): Promise<void> {
  attachmentGeneration += 1
  clearRetry()
  pendingEvents.length = 0
  if (!sessionID) return
  const id = sessionID
  sessionID = ''
  consoleReady.value = false
  await detachServerConsole(id).catch(showError)
}

function findNext(): void {
  if (!searchText.value) return
  status.value = searchAddon?.findNext(searchText.value, { caseSensitive: false })
    ? `已定位：${searchText.value}`
    : `未找到：${searchText.value}`
}

async function copySelection(): Promise<void> {
  const selection = terminal?.getSelection() ?? ''
  if (selection) await navigator.clipboard.writeText(selection)
}

async function sendCommand(): Promise<void> {
  const command = commandText.value.trim()
  if (!command || !sessionID) return
  try {
    await writeServerConsole(sessionID, `${command}\n`)
    commandText.value = ''
    status.value = '命令已发送'
  } catch (error) {
    showError(error)
  }
}

function clearTerminal(): void {
  terminal?.clear()
}

onMounted(() => {
  terminal = new Terminal({
    convertEol: true,
    cursorBlink: false,
    disableStdin: true,
    cursorStyle: terminalSettings.value.cursorStyle as 'block' | 'underline' | 'bar',
    cursorWidth: terminalSettings.value.cursorWidth,
    fontFamily: terminalSettings.value.fontFamily,
    fontSize: terminalSettings.value.fontSize,
    letterSpacing: terminalSettings.value.letterSpacing,
    lineHeight: terminalSettings.value.lineHeight,
    scrollback: terminalSettings.value.scrollback,
    theme: resolveTerminalTheme(terminalSettings.value, tokens.value),
  })
  searchAddon = new SearchAddon()
  terminal.loadAddon(searchAddon)
  terminal.loadAddon(new WebLinksAddon())
  terminal.open(terminalElement.value!)
  unsubscribe = subscribeConsoleEvents(handleEvent)
  if (shouldAttach.value) void attach()
})

watch(shouldAttach, (attachNow) => {
  if (attachNow) {
    void attach()
    return
  }
  void detach()
})

watch(canShowConsole, (show) => {
  if (show) return
  status.value = '服务器未运行'
  terminal?.reset()
  terminal?.clear()
})

watch([terminalSettings, tokens], () => {
  if (!terminal) return
  terminal.options.fontFamily = terminalSettings.value.fontFamily
  terminal.options.fontSize = terminalSettings.value.fontSize
  terminal.options.theme = resolveTerminalTheme(terminalSettings.value, tokens.value)
})

onUnmounted(() => {
  disposed = true
  unsubscribe?.()
  void detach()
  terminal?.dispose()
})
</script>

<template>
  <div class="server-console">
    <NEmpty v-if="!canShowConsole" description="服务器未运行，暂无实时日志" class="console-empty" />
    <template v-else>
      <NFlex class="console-toolbar" align="center" :wrap="true">
        <NInput
          v-model:value="searchText"
          class="console-search-input"
          clearable
          placeholder="搜索控制台"
          @keyup.enter="findNext"
        />
        <NButton @click="findNext">查找下一个</NButton>
        <NButton @click="copySelection">复制选择</NButton>
        <NButton :type="filterSpark ? 'primary' : 'default'" @click="filterSpark = !filterSpark">
          过滤spark
        </NButton>
        <NButton @click="clearTerminal">清屏</NButton>
        <NText class="console-status" depth="3">{{ status }}</NText>
      </NFlex>
      <NFlex class="console-command-row" align="center" :wrap="false">
        <NInput
          v-model:value="commandText"
          class="console-command-input"
          :disabled="!consoleReady"
          placeholder="输入 Java/Minecraft 命令"
          @keyup.enter="sendCommand"
        />
        <NButton :disabled="!consoleReady || !commandText.trim()" @click="sendCommand">发送命令</NButton>
      </NFlex>
    </template>
    <div ref="terminalElement" class="console-terminal" :class="{ hidden: !canShowConsole }" />
  </div>
</template>

<style scoped>
.server-console {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-height: 480px;
}

.console-empty {
  flex: 1;
  justify-content: center;
  min-height: 420px;
}

.console-toolbar,
.console-command-row {
  width: 100%;
}

.console-search-input {
  flex: 1 1 260px;
  min-width: 220px;
}

.console-command-row :deep(.n-input) {
  flex: 1;
  min-width: 0;
}

.console-status {
  max-width: 240px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.console-terminal {
  flex: 1;
  min-height: 420px;
  padding: 8px;
  overflow: hidden;
  border-radius: 6px;
  background: #0d1014;
}

.console-terminal.hidden {
  display: none;
}
</style>
