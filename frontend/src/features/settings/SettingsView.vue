<script setup lang="ts">
import {
  NAlert,
  NButton,
  NCheckbox,
  NColorPicker,
  NDescriptions,
  NDescriptionsItem,
  NDynamicTags,
  NFlex,
  NForm,
  NInput,
  NInputNumber,
  NProgress,
  NSelect,
  NSlider,
  NSpin,
  NTabPane,
  NTabs,
  NText,
  NTag,
} from 'naive-ui'
import { Browser } from '@wailsio/runtime'
import { storeToRefs } from 'pinia'
import { onMounted, ref, watch } from 'vue'

import { useUnsavedGuard } from '../../composables/use-unsaved-guard'
import AppFormActions from '../../shared/components/AppFormActions.vue'
import AppFormField from '../../shared/components/AppFormField.vue'
import { selectFile } from '../../services/native-dialog'
import { selectSavePath } from '../../services/native-dialog'
import {
  getBackgroundResource,
  importBackgroundResource,
  resetBackgroundResource,
} from '../../services/background-api'
import type {
  BackgroundResource,
  DesktopUpdateStatus,
  LogStatus,
  StorageStatus,
} from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/service'
import { clearArchivedLogs, getLogStatus, openLogDirectory } from '../../services/logging-api'
import { exportDiagnosticPackage } from '../../services/diagnostic-api'
import { listSSHSessions } from '../../services/ssh-session-api'
import {
  cancelPendingDatabaseMaintenance,
  createPortableBackup,
  getStorageStatus,
  openDataDirectory,
  scheduleDatabaseKeyRotation,
  scheduleDatabaseReset,
  scheduleDatabaseVacuum,
  schedulePortableRestore,
} from '../../services/storage-api'
import {
  checkDownloadSources,
  clearDownloadCache,
  clearProxyCredential,
  getDownloadCacheStatus,
  getProxyCredentialStatus,
  saveProxyCredential,
} from '../../services/download-api'
import type {
  DownloadCacheStatus,
  DownloadSourceStatus,
  ProxyCredentialStatus,
} from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/service'
import { enumerateMonospaceFonts, type FontOption } from '../../services/font-catalog'
import {
  cancelDesktopUpdate,
  checkDesktopUpdates,
  downloadDesktopUpdate,
  getDesktopUpdateStatus,
  restartDesktopUpdate,
} from '../../services/desktop-update-api'
import { getUnsavedItems } from '../../services/exit-guard-api'
import { useInteractionStore } from '../../stores/interactions'
import { useLocaleStore } from '../../stores/locale'
import { useNotificationStore } from '../../stores/notifications'
import { useSettingsStore } from '../../stores/settings'
import { themePresetAccents } from '../../themes/tokens'
import KnownHostsPanel from './KnownHostsPanel.vue'

const settings = useSettingsStore()
const interactions = useInteractionStore()
const locale = useLocaleStore()
const notifications = useNotificationStore()
const { dirty, draft, error: settingsError, loading, saving } = storeToRefs(settings)
const activeCategory = ref('general')
const settingsSearchTarget = ref<string | null>(null)
const shortcutModifier = /Mac|iPhone|iPad/.test(navigator.platform) ? '⌘' : 'Ctrl'
const fontOptions = ref<FontOption[]>(enumerateMonospaceFonts())
const sourceStatuses = ref<DownloadSourceStatus[]>([])
const cacheStatus = ref<DownloadCacheStatus | null>(null)
const proxyCredential = ref<ProxyCredentialStatus | null>(null)
const proxyUsername = ref('')
const proxyPassword = ref('')
const downloadActionLoading = ref(false)
const backgroundActionLoading = ref(false)
const runtimeActionLoading = ref(false)
const backgroundResource = ref<BackgroundResource | null>(null)
const logStatus = ref<LogStatus | null>(null)
const storageStatus = ref<StorageStatus | null>(null)
const desktopUpdateStatus = ref<DesktopUpdateStatus | null>(null)
const desktopUpdateLoading = ref(false)
const auxiliaryError = ref('')
const jumpHostOptions = ref<{ label: string; value: string }[]>([])

useUnsavedGuard('settings', 'Settings 页面有未保存修改', dirty)

watch(
  () => draft.value,
  (value) => {
    if (value) settings.previewTheme(value)
  },
  { deep: true },
)

const languageOptions = [
  { label: '简体中文', value: 'zh-CN' },
  { label: 'English', value: 'en-US' },
]
const themeOptions = [
  { label: '跟随系统', value: 'system' },
  { label: '浅色', value: 'light' },
  { label: '深色', value: 'dark' },
]
const themePresetOptions = [
  {
    label: 'MineOps',
    description: '清爽中性的默认界面',
    value: 'mineops',
    colours: ['#f4f6f8', '#ffffff', '#059669'],
  },
  {
    label: '森林',
    description: '高对比的深绿工作台',
    value: 'forest',
    colours: ['#e4f2e5', '#f7fff7', '#16a34a'],
  },
  {
    label: '深海',
    description: '清晰通透的蓝色空间',
    value: 'ocean',
    colours: ['#e3f2ff', '#f7fbff', '#0284c7'],
  },
  {
    label: '紫晶',
    description: '鲜明沉浸的紫色氛围',
    value: 'amethyst',
    colours: ['#f1e6ff', '#fcf8ff', '#7c3aed'],
  },
  {
    label: '极光',
    description: '通透鲜明的青绿冰蓝',
    value: 'graphite',
    colours: ['#dff8f5', '#f4fffd', '#0891b2'],
  },
  {
    label: '暮光',
    description: '温暖醒目的橙红界面',
    value: 'sunset',
    colours: ['#fff0e4', '#fffaf5', '#e11d48'],
  },
]
const accentOptions = [
  { label: 'Emerald', value: 'emerald' },
  { label: 'Amber', value: 'amber' },
  { label: 'Azure', value: 'azure' },
  { label: 'Violet', value: 'violet' },
  { label: 'Rose', value: 'rose' },
]
const backgroundModeOptions = [
  { label: '主题背景', value: 'theme' },
  { label: '纯色', value: 'color' },
  { label: '用户图片', value: 'image' },
]
const backgroundFitOptions = [
  { label: '填充', value: 'cover' },
  { label: '适应', value: 'contain' },
  { label: '居中', value: 'center' },
  { label: '平铺', value: 'tile' },
]
const closeBehaviorOptions = [
  { label: '退出应用', value: 'quit' },
  { label: '最小化到后台', value: 'minimize' },
]
const timeFormatOptions = [
  { label: '24 小时', value: '24h' },
  { label: '12 小时', value: '12h' },
]
const updateChannelOptions = [
  { label: 'Stable', value: 'stable' },
  { label: 'Beta', value: 'beta' },
]
const updatePolicyOptions = [
  { label: '仅通知', value: 'notify' },
  { label: '自动下载', value: 'download' },
  { label: '自动下载并提示重启', value: 'prompt_restart' },
]
const settingsSearchOptions = [
  {
    label: '通用 · 语言、开机启动、窗口关闭、时间格式、GPU 硬件加速、Desktop 更新',
    value: 'general',
  },
  { label: '主题 · 套装、模式、强调色、图片显示、透明度、高对比度', value: 'theme' },
  { label: '目录 · Server 目录、下载目录', value: 'paths' },
  { label: '镜像 · Java、Minecraft、Spark', value: 'mirrors' },
  { label: '下载与代理 · 下载源、HTTP、HTTPS、SOCKS5、缓存、限速', value: 'downloads' },
  { label: '日志 · 等级、轮转、保留、容量、诊断包', value: 'logging' },
  { label: '监控 · 采集、保留期、Spark、Profiler、告警、静默时段', value: 'monitoring' },
  { label: '防火墙 · Provider、安装放行、端口同步、安全回收', value: 'firewall' },
  { label: '布局 · TopBar、BottomBar、SideBar、宽度', value: 'layout' },
  { label: 'SSH · 超时、KeepAlive、重连、认证、Known Hosts、Jump Host', value: 'ssh' },
  { label: 'Terminal · 字体、字号、行高、滚动、光标、配色', value: 'terminal' },
  { label: '存储与安全 · SQLCipher、备份、恢复、密钥轮换、存储整理、清空数据库', value: 'storage' },
]

/**
 * 选中主题套装并同步其推荐强调色。
 * @param value - 主题套装标识
 * @returns 无返回值
 */
function selectThemePreset(value: string): void {
  if (!draft.value || !(value in themePresetAccents)) return
  const preset = value as keyof typeof themePresetAccents
  draft.value.theme.preset = preset
  draft.value.theme.accent = themePresetAccents[preset]
}

