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
import { computed, onMounted, ref, watch } from 'vue'

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
import { hasMessage } from '../../locales/runtime'
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

useUnsavedGuard(
  'settings',
  computed(() => locale.t('settings.unsavedGuard')),
  dirty,
)

/**
 * 把枚举值翻译成当前界面语言的标签。
 * @param prefix - 文案键前缀
 * @param value - 枚举值
 * @returns 本地化标签，未登记的值原样返回
 */
function enumLabel(prefix: string, value: string): string {
  const key = `${prefix}.${value}`
  return hasMessage(key) ? locale.t(key) : value
}

/**
 * 按前缀把一组枚举值构造成本地化下拉选项。
 * @param prefix - 文案键前缀
 * @param values - 枚举值列表
 * @returns 下拉选项数组
 */
function enumOptions(prefix: string, values: string[]): Array<{ label: string; value: string }> {
  return values.map((value) => ({ label: enumLabel(prefix, value), value }))
}

watch(
  () => draft.value,
  (value) => {
    if (value) settings.previewTheme(value)
  },
  { deep: true },
)

const settingsCategories = [
  'general',
  'theme',
  'paths',
  'mirrors',
  'downloads',
  'logging',
  'monitoring',
  'firewall',
  'layout',
  'ssh',
  'terminal',
  'storage',
]
const themePresetColours: Record<string, string[]> = {
  mineops: ['#f4f6f8', '#ffffff', '#059669'],
  forest: ['#e4f2e5', '#f7fff7', '#16a34a'],
  ocean: ['#e3f2ff', '#f7fbff', '#0284c7'],
  amethyst: ['#f1e6ff', '#fcf8ff', '#7c3aed'],
  graphite: ['#dff8f5', '#f4fffd', '#0891b2'],
  sunset: ['#fff0e4', '#fffaf5', '#e11d48'],
}
const languageOptions = computed(() => enumOptions('settings.language', ['zh-CN', 'en-US']))
const themeOptions = computed(() => enumOptions('settings.themeMode', ['system', 'light', 'dark']))
const themePresetOptions = computed(() =>
  Object.entries(themePresetColours).map(([value, colours]) => ({
    label: locale.t(`shell.themePreset.${value}` as Parameters<typeof locale.t>[0]),
    description: locale.t(`settings.preset.${value}.description` as Parameters<typeof locale.t>[0]),
    value,
    colours,
  })),
)
const accentOptions = [
  { label: 'Emerald', value: 'emerald' },
  { label: 'Amber', value: 'amber' },
  { label: 'Azure', value: 'azure' },
  { label: 'Violet', value: 'violet' },
  { label: 'Rose', value: 'rose' },
]
const backgroundModeOptions = computed(() =>
  enumOptions('settings.backgroundMode', ['theme', 'color', 'image']),
)
const backgroundFitOptions = computed(() =>
  enumOptions('settings.backgroundFit', ['cover', 'contain', 'center', 'tile']),
)
const closeBehaviorOptions = computed(() =>
  enumOptions('settings.closeBehavior', ['quit', 'minimize']),
)
const timeFormatOptions = computed(() => enumOptions('settings.timeFormat', ['24h', '12h']))
const updateChannelOptions = [
  { label: 'Stable', value: 'stable' },
  { label: 'Beta', value: 'beta' },
]
const updatePolicyOptions = computed(() =>
  enumOptions('settings.updatePolicy', ['notify', 'download', 'prompt_restart']),
)
const settingsSearchOptions = computed(() => enumOptions('settings.search', settingsCategories))

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

const alertNotificationOptions = computed(() =>
  enumOptions('settings.alertNotification', ['desktop', 'sound']),
)
const logLevelOptions = ['debug', 'info', 'warn', 'error'].map((value) => ({
  label: value.toUpperCase(),
  value,
}))
const sshAuthOptions = computed(() =>
  enumOptions('settings.sshAuth', ['private_key', 'agent', 'password']),
)
const hostKeyPolicyOptions = computed(() =>
  enumOptions('settings.hostKeyPolicy', ['strict', 'trust_on_first_use']),
)
const firewallProviderOptions = computed(() =>
  enumOptions('settings.firewallProvider', ['auto', 'ufw', 'firewalld', 'disabled']),
)
const firewallPolicyOptions = computed(() =>
  enumOptions('servers.firewall', ['automatic', 'prompt', 'disabled']),
)
const terminalThemeOptions = computed(() =>
  enumOptions('settings.terminalTheme', ['semantic', 'nord', 'solarized', 'custom']),
)
const cursorStyleOptions = computed(() =>
  enumOptions('settings.cursorStyle', ['block', 'underline', 'bar']),
)
const bellStyleOptions = computed(() =>
  enumOptions('settings.bellStyle', ['none', 'sound', 'visual', 'both']),
)
const proxyModeOptions = computed(() =>
  enumOptions('settings.proxyMode', ['none', 'system', 'http', 'https', 'socks5']),
)
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
      title: locale.t('settings.updateStatusFailed'),
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
      title: status.updateAvailable
        ? locale.t('settings.updateFound')
        : locale.t('settings.updateUpToDate'),
      content: status.updateAvailable
        ? `${status.currentVersion} → ${status.latestVersion || locale.t('settings.updateUnknownVersion')}`
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
      title: locale.t('settings.updateCheckFailed'),
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
      title: locale.t('settings.updateReady'),
      content: `${desktopUpdateStatus.value.currentVersion} → ${desktopUpdateStatus.value.latestVersion}`,
      dedupeKey: 'desktop-update:ready',
    })
  } catch (error) {
    await refreshDesktopUpdateStatus()
    notifications.push({
      kind: 'error',
      title: locale.t('settings.updatePrepareFailed'),
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
      title: locale.t('settings.updateCancelRequested'),
      content: locale.t('settings.updateCancelContent'),
      dedupeKey: 'desktop-update:cancelled',
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: locale.t('settings.updateCancelFailed'),
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
        title: locale.t('settings.updateRestartTitle'),
        content: locale.t('settings.updateRestartUnsaved', { items: labels }),
        impact: locale.t('settings.updateRestartUnsavedImpact'),
        positiveText: locale.t('settings.updateRestartUnsavedConfirm'),
        danger: true,
      })
      if (!confirmed) return
    } else {
      const confirmed = await interactions.confirm({
        title: locale.t('settings.updateRestartTitle'),
        content: locale.t('settings.updateRestartContent', {
          version:
            desktopUpdateStatus.value?.latestVersion || locale.t('settings.updatePreparedVersion'),
        }),
        impact: locale.t('settings.updateRestartImpact'),
        positiveText: locale.t('settings.updateRestartConfirm'),
      })
      if (!confirmed) return
    }
    desktopUpdateStatus.value = await restartDesktopUpdate(unsavedItems.length > 0)
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: locale.t('settings.updateRestartFailed'),
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
  return enumLabel('settings.updatePhase', phase)
}

