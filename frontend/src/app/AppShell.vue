<script setup lang="ts">
import { PanelLeftClose, PanelLeftOpen } from '@lucide/vue'
import {
  NAlert,
  NBadge,
  NButton,
  NDropdown,
  NFlex,
  NLayout,
  NLayoutContent,
  NLayoutFooter,
  NLayoutHeader,
  NLayoutSider,
  NText,
} from 'naive-ui'
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import AppIcon from '../shared/components/AppIcon.vue'
import AppStatusDrawer from '../shared/components/AppStatusDrawer.vue'
import { getDesktopUpdateStatus } from '../services/desktop-update-api'
import { confirmApplicationQuit, subscribeQuitBlocked } from '../services/exit-guard-api'
import { useAlertNotificationStore } from '../stores/alert-notifications'
import { useErrorCenterStore } from '../stores/error-center'
import { useLayoutStore } from '../stores/layout'
import { useInteractionStore } from '../stores/interactions'
import { useLocaleStore } from '../stores/locale'
import { useOperationsStore } from '../stores/operations'
import { useNotificationStore } from '../stores/notifications'
import { useSettingsStore, type ThemeSelectionUpdate } from '../stores/settings'
import { useThemeStore } from '../stores/theme'
import { themePresetAccents, themePresetNames, type ThemePresetName } from '../themes/tokens'
import logoURL from '../../public/logo.png'
import { navigationEntries } from './navigation'

const route = useRoute()
const router = useRouter()
const layout = useLayoutStore()
const locale = useLocaleStore()
const theme = useThemeStore()
const settings = useSettingsStore()
const operations = useOperationsStore()
const alertNotifications = useAlertNotificationStore()
const errors = useErrorCenterStore()
const interactions = useInteractionStore()
const notifications = useNotificationStore()
const statusDrawerVisible = ref(false)
const statusDrawerTab = ref<'operations' | 'alerts' | 'errors'>('operations')
let unsubscribeQuitBlocked: (() => void) | null = null
let desktopUpdatePoller: number | null = null
let lastDesktopUpdateNotice = ''
const pageTitle = computed(() => {
  const titleKey = route.meta.titleKey
  return typeof titleKey === 'string'
    ? locale.t(titleKey as Parameters<typeof locale.t>[0])
    : String(route.meta.title ?? locale.t('app.name'))
})
const latestOperation = computed(() => operations.active[0] ?? operations.history[0] ?? null)
const operationSummary = computed(() => {
  if (operations.activeCount > 0) return `运行中 ${operations.activeCount}`
  if (!latestOperation.value) return '无活动任务'
  return `最近 ${latestOperation.value.state} · ${latestOperation.value.type}`
})
const themePresetLabels: Record<ThemePresetName, string> = {
  mineops: 'MineOps',
  forest: '森林',
  ocean: '深海',
  amethyst: '紫晶',
  graphite: '极光',
  sunset: '暮光',
}
const currentPresetLabel = computed(() => themePresetLabels[theme.preset])
const quickThemeOptions = [
  ...themePresetNames.map((preset) => ({
    label: themePresetLabels[preset],
    key: `preset:${preset}`,
  })),
  { type: 'divider' as const, key: 'theme-divider' },
  { label: '跟随系统', key: 'mode:system' },
  { label: '浅色模式', key: 'mode:light' },
  { label: '深色模式', key: 'mode:dark' },
]

/**
 * 判断事件目标是否处于可编辑控件中，用于避免截获输入场景的按键。
 * @param target - 事件目标节点
 * @returns 目标为输入框、文本域或可编辑元素时返回 true
 */
function isEditingTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return (
    target.isContentEditable ||
    target instanceof HTMLInputElement ||
    target instanceof HTMLTextAreaElement ||
    target instanceof HTMLSelectElement
  )
}

/**
 * 判断事件目标是否位于内嵌终端内。
 * @param target - 事件目标节点
 * @returns 位于 xterm 容器内时返回 true
 */
function insideTerminal(target: EventTarget | null): boolean {
  return target instanceof Element && target.closest('.xterm') !== null
}