const alertNotificationOptions = [
  { label: '桌面通知', value: 'desktop' },
  { label: '声音', value: 'sound' },
]
const logLevelOptions = ['debug', 'info', 'warn', 'error'].map((value) => ({
  label: value.toUpperCase(),
  value,
}))
const sshAuthOptions = [
  { label: '私钥', value: 'private_key' },
  { label: 'SSH Agent', value: 'agent' },
  { label: '密码', value: 'password' },
]
const hostKeyPolicyOptions = [
  { label: '严格校验', value: 'strict' },
  { label: '首次确认后信任', value: 'trust_on_first_use' },
]
const firewallProviderOptions = [
  { label: '自动检测远程主机', value: 'auto' },
  { label: 'UFW', value: 'ufw' },
  { label: 'firewalld', value: 'firewalld' },
  { label: '禁用防火墙管理', value: 'disabled' },
]
const firewallPolicyOptions = [
  { label: '自动管理', value: 'automatic' },
  { label: '操作前确认', value: 'prompt' },
  { label: '不管理', value: 'disabled' },
]
const terminalThemeOptions = [
  { label: '跟随语义主题', value: 'semantic' },
  { label: 'Nord', value: 'nord' },
  { label: 'Solarized Dark', value: 'solarized' },
  { label: '自定义', value: 'custom' },
]
const cursorStyleOptions = [
  { label: '方块', value: 'block' },
  { label: '下划线', value: 'underline' },
  { label: '竖线', value: 'bar' },
]
const bellStyleOptions = [
  { label: '关闭', value: 'none' },
  { label: '声音', value: 'sound' },
  { label: '视觉', value: 'visual' },
  { label: '声音与视觉', value: 'both' },
]
const proxyModeOptions = [
  { label: '不使用代理', value: 'none' },
  { label: '跟随系统', value: 'system' },
  { label: 'HTTP', value: 'http' },
  { label: 'HTTPS', value: 'https' },
  { label: 'SOCKS5', value: 'socks5' },
]
const ansiLabels = [
  'Black',
  'Red',
  'Green',
  'Yellow',
  'Blue',
  'Magenta',
  'Cyan',
  'White',
  'Bright Black',
  'Bright Red',
  'Bright Green',
  'Bright Yellow',
  'Bright Blue',
  'Bright Magenta',
  'Bright Cyan',
  'Bright White',
]

onMounted(() => {
  if (!draft.value) void settings.load()
  void refreshDownloadState()
  void refreshBackgroundState()
  void refreshLogState()
  void refreshStorageState()
  void refreshDesktopUpdateStatus()
  void refreshJumpHosts()
  void document.fonts.ready.then(() => {
    fontOptions.value = enumerateMonospaceFonts()
  })
})

/**
 * 拉取当前桌面更新状态。
 * @returns 刷新完成后的 Promise
 */
async function refreshDesktopUpdateStatus(): Promise<void> {
  try {
    desktopUpdateStatus.value = await getDesktopUpdateStatus()
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '加载 Desktop 更新状态失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'desktop-update:status-error',
    })
  }
}

/**
 * 立即检查当前通道的新版本。
 * @returns 检查完成后的 Promise
 */
async function checkDesktopUpdate(): Promise<void> {
  desktopUpdateLoading.value = true
  try {
    if (dirty.value) await settings.save()
    const status = await checkDesktopUpdates()
    desktopUpdateStatus.value = status
    notifications.push({
      kind: 'success',
      title: status.updateAvailable ? '发现 Desktop 新版本' : '当前通道已是最新版本',
      content: status.updateAvailable
        ? `${status.currentVersion} → ${status.latestVersion || '未知版本'}`
        : `${status.channel} · ${status.currentVersion}`,
      dedupeKey: 'desktop-update:checked',
    })
  } catch (error) {
    try {
      desktopUpdateStatus.value = await getDesktopUpdateStatus()
    } catch {
      // 保留当前可见状态，原始检查错误会通过通知展示。
    }
    notifications.push({
      kind: 'error',
      title: '检查 Desktop 更新失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'desktop-update:check-error',
    })
  } finally {
    desktopUpdateLoading.value = false
  }
}

/**
 * 下载并校验更新包，准备到可重启安装的状态。
 * @returns 准备完成后的 Promise
 */
async function prepareDesktopUpdate(): Promise<void> {
  desktopUpdateLoading.value = true
  const poller = window.setInterval(() => void refreshDesktopUpdateStatus(), 300)
  try {
    desktopUpdateStatus.value = await downloadDesktopUpdate()
    notifications.push({
      kind: 'success',
      title: 'Desktop 更新已准备完成',
      content: `${desktopUpdateStatus.value.currentVersion} → ${desktopUpdateStatus.value.latestVersion}`,
      dedupeKey: 'desktop-update:ready',
    })
  } catch (error) {
    await refreshDesktopUpdateStatus()
    notifications.push({
      kind: 'error',
      title: '准备 Desktop 更新失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'desktop-update:download-error',
    })
  } finally {
    window.clearInterval(poller)
    desktopUpdateLoading.value = false
  }
}

/**
 * 取消正在进行的更新下载。
 * @returns 取消完成后的 Promise
 */
async function cancelDesktopUpdateDownload(): Promise<void> {
  try {
    desktopUpdateStatus.value = await cancelDesktopUpdate()
    notifications.push({
      kind: 'info',
      title: '已请求取消 Desktop 更新下载',
      content: '已完成验证的暂存更新不会被删除。',
      dedupeKey: 'desktop-update:cancelled',
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '取消 Desktop 更新失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'desktop-update:cancel-error',
    })
  }
}

/**
 * 确认后重启应用并安装已准备好的更新。
 * @returns 安装动作发起后的 Promise
 */
async function applyDesktopUpdate(): Promise<void> {
  try {
    const unsavedItems = await getUnsavedItems()
    if (unsavedItems.length > 0) {
      const labels = unsavedItems.map((item) => `• ${item.label}`).join('\n')
      const confirmed = await interactions.confirm({
        title: '重启并安装 Desktop 更新？',
        content: `以下内容尚未保存：\n${labels}`,
        impact:
          '确认后将放弃这些修改；活动 Operation 会先按正常关闭流程取消并持久化最终状态，然后 MineOps 将替换程序并重新启动。',
        positiveText: '放弃并重启安装',
        danger: true,
      })
      if (!confirmed) return
    } else {
      const confirmed = await interactions.confirm({
        title: '重启并安装 Desktop 更新？',
        content: `MineOps 将应用 ${desktopUpdateStatus.value?.latestVersion || '已准备版本'} 并重新启动。`,
        impact: '活动 Operation 会先按正常关闭流程取消并持久化最终状态。',
        positiveText: '重启并安装',
      })
      if (!confirmed) return
    }
    desktopUpdateStatus.value = await restartDesktopUpdate(unsavedItems.length > 0)
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '启动 Desktop 更新重启失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'desktop-update:restart-error',
    })
  }
}

/**
 * 用系统浏览器打开当前版本的 GitHub Release 页面。
 * @returns 打开完成后的 Promise
 */
async function openDesktopRelease(): Promise<void> {
  const releaseURL = desktopUpdateStatus.value?.releaseURL
  if (releaseURL) await Browser.OpenURL(releaseURL)
}

/**
 * 把桌面更新阶段映射成中文标签。
 * @param phase - 更新阶段标识
 * @returns 中文标签，未知阶段原样返回
 */
function desktopUpdatePhaseLabel(phase: string): string {
  const labels: Record<string, string> = {
    idle: '等待检查',
    checking: '正在检查',
    available: '发现新版本',
    downloading: '正在下载',
    verifying: '正在验证并准备',
    ready: '等待重启安装',
    restarting: '正在重启安装',
    'up-to-date': '当前已是最新版本',
    error: '更新失败',
  }
  return labels[phase] ?? phase
}

/**
 * 按当前时间格式设置渲染时间文本。
 * @param value - ISO 时间字符串，可为空
 * @returns 本地时间文本，空值返回尚未检查
 */
function formatDateTime(value?: string | null): string {
  return value ? locale.formatDateTime(value) : '尚未检查'
}

/**
 * 跳转到设置搜索选中的分类页签。
 * @param value - 目标分类标识，空值忽略
 * @returns 无返回值
 */
function jumpToSettingsCategory(value: string | null): void {
  if (!value) return
  activeCategory.value = value
  settingsSearchTarget.value = null
}

/**
 * 加载可作为默认 Jump Host 的 SSH Session 选项。
 * @returns 加载完成后的 Promise
 */
async function refreshJumpHosts(): Promise<void> {
  try {
    jumpHostOptions.value = (await listSSHSessions()).map((session) => ({
      label: `${session.name} · ${session.username}@${session.host}:${session.port}`,
      value: session.id,
    }))
  } catch (error) {
    auxiliaryError.value = error instanceof Error ? error.message : String(error)
  }
}

/**
 * 读取当前背景图资源状态。
 * @returns 刷新完成后的 Promise
 */
async function refreshBackgroundState(): Promise<void> {
  try {
    backgroundResource.value = (await getBackgroundResource()).resource
  } catch (error) {
    auxiliaryError.value = error instanceof Error ? error.message : String(error)
  }
}

/**
 * 读取日志目录容量与归档状态。
 * @returns 刷新完成后的 Promise
 */
async function refreshLogState(): Promise<void> {
  try {
    logStatus.value = await getLogStatus()
  } catch (error) {
    auxiliaryError.value = error instanceof Error ? error.message : String(error)
  }
}

/**
 * 读取数据库体积、加密状态与排队维护任务。
 * @returns 刷新完成后的 Promise
 */
async function refreshStorageState(): Promise<void> {
  try {
    storageStatus.value = await getStorageStatus()
  } catch (error) {
    auxiliaryError.value = error instanceof Error ? error.message : String(error)
  }
}

