<script setup lang="ts">
import { Edit3, FolderOpen, Plus, Star, StarOff, TerminalSquare, Trash2 } from '@lucide/vue'
import {
  NButton,
  NCard,
  NFlex,
  NInput,
  NSelect,
  NSwitch,
  NTag,
  NText,
  type DataTableColumns,
  type DropdownOption,
} from 'naive-ui'
import { computed, h, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'

import type { SSHSessionDTO } from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/models'
import {
  ensureSSHSessionHostSpecs,
  preflightSSHSession,
  testSSHSessionConnection,
} from '../../services/ssh-session-api'
import AppDataTable from '../../shared/components/AppDataTable.vue'
import AppIcon from '../../shared/components/AppIcon.vue'
import { useInteractionStore } from '../../stores/interactions'
import { useNotificationStore } from '../../stores/notifications'
import { useSSHSessionsStore } from '../../stores/ssh-sessions'
import { useTerminalTabsStore } from '../../stores/terminal-tabs'
import SSHSessionForm from './SSHSessionForm.vue'

const store = useSSHSessionsStore()
const router = useRouter()
const terminalTabs = useTerminalTabsStore()
const interactions = useInteractionStore()
const notifications = useNotificationStore()
const formVisible = ref(false)
const editing = ref<SSHSessionDTO | null>(null)
type ConnectionState = 'measuring' | 'latency' | 'unreachable'
interface ConnectionStatus {
  state: ConnectionState
  message: string
  latencyMs?: number
}
const connectionStates = ref<Record<string, ConnectionStatus>>({})
// 主机规格是 SSH Session 的持久化元数据,这里只跟踪历史会话补采一次的进行状态。
type HostSpecsState = 'collecting' | 'failed'
const hostSpecsStates = ref<Record<string, HostSpecsState>>({})
const hostSpecsConcurrency = 3
let hostSpecsGeneration = 0
const tableContainer = ref<HTMLElement | null>(null)
const tableWidth = ref(1280)
const latencyRefreshIntervalMs = 30_000
let measureGeneration = 0
let activeMeasureAllRuns = 0
let latencyRefreshTimer: ReturnType<typeof setInterval> | undefined
let tableResizeObserver: ResizeObserver | null = null

type SessionColumn = DataTableColumns<SSHSessionDTO>[number]

const sessionColumn: SessionColumn = {
  title: '名称',
  key: 'session',
  width: 180,
  sorter: (left, right) => left.name.localeCompare(right.name),
  render: (row) =>
    h('div', { class: 'session-cell' }, [
      h('div', { class: 'session-cell__title' }, [
        h(
          NText,
          { strong: true, class: 'session-cell__name', title: row.name },
          { default: () => row.name },
        ),
        ...(row.favourite
          ? [
              h(
                NTag,
                { size: 'small', type: 'warning', bordered: false },
                { default: () => '收藏' },
              ),
            ]
          : []),
      ]),

      ...(tableWidth.value < 680
        ? [
            h(
              NText,
              { class: 'session-cell__target', title: `${row.username}@${row.host}:${row.port}` },
              { default: () => `${row.username}@${row.host}:${row.port}` },
            ),
            h('div', { class: 'session-cell__meta' }, [
              h(NTag, { size: 'small', bordered: false }, { default: () => row.authType }),
              ...(row.group
                ? [h(NTag, { size: 'small', bordered: false }, { default: () => row.group })]
                : []),
              h(
                NTag,
                { size: 'small', bordered: false },
                { default: () => `${row.serverCount} Server` },
              ),
            ]),
            ...(row.remark
              ? [
                  h(
                    NText,
                    { depth: 3, class: 'session-cell__remark', title: row.remark },
                    { default: () => row.remark },
                  ),
                ]
              : []),
          ]
        : []),
    ]),
}

const connectionStatusColumn: SessionColumn = {
  title: '延迟',
  key: 'connectionStatus',
  width: 90,
  render: (row) => {
    const status = connectionStates.value[row.id]
    if (!status) {
      return h(NTag, { bordered: false, title: '等待自动延迟测量。' }, { default: () => '待测量' })
    }
    const label =
      status.state === 'latency'
        ? `${Math.round(status.latencyMs ?? 0)} ms`
        : status.state === 'measuring'
          ? '测量中'
          : '不可达'
    const type =
      status.state === 'latency' ? 'success' : status.state === 'measuring' ? 'info' : 'error'
    return h(
      NTag,
      { type, size: 'small', bordered: false, title: status.message },
      { default: () => label },
    )
  },
}

const actionsColumn: SessionColumn = {
  title: '操作',
  key: 'actions',
  render: (row) =>
    h(NFlex, { wrap: true, class: 'row-actions' }, () => [
      h(
        NButton,
        { quaternary: true, circle: true, size: 'small', onClick: () => openTerminal(row) },
        { default: () => h(AppIcon, { icon: TerminalSquare, label: '打开 Terminal' }) },
      ),
      h(
        NButton,
        { quaternary: true, circle: true, size: 'small', onClick: () => openFiles(row) },
        { default: () => h(AppIcon, { icon: FolderOpen, label: '文件' }) },
      ),
      h(
        NButton,
        {
          quaternary: true,
          circle: true,
          size: 'small',
          onClick: () => void toggleFavourite(row),
        },
        {
          default: () => h(AppIcon, { icon: row.favourite ? Star : StarOff, label: '切换收藏' }),
        },
      ),
      h(
        NButton,
        { quaternary: true, circle: true, size: 'small', onClick: () => openEdit(row) },
        { default: () => h(AppIcon, { icon: Edit3, label: '编辑' }) },
      ),
      h(
        NButton,
        {
          quaternary: true,
          circle: true,
          size: 'small',
          type: 'error',
          onClick: () => void remove(row),
        },
        { default: () => h(AppIcon, { icon: Trash2, label: '删除' }) },
      ),
    ]),
}

const targetColumn: SessionColumn = {
  title: '地址',
  key: 'target',
  width: 230,
  sorter: (left, right) =>
    left.host === right.host
      ? left.username.localeCompare(right.username)
      : left.host.localeCompare(right.host),
  render: (row) => {
    const target = `${row.username}@${row.host}:${row.port}`
    return h(NText, { class: 'ellipsis-cell', title: target }, { default: () => target })
  },
}

const informationColumn: SessionColumn = {
  title: '信息',
  key: 'information',
  width: 260,
  render: (row) => {
    const { cpuCount, memoryBytes, diskBytes, specsCollectedAt } = row
    if (!specsCollectedAt || !cpuCount || !memoryBytes || !diskBytes) {
      const collecting = hostSpecsStates.value[row.id] === 'collecting'
      return h(
        NText,
        { depth: 3, title: collecting ? '正在采集主机规格。' : '尚未采集主机规格,刷新可重试。' },
        { default: () => (collecting ? '采集中' : '—') },
      )
    }
    return h('div', { class: 'tag-cell', title: `采集于 ${formatCollectedAt(specsCollectedAt)}` }, [
      h(NTag, { size: 'small', bordered: false }, { default: () => `${cpuCount} 核` }),
      h(
        NTag,
        { size: 'small', bordered: false },
        { default: () => `${formatCapacity(memoryBytes)} 内存` },
      ),
      h(
        NTag,
        { size: 'small', bordered: false },
        { default: () => `${formatCapacity(diskBytes)} 硬盘` },
      ),
    ])
  },
}

const remarkColumn: SessionColumn = {
  title: '备注',
  key: 'remark',
  sorter: (left, right) => left.remark.localeCompare(right.remark),
  render: (row) =>
    h(
      NText,
      { depth: row.remark ? 1 : 3, class: 'ellipsis-cell', title: row.remark || '无备注' },
      { default: () => row.remark || '—' },
    ),
}

const columns = computed<DataTableColumns<SSHSessionDTO>>(() => {
  const responsiveActionsColumn = {
    ...actionsColumn,
    width: Math.min(180, Math.max(96, Math.round(tableWidth.value * 0.14))),
  } as SessionColumn

  if (tableWidth.value >= 1100) {
    return [
      connectionStatusColumn,
      sessionColumn,
      targetColumn,
      informationColumn,
      remarkColumn,
      responsiveActionsColumn,
    ]
  }
  if (tableWidth.value >= 850) {
    return [
      connectionStatusColumn,
      sessionColumn,
      targetColumn,
      informationColumn,
      responsiveActionsColumn,
    ]
  }
  if (tableWidth.value >= 680) {
    return [connectionStatusColumn, sessionColumn, targetColumn, responsiveActionsColumn]
  }
  return [sessionColumn, connectionStatusColumn, responsiveActionsColumn]
})

/**
 * 把字节容量换算成 GiB 展示文本。
 * @param bytes - 字节数
 * @returns 带 G 后缀的容量文本
 */
function formatCapacity(bytes: number): string {
  const gibibytes = bytes / 1024 / 1024 / 1024
  if (gibibytes >= 1) return `${Math.round(gibibytes)}G`
  return `${gibibytes.toFixed(1)}G`
}

/**
 * 把采集时间格式化为本地时间文本。
 * @param value - ISO 时间字符串
 * @returns 本地时间文本，无法解析时原样返回
 */
function formatCollectedAt(value: string): string {
  const collectedAt = new Date(value)
  return Number.isNaN(collectedAt.getTime()) ? value : collectedAt.toLocaleString()
}

/**
 * 构建单行 SSH Session 的右键菜单项。
 * @param row - 当前行的 SSH Session
 * @returns 下拉菜单项数组
 */
function contextOptions(row: SSHSessionDTO): DropdownOption[] {
  return [
    { label: '打开 Terminal', key: 'terminal' },
    { label: '文件', key: 'files' },
    { label: '编辑', key: 'edit' },
    { label: row.favourite ? '取消收藏' : '收藏', key: 'favourite' },
    { type: 'divider', key: 'divider' },
    { label: '删除', key: 'delete' },
  ]
}

/**
 * 跳转到该 SSH Session 的终端页面。
 * @param session - 目标 SSH Session
 * @returns 无返回值
 */
function openTerminal(session: SSHSessionDTO): void {
  void router.push({ name: 'terminal', params: { sshSessionID: session.id } })
}

/**
 * 跳转到该 SSH Session 的文件页面。
 * @param session - 目标 SSH Session
 * @returns 无返回值
 */
function openFiles(session: SSHSessionDTO): void {
  void router.push({ name: 'files', params: { sshSessionID: session.id } })
}

// 单会话延迟测量:按正式 SSH 路由执行预检并把结果写入状态列;过期代数的结果被丢弃。
/**
 * 按正式 SSH 路由执行预检并把延迟写入状态列，过期代数的结果被丢弃。
 * @param session - 目标 SSH Session
 * @param generation - 发起测量时的代数，用于丢弃过期结果
 * @returns 测量完成后的 Promise
 */
async function measure(session: SSHSessionDTO, generation: number): Promise<void> {
  connectionStates.value[session.id] = { state: 'measuring', message: '正在测量 SSH 路由延迟。' }
  try {
    const result = await preflightSSHSession(session.id)
    if (generation !== measureGeneration) return
    connectionStates.value[session.id] = {
      state: 'latency',
      latencyMs: Math.round(result.latencyMs),
      message: `SSH RTT 中位数 ${Math.round(result.latencyMs)} ms · 最小 ${Math.round(result.minLatencyMs)} ms · 平均 ${Math.round(result.averageLatencyMs)} ms · 最大 ${Math.round(result.maxLatencyMs)} ms · ${result.sampleCount} 次采样 · ${result.connectedAddress}`,
    }
  } catch (error) {
    if (generation !== measureGeneration) return
    connectionStates.value[session.id] = {
      state: 'unreachable',
      message: error instanceof Error ? error.message : String(error),
    }
  }
}

// 全量延迟测量:限流并发跑预检;刷新/保存会重新触发,旧一轮结果按代数作废。
/**
 * 依次测量全部 SSH Session 的路由延迟。
 * @returns 全部测量结束后的 Promise
 */
async function measureAll(): Promise<void> {
  activeMeasureAllRuns++
  const generation = ++measureGeneration
  try {
    const queue = [...store.sessions]
    const workers = Array.from({ length: Math.min(5, queue.length) }, async () => {
      while (queue.length > 0 && generation === measureGeneration) {
        const session = queue.shift()
        if (session) await measure(session, generation)
      }
    })
    await Promise.all(workers)
  } finally {
    activeMeasureAllRuns--
  }
}

// 主机规格惰性补采:只对迁移前遗留的、尚未采集过的会话各连一次 SSH,结果由后端落库后写回本行。
/**
 * 为尚未采集过主机规格的 SSH Session 惰性补采。
 * @returns 补采结束后的 Promise
 */
async function backfillHostSpecs(): Promise<void> {
  const generation = ++hostSpecsGeneration
  const queue = store.sessions.filter((session) => !session.specsCollectedAt)
  if (queue.length === 0) return
  for (const session of queue) hostSpecsStates.value[session.id] = 'collecting'
  const workers = Array.from({ length: Math.min(hostSpecsConcurrency, queue.length) }, async () => {
    while (queue.length > 0 && generation === hostSpecsGeneration) {
      const session = queue.shift()
      if (!session) continue
      try {
        const collected = await ensureSSHSessionHostSpecs(session.id)
        if (generation !== hostSpecsGeneration) return
        store.apply(collected)
        delete hostSpecsStates.value[session.id]
      } catch {
        if (generation !== hostSpecsGeneration) return
        hostSpecsStates.value[session.id] = 'failed'
      }
    }
  })
  await Promise.all(workers)
}

/**
 * 打开新建 SSH Session 表单。
 * @returns 无返回值
 */
function openCreate(): void {
  editing.value = null
  formVisible.value = true
}

/**
 * 打开指定 SSH Session 的编辑表单。
 * @param session - 待编辑的 SSH Session
 * @returns 无返回值
 */
function openEdit(session: SSHSessionDTO): void {
  editing.value = session
  formVisible.value = true
}

/**
 * 重新加载 SSH Session 列表并重新测量延迟与主机规格。
 * @returns 刷新完成后的 Promise
 */
async function refresh(): Promise<void> {
  try {
    await store.refresh()
    hostSpecsStates.value = {}
    void measureAll()
    void backfillHostSpecs()
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '加载 SSH Sessions 失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'ssh-sessions:load-error',
    })
  }
}