/**
 * 按当前时间格式设置渲染时间文本。
 * @param value - ISO 时间字符串，可为空
 * @returns 本地时间文本，空值返回尚未检查
 */
function formatDateTime(value?: string | null): string {
  return value ? locale.formatDateTime(value) : locale.t('settings.notChecked')
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
      title: locale.t('settings.downloadStateFailed'),
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
      title: locale.t('settings.proxySaved'),
      dedupeKey: 'downloads:proxy-saved',
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: locale.t('settings.proxySaveFailed'),
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
      title: locale.t('settings.proxyClearFailed'),
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
      title: locale.t('settings.sourceCheckFailed'),
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
    title: locale.t('settings.cacheClearTitle'),
    content: locale.t('settings.cacheClearContent'),
    positiveText: locale.t('settings.cacheClearConfirm'),
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
      title: locale.t('settings.cacheClearFailed'),
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
    name: locale.t('settings.customMirror', {
      index: draft.value.downloads.sources.length + 1,
    }),
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
    notifications.push({
      kind: 'success',
      title: locale.t('settings.saved'),
      dedupeKey: 'settings:saved',
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: locale.t('settings.saveFailed'),
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
    title: locale.t('settings.discardTitle'),
    content: locale.t('settings.discardContent'),
    positiveText: locale.t('settings.discardConfirm'),
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
    title: locale.t('settings.resetTitle'),
    content: locale.t('settings.resetContent'),
    objectLabel: activeCategory.value,
    impact: locale.t('settings.resetImpact'),
    positiveText: locale.t('settings.resetConfirm'),
    danger: true,
  })
  if (!confirmed) return
  try {
    await settings.resetCategory(activeCategory.value)
    notifications.push({
      kind: 'success',
      title: locale.t('settings.resetSucceeded'),
      dedupeKey: `settings:reset:${activeCategory.value}`,
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: locale.t('settings.resetFailed'),
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
    title: locale.t('settings.chooseBackgroundTitle'),
    directory: draft.value.paths.backgroundImage,
    filters: [
      { DisplayName: locale.t('settings.imageFilter'), Pattern: '*.png;*.jpg;*.jpeg;*.webp' },
    ],
    buttonText: locale.t('settings.chooseImageButton'),
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
      title: locale.t('settings.backgroundImported'),
      dedupeKey: 'settings:background-imported',
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: locale.t('settings.backgroundImportFailed'),
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
      title: locale.t('settings.backgroundResetFailed'),
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
    title: locale.t('settings.logClearTitle'),
    content: locale.t('settings.logClearContent'),
    positiveText: locale.t('settings.logClearConfirm'),
    danger: true,
  })
  if (!confirmed) return
  runtimeActionLoading.value = true
  try {
    logStatus.value = await clearArchivedLogs()
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: locale.t('settings.logClearFailed'),
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
      title: locale.t('settings.logOpenFailed'),
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
      title: locale.t('settings.dataOpenFailed'),
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
    title: locale.t('settings.diagnosticTitle'),
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
      title: locale.t('settings.diagnosticExported'),
      content: locale.t('settings.diagnosticContent', {
        path: result.path,
        size: formatBytes(result.sizeBytes),
        files: result.logFiles,
      }),
      dedupeKey: 'settings:diagnostic-exported',
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: locale.t('settings.diagnosticFailed'),
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
    title: locale.t('settings.backupTitle'),
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
      title: locale.t('settings.backupCreated'),
      content: `${backup.path} · ${formatBytes(backup.bytes)}`,
      dedupeKey: 'settings:backup-created',
    })
    await refreshStorageState()
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: locale.t('settings.backupFailed'),
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
    title: locale.t('settings.restoreSelectTitle'),
    filters: [{ DisplayName: 'MineOps Backup', Pattern: '*.mineops-backup' }],
  })
  if (!selected) return
  const confirmed = await interactions.confirm({
    title: locale.t('settings.restoreTitle'),
    content: locale.t('settings.restoreContent'),
    impact: locale.t('settings.restoreImpact'),
    positiveText: locale.t('settings.restoreConfirm'),
    danger: true,
  })
  if (!confirmed) return
  runtimeActionLoading.value = true
  try {
    storageStatus.value = await schedulePortableRestore(selected)
    notifications.push({
      kind: 'warning',
      title: locale.t('settings.restoreScheduled'),
      content: locale.t('settings.restartHint'),
      dedupeKey: 'settings:restore-scheduled',
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: locale.t('settings.restoreFailed'),
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
    title: locale.t('settings.keyRotationTitle'),
    content: locale.t('settings.keyRotationContent'),
    positiveText: locale.t('settings.keyRotationConfirm'),
    danger: true,
  })
  if (!confirmed) return
  runtimeActionLoading.value = true
  try {
    storageStatus.value = await scheduleDatabaseKeyRotation()
    notifications.push({
      kind: 'warning',
      title: locale.t('settings.keyRotationScheduled'),
      content: locale.t('settings.restartHint'),
      dedupeKey: 'settings:key-rotation-scheduled',
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: locale.t('settings.keyRotationFailed'),
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
    title: locale.t('settings.vacuumTitle'),
    content: locale.t('settings.vacuumContent'),
    objectLabel: storageStatus.value?.databasePath ?? '',
    impact: locale.t('settings.vacuumImpact', { size: formatBytes(allocated) }),
    positiveText: locale.t('settings.vacuumConfirm'),
  })
  if (!confirmed) return
  runtimeActionLoading.value = true
  try {
    storageStatus.value = await scheduleDatabaseVacuum()
    notifications.push({
      kind: 'warning',
      title: locale.t('settings.vacuumScheduled'),
      content: locale.t('settings.vacuumRestartHint'),
      dedupeKey: 'settings:vacuum-scheduled',
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: locale.t('settings.vacuumFailed'),
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
    title: locale.t('settings.resetDatabaseAckTitle'),
    content: '',
    objectLabel: storageStatus.value?.dataDirectory ?? '',
    impact: locale.t('settings.resetDatabaseAckImpact'),
    positiveText: locale.t('settings.resetDatabaseAckConfirm'),
    danger: true,
  })
  if (!acknowledged) return
  const confirmed = await interactions.confirm({
    title: locale.t('settings.resetDatabaseTitle'),
    content: locale.t('settings.resetDatabaseContent'),
    objectLabel: storageStatus.value?.databasePath ?? '',
    impact: locale.t('settings.resetDatabaseImpact'),
    positiveText: locale.t('settings.resetDatabaseConfirm'),
    danger: true,
  })
  if (!confirmed) return
  runtimeActionLoading.value = true
  try {
    storageStatus.value = await scheduleDatabaseReset()
    notifications.push({
      kind: 'warning',
      title: locale.t('settings.resetDatabaseScheduled'),
      content: locale.t('settings.resetDatabaseRestartHint'),
      dedupeKey: 'settings:reset-scheduled',
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: locale.t('settings.resetDatabaseFailed'),
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
      title: locale.t('settings.maintenanceCancelFailed'),
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
              {{ locale.t('settings.resetCategoryButton') }}
            </NButton>
          </template>
        </AppFormActions>
      </NFlex>

      <NSelect
        :value="settingsSearchTarget"
        :options="settingsSearchOptions"
        filterable
        clearable
        :placeholder="locale.t('settings.searchPlaceholder')"
        @update:value="jumpToSettingsCategory"
      />

      <NAlert v-if="settingsError && !draft" type="error" :title="locale.t('settings.loadFailed')">
        <NFlex align="center" justify="space-between">
          <span>{{ String(settingsError) }}</span>
          <NButton size="small" @click="settings.load">{{ locale.t('common.retry') }}</NButton>
        </NFlex>
      </NAlert>
      <NAlert
        v-if="auxiliaryError"
        closable
        type="warning"
        :title="locale.t('settings.auxiliaryTitle')"
        @close="auxiliaryError = ''"
      >
        {{ auxiliaryError }}
      </NAlert>

      <NTabs v-if="draft" v-model:value="activeCategory" type="line" animated>
        <NTabPane name="general" :tab="locale.t('settings.tab.general')">
          <NForm label-placement="left" label-width="160">
            <AppFormField :label="locale.t('settings.field.language')">
              <NSelect v-model:value="draft.general.language" :options="languageOptions" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.launchAtStartup')">
              <NCheckbox v-model:checked="draft.general.launchAtStartup">
                {{ locale.t('settings.launchAtStartupHint') }}
              </NCheckbox>
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.closeBehavior')">
              <NSelect
                v-model:value="draft.general.closeBehavior"
                :options="closeBehaviorOptions"
              />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.timeFormat')">
              <NSelect v-model:value="draft.general.timeFormat" :options="timeFormatOptions" />
            </AppFormField>
            <AppFormField
              :label="locale.t('settings.field.hardwareAcceleration')"
              :help="locale.t('settings.hardwareAccelerationHelp')"
            >
              <NCheckbox v-model:checked="draft.general.hardwareAcceleration">
                {{ locale.t('settings.hardwareAccelerationHint') }}
              </NCheckbox>
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.updateChannel')">
              <NSelect
                v-model:value="draft.general.updateChannel"
                :options="updateChannelOptions"
              />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.autoCheckUpdates')">
              <NCheckbox v-model:checked="draft.general.autoCheckUpdates">
                {{ locale.t('settings.autoCheckUpdatesHint') }}
              </NCheckbox>
            </AppFormField>
            <AppFormField
              :label="locale.t('settings.field.updatePolicy')"
              :help="locale.t('settings.updatePolicyHelp')"
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
                desktopUpdateStatus.updateAvailable
                  ? locale.t('settings.updateAvailableTitle')
                  : locale.t('settings.updateCheckTitle')
              "
            >
              <NFlex vertical :size="12">
                <NDescriptions bordered :columns="1" label-placement="left">
                  <NDescriptionsItem :label="locale.t('settings.currentVersion')">
                    {{ desktopUpdateStatus.currentVersion }}
                  </NDescriptionsItem>
                  <NDescriptionsItem :label="locale.t('settings.field.updateChannel')">
                    {{ desktopUpdateStatus.channel }} ·
                    {{
                      desktopUpdateStatus.enabled
                        ? locale.t('settings.autoCheckEnabled')
                        : locale.t('settings.autoCheckDisabled')
                    }}
                  </NDescriptionsItem>
                  <NDescriptionsItem :label="locale.t('settings.currentPhase')">
                    {{ desktopUpdatePhaseLabel(desktopUpdateStatus.phase) }}
                  </NDescriptionsItem>
                  <NDescriptionsItem :label="locale.t('settings.platform')">
                    {{ desktopUpdateStatus.platform }}/{{ desktopUpdateStatus.architecture }} ·
                    {{
                      desktopUpdateStatus.installSupported
                        ? locale.t('settings.selfUpdateSupported')
                        : locale.t('settings.manualUpdateOnly')
                    }}
                  </NDescriptionsItem>
                  <NDescriptionsItem :label="locale.t('settings.latestVersion')">
                    {{ desktopUpdateStatus.latestVersion || locale.t('settings.notChecked') }}
                    <NTag v-if="desktopUpdateStatus.updateAvailable" type="success" size="small">
                      {{ locale.t('settings.updatableTag') }}
                    </NTag>
                  </NDescriptionsItem>
                  <NDescriptionsItem :label="locale.t('settings.lastChecked')">
                    {{ formatDateTime(desktopUpdateStatus.lastCheckedAt) }}
                  </NDescriptionsItem>
                  <NDescriptionsItem
                    v-if="desktopUpdateStatus.publishedAt"
                    :label="locale.t('settings.publishedAt')"
                  >
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
                        : locale.t('settings.unknownSize')
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
                    {{ locale.t('settings.checkNow') }}
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
                    {{ locale.t('settings.downloadAndPrepare') }}
                  </NButton>
                  <NButton
                    v-if="
                      desktopUpdateStatus.phase === 'downloading' ||
                      desktopUpdateStatus.phase === 'verifying'
                    "
                    type="warning"
                    @click="cancelDesktopUpdateDownload"
                  >
                    {{ locale.t('settings.cancelDownload') }}
                  </NButton>
                  <NButton
                    v-if="desktopUpdateStatus.readyToRestart"
                    type="primary"
                    @click="applyDesktopUpdate"
                  >
                    {{ locale.t('settings.restartAndInstall') }}
                  </NButton>
                  <NButton v-if="desktopUpdateStatus.releaseURL" @click="openDesktopRelease">
                    {{ locale.t('settings.openRelease') }}
                  </NButton>
                </NFlex>
              </NFlex>
            </NAlert>
            <NAlert type="default" :title="locale.t('settings.shortcutsTitle')">
              <NText>
                {{ locale.t('settings.shortcutsContent', { modifier: shortcutModifier }) }}
              </NText>
            </NAlert>
          </NForm>
        </NTabPane>

        <NTabPane name="theme" :tab="locale.t('settings.tab.theme')">
          <NForm label-placement="left" label-width="160">
            <AppFormField
              :label="locale.t('settings.field.themePreset')"
              :help="locale.t('settings.themePresetHelp')"
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
            <AppFormField :label="locale.t('settings.field.themeMode')">
              <NSelect v-model:value="draft.theme.mode" :options="themeOptions" />
            </AppFormField>
            <AppFormField
              :label="locale.t('settings.field.accent')"
              :help="locale.t('settings.accentHelp')"
            >
              <NSelect v-model:value="draft.theme.accent" :options="accentOptions" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.backgroundMode')">
              <NSelect
                v-model:value="draft.theme.backgroundMode"
                :options="backgroundModeOptions"
              />
            </AppFormField>
            <AppFormField
              v-if="draft.theme.backgroundMode === 'color'"
              :label="locale.t('settings.field.backgroundColor')"
            >
              <NColorPicker v-model:value="draft.theme.backgroundColor" :show-alpha="false" />
            </AppFormField>
            <template v-if="draft.theme.backgroundMode === 'image'">
              <AppFormField
                :label="locale.t('settings.field.backgroundImage')"
                :help="locale.t('settings.backgroundImageHelp')"
              >
                <NFlex align="center">
                  <NButton :loading="backgroundActionLoading" @click="chooseBackgroundImage">
                    {{ locale.t('settings.chooseAndCopyImage') }}
                  </NButton>
                  <NButton
                    v-if="backgroundResource?.configured"
                    :loading="backgroundActionLoading"
                    @click="resetBackground"
                  >
                    {{ locale.t('settings.resetBackground') }}
                  </NButton>
                  <NTag v-if="backgroundResource?.available" type="success">{{
                    formatBytes(backgroundResource.bytes ?? 0)
                  }}</NTag>
                  <NTag v-else-if="backgroundResource?.configured" type="warning">{{
                    backgroundResource.reason
                  }}</NTag>
                </NFlex>
              </AppFormField>
              <AppFormField :label="locale.t('settings.field.backgroundFit')">
                <NSelect
                  v-model:value="draft.theme.backgroundFit"
                  :options="backgroundFitOptions"
                />
              </AppFormField>
              <AppFormField :label="locale.t('settings.field.backgroundOpacity')">
                <NSlider
                  v-model:value="draft.theme.backgroundOpacity"
                  :min="0"
                  :max="1"
                  :step="0.05"
                />
              </AppFormField>
              <AppFormField :label="locale.t('settings.field.overlayStrength')">
                <NSlider
                  v-model:value="draft.theme.overlayStrength"
                  :min="0"
                  :max="1"
                  :step="0.05"
                />
              </AppFormField>
              <AppFormField :label="locale.t('settings.field.blurPixels')">
                <NSlider v-model:value="draft.theme.blurPixels" :min="0" :max="40" :step="1" />
              </AppFormField>
            </template>
            <AppFormField
              v-else-if="draft.theme.backgroundMode === 'color'"
              :label="locale.t('settings.field.backgroundOpacity')"
            >
              <NSlider
                v-model:value="draft.theme.backgroundOpacity"
                :min="0"
                :max="1"
                :step="0.05"
              />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.panelOpacity')">
              <NSlider v-model:value="draft.theme.panelOpacity" :min="0.65" :max="1" :step="0.05" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.highContrast')">
              <NCheckbox v-model:checked="draft.theme.highContrast">
                {{ locale.t('settings.highContrastHint') }}
              </NCheckbox>
            </AppFormField>
          </NForm>
        </NTabPane>

        <NTabPane name="paths" :tab="locale.t('settings.tab.paths')">
          <NForm label-placement="left" label-width="160">
            <AppFormField
              :label="locale.t('settings.field.serversDirectory')"
              :help="locale.t('settings.serversDirectoryHelp')"
            >
              <NInput v-model:value="draft.paths.serversDirectory" placeholder="MineOps/Servers" />
            </AppFormField>
            <AppFormField
              :label="locale.t('settings.field.downloadsDirectory')"
              :help="locale.t('settings.downloadsDirectoryHelp')"
            >
              <NInput
                v-model:value="draft.paths.downloadsDirectory"
                placeholder="MineOps/Downloads"
              />
            </AppFormField>
            <NAlert type="info">{{ locale.t('settings.pathsNotice') }}</NAlert>
          </NForm>
        </NTabPane>

        <NTabPane name="mirrors" :tab="locale.t('settings.tab.mirrors')">
          <NForm label-placement="left" label-width="160">
            <NAlert type="info" :title="locale.t('settings.mirrorsNoticeTitle')">
              {{ locale.t('settings.mirrorsNoticeContent') }}
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

        <NTabPane name="downloads" :tab="locale.t('settings.tab.downloads')">
          <NForm label-placement="left" label-width="190">
            <AppFormField :label="locale.t('settings.field.timeoutSeconds')">
              <NInputNumber v-model:value="draft.downloads.timeoutSeconds" :min="1" :max="3600" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.overallTimeoutSeconds')">
              <NInputNumber
                v-model:value="draft.downloads.overallTimeoutSeconds"
                :min="draft.downloads.timeoutSeconds"
                :max="86400"
              />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.retries')">
              <NInputNumber v-model:value="draft.downloads.retries" :min="0" :max="3" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.retryBackoffSeconds')">
              <NInputNumber
                v-model:value="draft.downloads.retryBackoffSeconds"
                :min="0"
                :max="300"
              />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.concurrency')">
              <NInputNumber v-model:value="draft.downloads.concurrency" :min="1" :max="16" />
            </AppFormField>
            <AppFormField
              :label="locale.t('settings.field.maxArtifactMiB')"
              :help="locale.t('settings.maxArtifactHelp')"
            >
              <NInputNumber v-model:value="draft.downloads.maxArtifactMiB" :min="1" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.cacheCapacityMiB')">
              <NInputNumber
                v-model:value="draft.downloads.cacheCapacityMiB"
                :min="draft.downloads.maxArtifactMiB"
              />
            </AppFormField>
            <AppFormField
              :label="locale.t('settings.field.bandwidthLimitKiB')"
              :help="locale.t('settings.bandwidthLimitHelp')"
            >
              <NInputNumber v-model:value="draft.downloads.bandwidthLimitKiB" :min="0" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.autoCleanup')">
              <NCheckbox v-model:checked="draft.downloads.autoCleanup">
                {{ locale.t('settings.autoCleanupHint') }}
              </NCheckbox>
            </AppFormField>

            <NText tag="h3">Proxy</NText>
            <AppFormField :label="locale.t('settings.field.proxyMode')">
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
                :label="locale.t('settings.field.proxyBypass')"
                :help="locale.t('settings.proxyBypassHelp')"
              >
                <NDynamicTags v-model:value="draft.downloads.proxy.bypass" />
              </AppFormField>
              <AppFormField :label="locale.t('settings.field.proxyUsername')">
                <NInput v-model:value="proxyUsername" autocomplete="off" />
              </AppFormField>
              <AppFormField
                :label="locale.t('settings.field.proxyPassword')"
                :help="locale.t('settings.proxyPasswordHelp')"
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
                  >
                    {{ locale.t('settings.saveCredential') }}
                  </NButton>
                  <NButton
                    v-if="proxyCredential?.configured"
                    :loading="downloadActionLoading"
                    @click="removeProxyAuth"
                  >
                    {{ locale.t('settings.clearCredential') }}
                  </NButton>
                </NFlex>
              </AppFormField>
              <NAlert v-if="proxyCredential?.configured" type="success">
                {{
                  locale.t('settings.proxyConfigured', { username: proxyCredential.username ?? '' })
                }}
              </NAlert>
            </template>

            <NFlex align="center" justify="space-between">
              <NText tag="h3">{{ locale.t('settings.sourceRegistry') }}</NText>
              <NFlex>
                <NButton @click="addDownloadMirror">{{ locale.t('settings.addMirror') }}</NButton>
                <NButton :loading="downloadActionLoading" @click="checkSources">
                  {{ locale.t('settings.checkSources') }}
                </NButton>
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
                    source.official
                      ? locale.t('settings.officialSource')
                      : locale.t('settings.mirrorSource')
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
                  >
                    {{ locale.t('common.delete') }}
                  </NButton>
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
                  <NInput
                    v-model:value="source.name"
                    :placeholder="locale.t('settings.mirrorNamePlaceholder')"
                  />
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
                      ? locale.t('settings.sourceAvailable', {
                          latency: (status.latency / 1_000_000).toFixed(0),
                        })
                      : (status.errorCode ?? locale.t('settings.sourceUnavailable'))
                  "
                >
                  {{ status.available ? status.url : status.errorMessage }}
                </NAlert>
              </template>
            </div>

            <NFlex align="center" justify="space-between" class="cache-summary">
              <div>
                <NText tag="h3">{{ locale.t('settings.downloadCache') }}</NText>
                <div v-if="cacheStatus">
                  <NText depth="3">
                    {{
                      locale.t('settings.cacheSummary', {
                        directory: cacheStatus.directory,
                        files: cacheStatus.files,
                        used: formatBytes(cacheStatus.bytes),
                        capacity: formatBytes(cacheStatus.capacityBytes),
                      })
                    }}
                  </NText>
                </div>
              </div>
              <NButton type="warning" :loading="downloadActionLoading" @click="clearCache">
                {{ locale.t('settings.clearCache') }}
              </NButton>
            </NFlex>
          </NForm>
        </NTabPane>

        <NTabPane name="logging" :tab="locale.t('settings.tab.logging')">
          <NForm label-placement="left" label-width="160">
            <NAlert v-if="logStatus" type="info" :title="locale.t('settings.runtimeLogTitle')">
              {{
                locale.t('settings.logSummary', {
                  mode: logStatus.runtimeMode,
                  directory: logStatus.directory,
                  files: logStatus.files,
                  size: formatBytes(logStatus.bytes),
                })
              }}
            </NAlert>
            <AppFormField :label="locale.t('settings.field.logLevel')">
              <NSelect v-model:value="draft.logging.level" :options="logLevelOptions" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.maxFileMiB')">
              <NInputNumber v-model:value="draft.logging.maxFileMiB" :min="1" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.retentionDays')">
              <NInputNumber v-model:value="draft.logging.retentionDays" :min="1" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.totalCapacityMiB')">
              <NInputNumber v-model:value="draft.logging.totalCapacityMiB" :min="1" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.logActions')">
              <NFlex>
                <NButton :loading="runtimeActionLoading" @click="openLogs">
                  {{ locale.t('settings.openLogDirectory') }}
                </NButton>
                <NButton type="warning" :loading="runtimeActionLoading" @click="clearLogs">
                  {{ locale.t('settings.clearArchivedLogs') }}
                </NButton>
                <NButton type="primary" :loading="runtimeActionLoading" @click="exportDiagnostics">
                  {{ locale.t('settings.exportDiagnostics') }}
                </NButton>
              </NFlex>
            </AppFormField>
          </NForm>
        </NTabPane>

        <NTabPane name="monitoring" :tab="locale.t('settings.tab.monitoring')">
          <NForm label-placement="left" label-width="180">
            <AppFormField :label="locale.t('settings.field.intervalSeconds')">
              <NInputNumber v-model:value="draft.monitoring.intervalSeconds" :min="1" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.realtimeThrottleMillis')">
              <NInputNumber v-model:value="draft.monitoring.realtimeThrottleMillis" :min="100" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.offlineAfterSeconds')">
              <NInputNumber v-model:value="draft.monitoring.offlineAfterSeconds" :min="2" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.rawRetentionDays')">
              <NInputNumber v-model:value="draft.monitoring.rawRetentionDays" :min="1" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.minuteRetentionDays')">
              <NInputNumber v-model:value="draft.monitoring.minuteRetentionDays" :min="1" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.hourRetentionDays')">
              <NInputNumber v-model:value="draft.monitoring.hourRetentionDays" :min="1" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.databaseCapacityMiB')">
              <NInputNumber v-model:value="draft.monitoring.databaseCapacityMiB" :min="128" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.minimumFreeDiskMiB')">
              <NInputNumber v-model:value="draft.monitoring.minimumFreeDiskMiB" :min="64" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.maintenanceIntervalSeconds')">
              <NInputNumber v-model:value="draft.monitoring.maintenanceIntervalSeconds" :min="30" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.sparkIntervalSeconds')">
              <NInputNumber v-model:value="draft.monitoring.sparkIntervalSeconds" :min="5" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.profilerDefaultSeconds')">
              <NInputNumber
                v-model:value="draft.monitoring.profilerDefaultSeconds"
                :min="10"
                :max="3600"
              />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.reportPrivacyConfirmation')">
              <NCheckbox v-model:checked="draft.monitoring.reportPrivacyConfirmation">
                {{ locale.t('settings.reportPrivacyHint') }}
              </NCheckbox>
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.alertCooldownSeconds')">
              <NInputNumber v-model:value="draft.monitoring.alertCooldownSeconds" :min="0" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.alertNotifications')">
              <NSelect
                v-model:value="draft.monitoring.alertNotifications"
                multiple
                :options="alertNotificationOptions"
              />
            </AppFormField>
            <AppFormField
              :label="locale.t('settings.field.quietHoursStart')"
              :help="locale.t('settings.quietHoursStartHelp')"
            >
              <NInput v-model:value="draft.monitoring.quietHoursStart" placeholder="22:00" />
            </AppFormField>
            <AppFormField
              :label="locale.t('settings.field.quietHoursEnd')"
              :help="locale.t('settings.quietHoursEndHelp')"
            >
              <NInput v-model:value="draft.monitoring.quietHoursEnd" placeholder="07:00" />
            </AppFormField>
          </NForm>
        </NTabPane>

        <NTabPane name="firewall" :tab="locale.t('settings.tab.firewall')">
          <NForm label-placement="left" label-width="180">
            <NAlert type="info" :title="locale.t('settings.firewallNoticeTitle')">
              {{ locale.t('settings.firewallNoticeContent') }}
            </NAlert>
            <AppFormField label="Provider" :help="locale.t('settings.firewallProviderHelp')">
              <NSelect v-model:value="draft.firewall.provider" :options="firewallProviderOptions" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.defaultPolicy')">
              <NSelect
                v-model:value="draft.firewall.defaultPolicy"
                :options="firewallPolicyOptions"
              />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.autoOpenOnInstall')">
              <NCheckbox v-model:checked="draft.firewall.autoOpenOnInstall">
                {{ locale.t('settings.autoOpenOnInstallHint') }}
              </NCheckbox>
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.syncOnPortChange')">
              <NCheckbox v-model:checked="draft.firewall.syncOnPortChange">
                {{ locale.t('settings.syncOnPortChangeHint') }}
              </NCheckbox>
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.removeOldPort')">
              <NCheckbox
                v-model:checked="draft.firewall.removeOldPort"
                :disabled="!draft.firewall.syncOnPortChange"
              >
                {{ locale.t('settings.removeOldPortHint') }}
              </NCheckbox>
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.requireDestructiveConfirm')">
              <NCheckbox v-model:checked="draft.firewall.requireDestructiveConfirm">
                {{ locale.t('settings.requireDestructiveConfirmHint') }}
              </NCheckbox>
            </AppFormField>
          </NForm>
        </NTabPane>

        <NTabPane name="layout" :tab="locale.t('settings.tab.layout')">
          <NForm label-placement="left" label-width="180">
            <AppFormField :label="locale.t('settings.field.sidebarVisible')">
              <NCheckbox v-model:checked="draft.layout.sidebarVisible">
                {{ locale.t('settings.sidebarVisibleHint') }}
              </NCheckbox>
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.sidebarCollapsed')">
              <NCheckbox v-model:checked="draft.layout.sidebarCollapsed">
                {{ locale.t('settings.sidebarCollapsedHint') }}
              </NCheckbox>
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.sidebarWidth')">
              <NInputNumber v-model:value="draft.layout.sidebarWidth" :min="180" :max="360" />
            </AppFormField>
            <AppFormField label="TopBar">
              <NCheckbox v-model:checked="draft.layout.topBarVisible">
                {{ locale.t('settings.topBarHint') }}
              </NCheckbox>
            </AppFormField>
            <AppFormField label="BottomBar">
              <NCheckbox v-model:checked="draft.layout.bottomBarVisible">
                {{ locale.t('settings.bottomBarHint') }}
              </NCheckbox>
            </AppFormField>
          </NForm>
        </NTabPane>

        <NTabPane name="ssh" :tab="locale.t('settings.tab.ssh')">
          <NForm label-placement="left" label-width="190">
            <AppFormField :label="locale.t('settings.field.defaultPort')">
              <NInputNumber v-model:value="draft.ssh.defaultPort" :min="1" :max="65535" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.connectTimeoutSec')">
              <NInputNumber v-model:value="draft.ssh.connectTimeoutSec" :min="1" :max="300" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.handshakeTimeoutSec')">
              <NInputNumber v-model:value="draft.ssh.handshakeTimeoutSec" :min="1" :max="300" />
            </AppFormField>
            <AppFormField
              :label="locale.t('settings.field.keepAliveSec')"
              :help="locale.t('settings.keepAliveHelp')"
            >
              <NInputNumber v-model:value="draft.ssh.keepAliveSec" :min="0" :max="3600" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.maxFailures')">
              <NInputNumber v-model:value="draft.ssh.maxFailures" :min="1" :max="100" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.autoReconnect')">
              <NCheckbox v-model:checked="draft.ssh.autoReconnect">
                {{ locale.t('settings.autoReconnectHint') }}
              </NCheckbox>
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.reconnectAttempts')">
              <NInputNumber v-model:value="draft.ssh.reconnectAttempts" :min="0" :max="20" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.reconnectBackoffSec')">
              <NInputNumber v-model:value="draft.ssh.reconnectBackoffSec" :min="0" :max="300" />
            </AppFormField>
            <AppFormField
              :label="locale.t('settings.field.authPriority')"
              :help="locale.t('settings.authPriorityHelp')"
            >
              <NSelect v-model:value="draft.ssh.authPriority" multiple :options="sshAuthOptions" />
            </AppFormField>
            <AppFormField
              :label="locale.t('settings.field.defaultHostKeyPolicy')"
              :help="locale.t('settings.defaultHostKeyPolicyHelp')"
            >
              <NSelect
                v-model:value="draft.ssh.defaultHostKeyPolicy"
                :options="hostKeyPolicyOptions"
              />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.sshCompression')">
              <NCheckbox v-model:checked="draft.ssh.compression">
                {{ locale.t('settings.sshCompressionHint') }}
              </NCheckbox>
            </AppFormField>
            <AppFormField label="PTY Terminal Type">
              <NInput v-model:value="draft.ssh.ptyTerminalType" placeholder="xterm-256color" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.defaultEncoding')">
              <NSelect
                v-model:value="draft.ssh.defaultEncoding"
                :options="[{ label: 'UTF-8', value: 'UTF-8' }]"
              />
            </AppFormField>
            <AppFormField
              :label="locale.t('settings.field.defaultJumpHost')"
              :help="locale.t('settings.defaultJumpHostHelp')"
            >
              <NSelect
                :value="draft.ssh.defaultJumpHostID ?? null"
                clearable
                filterable
                :options="jumpHostOptions"
                :placeholder="locale.t('settings.noJumpHost')"
                @update:value="draft.ssh.defaultJumpHostID = $event ?? undefined"
              />
            </AppFormField>
          </NForm>
          <KnownHostsPanel />
        </NTabPane>

        <NTabPane name="terminal" :tab="locale.t('settings.tab.terminal')">
          <NForm label-placement="left" label-width="190">
            <AppFormField
              :label="locale.t('settings.field.fontFamily')"
              :help="locale.t('settings.fontFamilyHelp')"
            >
              <NSelect
                v-model:value="draft.terminal.fontFamily"
                filterable
                tag
                :options="fontOptions"
              />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.fontSize')">
              <NInputNumber v-model:value="draft.terminal.fontSize" :min="8" :max="40" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.letterSpacing')">
              <NInputNumber v-model:value="draft.terminal.letterSpacing" :min="-2" :max="10" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.lineHeight')">
              <NInputNumber
                v-model:value="draft.terminal.lineHeight"
                :min="1"
                :max="2"
                :step="0.05"
              />
            </AppFormField>
            <AppFormField
              :label="locale.t('settings.field.scrollback')"
              :help="locale.t('settings.scrollbackHelp')"
            >
              <NInputNumber
                v-model:value="draft.terminal.scrollback"
                :min="100"
                :max="200000"
                :step="1000"
              />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.cursorStyle')">
              <NSelect v-model:value="draft.terminal.cursorStyle" :options="cursorStyleOptions" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.cursorWidth')">
              <NInputNumber v-model:value="draft.terminal.cursorWidth" :min="1" :max="5" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.cursorBlink')">
              <NCheckbox v-model:checked="draft.terminal.cursorBlink">
                {{ locale.t('settings.cursorBlinkHint') }}
              </NCheckbox>
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.themePresetTerminal')">
              <NSelect v-model:value="draft.terminal.themePreset" :options="terminalThemeOptions" />
            </AppFormField>
            <template v-if="draft.terminal.themePreset === 'custom'">
              <AppFormField :label="locale.t('settings.field.foreground')">
                <NColorPicker v-model:value="draft.terminal.foreground" :show-alpha="false" />
              </AppFormField>
              <AppFormField :label="locale.t('settings.field.background')">
                <NColorPicker v-model:value="draft.terminal.background" :show-alpha="false" />
              </AppFormField>
              <AppFormField :label="locale.t('settings.field.cursorColour')">
                <NColorPicker v-model:value="draft.terminal.cursor" :show-alpha="false" />
              </AppFormField>
              <AppFormField :label="locale.t('settings.field.selection')">
                <NColorPicker v-model:value="draft.terminal.selection" :show-alpha="false" />
              </AppFormField>
              <AppFormField :label="locale.t('settings.field.ansiColours')">
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
            <AppFormField :label="locale.t('settings.field.copyOnSelect')">
              <NCheckbox v-model:checked="draft.terminal.copyOnSelect">
                {{ locale.t('settings.copyOnSelectHint') }}
              </NCheckbox>
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.autoFocus')">
              <NCheckbox v-model:checked="draft.terminal.autoFocus">
                {{ locale.t('settings.autoFocusHint') }}
              </NCheckbox>
            </AppFormField>
            <AppFormField label="Bell">
              <NSelect v-model:value="draft.terminal.bellStyle" :options="bellStyleOptions" />
            </AppFormField>
          </NForm>
        </NTabPane>

        <NTabPane name="storage" :tab="locale.t('settings.tab.storage')">
          <NForm label-placement="left" label-width="190">
            <AppFormField :label="locale.t('settings.field.databasePath')">
              <NInput
                :value="storageStatus?.databasePath ?? locale.t('settings.databaseLoading')"
                readonly
              />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.backupDirectory')">
              <NInput v-model:value="draft.storage.backupDirectory" />
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.verifyBackup')">
              <NCheckbox v-model:checked="draft.storage.verifyBackupAfterCreate">
                {{ locale.t('settings.verifyBackupHint') }}
              </NCheckbox>
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.requireDestructiveConfirm')">
              <NFlex vertical>
                <NCheckbox v-model:checked="draft.storage.requireRestoreConfirm">
                  {{ locale.t('settings.requireRestoreConfirmHint') }}
                </NCheckbox>
                <NCheckbox v-model:checked="draft.storage.requireKeyRotateConfirm">
                  {{ locale.t('settings.requireKeyRotateConfirmHint') }}
                </NCheckbox>
              </NFlex>
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.dataDirectory')">
              <NButton :loading="runtimeActionLoading" @click="openData">
                {{ locale.t('settings.openDataDirectory') }}
              </NButton>
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.backupRestore')">
              <NFlex>
                <NButton type="primary" :loading="runtimeActionLoading" @click="createBackup">
                  {{ locale.t('settings.createBackup') }}
                </NButton>
                <NButton type="warning" :loading="runtimeActionLoading" @click="scheduleRestore">
                  {{ locale.t('settings.scheduleRestore') }}
                </NButton>
                <NButton
                  type="warning"
                  :loading="runtimeActionLoading"
                  @click="scheduleKeyRotation"
                >
                  {{ locale.t('settings.scheduleKeyRotation') }}
                </NButton>
              </NFlex>
            </AppFormField>
            <AppFormField
              :label="locale.t('settings.field.vacuum')"
              :help="locale.t('settings.vacuumHelp')"
            >
              <NFlex align="center" wrap>
                <NButton type="warning" :loading="runtimeActionLoading" @click="scheduleVacuum">
                  {{ locale.t('settings.scheduleVacuum') }}
                </NButton>
                <NText depth="3">
                  {{
                    locale.t('settings.currentDatabaseFile', {
                      size: formatBytes(storageStatus?.databaseBytes ?? 0),
                    })
                  }}
                </NText>
              </NFlex>
            </AppFormField>
            <AppFormField :label="locale.t('settings.field.resetDatabase')">
              <NButton type="error" :loading="runtimeActionLoading" @click="resetDatabase">
                {{ locale.t('settings.resetDatabaseButton') }}
              </NButton>
            </AppFormField>
            <NAlert
              v-if="
                storageStatus?.pendingMaintenance.restorePending ||
                storageStatus?.pendingMaintenance.keyRotationPending ||
                storageStatus?.pendingMaintenance.vacuumPending ||
                storageStatus?.pendingMaintenance.resetPending
              "
              :type="storageStatus.pendingMaintenance.resetPending ? 'error' : 'warning'"
              :title="locale.t('settings.pendingMaintenanceTitle')"
            >
              {{
                locale.t('settings.pendingMaintenanceSummary', {
                  restore: storageStatus.pendingMaintenance.restorePending
                    ? locale.t('settings.queued')
                    : locale.t('common.none'),
                  keyRotation: storageStatus.pendingMaintenance.keyRotationPending
                    ? locale.t('settings.queued')
                    : locale.t('common.none'),
                  vacuum: storageStatus.pendingMaintenance.vacuumPending
                    ? locale.t('settings.queued')
                    : locale.t('common.none'),
                  reset: storageStatus.pendingMaintenance.resetPending
                    ? locale.t('settings.queued')
                    : locale.t('common.none'),
                })
              }}
              <NText v-if="storageStatus.pendingMaintenance.resetPending" type="error">
                {{ locale.t('settings.resetCancelsOthers') }}
              </NText>
              <NButton size="small" @click="cancelPendingMaintenance">
                {{ locale.t('settings.cancelPendingMaintenance') }}
              </NButton>
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