/**
 * 读取下载源探测结果、缓存体积与代理凭据状态。
 * @returns 刷新完成后的 Promise
 */
async function refreshDownloadState(): Promise<void> {
  try {
    proxyCredential.value = await getProxyCredentialStatus()
    proxyUsername.value = proxyCredential.value.username ?? ''
    cacheStatus.value = await getDownloadCacheStatus()
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '加载下载设置状态失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'downloads:state-error',
    })
  }
}

/**
 * 把代理用户名与密码保存到加密数据库。
 * @returns 保存完成后的 Promise
 */
async function saveProxyAuth(): Promise<void> {
  if (!draft.value) return
  downloadActionLoading.value = true
  try {
    proxyCredential.value = await saveProxyCredential(proxyUsername.value, proxyPassword.value)
    draft.value.downloads.proxy.credentialID = proxyCredential.value.credentialID
    proxyPassword.value = ''
    notifications.push({
      kind: 'success',
      title: '代理凭据已写入加密数据库',
      dedupeKey: 'downloads:proxy-saved',
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '保存代理凭据失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'downloads:proxy-error',
    })
  } finally {
    downloadActionLoading.value = false
  }
}

/**
 * 从加密数据库删除已保存的代理凭据。
 * @returns 删除完成后的 Promise
 */
async function removeProxyAuth(): Promise<void> {
  if (!draft.value) return
  downloadActionLoading.value = true
  try {
    await clearProxyCredential()
    draft.value.downloads.proxy.credentialID = ''
    proxyCredential.value = null
    proxyUsername.value = ''
    proxyPassword.value = ''
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '清除代理凭据失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'downloads:proxy-clear-error',
    })
  } finally {
    downloadActionLoading.value = false
  }
}

/**
 * 逐个探测下载源可用性并回填状态。
 * @returns 探测完成后的 Promise
 */
async function checkSources(): Promise<void> {
  downloadActionLoading.value = true
  try {
    if (dirty.value) await settings.save()
    sourceStatuses.value = await checkDownloadSources()
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '下载源连通性检查失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'downloads:check-error',
    })
  } finally {
    downloadActionLoading.value = false
  }
}

/**
 * 二次确认后清空本地下载缓存。
 * @returns 清理完成后的 Promise
 */
async function clearCache(): Promise<void> {
  const confirmed = await interactions.confirm({
    title: '清理下载缓存？',
    content: '将删除本地缓存 Artifact，不影响远程服务器或已安装文件。',
    positiveText: '清理缓存',
    danger: true,
  })
  if (!confirmed) return
  downloadActionLoading.value = true
  try {
    await clearDownloadCache()
    cacheStatus.value = await getDownloadCacheStatus()
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '清理下载缓存失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'downloads:cache-error',
    })
  } finally {
    downloadActionLoading.value = false
  }
}

/**
 * 把字节数换算成合适的存储单位。
 * @param value - 字节数
 * @returns 带单位的容量文本
 */
function formatBytes(value: number): string {
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KiB`
  if (value < 1024 * 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)} MiB`
  return `${(value / 1024 / 1024 / 1024).toFixed(2)} GiB`
}

/**
 * 在下载源列表末尾追加一个自定义镜像。
 * @returns 无返回值
 */
function addDownloadMirror(): void {
  if (!draft.value) return
  draft.value.downloads.sources.push({
    category: 'minecraft',
    provider: 'papermc',
    name: `自定义镜像 ${draft.value.downloads.sources.length + 1}`,
    baseURL: 'https://',
    probeURL: 'https://',
    official: false,
    enabled: true,
    priority: 5,
  })
}

/**
 * 删除一个自定义镜像，官方源不可删除。
 * @param index - 待删除镜像在列表中的下标
 * @returns 无返回值
 */
function removeDownloadMirror(index: number): void {
  if (!draft.value || draft.value.downloads.sources[index]?.official) return
  draft.value.downloads.sources.splice(index, 1)
}

/**
 * 提交当前设置草稿并同步应用主题、布局与语言。
 * @returns 保存完成后的 Promise
 */
async function saveSettings(): Promise<void> {
  try {
    await settings.save()
    notifications.push({ kind: 'success', title: '设置已保存', dedupeKey: 'settings:saved' })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '保存设置失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'settings:save-error',
    })
  }
}

/**
 * 二次确认后放弃未保存的设置修改。
 * @returns 放弃完成后的 Promise
 */
async function discardChanges(): Promise<void> {
  if (!dirty.value) return
  const confirmed = await interactions.confirm({
    title: '放弃设置修改？',
    content: '当前页面中尚未保存的设置将恢复为最近一次保存的值。',
    positiveText: '放弃修改',
    danger: true,
  })
  if (confirmed) settings.discard()
}

/**
 * 二次确认后把当前分类恢复为内置默认值。
 * @returns 重置完成后的 Promise
 */
async function resetCurrentCategory(): Promise<void> {
  const confirmed = await interactions.confirm({
    title: '恢复当前分类默认值？',
    content: '该分类会立即写入默认配置，原有值将被替换。',
    objectLabel: activeCategory.value,
    impact: '当前分类的自定义设置将丢失。',
    positiveText: '恢复默认',
    danger: true,
  })
  if (!confirmed) return
  try {
    await settings.resetCategory(activeCategory.value)
    notifications.push({
      kind: 'success',
      title: '已恢复当前分类',
      dedupeKey: `settings:reset:${activeCategory.value}`,
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '恢复默认设置失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'settings:reset-error',
    })
  }
}

/**
 * 选择并导入一张自定义背景图。
 * @returns 导入完成后的 Promise
 */
async function chooseBackgroundImage(): Promise<void> {
  if (!draft.value) return
  const selected = await selectFile({
    title: '选择背景图片',
    directory: draft.value.paths.backgroundImage,
    filters: [{ DisplayName: '图片', Pattern: '*.png;*.jpg;*.jpeg;*.webp' }],
    buttonText: '选择图片',
  })
  if (!selected) return
  backgroundActionLoading.value = true
  try {
    if (dirty.value) await settings.save()
    const result = await importBackgroundResource(selected)
    backgroundResource.value = result.resource
    await settings.load()
    notifications.push({
      kind: 'success',
      title: '背景图片已复制到受控数据目录',
      dedupeKey: 'settings:background-imported',
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '导入背景图片失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'settings:background-error',
    })
  } finally {
    backgroundActionLoading.value = false
  }
}

/**
 * 移除自定义背景图，回到主题内置背景。
 * @returns 重置完成后的 Promise
 */
async function resetBackground(): Promise<void> {
  backgroundActionLoading.value = true
  try {
    await resetBackgroundResource()
    await settings.load()
    await refreshBackgroundState()
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '恢复默认背景失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'settings:background-reset-error',
    })
  } finally {
    backgroundActionLoading.value = false
  }
}

/**
 * 二次确认后清理归档日志文件。
 * @returns 清理完成后的 Promise
 */
async function clearLogs(): Promise<void> {
  const confirmed = await interactions.confirm({
    title: '立即清理历史日志？',
    content: '当前进程正在写入的日志文件会保留，其他轮转日志将被删除。',
    positiveText: '清理日志',
    danger: true,
  })
  if (!confirmed) return
  runtimeActionLoading.value = true
  try {
    logStatus.value = await clearArchivedLogs()
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '清理日志失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'settings:logs-clear-error',
    })
  } finally {
    runtimeActionLoading.value = false
  }
}

/**
 * 用系统文件管理器打开日志目录。
 * @returns 打开完成后的 Promise
 */
async function openLogs(): Promise<void> {
  try {
    await openLogDirectory()
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '打开日志目录失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'settings:logs-open-error',
    })
  }
}

/**
 * 用系统文件管理器打开受控数据目录。
 * @returns 打开完成后的 Promise
 */
async function openData(): Promise<void> {
  try {
    await openDataDirectory()
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '打开数据目录失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'settings:data-open-error',
    })
  }
}

/**
 * 导出一份脱敏诊断包到用户选择的位置。
 * @returns 导出完成后的 Promise
 */
async function exportDiagnostics(): Promise<void> {
  const destination = await selectSavePath({
    title: '导出 MineOps 诊断包',
    filename: `mineops-diagnostic-${new Date().toISOString().slice(0, 10)}.mineops-diagnostic.zip`,
    filters: [{ DisplayName: 'MineOps Diagnostic', Pattern: '*.zip' }],
  })
  if (!destination) return
  runtimeActionLoading.value = true
  try {
    const result = await exportDiagnosticPackage(
      destination.endsWith('.zip') ? destination : `${destination}.mineops-diagnostic.zip`,
    )
    notifications.push({
      kind: result.warnings.length ? 'warning' : 'success',
      title: '诊断包已导出',
      content: `${result.path} · ${formatBytes(result.sizeBytes)} · ${result.logFiles} 个日志文件`,
      dedupeKey: 'settings:diagnostic-exported',
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '导出诊断包失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'settings:diagnostic-error',
    })
  } finally {
    runtimeActionLoading.value = false
  }
}

/**
 * 创建一份加密便携备份到用户选择的位置。
 * @returns 备份完成后的 Promise
 */
