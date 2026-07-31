<script setup lang="ts">
import { Eye, Pencil, Play, Plus, RotateCcw, RotateCw, Square, Trash2 } from '@lucide/vue'
import {
  NButton,
  NCard,
  NCheckbox,
  NDynamicTags,
  NAlert,
  NFlex,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NModal,
  NSelect,
  NTag,
  NText,
  type DataTableColumns,
} from 'naive-ui'
import { computed, h, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'

import type { MinecraftServerInput } from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/models'
import type {
  JavaRuntime,
  MinecraftServer,
} from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import type { RemoteServerInspection } from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/service/models'
import { ApplicationError } from '../../services/api-client'
import { listJavaRuntimes } from '../../services/java-runtime-api'
import { restartServer, startServer, stopServer } from '../../services/lifecycle-api'
import {
  hardDeleteRemoteMinecraftServer,
  importRemoteMinecraftServer,
  inspectRemoteMinecraftServer,
} from '../../services/minecraft-server-api'
import { getLatestMetrics, subscribeMetricRealtime } from '../../services/metric-api'
import { subscribeOperationProgress } from '../../services/operation-api'
import AppDataTable from '../../shared/components/AppDataTable.vue'
import AppIcon from '../../shared/components/AppIcon.vue'
import { useInteractionStore } from '../../stores/interactions'
import { useLocaleStore } from '../../stores/locale'
import { useMinecraftServersStore } from '../../stores/minecraft-servers'
import { useNotificationStore } from '../../stores/notifications'
import { useSettingsStore } from '../../stores/settings'
import { useSSHSessionsStore } from '../../stores/ssh-sessions'
import ServerWizard from './ServerWizard.vue'

const store = useMinecraftServersStore()
const router = useRouter()
const sshSessions = useSSHSessionsStore()
const interactions = useInteractionStore()
const locale = useLocaleStore()
const notifications = useNotificationStore()
const settings = useSettingsStore()
const wizardVisible = ref(false)
const importVisible = ref(false)
const importLoading = ref(false)
const inspection = ref<RemoteServerInspection | null>(null)
const importSSHSessionID = ref('')
const importPath = ref('')
const importName = ref('')
const importType = ref('vanilla')
const importVersion = ref('')
const importJar = ref('')
const importJavaRuntimeID = ref('')
const importJavaRuntimes = ref<JavaRuntime[]>([])
const importJavaLoading = ref(false)
const importJavaError = ref<unknown>(null)
const importXmsMiB = ref(1024)
const importXmxMiB = ref(2048)
const editVisible = ref(false)
const editLoading = ref(false)
const editTarget = ref<MinecraftServer | null>(null)
const editDraft = ref<MinecraftServerInput | null>(null)
const hardDeleteVisible = ref(false)
const hardDeleteLoading = ref(false)
const hardDeleteTarget = ref<MinecraftServer | null>(null)
const confirmedDeleteName = ref('')
const confirmedDeletePath = ref('')
type LifecycleAction = 'start' | 'stop' | 'restart'
interface ServerUptime {
  seconds: number
  sampledAt: number
}

const lifecycleActions = ref<Record<string, LifecycleAction>>({})
const serverUptimes = ref<Record<string, ServerUptime>>({})
const uptimeNow = ref(Date.now())
const tableContainer = ref<HTMLElement | null>(null)
const tableWidth = ref(1280)
const pendingHardDeleteTargets = new Set<string>()
let unsubscribeOperation: (() => void) | null = null
let unsubscribeMetricRealtime: (() => void) | null = null
let uptimeTimer: ReturnType<typeof setInterval> | null = null
let tableResizeObserver: ResizeObserver | null = null
const firewallPolicies = ['automatic', 'prompt', 'disabled']
const firewallPolicyOptions = computed(() =>
  firewallPolicies.map((value) => ({
    label: locale.t(`servers.firewall.${value}` as Parameters<typeof locale.t>[0]),
    value,
  })),
)
const stateOptions = computed(() => [
  { label: locale.t('servers.allStates'), value: '' },
  ...['creating', 'installing', 'ready', 'running', 'stopped', 'failed', 'deleted'].map(
    (value) => ({ label: value, value }),
  ),
])
const serverTypeOptions = [
  'vanilla',
  'paper',
  'purpur',
  'spigot',
  'fabric',
  'forge',
  'neoforge',
  'quilt',
  'folia',
  'velocity',
  'waterfall',
  'bungeecord',
].map((value) => ({ label: value, value }))

type ServerColumn = DataTableColumns<MinecraftServer>[number]

const serverColumn = computed<ServerColumn>(() => ({
  title: 'Server',
  key: 'server',
  sorter: (left, right) => left.name.localeCompare(right.name),
  render: (row) =>
    h('div', { class: 'server-cell' }, [
      h('div', { class: 'server-cell__title' }, [
        h(
          NText,
          { strong: true, class: 'server-cell__name', title: row.name },
          {
            default: () => row.name,
          },
        ),
        ...(row.favourite
          ? [
              h(
                NTag,
                { size: 'small', type: 'warning', bordered: false },
                { default: () => locale.t('servers.favourite') },
              ),
            ]
          : []),
        ...(row.deletedAt
          ? [
              h(
                NTag,
                { size: 'small', bordered: false },
                { default: () => locale.t('servers.deleted') },
              ),
            ]
          : []),
      ]),
      ...(tableWidth.value < 680
        ? [
            h('div', { class: 'server-cell__meta' }, [
              h(NTag, { size: 'small', bordered: false }, { default: () => row.type }),
              h(NTag, { size: 'small', bordered: false }, { default: () => row.version }),
              ...(row.group
                ? [h(NTag, { size: 'small', bordered: false }, { default: () => row.group })]
                : []),
            ]),
          ]
        : []),
      h(
        NText,
        { depth: 3, class: 'server-cell__path', title: row.remotePath },
        { default: () => row.remotePath },
      ),
    ]),
}))

const runtimeColumn = computed<ServerColumn>(() => ({
  title: locale.t('servers.column.runtime'),
  key: 'runtime',
  width: 190,
  render: (row) =>
    h('div', { class: 'runtime-cell' }, [
      h(NText, null, {
        default: () => `${row.launchProfile.xmsMiB} / ${row.launchProfile.xmxMiB} MiB`,
      }),
      h(
        NText,
        { depth: 3, class: 'runtime-cell__jar' },
        {
          default: () => row.launchProfile.jarPath || locale.t('servers.noJar'),
        },
      ),
    ]),
}))

const stateColumn = computed<ServerColumn>(() => ({
  title: locale.t('servers.column.state'),
  key: 'state',
  width: 112,
  render: (row) =>
    h(
      NTag,
      {
        type:
          row.state === 'running'
            ? 'success'
            : row.state === 'failed'
              ? 'error'
              : row.state === 'deleted'
                ? 'default'
                : 'info',
        bordered: false,
      },
      { default: () => row.state },
    ),
}))

const actionsColumn = computed<ServerColumn>(() => ({
  title: locale.t('common.actions'),
  key: 'actions',
  render: (row) =>
    row.deletedAt
      ? h(
          NFlex,
          {
            wrap: true,
            class: ['row-actions', `row-actions--${serverActionCount(row)}`],
          },
          () => [
            h(
              NButton,
              { quaternary: true, circle: true, size: 'small', onClick: () => void restore(row) },
              {
                default: () => h(AppIcon, { icon: RotateCcw, label: locale.t('servers.restore') }),
              },
            ),
            h(
              NButton,
              {
                quaternary: true,
                circle: true,
                size: 'small',
                type: 'error',
                onClick: () => openHardDelete(row),
              },
              {
                default: () =>
                  h(AppIcon, { icon: Trash2, label: locale.t('servers.hardDeleteIcon') }),
              },
            ),
          ],
        )
      : h(
          NFlex,
          {
            wrap: true,
            class: ['row-actions', `row-actions--${serverActionCount(row)}`],
          },
          () => [
            h(
              NButton,
              {
                quaternary: true,
                circle: true,
                size: 'small',
                onClick: () =>
                  void router.push({ name: 'server-detail', params: { serverID: row.id } }),
              },
              {
                default: () => h(AppIcon, { icon: Eye, label: locale.t('servers.openDetail') }),
              },
            ),
            ...(['stopped', 'ready', 'failed'].includes(row.state)
              ? [
                  h(
                    NButton,
                    {
                      quaternary: true,
                      circle: true,
                      size: 'small',
                      type: 'primary',
                      loading: lifecycleActions.value[row.id] === 'start',
                      disabled: Boolean(lifecycleActions.value[row.id]),
                      onClick: () => void startLifecycle(row),
                    },
                    {
                      default: () => h(AppIcon, { icon: Play, label: locale.t('servers.start') }),
                    },
                  ),
                ]
              : []),
            ...(['running', 'starting', 'failed'].includes(row.state)
              ? [
                  h(
                    NButton,
                    {
                      quaternary: true,
                      circle: true,
                      size: 'small',
                      loading: lifecycleActions.value[row.id] === 'stop',
                      disabled: Boolean(lifecycleActions.value[row.id]),
                      onClick: () => void stopLifecycle(row),
                    },
                    {
                      default: () => h(AppIcon, { icon: Square, label: locale.t('servers.stop') }),
                    },
                  ),
                ]
              : []),
            ...(row.state === 'running'
              ? [
                  h(
                    NButton,
                    {
                      quaternary: true,
                      circle: true,
                      size: 'small',
                      loading: lifecycleActions.value[row.id] === 'restart',
                      disabled: Boolean(lifecycleActions.value[row.id]),
                      onClick: () => void restartLifecycle(row),
                    },
                    {
                      default: () =>
                        h(AppIcon, { icon: RotateCw, label: locale.t('servers.restart') }),
                    },
                  ),
                ]
              : []),
            h(
              NButton,
              {
                quaternary: true,
                circle: true,
                size: 'small',
                disabled: Boolean(lifecycleActions.value[row.id]),
                onClick: () => openEdit(row),
              },
              {
                default: () =>
                  h(AppIcon, { icon: Pencil, label: locale.t('servers.editMetadata') }),
              },
            ),
            h(
              NButton,
              {
                quaternary: true,
                circle: true,
                size: 'small',
                type: 'error',
                disabled: Boolean(lifecycleActions.value[row.id]),
                onClick: () => void softDelete(row),
              },
              {
                default: () => h(AppIcon, { icon: Trash2, label: locale.t('servers.softDelete') }),
              },
            ),
          ],
        ),
}))

/**
 * 统计该行可用操作数量，用于决定操作列宽度。
 * @param server - 当前行的 Server
 * @returns 可用操作数量
 */
function serverActionCount(server: MinecraftServer): number {
  if (server.deletedAt) return 2
  let count = 3
  if (['stopped', 'ready', 'failed'].includes(server.state)) count += 1
  if (['running', 'starting', 'failed'].includes(server.state)) count += 1
  if (server.state === 'running') count += 1
  return count
}

const typeVersionColumn = computed<ServerColumn>(() => ({
  title: locale.t('servers.column.typeVersion'),
  key: 'typeVersion',
  width: 118,
  sorter: (left, right) =>
    left.type === right.type
      ? left.version.localeCompare(right.version)
      : left.type.localeCompare(right.type),
  render: (row) =>
    h('div', { class: 'metadata-cell' }, [
      h(NTag, { size: 'small', bordered: false }, { default: () => row.type }),
      h(NText, { depth: 3 }, { default: () => row.version }),
    ]),
}))

const uptimeColumn = computed<ServerColumn>(() => ({
  title: locale.t('servers.column.uptime'),
  key: 'uptime',
  width: 140,
  sorter: (left, right) => uptimeSeconds(left) - uptimeSeconds(right),
  render: (row) =>
    h(
      NText,
      { depth: row.state === 'running' ? 1 : 3 },
      { default: () => formatServerUptime(row) },
    ),
}))

const groupTagsColumn = computed<ServerColumn>(() => ({
  title: locale.t('servers.column.groupTags'),
  key: 'groupTags',
  width: 170,
  render: (row) =>
    row.group || row.tags.length
      ? h('div', { class: 'tag-cell' }, [
          ...(row.group
            ? [
                h(
                  NTag,
                  { size: 'small', type: 'info', bordered: false },
                  { default: () => row.group },
                ),
              ]
            : []),
          ...row.tags
            .slice(0, 2)
            .map((tag) =>
              h(NTag, { size: 'small', bordered: false, title: tag }, { default: () => tag }),
            ),
          ...(row.tags.length > 2
            ? [
                h(
                  NText,
                  { depth: 3, title: row.tags.slice(2).join('、') },
                  { default: () => `+${row.tags.length - 2}` },
                ),
              ]
            : []),
        ])
      : h(NText, { depth: 3 }, { default: () => '—' }),
}))

const updatedAtColumn = computed<ServerColumn>(() => ({
  title: locale.t('servers.column.updatedAt'),
  key: 'updatedAt',
  width: 170,
  sorter: (left, right) => left.updatedAt.localeCompare(right.updatedAt),
  render: (row) => locale.formatDateTime(row.updatedAt),
}))

const columns = computed<DataTableColumns<MinecraftServer>>(() => {
  const responsiveActionsColumn = {
    ...actionsColumn.value,
    width: Math.min(180, Math.max(96, Math.round(tableWidth.value * 0.14))),
  } as ServerColumn

  if (tableWidth.value >= 1280) {
    return [
      serverColumn.value,
      typeVersionColumn.value,
      uptimeColumn.value,
      groupTagsColumn.value,
      runtimeColumn.value,
      stateColumn.value,
      updatedAtColumn.value,
      responsiveActionsColumn,
    ]
  }
  if (tableWidth.value >= 1050) {
    return [
      serverColumn.value,
      typeVersionColumn.value,
      uptimeColumn.value,
      runtimeColumn.value,
      stateColumn.value,
      updatedAtColumn.value,
      responsiveActionsColumn,
    ]
  }
  if (tableWidth.value >= 820) {
    return [
      serverColumn.value,
      typeVersionColumn.value,
      uptimeColumn.value,
      runtimeColumn.value,
      stateColumn.value,
      responsiveActionsColumn,
    ]
  }
  if (tableWidth.value >= 680) {
    return [
      serverColumn.value,
      typeVersionColumn.value,
      runtimeColumn.value,
      stateColumn.value,
      responsiveActionsColumn,
    ]
  }
  return [serverColumn.value, stateColumn.value, responsiveActionsColumn]
})

/**
 * 计算该 Server 的运行时长秒数。
 * @param server - 目标 Server
 * @returns 运行秒数，未运行时为 0
 */
function uptimeSeconds(server: MinecraftServer): number {
  if (server.state !== 'running') return 0
  const uptime = serverUptimes.value[server.id]
  if (!uptime) return 0
  const elapsedSinceSample = Math.max(0, Math.floor((uptimeNow.value - uptime.sampledAt) / 1000))
  return Math.max(0, Math.floor(uptime.seconds) + elapsedSinceSample)
}

/**
 * 把运行时长格式化成天时分秒文本。
 * @param server - 目标 Server
 * @returns 运行时长文本，未运行时返回破折号
 */
function formatServerUptime(server: MinecraftServer): string {
  if (server.state !== 'running') return '—'
  const seconds = uptimeSeconds(server)
  if (!serverUptimes.value[server.id]) return locale.t('servers.uptimeCollecting')
  if (seconds < 60) return locale.t('servers.uptimeLessThanMinute')
  const totalMinutes = Math.floor(seconds / 60)
  const days = Math.floor(totalMinutes / 1440)
  const hours = Math.floor((totalMinutes % 1440) / 60)
  const minutes = totalMinutes % 60
  if (days > 0) return locale.t('servers.uptimeDaysHours', { days, hours })
  if (hours > 0) return locale.t('servers.uptimeHoursMinutes', { hours, minutes })
  return locale.t('servers.uptimeMinutes', { minutes })
}

/**
 * 以一次采样为基准记录运行时长，供本地按秒推算。
 * @param serverID - 目标 Server ID
 * @param seconds - 采样时刻的运行秒数
 * @param timestamp - 采样时间戳
 * @returns 无返回值
 */
function updateServerUptime(serverID: string, seconds: number, timestamp: string): void {
  const sampledAt = Date.parse(timestamp)
  if (!Number.isFinite(sampledAt)) return
  const current = serverUptimes.value[serverID]
  if (current && current.sampledAt > sampledAt) return
  serverUptimes.value = {
    ...serverUptimes.value,
    [serverID]: { seconds, sampledAt },
  }
}

/**
 * 批量拉取运行中 Server 的运行时长基准。
 * @returns 加载完成后的 Promise
 */
async function loadServerUptimes(): Promise<void> {
  const runningServers = store.servers.filter((server) => server.state === 'running')
  const runningIDs = new Set(runningServers.map((server) => server.id))
  serverUptimes.value = Object.fromEntries(
    Object.entries(serverUptimes.value).filter(([serverID]) => runningIDs.has(serverID)),
  )
  await Promise.all(
    runningServers.map(async (server) => {
      try {
        const samples = await getLatestMetrics(server.id)
        const uptimeSample = samples
          .filter((sample) => sample.metric === 'process.uptime')
          .sort((left, right) => Date.parse(right.timestamp) - Date.parse(left.timestamp))[0]
        if (uptimeSample) updateServerUptime(server.id, uptimeSample.value, uptimeSample.timestamp)
      } catch {
        // 运行时长属于列表的补充数据;实时指标流仍可能继续回填它。
      }
    }),
  )
}

/**
 * 重新加载 Server 列表与运行时长。
 * @returns 刷新完成后的 Promise
 */
async function refresh(): Promise<void> {
  try {
    await store.refresh()
    await loadServerUptimes()
  } catch (error) {
    notifyError(locale.t('servers.loadFailed'), error)
  }
}

/**
 * 按生命周期操作结果推送成功或失败通知。
 * @param server - 目标 Server
 * @param action - 生命周期动作
 * @param error - 失败时的错误，成功时为空
 * @returns 无返回值
 */
function notifyLifecycleOperation(
  server: MinecraftServer,
  action: string,
  operationID: string,
): void {
  notifications.push({
    kind: 'info',
    title: locale.t('servers.lifecycleStarted', { action }),
    content: operationID || locale.t('servers.lifecycleHandled', { name: server.name }),
    dedupeKey: `server:lifecycle:${server.id}:${action}:${operationID}`,
  })
}

/**
 * 启动 Server 并跟踪操作进度。
 * @param server - 目标 Server
 * @returns 启动完成后的 Promise
 */
async function startLifecycle(server: MinecraftServer): Promise<void> {
  lifecycleActions.value[server.id] = 'start'
  let operationID = ''
  try {
    let firewallConfirmed = false
    let tmuxInstallConfirmed = false
    while (!operationID) {
      try {
        operationID = await startServer(server.id, firewallConfirmed, tmuxInstallConfirmed)
      } catch (error) {
        if (
          error instanceof ApplicationError &&
          error.details.requiresTmuxInstallConfirmation === true &&
          !tmuxInstallConfirmed
        ) {
          const confirmed = await interactions.confirm({
            title: locale.t('servers.tmuxTitle'),
            content: String(error.details.intent ?? locale.t('servers.tmuxContent')),
            objectLabel: `${error.details.os ?? 'Linux'} · ${error.details.packageManager ?? locale.t('servers.tmuxPackageManager')}`,
            impact: `${error.details.needsSudo === true ? locale.t('servers.tmuxSudo') : ''}${
              error.details.command
                ? locale.t('servers.tmuxCommand', { command: String(error.details.command) })
                : ''
            }`,
            positiveText: locale.t('servers.tmuxConfirmStart'),
          })
          if (!confirmed) return
          tmuxInstallConfirmed = true
          continue
        }
        if (
          error instanceof ApplicationError &&
          error.details.requiresFirewallConfirmation === true &&
          !firewallConfirmed
        ) {
          const confirmed = await interactions.confirm({
            title: locale.t('servers.firewallTitle'),
            content: String(error.details.intent ?? locale.t('servers.firewallContent')),
            objectLabel: `${error.details.backend ?? 'firewall'} · TCP ${error.details.port ?? ''}`,
            impact: locale.t('servers.firewallImpact'),
            positiveText: locale.t('servers.firewallConfirm'),
          })
          if (!confirmed) return
          firewallConfirmed = true
          continue
        }
        throw error
      }
    }
    notifyLifecycleOperation(server, locale.t('servers.action.start'), operationID)
    await refresh()
    await router.push({
      name: 'server-detail',
      params: { serverID: server.id },
      query: { tab: 'console' },
    })
  } catch (error) {
    notifyError(locale.t('servers.startFailed'), error)
  } finally {
    if (!operationID && lifecycleActions.value[server.id] === 'start')
      delete lifecycleActions.value[server.id]
  }
}

/**
 * 停止 Server 并跟踪操作进度。
 * @param server - 目标 Server
 * @returns 停止完成后的 Promise
 */
async function stopLifecycle(server: MinecraftServer): Promise<void> {
  lifecycleActions.value[server.id] = 'stop'
  let operationID = ''
  try {
    operationID = await stopServer(server.id)
    notifyLifecycleOperation(server, locale.t('servers.action.stop'), operationID)
    await refresh()
  } catch (error) {
    notifyError(locale.t('servers.stopFailed'), error)
  } finally {
    if (!operationID && lifecycleActions.value[server.id] === 'stop')
      delete lifecycleActions.value[server.id]
  }
}

/**
 * 重启 Server 并跟踪操作进度。
 * @param server - 目标 Server
 * @returns 重启完成后的 Promise
 */
async function restartLifecycle(server: MinecraftServer): Promise<void> {
  lifecycleActions.value[server.id] = 'restart'
  let operationID = ''
  try {
    try {
      operationID = await restartServer(server.id)
    } catch (error) {
      if (
        error instanceof ApplicationError &&
        error.details.requiresTmuxInstallConfirmation === true
      ) {
        const confirmed = await interactions.confirm({
          title: locale.t('servers.tmuxTitle'),
          content: String(error.details.intent ?? locale.t('servers.tmuxContent')),
          objectLabel: `${error.details.os ?? 'Linux'} · ${error.details.packageManager ?? locale.t('servers.tmuxPackageManager')}`,
          impact: `${error.details.needsSudo === true ? locale.t('servers.tmuxSudo') : ''}${
            error.details.command
              ? locale.t('servers.tmuxCommand', { command: String(error.details.command) })
              : ''
          }`,
          positiveText: locale.t('servers.tmuxConfirmRestart'),
        })
        if (!confirmed) return
        operationID = await restartServer(server.id, true)
      } else {
        throw error
      }
    }
    notifyLifecycleOperation(server, locale.t('servers.action.restart'), operationID)
    await refresh()
    await router.push({
      name: 'server-detail',
      params: { serverID: server.id },
      query: { tab: 'console' },
    })
  } catch (error) {
    notifyError(locale.t('servers.restartFailed'), error)
  } finally {
    if (!operationID && lifecycleActions.value[server.id] === 'restart')
      delete lifecycleActions.value[server.id]
  }
}

/**
 * 二次确认后软删除 Server，保留远端文件。
 * @param server - 目标 Server
 * @returns 删除完成后的 Promise
 */
async function softDelete(server: MinecraftServer): Promise<void> {
  const confirmed = await interactions.confirm({
    title: locale.t('servers.softDeleteTitle'),
    content: locale.t('servers.softDeleteContent'),
    objectLabel: `${server.name} · ${server.remotePath}`,
    positiveText: locale.t('servers.softDelete'),
    danger: true,
  })
  if (!confirmed) return
  try {
    await store.softDelete(server.id)
  } catch (error) {
    notifyError(locale.t('servers.softDeleteFailed'), error)
  }
}

/**
 * 恢复一个已软删除的 Server。
 * @param server - 目标 Server
 * @returns 恢复完成后的 Promise
 */
async function restore(server: MinecraftServer): Promise<void> {
  try {
    await store.restore(server.id)
  } catch (error) {
    notifyError(locale.t('servers.restoreFailed'), error)
  }
}

/**
 * 打开导入已有 Server 的表单。
 * @returns 无返回值
 */
function openImport(): void {
  inspection.value = null
  importSSHSessionID.value = sshSessions.sessions[0]?.id ?? ''
  importPath.value = ''
  importName.value = ''
  importType.value = 'vanilla'
  importVersion.value = ''
  importJar.value = ''
  importJavaRuntimeID.value = ''
  importVisible.value = true
  void loadImportJavaRuntimes()
}

/**
 * 为导入表单加载所选 SSH Session 上的 Java 运行时。
 * @returns 加载完成后的 Promise
 */
async function loadImportJavaRuntimes(): Promise<void> {
  importJavaError.value = null
  importJavaRuntimes.value = []
  importJavaRuntimeID.value = ''
  if (!importSSHSessionID.value) return
  importJavaLoading.value = true
  try {
    importJavaRuntimes.value = await listJavaRuntimes(importSSHSessionID.value)
    importJavaRuntimeID.value =
      importJavaRuntimes.value.find((runtime) => runtime.default)?.id ??
      importJavaRuntimes.value[0]?.id ??
      ''
  } catch (error) {
    importJavaError.value = error
  } finally {
    importJavaLoading.value = false
  }
}

/**
 * 探测远端目录，识别可导入的 Server 信息。
 * @returns 探测完成后的 Promise
 */
async function inspectImport(): Promise<void> {
  if (!importSSHSessionID.value || !importPath.value.trim()) return
  importLoading.value = true
  try {
    inspection.value = await inspectRemoteMinecraftServer(
      importSSHSessionID.value,
      importPath.value.trim(),
    )
    importPath.value = inspection.value.remotePath
    importName.value ||=
      inspection.value.remotePath.split('/').filter(Boolean).at(-1) ?? 'Imported Server'
    importType.value = inspection.value.suggestedType
    importVersion.value = inspection.value.suggestedVersion
    importJar.value = inspection.value.suggestedJar
  } catch (error) {
    notifyError(locale.t('servers.inspectFailed'), error)
  } finally {
    importLoading.value = false
  }
}

/**
 * 按探测结果提交 Server 导入。
 * @returns 导入完成后的 Promise
 */
async function submitImport(): Promise<void> {
  if (
    !inspection.value ||
    !importName.value.trim() ||
    !importVersion.value.trim() ||
    !importJar.value ||
    !importJavaRuntimeID.value
  )
    return
  importLoading.value = true
  try {
    const input: MinecraftServerInput = {
      sshSessionID: importSSHSessionID.value,
      javaRuntimeID: importJavaRuntimeID.value,
      name: importName.value.trim(),
      type: importType.value,
      version: importVersion.value.trim(),
      remotePath: inspection.value.remotePath,
      group: '',
      tags: ['imported'],
      favourite: false,
      launchProfile: {
        xmsMiB: importXmsMiB.value,
        xmxMiB: importXmxMiB.value,
        jvmArguments: [],
        jarPath: importJar.value,
        workingDirectory: inspection.value.remotePath,
        serverArguments: ['nogui'],
      },
      firewallPolicy: settings.committed?.firewall.defaultPolicy ?? 'automatic',
      eulaAccepted: inspection.value.eulaAccepted,
    }
    const server = await importRemoteMinecraftServer(input)
    importVisible.value = false
    await store.refresh()
    notifications.push({
      kind: 'success',
      title: locale.t('servers.imported'),
      content: locale.t('servers.importedContent', { name: server.name }),
      dedupeKey: `server:imported:${server.id}`,
    })
  } catch (error) {
    notifyError(locale.t('servers.importFailed'), error)
  } finally {
    importLoading.value = false
  }
}

/**
 * 打开指定 Server 的编辑表单。
 * @param server - 待编辑的 Server
 * @returns 无返回值
 */
function openEdit(server: MinecraftServer): void {
  editTarget.value = server
  editDraft.value = {
    sshSessionID: server.sshSessionID,
    javaRuntimeID: server.javaRuntimeID ?? '',
    name: server.name,
    type: server.type,
    version: server.version,
    remotePath: server.remotePath,
    group: server.group,
    tags: [...server.tags],
    favourite: server.favourite,
    launchProfile: {
      xmsMiB: server.launchProfile.xmsMiB,
      xmxMiB: server.launchProfile.xmxMiB,
      jvmArguments: [...server.launchProfile.jvmArguments],
      jarPath: server.launchProfile.jarPath,
      workingDirectory: server.launchProfile.workingDirectory,
      serverArguments: [...server.launchProfile.serverArguments],
    },
    firewallPolicy: server.firewallPolicy,
    eulaAccepted: server.eulaAccepted,
  }
  editVisible.value = true
}

/**
 * 提交 Server 编辑内容。
 * @returns 保存完成后的 Promise
 */
async function submitEdit(): Promise<void> {
  if (!editTarget.value || !editDraft.value) return
  editLoading.value = true
  try {
    const updated = await store.update(editTarget.value.id, editDraft.value)
    editVisible.value = false
    notifications.push({
      kind: 'success',
      title: locale.t('servers.metadataUpdated'),
      content: locale.t('servers.metadataUpdatedContent', { name: updated.name }),
      dedupeKey: 'server:update:' + updated.id,
    })
  } catch (error) {
    notifyError(locale.t('servers.updateFailed'), error)
  } finally {
    editLoading.value = false
  }
}

/**
 * 打开硬删除确认表单。
 * @param server - 待硬删除的 Server
 * @returns 无返回值
 */
function openHardDelete(server: MinecraftServer): void {
  hardDeleteTarget.value = server
  confirmedDeleteName.value = ''
  confirmedDeletePath.value = ''
  hardDeleteVisible.value = true
}

/**
 * 把确认输入框自动填成目标 Server 名称。
 * @returns 无返回值
 */
function fillHardDeleteConfirmation(): void {
  if (!hardDeleteTarget.value) return
  confirmedDeleteName.value = hardDeleteTarget.value.name
  confirmedDeletePath.value = hardDeleteTarget.value.remotePath
}

/**
 * 校验确认输入后硬删除 Server 及其远端文件。
 * @returns 删除完成后的 Promise
 */
async function submitHardDelete(): Promise<void> {
  if (!hardDeleteTarget.value) return
  const targetID = hardDeleteTarget.value.id
  pendingHardDeleteTargets.add(targetID)
  hardDeleteLoading.value = true
  try {
    const operationID = await hardDeleteRemoteMinecraftServer(
      targetID,
      confirmedDeleteName.value,
      confirmedDeletePath.value,
    )
    hardDeleteVisible.value = false
    notifications.push({
      kind: 'warning',
      title: locale.t('servers.hardDeleteStarted'),
      content: operationID,
      dedupeKey: `server:hard-delete:${operationID}`,
    })
  } catch (error) {
    pendingHardDeleteTargets.delete(targetID)
    notifyError(locale.t('servers.hardDeleteFailed'), error)
  } finally {
    hardDeleteLoading.value = false
  }
}

/**
 * 推送一条错误通知。
 * @param title - 通知标题
 * @param error - 捕获到的错误
 * @returns 无返回值
 */
function notifyError(title: string, error: unknown): void {
  notifications.push({
    kind: 'error',
    title,
    content: error instanceof Error ? error.message : String(error),
    dedupeKey: `server:error:${title}`,
  })
}

onMounted(async () => {
  if (tableContainer.value) {
    tableWidth.value = tableContainer.value.clientWidth
    if (typeof ResizeObserver !== 'undefined') {
      tableResizeObserver = new ResizeObserver(([entry]) => {
        if (!entry) return
        const width = entry.contentRect.width
        // ±24px 内抖动直接忽略：列显隐切换滚动条约 ±15px 反向改动容器宽度，
        // 照单全收会与列布局形成跨帧持续重排回路（CPU 跑满）；列宽分层间距 ≥140px 不受影响。
        if (Math.abs(width - tableWidth.value) < 24) return
        requestAnimationFrame(() => {
          tableWidth.value = width
        })
      })
      tableResizeObserver.observe(tableContainer.value)
    }
  }

  uptimeTimer = setInterval(() => {
    uptimeNow.value = Date.now()
  }, 30_000)
  unsubscribeMetricRealtime = subscribeMetricRealtime((event) => {
    const uptimeSample = event.samples
      .filter((sample) => sample.metric === 'process.uptime')
      .sort((left, right) => Date.parse(right.timestamp) - Date.parse(left.timestamp))[0]
    if (uptimeSample) updateServerUptime(event.serverID, uptimeSample.value, uptimeSample.timestamp)
  })

  unsubscribeOperation = subscribeOperationProgress((operation) => {
    if (operation.targetType !== 'server') return
    const isLifecycleOperation = Boolean(lifecycleActions.value[operation.targetID])
    const isHardDeleteOperation = pendingHardDeleteTargets.has(operation.targetID)
    if (!isLifecycleOperation && !isHardDeleteOperation) return
    if (!['pending', 'running'].includes(operation.state)) {
      if (isLifecycleOperation) delete lifecycleActions.value[operation.targetID]
      if (isHardDeleteOperation) pendingHardDeleteTargets.delete(operation.targetID)
      void refresh()
    }
  })
  const [, serverResult] = await Promise.allSettled([
    sshSessions.sessions.length ? Promise.resolve() : sshSessions.refresh(),
    store.refresh(),
  ])
  if (serverResult.status === 'rejected') {
    notifyError(locale.t('servers.loadFailed'), serverResult.reason)
  } else {
    await loadServerUptimes()
  }
})

onUnmounted(() => {
  tableResizeObserver?.disconnect()
  unsubscribeMetricRealtime?.()
  unsubscribeOperation?.()
  if (uptimeTimer) clearInterval(uptimeTimer)
})

watch(importSSHSessionID, () => {
  inspection.value = null
  if (importVisible.value) void loadImportJavaRuntimes()
})
</script>

<template>
  <NCard>
    <NFlex justify="end" wrap class="page-actions">
      <NButton type="primary" @click="wizardVisible = true">
        <template #icon>
          <AppIcon :icon="Plus" :label="locale.t('servers.createIcon')" />
        </template>
        {{ locale.t('servers.createInstall') }}
      </NButton>
      <NButton :disabled="!sshSessions.sessions.length" @click="openImport">
        {{ locale.t('servers.importRemote') }}
      </NButton>
    </NFlex>

    <div class="toolbar">
      <NInput
        v-model:value="store.search"
        class="toolbar-search"
        clearable
        :placeholder="locale.t('servers.searchPlaceholder')"
        @keyup.enter="refresh"
      />
      <NSelect
        v-model:value="store.sshSessionID"
        clearable
        :placeholder="locale.t('servers.allSessions')"
        :options="
          sshSessions.sessions.map((session) => ({ label: session.name, value: session.id }))
        "
        @update:value="refresh"
      />
      <NSelect v-model:value="store.state" :options="stateOptions" @update:value="refresh" />
      <NCheckbox
        v-model:checked="store.includeDeleted"
        class="include-deleted"
        @update:checked="refresh"
      >
        {{ locale.t('servers.includeDeleted') }}
      </NCheckbox>
      <NButton :loading="store.loading" @click="refresh">
        {{ locale.t('common.refresh') }}
      </NButton>
    </div>

    <div ref="tableContainer" class="server-table">
      <AppDataTable
        :columns="columns"
        :data="store.servers"
        :loading="store.loading"
        :error="store.error"
        :empty-description="locale.t('servers.empty')"
        @retry="refresh"
        @open="
          (server) =>
            !server.deletedAt &&
            router.push({ name: 'server-detail', params: { serverID: server.id } })
        "
      />
    </div>
  </NCard>

  <ServerWizard v-model:show="wizardVisible" />

  <NModal
    v-model:show="editVisible"
    preset="card"
    :title="locale.t('servers.editTitle')"
    style="width: min(760px, calc(100vw - 32px))"
  >
    <NForm v-if="editDraft && editTarget">
      <NAlert type="info" :title="locale.t('servers.editNoticeTitle')">
        {{
          locale.t('servers.editNoticeContent', {
            session: editTarget.sshSessionID,
            path: editTarget.remotePath,
          })
        }}
      </NAlert>
      <NFlex :wrap="false">
        <NFormItem :label="locale.t('servers.form.name')" style="flex: 2">
          <NInput v-model:value="editDraft.name" />
        </NFormItem>
        <NFormItem :label="locale.t('servers.form.group')" style="flex: 1">
          <NInput v-model:value="editDraft.group" />
        </NFormItem>
        <NFormItem :label="locale.t('servers.form.favourite')">
          <NCheckbox v-model:checked="editDraft.favourite">
            {{ locale.t('servers.form.pin') }}
          </NCheckbox>
        </NFormItem>
      </NFlex>
      <NFormItem :label="locale.t('servers.form.tags')">
        <NDynamicTags v-model:value="editDraft.tags" />
      </NFormItem>
      <NFlex :wrap="false">
        <NFormItem label="Xms MiB" style="flex: 1">
          <NInputNumber v-model:value="editDraft.launchProfile.xmsMiB" :min="64" />
        </NFormItem>
        <NFormItem label="Xmx MiB" style="flex: 1">
          <NInputNumber
            v-model:value="editDraft.launchProfile.xmxMiB"
            :min="editDraft.launchProfile.xmsMiB"
          />
        </NFormItem>
        <NFormItem :label="locale.t('servers.form.jar')" style="flex: 2">
          <NInput v-model:value="editDraft.launchProfile.jarPath" />
        </NFormItem>
      </NFlex>
      <NFormItem :label="locale.t('servers.form.jvmArgs')">
        <NDynamicTags v-model:value="editDraft.launchProfile.jvmArguments" />
      </NFormItem>
      <NFormItem :label="locale.t('servers.form.serverArgs')">
        <NDynamicTags v-model:value="editDraft.launchProfile.serverArguments" />
      </NFormItem>
      <NFormItem :label="locale.t('servers.form.firewall')">
        <NSelect v-model:value="editDraft.firewallPolicy" :options="firewallPolicyOptions" />
      </NFormItem>
      <NFlex justify="end">
        <NButton @click="editVisible = false">{{ locale.t('common.cancel') }}</NButton>
        <NButton
          type="primary"
          :loading="editLoading"
          :disabled="
            !editDraft.name.trim() ||
            !editDraft.launchProfile.jarPath.trim() ||
            editDraft.launchProfile.xmsMiB > editDraft.launchProfile.xmxMiB
          "
          @click="submitEdit"
        >
          {{ locale.t('servers.saveMetadata') }}
        </NButton>
      </NFlex>
    </NForm>
  </NModal>

  <NModal
    v-model:show="importVisible"
    preset="card"
    :title="locale.t('servers.importTitle')"
    style="width: min(760px, calc(100vw - 32px))"
  >
    <NForm>
      <NFlex :wrap="false">
        <NFormItem label="SSH Session" style="flex: 1">
          <NSelect
            v-model:value="importSSHSessionID"
            :options="sshSessions.sessions.map((item) => ({ label: item.name, value: item.id }))"
          />
        </NFormItem>
        <NFormItem :label="locale.t('servers.form.remoteDir')" style="flex: 2">
          <NInput v-model:value="importPath" placeholder="/home/minecraft/server" />
        </NFormItem>
        <NFormItem :label="locale.t('servers.form.inspect')">
          <NButton :loading="importLoading" @click="inspectImport">
            {{ locale.t('servers.inspectButton') }}
          </NButton>
        </NFormItem>
      </NFlex>
      <template v-if="inspection">
        <NAlert
          :type="inspection.eulaAccepted && inspection.propertiesFound ? 'success' : 'warning'"
          :title="locale.t('servers.inspectResult')"
        >
          {{
            locale.t('servers.inspectSummary', {
              jars: inspection.jars.length,
              properties: inspection.propertiesFound
                ? locale.t('servers.present')
                : locale.t('servers.missing'),
              eula: inspection.eulaAccepted
                ? locale.t('servers.eulaAccepted')
                : locale.t('servers.eulaNotAccepted'),
            })
          }}
        </NAlert>
        <NAlert
          v-if="inspection.warnings.length"
          type="warning"
          :title="locale.t('servers.needsConfirm')"
        >
          {{ inspection.warnings.join(locale.t('common.listSeparator')) }}
        </NAlert>
        <NFlex :wrap="false">
          <NFormItem :label="locale.t('servers.form.name')" style="flex: 1">
            <NInput v-model:value="importName" />
          </NFormItem>
          <NFormItem :label="locale.t('servers.form.type')" style="flex: 1">
            <NSelect v-model:value="importType" :options="serverTypeOptions" />
          </NFormItem>
          <NFormItem :label="locale.t('servers.form.version')" style="flex: 1">
            <NInput v-model:value="importVersion" />
          </NFormItem>
        </NFlex>
        <NFormItem :label="locale.t('servers.form.jar')">
          <NSelect
            v-model:value="importJar"
            :options="
              inspection.jars.map((jar) => ({
                label: locale.t('servers.jarOption', {
                  name: jar.name,
                  size: (jar.size / 1024 / 1024).toFixed(1),
                }),
                value: jar.name,
              }))
            "
          />
        </NFormItem>
        <NFormItem label="Java Runtime">
          <NSelect
            v-model:value="importJavaRuntimeID"
            :loading="importJavaLoading"
            :options="
              importJavaRuntimes.map((runtime) => ({
                label: `${runtime.vendor} Java ${runtime.majorVersion} · ${runtime.javaHome}${
                  runtime.default ? locale.t('servers.javaOptionDefault') : ''
                }`,
                value: runtime.id,
              }))
            "
            :placeholder="locale.t('servers.selectJava')"
          />
        </NFormItem>
        <NAlert v-if="importJavaError" type="warning" :title="locale.t('servers.javaUnavailable')">
          {{ importJavaError instanceof Error ? importJavaError.message : String(importJavaError) }}
        </NAlert>
        <NAlert
          v-else-if="!importJavaLoading && !importJavaRuntimes.length"
          type="warning"
          :title="locale.t('servers.javaRequired')"
        >
          {{ locale.t('servers.javaRequiredContent') }}
        </NAlert>
        <NFlex :wrap="false">
          <NFormItem label="Xms MiB" style="flex: 1">
            <NInputNumber v-model:value="importXmsMiB" :min="64" />
          </NFormItem>
          <NFormItem label="Xmx MiB" style="flex: 1">
            <NInputNumber v-model:value="importXmxMiB" :min="64" />
          </NFormItem>
        </NFlex>
      </template>
      <NFlex justify="end">
        <NButton @click="importVisible = false">{{ locale.t('common.cancel') }}</NButton>
        <NButton
          type="primary"
          :loading="importLoading"
          :disabled="
            !inspection ||
            !importName.trim() ||
            !importVersion.trim() ||
            !importJar ||
            !importJavaRuntimeID
          "
          @click="submitImport"
        >
          {{ locale.t('servers.importSubmit') }}
        </NButton>
      </NFlex>
    </NForm>
  </NModal>

  <NModal
    v-model:show="hardDeleteVisible"
    preset="card"
    :title="locale.t('servers.hardDeleteTitle')"
    style="width: min(640px, calc(100vw - 32px))"
  >
    <NAlert type="error" :title="locale.t('servers.hardDeleteWarnTitle')">
      {{ locale.t('servers.hardDeleteWarnContent') }}
    </NAlert>
    <NForm v-if="hardDeleteTarget">
      <NFlex justify="end">
        <NButton @click="fillHardDeleteConfirmation">
          {{ locale.t('servers.fillConfirmation') }}
        </NButton>
      </NFlex>
      <NFormItem :label="locale.t('servers.confirmName', { name: hardDeleteTarget.name })">
        <NInput v-model:value="confirmedDeleteName" />
      </NFormItem>
      <NFormItem :label="locale.t('servers.confirmPath', { path: hardDeleteTarget.remotePath })">
        <NInput v-model:value="confirmedDeletePath" />
      </NFormItem>
      <NFlex justify="end">
        <NButton @click="hardDeleteVisible = false">{{ locale.t('common.cancel') }}</NButton>
        <NButton
          type="error"
          :loading="hardDeleteLoading"
          :disabled="
            confirmedDeleteName !== hardDeleteTarget.name ||
            confirmedDeletePath !== hardDeleteTarget.remotePath
          "
          @click="submitHardDelete"
        >
          {{ locale.t('servers.hardDeleteSubmit') }}
        </NButton>
      </NFlex>
    </NForm>
  </NModal>
</template>

<style scoped>
.page-actions {
  margin-bottom: 16px;
}

.toolbar {
  display: grid;
  grid-template-columns: minmax(240px, 1.4fr) minmax(220px, 1.2fr) minmax(160px, 0.8fr) auto auto;
  gap: 12px;
  align-items: center;
  margin-bottom: 16px;
}

.toolbar > * {
  min-width: 0;
}

.include-deleted {
  white-space: nowrap;
}

.server-table,
.server-cell,
.runtime-cell,
.metadata-cell {
  min-width: 0;
}

.server-cell {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 4px 0;
}

.server-cell__title,
.server-cell__meta {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 6px;
}

.server-cell__title {
  flex-wrap: wrap;
}

.server-cell__name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.server-cell__path,
.runtime-cell__jar {
  display: block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.runtime-cell,
.metadata-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.tag-cell {
  display: flex;
  min-width: 0;
  flex-wrap: wrap;
  gap: 4px;
}

.tag-cell :deep(.n-tag) {
  max-width: 100%;
}

.row-actions {
  width: 100%;
  min-width: 0;
  gap: 2px;
  flex-wrap: wrap;
  justify-content: center;
  align-content: center;
}

.row-actions--2,
.row-actions--4 {
  max-width: 58px;
  margin-inline: auto;
}

.row-actions--3,
.row-actions--5 {
  max-width: 88px;
  margin-inline: auto;
}

.row-actions :deep(.n-button) {
  flex: 0 0 28px;
}

@media (max-width: 1100px) {
  .toolbar {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .include-deleted {
    justify-self: start;
  }
}

@media (max-width: 680px) {
  .page-actions,
  .page-actions :deep(.n-button) {
    width: 100%;
  }

  .toolbar {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
