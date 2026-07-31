<script setup lang="ts">
import { FitAddon } from '@xterm/addon-fit'
import { SearchAddon } from '@xterm/addon-search'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { Terminal } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'
import { NButton, NDropdown, NFlex, NInput, NText, type DropdownOption } from 'naive-ui'
import { storeToRefs } from 'pinia'
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'

import type { TerminalEvent } from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/models'
import { ApplicationError } from '../../services/api-client'
import { confirmAndTrustHostKey, isHostKeyRejected } from '../../services/host-key-trust'
import {
  closeTerminal,
  openTerminal,
  resizeTerminal,
  subscribeTerminalEvents,
  writeTerminal,
} from '../../services/terminal-api'
import { useNotificationStore } from '../../stores/notifications'
import { useSettingsStore } from '../../stores/settings'
import { useThemeStore } from '../../stores/theme'
import { resolveTerminalTheme } from '../../themes/terminal-presets'

const props = defineProps<{
  sshSessionId: string
  active: boolean
}>()

const notifications = useNotificationStore()
const theme = useThemeStore()
const settings = useSettingsStore()
const { tokens } = storeToRefs(theme)
const terminalElement = ref<HTMLElement | null>(null)
const searchText = ref('')
const searchInput = ref<InstanceType<typeof NInput> | null>(null)
const status = ref('准备连接')
const connecting = ref(false)
const reconnecting = ref(false)
const menuVisible = ref(false)
const menuX = ref(0)
const menuY = ref(0)
const menuHasSelection = ref(false)
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
let fitAddon: FitAddon | null = null
let searchAddon: SearchAddon | null = null
let resizeObserver: ResizeObserver | null = null
let unsubscribe: (() => void) | null = null
let terminalSessionID = ''
let resizeTimer: number | null = null
let inputTimer: number | null = null
let inputQueue = ''
const pendingEvents: TerminalEvent[] = []
let disposed = false
let reconnectAttempts = 0
let reconnectTimer: number | null = null
let connectionGeneration = 0

/**
 * 按当前终端设置与语义主题解析出 xterm 主题。
 * @returns xterm 主题对象
 */
function terminalTheme() {
  return resolveTerminalTheme(terminalSettings.value, tokens.value)
}

/**
 * 把后端推送的 Base64 输出解码成字节数组。
 * @param value - Base64 字符串
 * @returns 解码后的字节数组
 */
function decodeBase64(value: string): Uint8Array {
  const binary = window.atob(value)
  const bytes = new Uint8Array(binary.length)
  for (let index = 0; index < binary.length; index++) bytes[index] = binary.charCodeAt(index)
  return bytes
}

/**
 * 把键盘输入放入缓冲并合并发送，避免逐字符往返。
 * @param data - 本次输入的数据
 * @returns 无返回值
 */
function queueInput(data: string): void {
  inputQueue += data
  if (inputTimer !== null) return
  inputTimer = window.setTimeout(() => {
    const pending = inputQueue
    inputQueue = ''
    inputTimer = null
    if (terminalSessionID && pending)
      void writeTerminal(terminalSessionID, pending).catch(showError)
  }, 8)
}

/**
 * 重新适配终端尺寸并把新行列数同步给远端。
 * @returns 无返回值
 */
function scheduleResize(): void {
  if (!props.active) return
  fitAddon?.fit()
  if (!terminal || !terminalSessionID) return
  if (resizeTimer !== null) window.clearTimeout(resizeTimer)
  resizeTimer = window.setTimeout(() => {
    resizeTimer = null
    if (terminal && terminalSessionID) {
      void resizeTerminal(terminalSessionID, terminal.cols, terminal.rows).catch(showError)
      status.value = `${terminal.cols} × ${terminal.rows}`
    }
  }, 100)
}

/**
 * 在终端回滚缓冲中查找下一处匹配。
 * @returns 无返回值
 */
function findNext(): void {
  if (!searchText.value) return
  const found = searchAddon?.findNext(searchText.value, { caseSensitive: false }) ?? false
  status.value = found ? `已定位：${searchText.value}` : `未找到：${searchText.value}`
}

/**
 * 把终端当前选区复制到剪贴板。
 * @returns 复制完成后的 Promise
 */
async function copySelection(): Promise<void> {
  const selection = terminal?.getSelection() ?? ''
  if (!selection) return
  await navigator.clipboard.writeText(selection)
  status.value = `已复制 ${selection.length} 个字符`
}

/**
 * 把剪贴板内容粘贴到终端。
 * @returns 粘贴完成后的 Promise
 */
async function pasteClipboard(): Promise<void> {
  try {
    const text = await navigator.clipboard.readText()
    if (text) queueInput(text)
  } catch (error) {
    showError(error)
  }
}

const menuOptions = computed<DropdownOption[]>(() => [
  { label: '复制', key: 'copy', disabled: !menuHasSelection.value },
  { label: '粘贴', key: 'paste' },
  { type: 'divider', key: 'divider-clipboard' },
  { label: '全选', key: 'select-all' },
  { label: '清屏', key: 'clear' },
  { label: '搜索', key: 'search' },
])