async function createBackup(): Promise<void> {
  const destination = await selectSavePath({
    title: '创建 MineOps 备份',
    filename: `mineops-${new Date().toISOString().slice(0, 10)}.mineops-backup`,
    filters: [{ DisplayName: 'MineOps Backup', Pattern: '*.mineops-backup' }],
  })
  if (!destination) return
  runtimeActionLoading.value = true
  try {
    const backup = await createPortableBackup(
      destination.endsWith('.mineops-backup') ? destination : `${destination}.mineops-backup`,
    )
    notifications.push({
      kind: 'success',
      title: '加密备份已创建',
      content: `${backup.path} · ${formatBytes(backup.bytes)}`,
      dedupeKey: 'settings:backup-created',
    })
    await refreshStorageState()
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '创建备份失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'settings:backup-error',
    })
  } finally {
    runtimeActionLoading.value = false
  }
}

/**
 * 选择备份文件、校验并排队到下次启动恢复。
 * @returns 排队完成后的 Promise
 */
async function scheduleRestore(): Promise<void> {
  const selected = await selectFile({
    title: '选择 MineOps 备份',
    filters: [{ DisplayName: 'MineOps Backup', Pattern: '*.mineops-backup' }],
  })
  if (!selected) return
  const confirmed = await interactions.confirm({
    title: '排队恢复数据库？',
    content: '备份会先完成完整性校验并复制到受控目录。恢复将在下次启动、数据库打开前执行。',
    impact: '当前数据库将在下次启动时被备份内容替换。',
    positiveText: '校验并排队恢复',
    danger: true,
  })
  if (!confirmed) return
  runtimeActionLoading.value = true
  try {
    storageStatus.value = await schedulePortableRestore(selected)
    notifications.push({
      kind: 'warning',
      title: '数据库恢复已排队',
      content: '请正常退出并重新启动 MineOps。',
      dedupeKey: 'settings:restore-scheduled',
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '排队恢复失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'settings:restore-error',
    })
  } finally {
    runtimeActionLoading.value = false
  }
}

/**
 * 二次确认后排队一次下次启动执行的密钥轮换。
 * @returns 排队完成后的 Promise
 */
async function scheduleKeyRotation(): Promise<void> {
  const confirmed = await interactions.confirm({
    title: '排队数据库密钥轮换？',
    content: '密钥轮换将在下次启动、数据库打开前执行，并同步更新系统安全存储。',
    positiveText: '排队轮换',
    danger: true,
  })
  if (!confirmed) return
  runtimeActionLoading.value = true
  try {
    storageStatus.value = await scheduleDatabaseKeyRotation()
    notifications.push({
      kind: 'warning',
      title: '密钥轮换已排队',
      content: '请正常退出并重新启动 MineOps。',
      dedupeKey: 'settings:key-rotation-scheduled',
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '排队密钥轮换失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'settings:key-rotation-error',
    })
  } finally {
    runtimeActionLoading.value = false
  }
}

/**
 * 二次确认后排队一次下次启动执行的存储整理。
 * @returns 排队完成后的 Promise
 */
async function scheduleVacuum(): Promise<void> {
  const allocated = storageStatus.value?.databaseBytes ?? 0
  const confirmed = await interactions.confirm({
    title: '排队存储整理（VACUUM）？',
    content:
      '整理会在下次启动、数据库打开前离线执行，把删除历史数据后留下的空闲页还给磁盘。执行期间应用不会响应，GB 级数据库可能耗时数分钟，并需要与数据库等大的临时磁盘空间。',
    objectLabel: storageStatus.value?.databasePath ?? '',
    impact: `当前数据库文件 ${formatBytes(allocated)}，整理期间需要额外约同等大小的可用空间。`,
    positiveText: '排队整理',
  })
  if (!confirmed) return
  runtimeActionLoading.value = true
  try {
    storageStatus.value = await scheduleDatabaseVacuum()
    notifications.push({
      kind: 'warning',
      title: '存储整理已排队',
      content: '请正常退出并重新启动 MineOps，启动过程会比平时慢。',
      dedupeKey: 'settings:vacuum-scheduled',
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '排队存储整理失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'settings:vacuum-error',
    })
  } finally {
    runtimeActionLoading.value = false
  }
}

/**
 * 两道确认后排队一次下次启动执行的清空数据库。
 * @returns 排队完成后的 Promise
 */
async function resetDatabase(): Promise<void> {
  // 恢复出厂不可撤销，用两道确认：第一道说明范围，第二道逐条列出会永久消失的内容。
  const acknowledged = await interactions.confirm({
    title: '重置',
    content: '',
    objectLabel: storageStatus.value?.dataDirectory ?? '',
    impact: '此操作不可撤销。建议先创建加密备份',
    positiveText: '我已了解，继续',
    danger: true,
  })
  if (!acknowledged) return
  const confirmed = await interactions.confirm({
    title: '确认永久删除全部数据？',
    content: '所有数据将清空。',
    objectLabel: storageStatus.value?.databasePath ?? '',
    impact: '排队后请正常退出并重新启动 MineOps；重启前可以随时取消排队任务。',
    positiveText: '排队清空数据库',
    danger: true,
  })
  if (!confirmed) return
  runtimeActionLoading.value = true
  try {
    storageStatus.value = await scheduleDatabaseReset()
    notifications.push({
      kind: 'warning',
      title: '清空数据库已排队',
      content: '请正常退出并重新启动 MineOps，重启前可以取消排队任务。',
      dedupeKey: 'settings:reset-scheduled',
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '排队清空数据库失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'settings:reset-error',
    })
  } finally {
    runtimeActionLoading.value = false
  }
}

/**
 * 取消全部已排队的下次启动维护任务。
 * @returns 取消完成后的 Promise
 */
async function cancelPendingMaintenance(): Promise<void> {
  runtimeActionLoading.value = true
  try {
    storageStatus.value = await cancelPendingDatabaseMaintenance()
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '取消离线维护失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'settings:maintenance-cancel-error',
    })
  } finally {
    runtimeActionLoading.value = false
  }
}
</script>