// 桌面外壳去浏览器化兜底:拦截 webview 自带的缩放/刷新/历史导航键(macOS 菜单项已在 Go 侧移除,
// 此处覆盖 Windows/Linux 与残余加速键)。终端内的键必须放行给远程 shell(如 Ctrl+R 反向搜索)。
/**
 * 拦截 webview 自带的刷新、缩放与前进后退按键，终端内的按键放行。
 * @param event - 键盘事件
 * @returns 无返回值
 */
function blockBrowserDefaultKeys(event: KeyboardEvent): void {
  if (event.defaultPrevented || insideTerminal(event.target)) return
  const key = event.key
  const command = event.metaKey || event.ctrlKey
  if (key === 'F5') {
    event.preventDefault()
    return
  }
  if (command && !event.altKey) {
    if (key === '=' || key === '+' || key === '-' || key === '0') {
      event.preventDefault()
      return
    }
    if (key.toLowerCase() === 'r') {
      event.preventDefault()
      return
    }
    if (event.metaKey && (key === '[' || key === ']')) {
      event.preventDefault()
      return
    }
  }
  if (
    event.altKey &&
    !command &&
    (key === 'ArrowLeft' || key === 'ArrowRight') &&
    !isEditingTarget(event.target)
  ) {
    event.preventDefault()
  }
}

// Linux WebKitGTK 兜底:Ctrl+滚轮缩放不经过 keydown,只能在 wheel 事件拦截。
/**
 * 拦截 Ctrl/⌘ + 滚轮触发的页面缩放。
 * @param event - 滚轮事件
 * @returns 无返回值
 */
function blockBrowserZoomWheel(event: WheelEvent): void {
  if (event.ctrlKey) event.preventDefault()
}

// 右键菜单兜底(Go 侧 DefaultContextMenuDisabled 之外的平台差异):终端有自绘菜单,放行;
// 其余区域(含前端自绘菜单组件,它们已自行 preventDefault)一律阻止默认菜单。
/**
 * 屏蔽默认右键菜单，终端有自绘菜单因此放行。
 * @param event - 鼠标事件
 * @returns 无返回值
 */
function blockDefaultContextMenu(event: MouseEvent): void {
  if (event.defaultPrevented || insideTerminal(event.target)) return
  event.preventDefault()
}

/**
 * 分发全局快捷键；输入框、编辑器与终端获得焦点时不截获。
 * @param event - 键盘事件
 * @returns 无返回值
 */
function handleGlobalShortcut(event: KeyboardEvent): void {
  if (
    event.defaultPrevented ||
    event.repeat ||
    event.altKey ||
    (!event.ctrlKey && !event.metaKey) ||
    isEditingTarget(event.target)
  )
    return
  const key = event.key.toLowerCase()
  if (!event.shiftKey && key === ',') {
    event.preventDefault()
    void router.push('/settings')
  } else if (event.shiftKey && key === 'o') {
    event.preventDefault()
    void router.push('/operations')
  } else if (event.shiftKey && key === 't') {
    event.preventDefault()
    const currentIndex = themePresetNames.indexOf(theme.preset)
    const nextPreset = themePresetNames[(currentIndex + 1) % themePresetNames.length] ?? 'mineops'
    persistThemeSelection({
      preset: nextPreset,
      accent: themePresetAccents[nextPreset],
    })
  } else if (!event.shiftKey && key === 'b') {
    event.preventDefault()
    layout.toggleSidebar()
  }
}

/**
 * 持久化顶栏的主题选择，失败时记入错误中心而不打断交互。
 * @param selection - 本次变更的主题模式、套装或强调色
 * @returns 无返回值
 */
function persistThemeSelection(selection: ThemeSelectionUpdate): void {
  void settings
    .saveThemeSelection(selection)
    .catch((error) => errors.capture(error, 'settings:quick-theme'))
}

/**
 * 在浅色与深色之间切换并持久化。
 * @returns 无返回值
 */
function toggleThemeMode(): void {
  persistThemeSelection({
    mode: theme.isDark ? 'light' : 'dark',
  })
}