/**
 * 切换 SSH Session 的收藏状态。
 * @param session - 目标 SSH Session
 * @returns 切换完成后的 Promise
 */
async function toggleFavourite(session: SSHSessionDTO): Promise<void> {
  try {
    await store.toggleFavourite(session)
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '更新收藏状态失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: `ssh-session:favourite-error:${session.id}`,
    })
  }
}

/**
 * 二次确认后删除 SSH Session 及其关联凭据。
 * @param session - 待删除的 SSH Session
 * @returns 删除完成后的 Promise
 */
async function remove(session: SSHSessionDTO): Promise<void> {
  const confirmed = await interactions.confirm({
    title: '删除 SSH Session？',
    content: '关联凭据也会从加密数据库删除。存在 Server 引用时操作会被拒绝。',
    objectLabel: `${session.name} · ${session.username}@${session.host}:${session.port}`,
    impact: `当前关联 ${session.serverCount} 个 Server。`,
    positiveText: '删除',
    danger: true,
  })
  if (!confirmed) return
  try {
    await store.remove(session.id)
    terminalTabs.closeForSSHSession(session.id)
    delete connectionStates.value[session.id]
    delete hostSpecsStates.value[session.id]
    notifications.push({
      kind: 'success',
      title: 'SSH Session 已删除',
      dedupeKey: `ssh-session:deleted:${session.id}`,
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '删除 SSH Session 失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: `ssh-session:delete-error:${session.id}`,
    })
  }
}