<template>
  <NSpin :show="loading">
    <section class="settings-page">
      <NFlex align="center" justify="end" class="settings-header">
        <AppFormActions
          :dirty="dirty"
          :submitting="saving"
          @discard="discardChanges"
          @submit="saveSettings"
        >
          <template #before>
            <NButton type="warning" :loading="saving" @click="resetCurrentCategory">
              恢复当前分类
            </NButton>
          </template>
        </AppFormActions>
      </NFlex>

      <NSelect
        :value="settingsSearchTarget"
        :options="settingsSearchOptions"
        filterable
        clearable
        placeholder="搜索设置：代理、主题、SSH、告警、更新…"
        @update:value="jumpToSettingsCategory"
      />

      <NAlert v-if="settingsError && !draft" type="error" title="Settings 加载失败">
        <NFlex align="center" justify="space-between">
          <span>{{ String(settingsError) }}</span>
          <NButton size="small" @click="settings.load">重试</NButton>
        </NFlex>
      </NAlert>
      <NAlert
        v-if="auxiliaryError"
        closable
        type="warning"
        title="部分运行状态不可用"
        @close="auxiliaryError = ''"
      >
        {{ auxiliaryError }}
      </NAlert>

      <NTabs v-if="draft" v-model:value="activeCategory" type="line" animated>
        <NTabPane name="general" tab="通用">
          <NForm label-placement="left" label-width="160">
            <AppFormField label="语言">
              <NSelect v-model:value="draft.general.language" :options="languageOptions" />
            </AppFormField>
            <AppFormField label="开机启动">
              <NCheckbox v-model:checked="draft.general.launchAtStartup"
                >登录系统后自动启动 MineOps</NCheckbox
              >
            </AppFormField>
            <AppFormField label="窗口关闭行为">
              <NSelect
                v-model:value="draft.general.closeBehavior"
                :options="closeBehaviorOptions"
              />
            </AppFormField>
            <AppFormField label="时间格式">
              <NSelect v-model:value="draft.general.timeFormat" :options="timeFormatOptions" />
            </AppFormField>
            <AppFormField label="GPU 硬件加速" help="重启生效">
              <NCheckbox v-model:checked="draft.general.hardwareAcceleration"
                >启用 GPU 硬件加速渲染</NCheckbox
              >
            </AppFormField>
            <AppFormField label="更新通道">
              <NSelect
                v-model:value="draft.general.updateChannel"
                :options="updateChannelOptions"
              />
            </AppFormField>
            <AppFormField label="自动检查更新">
              <NCheckbox v-model:checked="draft.general.autoCheckUpdates"
                >启动后检查当前通道的新版本</NCheckbox
              >
            </AppFormField>
            <AppFormField
              label="自动更新策略"
              help="自动下载只会校验并准备更新；实际替换程序前始终需要确认重启。"
            >
              <NSelect v-model:value="draft.general.updatePolicy" :options="updatePolicyOptions" />
            </AppFormField>
            <NAlert
              v-if="desktopUpdateStatus"
              :type="
                desktopUpdateStatus.lastErrorMessage
                  ? 'warning'
                  : desktopUpdateStatus.updateAvailable
                    ? 'success'
                    : 'info'
              "
              :title="
                desktopUpdateStatus.updateAvailable ? '发现可用 Desktop 更新' : 'Desktop 版本检查'
              "
            >
              <NFlex vertical :size="12">
                <NDescriptions bordered :columns="1" label-placement="left">
                  <NDescriptionsItem label="当前版本">
                    {{ desktopUpdateStatus.currentVersion }}
                  </NDescriptionsItem>
                  <NDescriptionsItem label="更新通道">
                    {{ desktopUpdateStatus.channel }} ·
                    {{ desktopUpdateStatus.enabled ? '自动检查已启用' : '自动检查已禁用' }}
                  </NDescriptionsItem>
                  <NDescriptionsItem label="当前状态">
                    {{ desktopUpdatePhaseLabel(desktopUpdateStatus.phase) }}
                  </NDescriptionsItem>
                  <NDescriptionsItem label="运行平台">
                    {{ desktopUpdateStatus.platform }}/{{ desktopUpdateStatus.architecture }} ·
                    {{ desktopUpdateStatus.installSupported ? '支持自更新' : '仅支持手动更新' }}
                  </NDescriptionsItem>
                  <NDescriptionsItem label="最新版本">
                    {{ desktopUpdateStatus.latestVersion || '尚未检查' }}
                    <NTag v-if="desktopUpdateStatus.updateAvailable" type="success" size="small">
                      可更新
                    </NTag>
                  </NDescriptionsItem>
                  <NDescriptionsItem label="最近检查">
                    {{ formatDateTime(desktopUpdateStatus.lastCheckedAt) }}
                  </NDescriptionsItem>
                  <NDescriptionsItem v-if="desktopUpdateStatus.publishedAt" label="发布时间">
                    {{ formatDateTime(desktopUpdateStatus.publishedAt) }}
                  </NDescriptionsItem>
                  <NDescriptionsItem v-if="desktopUpdateStatus.releaseURL" label="Release URL">
                    {{ desktopUpdateStatus.releaseURL }}
                  </NDescriptionsItem>
                  <NDescriptionsItem label="Manifest URL">
                    {{ desktopUpdateStatus.manifestURL }}
                  </NDescriptionsItem>
                  <NDescriptionsItem v-if="desktopUpdateStatus.notes" label="Release Notes">
                    <span style="white-space: pre-wrap">{{ desktopUpdateStatus.notes }}</span>
                  </NDescriptionsItem>
                </NDescriptions>
                <NText v-if="desktopUpdateStatus.installMessage" depth="3">
                  {{ desktopUpdateStatus.installMessage }}
                </NText>
                <NFlex
                  v-if="
                    desktopUpdateStatus.phase === 'downloading' ||
                    desktopUpdateStatus.phase === 'verifying'
                  "
                  vertical
                  :size="6"
                >
                  <NProgress
                    type="line"
                    :percentage="Math.round(desktopUpdateStatus.progress * 100)"
                    :status="desktopUpdateStatus.lastErrorMessage ? 'error' : 'default'"
                  />
                  <NText depth="3">
                    {{ formatBytes(desktopUpdateStatus.downloadedBytes) }} /
                    {{
                      desktopUpdateStatus.totalBytes > 0
                        ? formatBytes(desktopUpdateStatus.totalBytes)
                        : '未知大小'
                    }}
                  </NText>
                </NFlex>
                <NText v-if="desktopUpdateStatus.lastErrorMessage" type="error">
                  {{ desktopUpdateStatus.lastErrorCode }} ·
                  {{ desktopUpdateStatus.lastErrorMessage }}
                </NText>
                <NFlex align="center" wrap>
                  <NButton
                    :loading="desktopUpdateLoading || desktopUpdateStatus.running"
                    :disabled="desktopUpdateStatus.phase === 'downloading'"
                    @click="checkDesktopUpdate"
                  >
                    立即检查
                  </NButton>
                  <NButton
                    v-if="
                      desktopUpdateStatus.updateAvailable && !desktopUpdateStatus.readyToRestart
                    "
                    type="primary"
                    :loading="desktopUpdateLoading"
                    :disabled="!desktopUpdateStatus.installSupported || desktopUpdateStatus.running"
                    @click="prepareDesktopUpdate"
                  >
                    下载并准备安装
                  </NButton>
                  <NButton
                    v-if="
                      desktopUpdateStatus.phase === 'downloading' ||
                      desktopUpdateStatus.phase === 'verifying'
                    "
                    type="warning"
                    @click="cancelDesktopUpdateDownload"
                  >
                    取消下载
                  </NButton>
                  <NButton
                    v-if="desktopUpdateStatus.readyToRestart"
                    type="primary"
                    @click="applyDesktopUpdate"
                  >
                    重启并安装
                  </NButton>
                  <NButton v-if="desktopUpdateStatus.releaseURL" @click="openDesktopRelease">
                    打开 GitHub Release
                  </NButton>
                </NFlex>
              </NFlex>
            </NAlert>
            <NAlert type="default" title="全局快捷键">
              <NText>
                {{ shortcutModifier }} + , 打开 Settings · {{ shortcutModifier }} + Shift + O 打开
                Operations · {{ shortcutModifier }} + Shift + T 切换主题套装 ·
                {{ shortcutModifier }} + B 折叠或展开侧栏。输入框、编辑器和 Terminal
                获得焦点时不会截获快捷键。
              </NText>
            </NAlert>
          </NForm>
        </NTabPane>

        <NTabPane name="theme" tab="主题">
          <NForm label-placement="left" label-width="160">
            <AppFormField
              label="主题套装"
              help="套装会统一调整页面底色、面板、边框、编辑器与终端的基础配色。"
            >
              <div class="theme-preset-grid">
                <button
                  v-for="option in themePresetOptions"
                  :key="option.value"
                  type="button"
                  class="theme-preset-card"
                  :class="{ 'theme-preset-card--active': draft.theme.preset === option.value }"
                  @click="selectThemePreset(option.value)"
                >
                  <span class="theme-preset-swatches" aria-hidden="true">
                    <span
                      v-for="colour in option.colours"
                      :key="colour"
                      :style="{ background: colour }"
                    />
                  </span>
                  <strong>{{ option.label }}</strong>
                  <small>{{ option.description }}</small>
                </button>
              </div>
            </AppFormField>
            <AppFormField label="模式">
              <NSelect v-model:value="draft.theme.mode" :options="themeOptions" />
            </AppFormField>
            <AppFormField label="强调色" help="强调色不会改变成功、警告和危险状态色。">
              <NSelect v-model:value="draft.theme.accent" :options="accentOptions" />
            </AppFormField>
            <AppFormField label="背景模式">
              <NSelect
                v-model:value="draft.theme.backgroundMode"
                :options="backgroundModeOptions"
              />
            </AppFormField>
            <AppFormField v-if="draft.theme.backgroundMode === 'color'" label="背景颜色">
              <NColorPicker v-model:value="draft.theme.backgroundColor" :show-alpha="false" />
            </AppFormField>
            <template v-if="draft.theme.backgroundMode === 'image'">
              <AppFormField
                label="用户图片"
                help="图片复制到受控数据目录；原始外部路径不会保存到 Settings。"
              >
                <NFlex align="center">
                  <NButton :loading="backgroundActionLoading" @click="chooseBackgroundImage"
                    >选择并复制图片…</NButton
                  >
                  <NButton
                    v-if="backgroundResource?.configured"
                    :loading="backgroundActionLoading"
                    @click="resetBackground"
                    >恢复默认背景</NButton
                  >
                  <NTag v-if="backgroundResource?.available" type="success">{{
                    formatBytes(backgroundResource.bytes ?? 0)
                  }}</NTag>
                  <NTag v-else-if="backgroundResource?.configured" type="warning">{{
                    backgroundResource.reason
                  }}</NTag>
                </NFlex>
              </AppFormField>
              <AppFormField label="图片适配">
                <NSelect
                  v-model:value="draft.theme.backgroundFit"
                  :options="backgroundFitOptions"
                />
              </AppFormField>
              <AppFormField label="背景透明度">
                <NSlider
                  v-model:value="draft.theme.backgroundOpacity"
                  :min="0"
                  :max="1"
                  :step="0.05"
                />
              </AppFormField>
              <AppFormField label="遮罩强度">
                <NSlider
                  v-model:value="draft.theme.overlayStrength"
                  :min="0"
                  :max="1"
                  :step="0.05"
                />
              </AppFormField>
              <AppFormField label="模糊度（px）">
                <NSlider v-model:value="draft.theme.blurPixels" :min="0" :max="40" :step="1" />
              </AppFormField>
            </template>
            <AppFormField v-else-if="draft.theme.backgroundMode === 'color'" label="背景透明度">
              <NSlider
                v-model:value="draft.theme.backgroundOpacity"
                :min="0"
                :max="1"
                :step="0.05"
              />
            </AppFormField>
            <AppFormField label="面板透明度">
              <NSlider v-model:value="draft.theme.panelOpacity" :min="0.65" :max="1" :step="0.05" />
            </AppFormField>
            <AppFormField label="高对比度">
              <NCheckbox v-model:checked="draft.theme.highContrast">增强边框与焦点可见性</NCheckbox>
            </AppFormField>
          </NForm>
        </NTabPane>

        <NTabPane name="paths" tab="目录">
          <NForm label-placement="left" label-width="160">
            <AppFormField
              label="服务器目录"
              help="MineOps 数据目录下的受控相对路径，用作本地上传文件选择器的默认目录。"
            >
              <NInput v-model:value="draft.paths.serversDirectory" placeholder="MineOps/Servers" />
            </AppFormField>
            <AppFormField
              label="下载目录"
              help="MineOps 数据目录下的受控相对路径，用于 Artifact 缓存和远程文件下载默认位置。"
            >
              <NInput
                v-model:value="draft.paths.downloadsDirectory"
                placeholder="MineOps/Downloads"
              />
            </AppFormField>
            <NAlert type="info">
              两个目录均禁止绝对路径和 `..`。保存后由 MineOps
              在受控数据目录内按需创建，不会写入外部配置文件。
            </NAlert>
          </NForm>
        </NTabPane>

        <NTabPane name="mirrors" tab="镜像">
          <NForm label-placement="left" label-width="160">
            <NAlert type="info" title="分类镜像覆盖规则">
              Provider 专用下载源优先于这里的分类镜像。Java 和 Minecraft spark 使用填写的 HTTPS Base
              URL；Minecraft 会按 Provider 追加
              /mojang、/papermc、/purpur、/fabric、/quilt、/spigot、/bungeecord、/forge 或
              /neoforge。
            </NAlert>
            <AppFormField label="Java">
              <NInput
                v-model:value="draft.mirrors.java"
                placeholder="https://mirror.example/adoptium"
              />
            </AppFormField>
            <AppFormField label="Minecraft">
              <NInput
                v-model:value="draft.mirrors.minecraft"
                placeholder="https://mirror.example/minecraft"
              />
            </AppFormField>
            <AppFormField label="Minecraft spark">
              <NInput
                v-model:value="draft.mirrors.spark"
                placeholder="https://mirror.example/spark-jenkins"
              />
            </AppFormField>
          </NForm>
        </NTabPane>

        <NTabPane name="downloads" tab="下载与代理">
          <NForm label-placement="left" label-width="190">
            <AppFormField label="请求超时（秒）">
              <NInputNumber v-model:value="draft.downloads.timeoutSeconds" :min="1" :max="3600" />
            </AppFormField>
            <AppFormField label="整体超时（秒）">
              <NInputNumber
                v-model:value="draft.downloads.overallTimeoutSeconds"
                :min="draft.downloads.timeoutSeconds"
                :max="86400"
              />
            </AppFormField>
            <AppFormField label="有限重试次数">
              <NInputNumber v-model:value="draft.downloads.retries" :min="0" :max="3" />
            </AppFormField>
            <AppFormField label="重试退避（秒）">
              <NInputNumber
                v-model:value="draft.downloads.retryBackoffSeconds"
                :min="0"
                :max="300"
              />
            </AppFormField>
            <AppFormField label="下载并发">
              <NInputNumber v-model:value="draft.downloads.concurrency" :min="1" :max="16" />
            </AppFormField>
            <AppFormField label="单 Artifact 上限 MiB" help="完整性校验不可关闭。">
              <NInputNumber v-model:value="draft.downloads.maxArtifactMiB" :min="1" />
            </AppFormField>
            <AppFormField label="缓存容量 MiB">
              <NInputNumber
                v-model:value="draft.downloads.cacheCapacityMiB"
                :min="draft.downloads.maxArtifactMiB"
              />
            </AppFormField>
            <AppFormField label="带宽上限 KiB/s" help="0 表示不限制。">
              <NInputNumber v-model:value="draft.downloads.bandwidthLimitKiB" :min="0" />
            </AppFormField>
            <AppFormField label="自动清理">
              <NCheckbox v-model:checked="draft.downloads.autoCleanup"
                >超过容量时优先清理旧缓存</NCheckbox
              >
            </AppFormField>

            <NText tag="h3">Proxy</NText>
            <AppFormField label="代理模式">
              <NSelect v-model:value="draft.downloads.proxy.mode" :options="proxyModeOptions" />
            </AppFormField>
            <template v-if="['http', 'https', 'socks5'].includes(draft.downloads.proxy.mode)">
              <AppFormField label="Proxy Host" required
                ><NInput v-model:value="draft.downloads.proxy.host"
              /></AppFormField>
              <AppFormField label="Proxy Port" required
                ><NInputNumber v-model:value="draft.downloads.proxy.port" :min="1" :max="65535"
              /></AppFormField>
              <AppFormField
                label="绕过列表"
                help="支持精确 Host、.example.com、*.example.com 和 *。"
              >
                <NDynamicTags v-model:value="draft.downloads.proxy.bypass" />
              </AppFormField>
              <AppFormField label="代理用户名"
                ><NInput v-model:value="proxyUsername" autocomplete="off"
              /></AppFormField>
              <AppFormField
                label="代理密码"
                help="密码只写入 SQLCipher Credential 记录，不进入 Settings JSON。"
              >
                <NFlex :wrap="false">
                  <NInput
                    v-model:value="proxyPassword"
                    type="password"
                    show-password-on="click"
                    autocomplete="new-password"
                  />
                  <NButton
                    type="primary"
                    :loading="downloadActionLoading"
                    :disabled="!proxyUsername || !proxyPassword"
                    @click="saveProxyAuth"
                    >保存凭据</NButton
                  >
                  <NButton
                    v-if="proxyCredential?.configured"
                    :loading="downloadActionLoading"
                    @click="removeProxyAuth"
                    >清除</NButton
                  >
                </NFlex>
              </AppFormField>
              <NAlert v-if="proxyCredential?.configured" type="success"
                >已配置加密代理凭据：{{ proxyCredential.username }}</NAlert
              >
            </template>

            <NFlex align="center" justify="space-between">
              <NText tag="h3">下载源注册表</NText>
              <NFlex>
                <NButton @click="addDownloadMirror">添加镜像</NButton>
                <NButton :loading="downloadActionLoading" @click="checkSources"
                  >保存并检查全部启用源</NButton
                >
              </NFlex>
            </NFlex>
            <div
              v-for="(source, sourceIndex) in draft.downloads.sources"
              :key="sourceIndex"
              class="download-source-card"
            >
              <NFlex align="center" justify="space-between">
                <NFlex align="center">
                  <NCheckbox v-model:checked="source.enabled" />
                  <NText strong>{{ source.name }}</NText>
                  <NTag :type="source.official ? 'success' : 'warning'" :bordered="false">{{
                    source.official ? '官方源' : '镜像'
                  }}</NTag>
                  <NTag :bordered="false">{{ source.category }}</NTag>
                </NFlex>
                <NFlex>
                  <NInputNumber
                    v-model:value="source.priority"
                    :min="0"
                    size="small"
                    style="width: 110px"
                  />
                  <NButton
                    v-if="!source.official"
                    type="error"
                    size="small"
                    @click="removeDownloadMirror(sourceIndex)"
                    >删除</NButton
                  >
                </NFlex>
              </NFlex>
              <template v-if="source.official">
                <NInput v-model:value="source.baseURL" disabled />
                <NText depth="3"
                  >Provider: {{ source.provider }} · Probe: {{ source.probeURL }}</NText
                >
              </template>
              <template v-else>
                <NFlex :wrap="false">
                  <NInput v-model:value="source.category" placeholder="Category" />
                  <NInput
                    v-model:value="source.provider"
                    placeholder="Provider: mojang/papermc/purpur/fabric/quilt/adoptium"
                  />
                  <NInput v-model:value="source.name" placeholder="镜像名称" />
                </NFlex>
                <NInput v-model:value="source.baseURL" placeholder="Base URL" />
                <NInput v-model:value="source.probeURL" placeholder="Probe URL" />
              </template>
              <template
                v-for="status in sourceStatuses.filter((item) => item.name === source.name)"
                :key="status.name"
              >
                <NAlert
                  :type="status.available ? 'success' : 'error'"
                  :title="
                    status.available
                      ? `可用 · ${(status.latency / 1_000_000).toFixed(0)}ms`
                      : (status.errorCode ?? '不可用')
                  "
                >
                  {{ status.available ? status.url : status.errorMessage }}
                </NAlert>
              </template>
            </div>

            <NFlex align="center" justify="space-between" class="cache-summary">
              <div>
                <NText tag="h3">下载缓存</NText>
                <div v-if="cacheStatus">
                  <NText depth="3"
                    >{{ cacheStatus.directory }} · {{ cacheStatus.files }} 文件 ·
                    {{ formatBytes(cacheStatus.bytes) }} /
                    {{ formatBytes(cacheStatus.capacityBytes) }}</NText
                  >
                </div>
              </div>
              <NButton type="warning" :loading="downloadActionLoading" @click="clearCache"
                >清理缓存</NButton
              >
            </NFlex>
          </NForm>
        </NTabPane>

        <NTabPane name="logging" tab="日志">
          <NForm label-placement="left" label-width="160">
            <NAlert v-if="logStatus" type="info" title="当前运行日志">
              {{ logStatus.runtimeMode }} · {{ logStatus.directory }} · {{ logStatus.files }} 个文件
              · {{ formatBytes(logStatus.bytes) }}
            </NAlert>
            <AppFormField label="等级">
              <NSelect v-model:value="draft.logging.level" :options="logLevelOptions" />
            </AppFormField>
            <AppFormField label="单文件 MiB">
              <NInputNumber v-model:value="draft.logging.maxFileMiB" :min="1" />
            </AppFormField>
            <AppFormField label="保留天数">
              <NInputNumber v-model:value="draft.logging.retentionDays" :min="1" />
            </AppFormField>
            <AppFormField label="总容量 MiB">
              <NInputNumber v-model:value="draft.logging.totalCapacityMiB" :min="1" />
            </AppFormField>
            <AppFormField label="日志操作">
              <NFlex>
                <NButton :loading="runtimeActionLoading" @click="openLogs">打开日志目录</NButton>
                <NButton type="warning" :loading="runtimeActionLoading" @click="clearLogs"
                  >立即清理历史日志</NButton
                >
                <NButton type="primary" :loading="runtimeActionLoading" @click="exportDiagnostics"
                  >导出脱敏诊断包</NButton
                >
              </NFlex>
            </AppFormField>
          </NForm>
        </NTabPane>

        <NTabPane name="monitoring" tab="监控">
          <NForm label-placement="left" label-width="180">
            <AppFormField label="采集间隔（秒）">
              <NInputNumber v-model:value="draft.monitoring.intervalSeconds" :min="1" />
            </AppFormField>
            <AppFormField label="实时推送节流（毫秒）">
              <NInputNumber v-model:value="draft.monitoring.realtimeThrottleMillis" :min="100" />
            </AppFormField>
            <AppFormField label="离线判定（秒）">
              <NInputNumber v-model:value="draft.monitoring.offlineAfterSeconds" :min="2" />
            </AppFormField>
            <AppFormField label="原始数据保留（天）">
              <NInputNumber v-model:value="draft.monitoring.rawRetentionDays" :min="1" />
            </AppFormField>
            <AppFormField label="分钟数据保留（天）">
              <NInputNumber v-model:value="draft.monitoring.minuteRetentionDays" :min="1" />
            </AppFormField>
            <AppFormField label="小时数据保留（天）">
              <NInputNumber v-model:value="draft.monitoring.hourRetentionDays" :min="1" />
            </AppFormField>
            <AppFormField label="指标数据库容量（MiB）">
              <NInputNumber v-model:value="draft.monitoring.databaseCapacityMiB" :min="128" />
            </AppFormField>
            <AppFormField label="最小磁盘余量（MiB）">
              <NInputNumber v-model:value="draft.monitoring.minimumFreeDiskMiB" :min="64" />
            </AppFormField>
            <AppFormField label="后台维护周期（秒）">
              <NInputNumber v-model:value="draft.monitoring.maintenanceIntervalSeconds" :min="30" />
            </AppFormField>
            <AppFormField label="Spark 采集周期（秒）">
              <NInputNumber v-model:value="draft.monitoring.sparkIntervalSeconds" :min="5" />
            </AppFormField>
            <AppFormField label="Profiler 默认时长（秒）">
              <NInputNumber
                v-model:value="draft.monitoring.profilerDefaultSeconds"
                :min="10"
                :max="3600"
              />
            </AppFormField>
            <AppFormField label="报告隐私确认">
              <NCheckbox v-model:checked="draft.monitoring.reportPrivacyConfirmation"
                >生成或打开 Spark 报告前始终确认</NCheckbox
              >
            </AppFormField>
            <AppFormField label="告警冷却（秒）">
              <NInputNumber v-model:value="draft.monitoring.alertCooldownSeconds" :min="0" />
            </AppFormField>
            <AppFormField label="告警通知">
              <NSelect
                v-model:value="draft.monitoring.alertNotifications"
                multiple
                :options="alertNotificationOptions"
              />
            </AppFormField>
            <AppFormField
              label="静默开始"
              help="开始和结束必须同时填写；按本地时间生效，并支持 22:00 → 07:00 跨午夜。"
            >
              <NInput v-model:value="draft.monitoring.quietHoursStart" placeholder="22:00" />
            </AppFormField>
            <AppFormField label="静默结束" help="开始和结束都留空时不启用静默时段。">
              <NInput v-model:value="draft.monitoring.quietHoursEnd" placeholder="07:00" />
            </AppFormField>
          </NForm>
        </NTabPane>

        <NTabPane name="firewall" tab="防火墙">
          <NForm label-placement="left" label-width="180">
            <NAlert type="info" title="远程防火墙规则安全边界">
              MineOps 只会删除自身创建、没有其他 Server
              引用且当前不再使用的规则。用户原有规则不会自动删除。
            </NAlert>
            <AppFormField
              label="Provider"
              help="auto 会优先选择远程主机上已启用的 UFW 或 firewalld。"
            >
              <NSelect v-model:value="draft.firewall.provider" :options="firewallProviderOptions" />
            </AppFormField>
            <AppFormField label="新 Server 默认策略">
              <NSelect
                v-model:value="draft.firewall.defaultPolicy"
                :options="firewallPolicyOptions"
              />
            </AppFormField>
            <AppFormField label="安装阶段放行">
              <NCheckbox v-model:checked="draft.firewall.autoOpenOnInstall">
                Server 文件安装完成后、首次启动初始化前自动放行默认端口
              </NCheckbox>
            </AppFormField>
            <AppFormField label="配置端口同步">
              <NCheckbox v-model:checked="draft.firewall.syncOnPortChange">
                保存或恢复 server.properties 时先放行新端口
              </NCheckbox>
            </AppFormField>
            <AppFormField label="回收原端口">
              <NCheckbox
                v-model:checked="draft.firewall.removeOldPort"
                :disabled="!draft.firewall.syncOnPortChange"
              >
                新配置保存后安全移除 MineOps 管理的原端口规则
              </NCheckbox>
            </AppFormField>
            <AppFormField label="危险操作确认">
              <NCheckbox v-model:checked="draft.firewall.requireDestructiveConfirm">
                自动移除旧端口前必须确认端口切换计划
              </NCheckbox>
            </AppFormField>
          </NForm>
        </NTabPane>

        <NTabPane name="layout" tab="布局">
          <NForm label-placement="left" label-width="180">
            <AppFormField label="侧栏可见">
              <NCheckbox v-model:checked="draft.layout.sidebarVisible">显示主导航侧栏</NCheckbox>
            </AppFormField>
            <AppFormField label="侧栏折叠">
              <NCheckbox v-model:checked="draft.layout.sidebarCollapsed"
                >默认使用紧凑侧栏</NCheckbox
              >
            </AppFormField>
            <AppFormField label="侧栏宽度">
              <NInputNumber v-model:value="draft.layout.sidebarWidth" :min="180" :max="360" />
            </AppFormField>
            <AppFormField label="TopBar">
              <NCheckbox v-model:checked="draft.layout.topBarVisible">显示顶栏</NCheckbox>
            </AppFormField>
            <AppFormField label="BottomBar">
              <NCheckbox v-model:checked="draft.layout.bottomBarVisible">显示底栏</NCheckbox>
            </AppFormField>
          </NForm>
        </NTabPane>

        <NTabPane name="ssh" tab="SSH">
          <NForm label-placement="left" label-width="190">
            <AppFormField label="默认端口">
              <NInputNumber v-model:value="draft.ssh.defaultPort" :min="1" :max="65535" />
            </AppFormField>
            <AppFormField label="连接超时（秒）">
              <NInputNumber v-model:value="draft.ssh.connectTimeoutSec" :min="1" :max="300" />
            </AppFormField>
            <AppFormField label="握手超时（秒）">
              <NInputNumber v-model:value="draft.ssh.handshakeTimeoutSec" :min="1" :max="300" />
            </AppFormField>
            <AppFormField label="KeepAlive（秒）" help="设为 0 表示关闭主动 KeepAlive。">
              <NInputNumber v-model:value="draft.ssh.keepAliveSec" :min="0" :max="3600" />
            </AppFormField>
            <AppFormField label="最大连续失败次数">
              <NInputNumber v-model:value="draft.ssh.maxFailures" :min="1" :max="100" />
            </AppFormField>
            <AppFormField label="自动重连">
              <NCheckbox v-model:checked="draft.ssh.autoReconnect"
                >连接中断后按退避策略重连</NCheckbox
              >
            </AppFormField>
            <AppFormField label="重连次数">
              <NInputNumber v-model:value="draft.ssh.reconnectAttempts" :min="0" :max="20" />
            </AppFormField>
            <AppFormField label="重连退避（秒）">
              <NInputNumber v-model:value="draft.ssh.reconnectBackoffSec" :min="0" :max="300" />
            </AppFormField>
            <AppFormField label="认证优先级" help="依次尝试私钥、Agent 和密码；不允许重复。">
              <NSelect v-model:value="draft.ssh.authPriority" multiple :options="sshAuthOptions" />
            </AppFormField>
            <AppFormField label="默认主机密钥策略" help="不提供全局忽略全部指纹的选项。">
              <NSelect
                v-model:value="draft.ssh.defaultHostKeyPolicy"
                :options="hostKeyPolicyOptions"
              />
            </AppFormField>
            <AppFormField label="SSH 压缩">
              <NCheckbox v-model:checked="draft.ssh.compression">默认启用压缩</NCheckbox>
            </AppFormField>
            <AppFormField label="PTY Terminal Type">
              <NInput v-model:value="draft.ssh.ptyTerminalType" placeholder="xterm-256color" />
            </AppFormField>
            <AppFormField label="默认字符编码">
              <NSelect
                v-model:value="draft.ssh.defaultEncoding"
                :options="[{ label: 'UTF-8', value: 'UTF-8' }]"
              />
            </AppFormField>
            <AppFormField
              label="默认 Jump Host"
              help="通过已保存 SSH Session 建立 ProxyJump；目标连接仍独立执行主机密钥校验。"
            >
              <NSelect
                :value="draft.ssh.defaultJumpHostID ?? null"
                clearable
                filterable
                :options="jumpHostOptions"
                placeholder="不使用 Jump Host"
                @update:value="draft.ssh.defaultJumpHostID = $event ?? undefined"
              />
            </AppFormField>
          </NForm>
          <KnownHostsPanel />
        </NTabPane>

        <NTabPane name="terminal" tab="Terminal">
          <NForm label-placement="left" label-width="190">
            <AppFormField
              label="等宽字体"
              help="仅显示浏览器确认可用的字体，并始终保留系统回退链。"
            >
              <NSelect
                v-model:value="draft.terminal.fontFamily"
                filterable
                tag
                :options="fontOptions"
              />
            </AppFormField>
            <AppFormField label="字号">
              <NInputNumber v-model:value="draft.terminal.fontSize" :min="8" :max="40" />
            </AppFormField>
            <AppFormField label="字间距">
              <NInputNumber v-model:value="draft.terminal.letterSpacing" :min="-2" :max="10" />
            </AppFormField>
            <AppFormField label="行高">
              <NInputNumber
                v-model:value="draft.terminal.lineHeight"
                :min="1"
                :max="2"
                :step="0.05"
              />
            </AppFormField>
            <AppFormField
              label="保存行数"
              help="范围 100–200000；数值越大，长时间会话占用的前端内存越多。"
            >
              <NInputNumber
                v-model:value="draft.terminal.scrollback"
                :min="100"
                :max="200000"
                :step="1000"
              />
            </AppFormField>
            <AppFormField label="光标样式">
              <NSelect v-model:value="draft.terminal.cursorStyle" :options="cursorStyleOptions" />
            </AppFormField>
            <AppFormField label="光标宽度">
              <NInputNumber v-model:value="draft.terminal.cursorWidth" :min="1" :max="5" />
            </AppFormField>
            <AppFormField label="光标闪烁">
              <NCheckbox v-model:checked="draft.terminal.cursorBlink">启用光标闪烁</NCheckbox>
            </AppFormField>
            <AppFormField label="配色预设">
              <NSelect v-model:value="draft.terminal.themePreset" :options="terminalThemeOptions" />
            </AppFormField>
            <template v-if="draft.terminal.themePreset === 'custom'">
              <AppFormField label="前景色">
                <NColorPicker v-model:value="draft.terminal.foreground" :show-alpha="false" />
              </AppFormField>
              <AppFormField label="背景色">
                <NColorPicker v-model:value="draft.terminal.background" :show-alpha="false" />
              </AppFormField>
              <AppFormField label="光标色">
                <NColorPicker v-model:value="draft.terminal.cursor" :show-alpha="false" />
              </AppFormField>
              <AppFormField label="选择区域">
                <NColorPicker v-model:value="draft.terminal.selection" :show-alpha="false" />
              </AppFormField>
              <AppFormField label="ANSI 16 色">
                <div class="ansi-grid">
                  <label v-for="(label, index) in ansiLabels" :key="label" class="ansi-colour">
                    <NText depth="3">{{ label }}</NText>
                    <NColorPicker
                      :value="draft.terminal.ansiColours[index] ?? null"
                      :show-alpha="false"
                      @update:value="draft.terminal.ansiColours[index] = $event ?? ''"
                    />
                  </label>
                </div>
              </AppFormField>
            </template>
            <AppFormField label="复制行为">
              <NCheckbox v-model:checked="draft.terminal.copyOnSelect"
                >选择文本后自动复制</NCheckbox
              >
            </AppFormField>
            <AppFormField label="自动聚焦">
              <NCheckbox v-model:checked="draft.terminal.autoFocus"
                >切换标签时聚焦 Terminal</NCheckbox
              >
            </AppFormField>
            <AppFormField label="Bell">
              <NSelect v-model:value="draft.terminal.bellStyle" :options="bellStyleOptions" />
            </AppFormField>
          </NForm>
        </NTabPane>

        <NTabPane name="storage" tab="存储与安全">
          <NForm label-placement="left" label-width="190">
            <AppFormField label="数据库路径">
              <NInput :value="storageStatus?.databasePath ?? '正在读取…'" readonly />
            </AppFormField>
            <AppFormField label="默认备份目录">
              <NInput v-model:value="draft.storage.backupDirectory" />
            </AppFormField>
            <AppFormField label="备份完整性校验">
              <NCheckbox v-model:checked="draft.storage.verifyBackupAfterCreate"
                >创建后保持完整性校验</NCheckbox
              >
            </AppFormField>
            <AppFormField label="危险操作确认">
              <NFlex vertical>
                <NCheckbox v-model:checked="draft.storage.requireRestoreConfirm"
                  >恢复前必须二次确认</NCheckbox
                >
                <NCheckbox v-model:checked="draft.storage.requireKeyRotateConfirm"
                  >密钥轮换前必须二次确认</NCheckbox
                >
              </NFlex>
            </AppFormField>
            <AppFormField label="数据目录">
              <NButton :loading="runtimeActionLoading" @click="openData">打开数据目录</NButton>
            </AppFormField>
            <AppFormField label="备份与恢复">
              <NFlex>
                <NButton type="primary" :loading="runtimeActionLoading" @click="createBackup"
                  >创建加密备份…</NButton
                >
                <NButton type="warning" :loading="runtimeActionLoading" @click="scheduleRestore"
                  >校验并排队恢复…</NButton
                >
                <NButton type="warning" :loading="runtimeActionLoading" @click="scheduleKeyRotation"
                  >排队密钥轮换</NButton
                >
              </NFlex>
            </AppFormField>
            <AppFormField
              label="存储整理"
              help="删除监控历史或缩短保留期后，SQLite 的空闲页不会自动还给磁盘，文件会一直停在历史最大体积。整理会在下次启动、数据库打开前离线执行 VACUUM。"
            >
              <NFlex align="center" wrap>
                <NButton type="warning" :loading="runtimeActionLoading" @click="scheduleVacuum"
                  >排队存储整理…</NButton
                >
                <NText depth="3">
                  当前文件 {{ formatBytes(storageStatus?.databaseBytes ?? 0) }}
                </NText>
              </NFlex>
            </AppFormField>
            <AppFormField label="清空数据库">
              <NButton type="error" :loading="runtimeActionLoading" @click="resetDatabase"
                >清空数据</NButton
              >
            </AppFormField>
            <NAlert
              v-if="
                storageStatus?.pendingMaintenance.restorePending ||
                storageStatus?.pendingMaintenance.keyRotationPending ||
                storageStatus?.pendingMaintenance.vacuumPending ||
                storageStatus?.pendingMaintenance.resetPending
              "
              :type="storageStatus.pendingMaintenance.resetPending ? 'error' : 'warning'"
              title="存在下次启动维护任务"
            >
              恢复：{{ storageStatus.pendingMaintenance.restorePending ? '已排队' : '无' }} ·
              密钥轮换：{{
                storageStatus.pendingMaintenance.keyRotationPending ? '已排队' : '无'
              }}
              · 存储整理：{{ storageStatus.pendingMaintenance.vacuumPending ? '已排队' : '无' }} ·
              清空数据库：{{ storageStatus.pendingMaintenance.resetPending ? '已排队' : '无' }}
              <NText v-if="storageStatus.pendingMaintenance.resetPending" type="error">
                清空数据库会取消同时排队的其他维护任务。
              </NText>
              <NButton size="small" @click="cancelPendingMaintenance">取消排队任务</NButton>
            </NAlert>
          </NForm>
        </NTabPane>
      </NTabs>
    </section>
  </NSpin>