/**
 * 处理顶栏主题下拉的选择项，按前缀区分模式、套装与强调色。
 * @param selectedKey - 下拉项的 key，形如 mode:dark 或 preset:forest
 * @returns 无返回值
 */
function handleQuickThemeSelect(selectedKey: string | number): void {
  const key = String(selectedKey)
  if (key.startsWith('mode:')) {
    const mode = key.slice('mode:'.length)
    if (mode === 'light' || mode === 'dark' || mode === 'system') {
      persistThemeSelection({ mode })
    }
    return
  }
  if (!key.startsWith('preset:')) return
  const preset = key.slice('preset:'.length) as ThemePresetName
  if (!themePresetNames.includes(preset)) return
  persistThemeSelection({ preset, accent: themePresetAccents[preset] })
}

/**
 * 打开底部状态抽屉并切换到指定分页。
 * @param tab - 目标分页：操作、告警或错误
 * @returns 无返回值
 */
function openStatusDrawer(tab: 'operations' | 'alerts' | 'errors'): void {
  statusDrawerTab.value = tab
  statusDrawerVisible.value = true
}

/**
 * 轮询桌面更新状态，仅在出现可用或待重启更新时推送一次通知。
 * @returns 轮询完成后的 Promise
 */
async function pollDesktopUpdateNotice(): Promise<void> {
  try {
    const status = await getDesktopUpdateStatus()
    if (status.phase !== 'available' && status.phase !== 'ready') return
    const noticeKey = `${status.phase}:${status.latestVersion}`
    if (noticeKey === lastDesktopUpdateNotice) return
    lastDesktopUpdateNotice = noticeKey
    notifications.push({
      kind: 'info',
      title: status.phase === 'ready' ? 'Desktop 更新等待重启安装' : '发现 Desktop 新版本',
      content:
        status.phase === 'ready'
          ? `${status.latestVersion} 已下载并通过验证，可在 Settings 中重启安装。`
          : `${status.currentVersion} → ${status.latestVersion}`,
      dedupeKey: `desktop-update:${noticeKey}`,
    })
  } catch {
    // Settings 页面会展示稳定错误；全局轮询保持静默。
  }
}

onMounted(() => {
  operations.startSubscription()
  alertNotifications.start()
  window.addEventListener('keydown', blockBrowserDefaultKeys, true)
  window.addEventListener('wheel', blockBrowserZoomWheel, { passive: false })
  window.addEventListener('contextmenu', blockDefaultContextMenu)
  window.addEventListener('keydown', handleGlobalShortcut)
  void operations.refresh()
  void pollDesktopUpdateNotice()
  desktopUpdatePoller = window.setInterval(() => void pollDesktopUpdateNotice(), 30_000)
  unsubscribeQuitBlocked = subscribeQuitBlocked(async (event) => {
    const labels = event.items.map((item) => `• ${item.label}`).join('\n')
    const confirmed = await interactions.confirm({
      title: '退出并放弃未保存内容？',
      content: `以下内容尚未保存：\n${labels}`,
      impact:
        '退出后这些本地编辑和未提交表单将丢失；后台 Operation 会先按关闭流程取消并持久化最终状态。',
      positiveText: '放弃并退出',
      danger: true,
    })
    if (confirmed) await confirmApplicationQuit()
  })
})
onUnmounted(() => {
  operations.stopSubscription()
  alertNotifications.stop()
  window.removeEventListener('keydown', blockBrowserDefaultKeys, true)
  window.removeEventListener('wheel', blockBrowserZoomWheel)
  window.removeEventListener('contextmenu', blockDefaultContextMenu)
  window.removeEventListener('keydown', handleGlobalShortcut)
  unsubscribeQuitBlocked?.()
  if (desktopUpdatePoller !== null) window.clearInterval(desktopUpdatePoller)
})
</script>

