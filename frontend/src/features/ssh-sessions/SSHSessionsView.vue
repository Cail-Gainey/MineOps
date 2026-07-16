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
import { h, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'

import type { SSHSessionDTO } from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/models'
import { preflightSSHSession } from '../../services/ssh-session-api'
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
const latencyRefreshIntervalMs = 30_000
let measureGeneration = 0
let activeMeasureAllRuns = 0
let latencyRefreshTimer: ReturnType<typeof setInterval> | undefined

const columns: DataTableColumns<SSHSessionDTO> = [
  {
    title: 'Session',
    key: 'session',
    sorter: (left, right) => left.name.localeCompare(right.name),
    minWidth: 520,
    render: (row) =>
      h('div', { class: 'session-cell' }, [
        h('div', { class: 'session-cell__title' }, [
          h(NText, { strong: true }, { default: () => row.name }),
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
        h(
          NText,
          { class: 'session-cell__target' },
          {
            default: () => `${row.username}@${row.host}:${row.port}`,
          },
        ),
        h('div', { class: 'session-cell__meta' }, [
          h(NTag, { size: 'small', bordered: false }, { default: () => row.authType }),
          h(
            NTag,
            {
              size: 'small',
              type: row.hostKeyPolicy === 'strict' ? 'success' : 'warning',
              bordered: false,
            },
            { default: () => (row.hostKeyPolicy === 'strict' ? '严格校验' : '首次确认') },
          ),
          h(
            NTag,
            { size: 'small', bordered: false },
            { default: () => `${row.serverCount} Server` },
          ),
          ...(row.group
            ? [h(NTag, { size: 'small', bordered: false }, { default: () => row.group })]
            : []),
          ...(row.overrideSettings
            ? [h(NTag, { size: 'small', bordered: false }, { default: () => '连接覆盖' })]
            : []),
        ]),
        ...(row.remark
          ? [h(NText, { depth: 3, class: 'session-cell__remark' }, { default: () => row.remark })]
          : []),
      ]),
  },
  {
    title: '连接状态',
    key: 'connectionStatus',
    width: 126,
    render: (row) => {
      const status = connectionStates.value[row.id]
      if (!status) {
        return h(
          NTag,
          { bordered: false, title: '等待自动延迟测量。' },
          { default: () => '待测量' },
        )
      }
      const label =
        status.state === 'latency'
          ? `${status.latencyMs} ms`
          : status.state === 'measuring'
            ? '测量中'
            : '不可达'
      const type =
        status.state === 'latency' ? 'success' : status.state === 'measuring' ? 'info' : 'error'
      return h(NTag, { type, bordered: false, title: status.message }, { default: () => label })
    },
  },
  {
    title: '操作',
    key: 'actions',
    width: 248,
    fixed: 'right',
    render: (row) =>
      h(NFlex, { wrap: false, justify: 'end', class: 'row-actions' }, () => [
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
  },
]

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

function openTerminal(session: SSHSessionDTO): void {
  void router.push({ name: 'terminal', params: { sshSessionID: session.id } })
}

function openFiles(session: SSHSessionDTO): void {
  void router.push({ name: 'files', params: { sshSessionID: session.id } })
}

// 单会话延迟测量:按正式 SSH 路由执行预检并把结果写入状态列;过期代数的结果被丢弃。
async function measure(session: SSHSessionDTO, generation: number): Promise<void> {
  connectionStates.value[session.id] = { state: 'measuring', message: '正在测量 SSH 路由延迟。' }
  try {
    const result = await preflightSSHSession(session.id)
    if (generation !== measureGeneration) return
    connectionStates.value[session.id] = {
      state: 'latency',
      latencyMs: result.latencyMs,
      message: `SSH RTT 中位数 ${result.latencyMs} ms · 最小 ${result.minLatencyMs} ms · 平均 ${result.averageLatencyMs} ms · 最大 ${result.maxLatencyMs} ms · ${result.sampleCount} 次采样 · ${result.connectedAddress}`,
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

function openCreate(): void {
  editing.value = null
  formVisible.value = true
}

function openEdit(session: SSHSessionDTO): void {
  editing.value = session
  formVisible.value = true
}

async function refresh(): Promise<void> {
  try {
    await store.refresh()
    void measureAll()
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '加载 SSH Sessions 失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'ssh-sessions:load-error',
    })
  }
}

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

function handleContextAction(key: string | number, session: SSHSessionDTO): void {
  if (key === 'edit') openEdit(session)
  if (key === 'terminal') openTerminal(session)
  if (key === 'files') openFiles(session)
  if (key === 'favourite') void toggleFavourite(session)
  if (key === 'delete') void remove(session)
}

async function saved(session: SSHSessionDTO): Promise<void> {
  delete connectionStates.value[session.id]
  notifications.push({
    kind: 'success',
    title: editing.value ? 'SSH Session 已更新' : 'SSH Session 已创建',
    content: session.name,
    dedupeKey: `ssh-session:saved:${session.id}`,
  })
  await refresh()
}

function saveFailed(error: unknown): void {
  notifications.push({
    kind: 'error',
    title: '保存 SSH Session 失败',
    content: error instanceof Error ? error.message : String(error),
    dedupeKey: 'ssh-session:save-error',
  })
}

onMounted(() => {
  void refresh()
  latencyRefreshTimer = setInterval(() => {
    if (document.visibilityState === 'visible' && !store.loading && activeMeasureAllRuns === 0) {
      void measureAll()
    }
  }, latencyRefreshIntervalMs)
})
onUnmounted(() => {
  measureGeneration++
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

    <AppDataTable
      :columns="columns"
      :data="store.sessions"
      :loading="store.loading"
      :error="store.error"
      :partial-message="store.partialMessage"
      :context-options="contextOptions"
      :scroll-x="930"
      empty-description="尚未创建 SSH Session"
      @retry="refresh"
      @context-action="handleContextAction"
    >
      <template #empty-action>
        <NButton type="primary" @click="openCreate">创建第一个 Session</NButton>
      </template>
    </AppDataTable>
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

.session-cell {
  display: flex;
  min-width: 0;
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

.session-cell__target,
.session-cell__remark {
  display: block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.row-actions {
  gap: 4px;
}
</style>