/**
 * 分发右键菜单选中的动作。
 * @param key - 菜单项 key
 * @param session - 当前行的 SSH Session
 * @returns 无返回值
 */
function handleContextAction(key: string | number, session: SSHSessionDTO): void {
  if (key === 'edit') openEdit(session)
  if (key === 'terminal') openTerminal(session)
  if (key === 'files') openFiles(session)
  if (key === 'favourite') void toggleFavourite(session)
  if (key === 'delete') void remove(session)
}

/**
 * 表单保存成功后清理该会话的缓存状态并刷新列表。
 * @param session - 刚保存的 SSH Session
 * @returns 处理完成后的 Promise
 */
async function saved(session: SSHSessionDTO): Promise<void> {
  delete connectionStates.value[session.id]
  delete hostSpecsStates.value[session.id]
  const action = editing.value ? '更新' : '创建'
  try {
    const result = await testSSHSessionConnection(session.id)
    notifications.push({
      kind: 'success',
      title: `SSH Session 已${action}并通过连接测试`,
      content: `${session.name} · ${result.serverVersion} · ${result.remoteAddress} · ${result.connectDurationMs} ms`,
      dedupeKey: `ssh-session:saved:${session.id}`,
    })
  } catch (error) {
    notifications.push({
      kind: 'warning',
      title: `SSH Session 已${action}，但自动连接测试未通过`,
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: `ssh-session:saved-test-error:${session.id}`,
    })
  }
  await refresh()
}