<template>
  <NLayout class="app-shell">
    <NLayoutHeader v-if="layout.topBarVisible" bordered class="topbar">
      <NFlex align="center" justify="space-between" :wrap="false">
        <NFlex align="center" :wrap="false">
          <NText tag="h1" class="page-title">{{ pageTitle }}</NText>
        </NFlex>
        <NFlex align="center" :wrap="false">
          <NButton size="small" @click="toggleThemeMode">
            {{ theme.isDark ? locale.t('shell.lightTheme') : locale.t('shell.darkTheme') }}
          </NButton>
          <NDropdown :options="quickThemeOptions" trigger="click" @select="handleQuickThemeSelect">
            <NButton size="small" secondary aria-label="快捷切换主题">
              {{ currentPresetLabel }} ▾
            </NButton>
          </NDropdown>
          <NBadge :value="operations.activeCount" :show-zero="true" :max="99" type="info">
            <NButton size="small" quaternary @click="openStatusDrawer('operations')">
              活动
            </NButton>
          </NBadge>
        </NFlex>
      </NFlex>
    </NLayoutHeader>

    <NLayout class="workspace-layout" has-sider>
      <NLayoutSider
        v-if="layout.sidebarVisible"
        bordered
        collapse-mode="width"
        :collapsed="layout.sidebarCollapsed"
        :collapsed-width="68"
        :width="layout.sidebarWidth"
        class="sidebar"
        :class="{ 'sidebar--collapsed': layout.sidebarCollapsed }"
      >
        <div class="sidebar-header">
          <button
            type="button"
            class="sidebar-brand"
            :class="{ 'sidebar-brand--expandable': layout.sidebarCollapsed }"
            :aria-label="layout.sidebarCollapsed ? locale.t('shell.toggleSidebar') : 'MineOps'"
            :aria-disabled="!layout.sidebarCollapsed"
            :tabindex="layout.sidebarCollapsed ? 0 : -1"
            @click="layout.sidebarCollapsed && layout.toggleSidebar()"
          >
            <img :src="logoURL" alt="" class="brand-logo" />
            <NText v-if="!layout.sidebarCollapsed" strong>MineOps</NText>
          </button>
          <NButton
            v-if="!layout.sidebarCollapsed"
            quaternary
            circle
            size="small"
            aria-keyshortcuts="Control+B Meta+B"
            @click="layout.toggleSidebar"
          >
            <AppIcon
              :icon="layout.sidebarCollapsed ? PanelLeftOpen : PanelLeftClose"
              :label="locale.t('shell.toggleSidebar')"
            />
          </NButton>
        </div>
        <nav class="navigation" aria-label="主导航">
          <RouterLink
            v-for="entry in navigationEntries"
            :key="entry.path"
            v-slot="{ isActive, navigate }"
            :to="entry.path"
            custom
          >
            <NButton
              quaternary
              block
              :type="isActive ? 'primary' : 'default'"
              class="nav-button"
              @click="navigate"
            >
              <AppIcon :icon="entry.icon" :label="locale.t(entry.titleKey)" />
              <span v-if="!layout.sidebarCollapsed">{{ locale.t(entry.titleKey) }}</span>
            </NButton>
          </RouterLink>
        </nav>
      </NLayoutSider>

      <NLayout class="main-layout">
        <NLayoutContent class="main-content" content-style="padding: 24px;">
          <NAlert
            v-if="errors.latest"
            closable
            type="error"
            :title="errors.latest.source"
            class="global-error"
            @close="errors.dismiss(errors.latest.id)"
          >
            {{ errors.latest.message }}
          </NAlert>
          <RouterView />
        </NLayoutContent>

        <NLayoutFooter v-if="layout.bottomBarVisible" bordered class="bottombar">
          <NFlex align="center" justify="space-between" :wrap="false" class="bottombar-content">
            <NFlex align="center" :wrap="false" class="bottombar-group">
              <NText depth="3">当前页：{{ pageTitle }}</NText>
              <NText depth="3">Operation：{{ operationSummary }}</NText>
            </NFlex>
            <NFlex align="center" :wrap="false" class="bottombar-group">
              <NBadge
                :value="alertNotifications.activeCount"
                :show-zero="true"
                :max="99"
                type="error"
              >
                <NButton
                  text
                  size="tiny"
                  :type="alertNotifications.activeCount ? 'error' : 'default'"
                  @click="openStatusDrawer('alerts')"
                >
                  告警
                </NButton>
              </NBadge>
              <NBadge :value="errors.entries.length" :show-zero="true" :max="99" type="error">
                <NButton
                  text
                  size="tiny"
                  :type="errors.entries.length ? 'error' : 'default'"
                  @click="openStatusDrawer('errors')"
                >
                  错误
                </NButton>
              </NBadge>
              <NText v-if="operations.partialMessage" type="warning" depth="3">{{
                operations.partialMessage
              }}</NText>
            </NFlex>
          </NFlex>
        </NLayoutFooter>
      </NLayout>
    </NLayout>
    <AppStatusDrawer v-model:show="statusDrawerVisible" :initial-tab="statusDrawerTab" />
  </NLayout>