/**
 * 在终端内打开自绘右键菜单。
 * @param event - 鼠标事件
 * @returns 无返回值
 */
function openTerminalMenu(event: MouseEvent): void {
  event.preventDefault()
  menuHasSelection.value = Boolean(terminal?.getSelection())
  menuVisible.value = false
  menuX.value = event.clientX
  menuY.value = event.clientY
  requestAnimationFrame(() => {
    menuVisible.value = true
  })
}

/**
 * 分发终端右键菜单选中的动作。
 * @param key - 菜单项 key
 * @returns 无返回值
 */
function handleMenuSelect(key: string | number): void {
  menuVisible.value = false
  if (key === 'copy') void copySelection()
  if (key === 'paste') void pasteClipboard()
  if (key === 'select-all') terminal?.selectAll()
  if (key === 'clear') terminal?.clear()
  if (key === 'search') searchInput.value?.focus()
}

/**
 * 推送一条终端错误通知。
 * @param error - 捕获到的错误
 * @returns 无返回值
 */
function showError(error: unknown): void {
  notifications.push({
    kind: 'error',
    title: 'Terminal 操作失败',
    content: error instanceof Error ? error.message : String(error),
    dedupeKey: `terminal:error:${terminalSessionID || props.sshSessionId}`,
  })
}

/**
 * 按 SSH 重连设置安排一次退避重连。
 * @returns 无返回值
 */
function scheduleReconnect(): void {
  const sshSettings = settings.draft?.ssh ?? settings.committed?.ssh
  if (
    disposed ||
    !sshSettings?.autoReconnect ||
    reconnectAttempts >= sshSettings.reconnectAttempts ||
    reconnectTimer !== null
  ) {
    return
  }
  reconnectAttempts++
  const delay = Math.max(0, sshSettings.reconnectBackoffSec) * 1000 * reconnectAttempts
  status.value = `连接已断开，${Math.round(delay / 1000)} 秒后进行第 ${reconnectAttempts} 次重连`
  reconnectTimer = window.setTimeout(() => {
    reconnectTimer = null
    void connectTerminal(true)
  }, delay)
}

/**
 * 建立终端会话；重连时复用同一个 SSH Session。
 * @param reconnect - 是否为自动重连
 * @returns 连接完成后的 Promise
 */
async function connectTerminal(reconnect = false): Promise<void> {
  if (connecting.value) return
  const generation = ++connectionGeneration
  connecting.value = true
  reconnecting.value = reconnect
  if (terminalSessionID) {
    const previousID = terminalSessionID
    terminalSessionID = ''
    await closeTerminal(previousID).catch(() => undefined)
  }
  if (reconnect) terminal?.writeln('\r\n\x1b[33m[MineOps] 正在重新连接…\x1b[0m')
  try {
    const session = await openTerminal(
      props.sshSessionId,
      terminal?.cols ?? 80,
      terminal?.rows ?? 24,
    )
    if (disposed || generation !== connectionGeneration) {
      await closeTerminal(session.id).catch(() => undefined)
      return
    }
    terminalSessionID = session.id
    reconnectAttempts = 0
    for (const event of pendingEvents.splice(0)) handleTerminalEvent(event)
    status.value = `已连接 · ${terminal?.cols ?? 80} × ${terminal?.rows ?? 24}`
    if (reconnect) terminal?.writeln('\r\n\x1b[32m[MineOps] 已重新连接\x1b[0m')
    if (props.active && terminalSettings.value.autoFocus) terminal?.focus()
  } catch (error) {
    if (generation !== connectionGeneration || disposed) return
    status.value = error instanceof Error ? error.message : String(error)
    if (error instanceof ApplicationError && error.code === 'io.not_found') {
      notifications.push({
        kind: 'error',
        title: 'Terminal 打开失败',
        content: `后端未找到请求的 SSH Session（ID：${props.sshSessionId}）。标签已保留，可在确认 Session 后重试。`,
        dedupeKey: `terminal:missing-ssh-session:${props.sshSessionId}`,
      })
      return
    }
    if (isHostKeyRejected(error)) {
      status.value = 'SSH 主机指纹待确认'
      const trusted = await confirmAndTrustHostKey(error)
      if (disposed || generation !== connectionGeneration) return
      if (trusted) {
        void connectTerminal(true)
        return
      }
      status.value = 'SSH 主机指纹未被信任，连接已停止。'
      return
    }
    showError(error)
    if (reconnect) scheduleReconnect()
  } finally {
    if (generation === connectionGeneration) {
      connecting.value = false
      reconnecting.value = false
    }
  }
}

/**
 * 处理后端推送的终端输出与关闭事件。
 * @param event - 终端事件
 * @returns 无返回值
 */