/**
 * 表单保存失败时推送错误通知。
 * @param error - 保存过程抛出的错误
 * @returns 无返回值
 */
function saveFailed(error: unknown): void {
  notifications.push({
    kind: 'error',
    title: '保存 SSH Session 失败',
    content: error instanceof Error ? error.message : String(error),
    dedupeKey: 'ssh-session:save-error',
  })
}

onMounted(() => {
  if (tableContainer.value) {
    tableWidth.value = tableContainer.value.clientWidth
    if (typeof ResizeObserver !== 'undefined') {
      tableResizeObserver = new ResizeObserver(([entry]) => {
        if (!entry) return
        const width = entry.contentRect.width
        // ±24px 内抖动直接忽略：列显隐切换滚动条约 ±15px 反向改动容器宽度，
        if (Math.abs(width - tableWidth.value) < 24) return
        requestAnimationFrame(() => {
          tableWidth.value = width
        })
      })
      tableResizeObserver.observe(tableContainer.value)
    }
  }

  void refresh()
  latencyRefreshTimer = setInterval(() => {
    if (document.visibilityState === 'visible' && !store.loading && activeMeasureAllRuns === 0) {
      void measureAll()
    }
  }, latencyRefreshIntervalMs)
})
onUnmounted(() => {
  tableResizeObserver?.disconnect()
  measureGeneration++
  hostSpecsGeneration++
  if (latencyRefreshTimer !== undefined) clearInterval(latencyRefreshTimer)
})
</script>