</template>

<style scoped>
.app-shell {
  display: flex;
  height: 100vh;
  min-width: 0;
  min-height: 0;
  flex-direction: column;
  overflow: hidden;
  background: transparent;
}

.app-shell :deep(.n-layout) {
  background: transparent;
}

.workspace-layout {
  height: calc(100vh - 64px);
  flex: 1 1 auto;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}

.workspace-layout :deep(> .n-layout-scroll-container) {
  height: 100%;
  display: flex;
  min-width: 0;
  min-height: 0;
}

.main-layout {
  display: flex;
  height: 100%;
  min-width: 0;
  min-height: 0;
  flex: 1 1 auto;
  flex-direction: column;
}

.main-layout :deep(> .n-layout-scroll-container) {
  height: 100%;
  display: flex;
  min-width: 0;
  min-height: 0;
  flex-direction: column;
}

.sidebar {
  height: 100%;
}

.topbar,
.sidebar,
.bottombar {
  background: var(--chrome-background, var(--surface-panel));
  backdrop-filter: blur(18px) saturate(1.08);
  -webkit-backdrop-filter: blur(18px) saturate(1.08);
}

/* 关闭 GPU 硬件加速时去掉背景模糊：backdrop-filter 依赖 GPU 合成，
   退回 CPU 光栅化后，正文每次滚动或重绘都要重新模糊整条顶栏/侧栏/底栏区域。 */
html[data-hardware-acceleration='off'] .topbar,
html[data-hardware-acceleration='off'] .sidebar,
html[data-hardware-acceleration='off'] .bottombar {
  backdrop-filter: none;
  -webkit-backdrop-filter: none;
}

.topbar,
.bottombar {
  border-color: var(--chrome-border, var(--border-default));
}

.sidebar :deep(.n-layout-sider-scroll-container) {
  background: transparent;
}

.sidebar-header {
  display: flex;
  height: 64px;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 0 14px;
  border-bottom: 1px solid var(--chrome-border, var(--border-default));
}

.sidebar-brand {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 10px;
  padding: 0;
  border: 0;
  color: inherit;
  background: transparent;
  font: inherit;
  text-align: left;
}

.sidebar-brand--expandable {
  cursor: pointer;
}

.sidebar--collapsed .sidebar-header {
  justify-content: center;
  padding: 0;
}

.brand-logo {
  width: 34px;
  height: 34px;
  border: 1px solid var(--n-border-color);
  border-radius: 8px;
  object-fit: contain;
}

.navigation {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 0 8px 8px;
}

.nav-button {
  justify-content: flex-start;
  gap: 12px;
  height: 42px;
}

.topbar {
  height: 64px;
  padding: 0 24px 0 88px;
  display: flex;
  flex: 0 0 64px;
  align-items: center;
}

.topbar > :deep(.n-flex) {
  width: 100%;
}

.page-title {
  margin: 0;
  font-size: 20px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.main-content {
  min-width: 0;
  min-height: 0;
  flex: 1 1 auto;
  overflow: auto;
}

.global-error {
  margin-bottom: 16px;
}

.bottombar {
  height: 36px;
  padding: 0 24px;
  display: flex;
  align-items: center;
}

.bottombar-content {
  width: 100%;
  min-width: 0;
}

.bottombar-group {
  min-width: 0;
  gap: 16px;
}

.bottombar :deep(.n-text) {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