function handleTerminalEvent(event: TerminalEvent): void {
  if (!terminalSessionID) {
    if (connecting.value) pendingEvents.push(event)
    return
  }
  if (event.sessionID !== terminalSessionID) return
  if (event.type === 'data' && event.data) terminal?.write(decodeBase64(event.data))
  if (event.type === 'closed') {
    terminalSessionID = ''
    status.value = '远程 Terminal 已关闭'
    scheduleReconnect()
  }
  if (event.type === 'dropped') {
    status.value = event.message || 'Terminal 输出过快，部分内容已丢弃'
    notifications.push({
      kind: 'warning',
      title: 'Terminal 输出触发背压保护',
      content: status.value,
      dedupeKey: `terminal:dropped:${terminalSessionID}`,
    })
  }
  if (event.type === 'error') {
    terminalSessionID = ''
    status.value = event.error?.message ?? '远程 Terminal 发生错误'
    showError(new Error(status.value))
    scheduleReconnect()
  }
}

onMounted(async () => {
  terminal = new Terminal({
    cursorBlink: terminalSettings.value.cursorBlink,
    cursorStyle: terminalSettings.value.cursorStyle as 'block' | 'underline' | 'bar',
    cursorWidth: terminalSettings.value.cursorWidth,
    fontFamily: terminalSettings.value.fontFamily,
    fontSize: terminalSettings.value.fontSize,
    letterSpacing: terminalSettings.value.letterSpacing,
    lineHeight: terminalSettings.value.lineHeight,
    scrollback: terminalSettings.value.scrollback,
    theme: terminalTheme(),
  })
  fitAddon = new FitAddon()
  searchAddon = new SearchAddon()
  terminal.loadAddon(fitAddon)
  terminal.loadAddon(searchAddon)
  terminal.loadAddon(new WebLinksAddon())
  terminal.open(terminalElement.value!)
  terminal.onData(queueInput)
  terminal.onSelectionChange(() => {
    if (!terminalSettings.value.copyOnSelect) return
    const selection = terminal?.getSelection() ?? ''
    if (selection) void navigator.clipboard.writeText(selection).catch(() => undefined)
  })
  terminalElement.value?.addEventListener('contextmenu', openTerminalMenu)
  unsubscribe = subscribeTerminalEvents(handleTerminalEvent)
  resizeObserver = new ResizeObserver(scheduleResize)
  resizeObserver.observe(terminalElement.value!)
  await nextTick()
  fitAddon.fit()
  if (props.active) await connectTerminal()
})

watch(
  [tokens, terminalSettings],
  () => {
    if (!terminal) return
    terminal.options.theme = terminalTheme()
    terminal.options.fontFamily = terminalSettings.value.fontFamily
    terminal.options.fontSize = terminalSettings.value.fontSize
    terminal.options.letterSpacing = terminalSettings.value.letterSpacing
    terminal.options.lineHeight = terminalSettings.value.lineHeight
    terminal.options.scrollback = terminalSettings.value.scrollback
    terminal.options.cursorStyle = terminalSettings.value.cursorStyle as
      'block' | 'underline' | 'bar'
    terminal.options.cursorBlink = terminalSettings.value.cursorBlink
    terminal.options.cursorWidth = terminalSettings.value.cursorWidth
    scheduleResize()
  },
  { deep: true },
)

watch(
  () => props.active,
  async (active) => {
    if (!active) return
    await nextTick()
    scheduleResize()
    if (!terminalSessionID && !connecting.value && !reconnecting.value) await connectTerminal()
    if (terminalSettings.value.autoFocus) terminal?.focus()
  },
)

onUnmounted(() => {
  disposed = true
  connectionGeneration++
  unsubscribe?.()
  resizeObserver?.disconnect()
  if (resizeTimer !== null) window.clearTimeout(resizeTimer)
  if (inputTimer !== null) window.clearTimeout(inputTimer)
  if (reconnectTimer !== null) window.clearTimeout(reconnectTimer)
  if (terminalSessionID) void closeTerminal(terminalSessionID).catch(() => undefined)
  terminal?.dispose()
})
</script>

<template>
  <section class="terminal-pane">
    <NFlex class="terminal-toolbar" align="center" justify="space-between">
      <NText depth="3">{{ status }}</NText>
      <NFlex :wrap="false">
        <NButton
          v-if="!terminalSessionID"
          :loading="connecting || reconnecting"
          @click="connectTerminal(true)"
        >
          重连
        </NButton>
        <NInput
          ref="searchInput"
          v-model:value="searchText"
          clearable
          placeholder="搜索终端内容"
          @keyup.enter="findNext"
        />
        <NButton @click="findNext">查找</NButton>
        <NButton @click="copySelection">复制</NButton>
      </NFlex>
    </NFlex>
    <div ref="terminalElement" class="terminal-host" />
    <NDropdown
      placement="bottom-start"
      trigger="manual"
      :x="menuX"
      :y="menuY"
      :options="menuOptions"
      :show="menuVisible"
      :on-clickoutside="() => (menuVisible = false)"
      @select="handleMenuSelect"
    />
  </section>
</template>

<style scoped>
.terminal-pane {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.terminal-toolbar {
  margin-bottom: 8px;
}

.terminal-host {
  flex: 1;
  min-height: 320px;
  overflow: hidden;
  padding: 8px;
  border: 1px solid var(--border-default);
  border-radius: 6px;
  background: var(--terminal-background);
}
</style>