<template>
  <NCard>
    <NFlex justify="end" class="page-actions">
      <NButton type="primary" @click="openCreate">
        <template #icon><AppIcon :icon="Plus" label="新建" /></template>
        新建 Session
      </NButton>
    </NFlex>

    <NFlex class="toolbar" wrap>
      <NInput
        v-model:value="store.search"
        class="toolbar-search"
        clearable
        placeholder="搜索名称、主机、用户名或备注"
        @keyup.enter="refresh"
        @clear="refresh"
      />
      <NSelect
        v-model:value="store.group"
        class="toolbar-group"
        clearable
        placeholder="全部分组"
        :options="store.groups.map((value) => ({ label: value, value }))"
        @update:value="refresh"
      />
      <NFlex align="center" :wrap="false" class="favourite-filter">
        <NSwitch v-model:value="store.favouriteOnly" @update:value="refresh" />
        <NText>仅收藏</NText>
      </NFlex>
      <NButton :loading="store.loading" @click="refresh">刷新</NButton>
    </NFlex>

    <div ref="tableContainer" class="session-table">
      <AppDataTable
        :columns="columns"
        :data="store.sessions"
        :loading="store.loading"
        :error="store.error"
        :partial-message="store.partialMessage"
        :context-options="contextOptions"
        empty-description="尚未创建 SSH Session"
        @retry="refresh"
        @context-action="handleContextAction"
      >
        <template #empty-action>
          <NButton type="primary" @click="openCreate">创建第一个 Session</NButton>
        </template>
      </AppDataTable>
    </div>
  </NCard>

  <SSHSessionForm
    v-model:show="formVisible"
    :session="editing"
    @saved="saved"
    @failed="saveFailed"
  />
</template>

<style scoped>
.page-actions {
  margin-bottom: 16px;
}

.toolbar {
  margin-bottom: 16px;
}

.toolbar-search {
  min-width: 260px;
  flex: 1 1 420px;
}

.toolbar-group {
  width: 180px;
  flex: 0 0 180px;
}

.favourite-filter {
  flex: 0 0 auto;
  white-space: nowrap;
}

.session-table,
.session-cell,
.metadata-cell {
  min-width: 0;
}

.session-cell {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 4px 0;
}

.session-cell__title,
.session-cell__meta {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 6px;
}

.session-cell__name,
.session-cell__target,
.session-cell__remark,
.ellipsis-cell {
  display: block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

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

.row-actions {
  width: 100%;
  max-width: 88px;
  min-width: 0;
  margin-inline: auto;
  gap: 2px;
  flex-wrap: wrap;
  justify-content: center;
  align-content: center;
}

.row-actions :deep(.n-button) {
  flex: 0 0 28px;
}
</style>