</template>

<style scoped>
.settings-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
}

.theme-preset-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(132px, 1fr));
  gap: 10px;
  width: 100%;
}

.theme-preset-card {
  min-width: 0;
  padding: 10px;
  border: 1px solid var(--border-default);
  border-radius: 10px;
  color: var(--text-primary);
  background: color-mix(in srgb, var(--surface-panel) 88%, transparent);
  cursor: pointer;
  text-align: left;
  transition:
    border-color 160ms ease,
    box-shadow 160ms ease,
    transform 160ms ease;
}

.theme-preset-card:hover {
  border-color: var(--border-strong);
  transform: translateY(-1px);
}

.theme-preset-card--active {
  border-color: var(--accent-primary);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--accent-primary) 22%, transparent);
}

.theme-preset-card strong,
.theme-preset-card small {
  display: block;
}

.theme-preset-card small {
  margin-top: 3px;
  color: var(--text-muted);
}

.theme-preset-swatches {
  display: flex;
  height: 28px;
  margin-bottom: 8px;
  overflow: hidden;
  border: 1px solid rgb(15 23 42 / 10%);
  border-radius: 6px;
}

.theme-preset-swatches > span {
  flex: 1;
}

.ansi-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
  gap: 12px;
}

.ansi-colour {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.download-source-card {
  margin-bottom: 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px;
  border: 1px solid var(--border-default);
  border-radius: 8px;
  background: rgb(255 255 255 / 4%);
}

.cache-summary {
  margin-top: 18px;
}
</style>
