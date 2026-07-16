<script setup lang="ts">
import {
  NButton,
  NFlex,
  NInput,
  NProgress,
  NSelect,
  NTabPane,
  NTabs,
  NTag,
  type DataTableColumns,
  type DropdownOption,
} from 'naive-ui'
import { computed, h, onMounted, ref } from 'vue'

import type { Operation } from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import AppDataTable from '../../shared/components/AppDataTable.vue'
import { useInteractionStore } from '../../stores/interactions'
import { useLocaleStore } from '../../stores/locale'
import { useNotificationStore } from '../../stores/notifications'
import { useOperationsStore } from '../../stores/operations'

const operations = useOperationsStore()
const interactions = useInteractionStore()
const locale = useLocaleStore()
const notifications = useNotificationStore()
const pendingActionIDs = ref<Set<string>>(new Set())
const clearingHistory = ref(false)
const keyword = ref('')
const typeFilter = ref<string | null>(null)
const targetFilter = ref<string | null>(null)
const stateFilter = ref<string | null>(null)

const typeLabels: Record<string, string> = {
  install: '安装',
  download: '下载',
  upload: '上传',
  extract: '解压',
  start: '启动',
  stop: '停止',
  restart: '重启',
  backup: '备份',
  restore: '恢复',
  update: '更新',
  delete: '删除',
  profile: '性能分析',
}
const targetLabels: Record<string, string> = {
  server: '服务器',
  ssh_session: 'SSH 会话',
  file: '文件',
  java_runtime: 'Java 运行时',
  agent: 'Agent',
  settings: '设置',
  spark: 'Spark',
}
const stateLabels: Record<string, string> = {
  pending: '等待中',
  running: '运行中',
  succeeded: '已成功',
  failed: '失败',
  cancelled: '已取消',
}
const typeOptions = Object.entries(typeLabels).map(([value, label]) => ({ label, value }))
const targetOptions = Object.entries(targetLabels).map(([value, label]) => ({ label, value }))
const stateOptions = Object.entries(stateLabels).map(([value, label]) => ({ label, value }))

function typeLabel(value: string): string {
  return typeLabels[value] ?? value
}

function targetLabel(value: string): string {
  return targetLabels[value] ?? value
}

function stateLabel(value: string): string {
  return stateLabels[value] ?? value
}

function matchesFilters(operation: Operation): boolean {
  if (typeFilter.value && operation.type !== typeFilter.value) return false
  if (targetFilter.value && operation.targetType !== targetFilter.value) return false
  if (stateFilter.value && operation.state !== stateFilter.value) return false
  const query = keyword.value.trim().toLowerCase()
  if (!query) return true
  return [
    operation.id,
    operation.type,
    typeLabel(operation.type),
    operation.targetType,
    targetLabel(operation.targetType),
    operation.targetID,
    operation.state,
    stateLabel(operation.state),
    operation.stage,
    operation.message,
    operation.errorCode,
  ].some((value) =>
    String(value ?? '')
      .toLowerCase()
      .includes(query),
  )
}

const filteredActive = computed(() => operations.active.filter(matchesFilters))
const filteredHistory = computed(() => operations.history.filter(matchesFilters))

function clearFilters(): void {
  keyword.value = ''
  typeFilter.value = null
  targetFilter.value = null
  stateFilter.value = null
}

function operationActionPending(id: string): boolean {
  return pendingActionIDs.value.has(id)
}

function setOperationActionPending(id: string, pending: boolean): void {
  const next = new Set(pendingActionIDs.value)
  if (pending) next.add(id)
  else next.delete(id)
  pendingActionIDs.value = next
}

function canRetry(operation: Operation): boolean {
  return (
    operation.type === 'install' &&
    (operation.state === 'failed' || operation.state === 'cancelled')
  )
}

function stateType(state: string): 'default' | 'info' | 'success' | 'warning' | 'error' {
  switch (state) {
    case 'running':
      return 'info'
    case 'succeeded':
      return 'success'
    case 'failed':
      return 'error'
    case 'cancelled':
      return 'warning'
    default:
      return 'default'
  }
}

const columns: DataTableColumns<Operation> = [
  {
    title: '任务',
    key: 'type',
    width: 220,
    render: (row) =>
      h('div', { class: 'operation-summary' }, [
        h('strong', typeLabel(row.type)),
        h('span', { class: 'operation-summary__secondary' }, targetLabel(row.targetType)),
        h(
          'span',
          { class: 'operation-summary__meta' },
          locale.formatDateTime(row.startedAt ?? row.createdAt),
        ),
      ]),
  },
  {
    title: '状态与进度',
    key: 'progress',
    width: 190,
    render: (row) =>
      h('div', { class: 'operation-state' }, [
        h(
          NTag,
          { type: stateType(row.state), size: 'small' },
          { default: () => stateLabel(row.state) },
        ),
        h(NProgress, {
          percentage: Math.round(row.progress * 100),
          status: row.state === 'failed' ? 'error' : 'default',
          height: 8,
        }),
      ]),
  },
  {
    title: '当前活动',
    key: 'message',
    render: (row) =>
      h('div', { class: 'operation-activity' }, [
        h('strong', row.stage || '等待任务更新'),
        h('span', row.message || '暂无详细消息'),
      ]),
  },
  {
    title: '操作',
    key: 'actions',
    width: 210,
    render: (row) =>
      h(
        NFlex,
        { size: 6, wrap: true },
        {
          default: () => [
            h(
              NButton,
              { size: 'small', onClick: () => void showDetails(row) },
              { default: () => '详情' },
            ),
            row.state === 'pending' || row.state === 'running'
              ? h(
                  NButton,
                  {
                    size: 'small',
                    type: 'warning',
                    loading: operationActionPending(row.id),
                    onClick: () => void confirmCancel(row),
                  },
                  { default: () => '取消' },
                )
              : null,
            canRetry(row)
              ? h(
                  NButton,
                  {
                    size: 'small',
                    type: 'primary',
                    loading: operationActionPending(row.id),
                    onClick: () => void confirmRetry(row),
                  },
                  { default: () => '重试' },
                )
              : null,
            row.state !== 'pending' && row.state !== 'running'
              ? h(
                  NButton,
                  {
                    size: 'small',
                    type: 'error',
                    loading: operationActionPending(row.id),
                    onClick: () => void confirmDelete(row),
                  },
                  { default: () => '删除' },
                )
              : null,
          ],
        },
      ),
  },
]

function contextOptions(row: Operation): DropdownOption[] {
  const options: DropdownOption[] = [{ label: '查看详情', key: 'details' }]
  if (row.state === 'pending' || row.state === 'running') {
    options.push({ type: 'divider', key: 'divider' }, { label: '取消任务', key: 'cancel' })
  }
  if (canRetry(row)) {
    options.push({ type: 'divider', key: 'retry-divider' }, { label: '重试任务', key: 'retry' })
  }
  if (row.state !== 'pending' && row.state !== 'running') {
    options.push({ type: 'divider', key: 'delete-divider' }, { label: '删除记录', key: 'delete' })
  }
  options.push({ label: '复制 Operation ID', key: 'copy-id' })
  return options
}

async function confirmCancel(operation: Operation): Promise<void> {
  const confirmed = await interactions.confirm({
    title: '取消后台任务？',
    content: '取消请求会发送给正在运行的 Operation，已完成的步骤不会自动回滚。',
    objectLabel: `${typeLabel(operation.type)} · ${operation.id}`,
    impact: operation.message || operation.stage,
    positiveText: '确认取消',
    danger: true,
  })
  if (!confirmed) return
  setOperationActionPending(operation.id, true)
  try {
    await operations.cancel(operation.id)
    notifications.push({
      kind: 'warning',
      title: '已提交取消请求',
      content: operation.message,
      dedupeKey: `operation:cancel:${operation.id}`,
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '取消 Operation 失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: `operation:cancel-error:${operation.id}`,
    })
  } finally {
    setOperationActionPending(operation.id, false)
  }
}

async function confirmRetry(operation: Operation): Promise<void> {
  const confirmed = await interactions.confirm({
    title: '重试安装任务？',
    content: '将创建新的 Operation，并从 Installation Task 未完成的检查点继续执行。',
    objectLabel: `${typeLabel(operation.type)} · ${operation.targetID}`,
    impact: operation.message || operation.errorCode || operation.stage,
    positiveText: '确认重试',
  })
  if (!confirmed) return
  setOperationActionPending(operation.id, true)
  try {
    const replacementID = await operations.retry(operation)
    notifications.push({
      kind: 'success',
      title: '重试任务已启动',
      content: `新的 Operation：${replacementID}`,
      dedupeKey: `operation:retry:${operation.id}:${replacementID}`,
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '重试 Operation 失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: `operation:retry-error:${operation.id}`,
    })
  } finally {
    setOperationActionPending(operation.id, false)
  }
}

async function confirmDelete(operation: Operation): Promise<void> {
  const confirmed = await interactions.confirm({
    title: '删除任务记录？',
    content: '只会删除任务中心中的终态 Operation 记录，不会撤销已经执行的远端操作。',
    objectLabel: `${typeLabel(operation.type)} · ${operation.id}`,
    impact: operation.message || operation.errorCode || operation.stage,
    positiveText: '确认删除',
    danger: true,
  })
  if (!confirmed) return
  setOperationActionPending(operation.id, true)
  try {
    await operations.deleteHistory(operation.id)
    notifications.push({
      kind: 'success',
      title: '任务记录已删除',
      content: operation.id,
      dedupeKey: `operation:delete:${operation.id}`,
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '删除任务记录失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: `operation:delete-error:${operation.id}`,
    })
  } finally {
    setOperationActionPending(operation.id, false)
  }
}

async function confirmClearHistory(): Promise<void> {
  if (!operations.history.length) return
  const confirmed = await interactions.confirm({
    title: '清空任务历史？',
    content: '将删除全部已完成、失败和已取消的 Operation 记录，正在等待或运行的任务会保留。',
    objectLabel: `${operations.history.length} 条当前已加载记录`,
    impact: '该操作不会撤销远端执行结果，删除后的历史记录无法恢复。',
    positiveText: '确认清空',
    danger: true,
  })
  if (!confirmed) return
  clearingHistory.value = true
  try {
    const deleted = await operations.clearHistory()
    notifications.push({
      kind: 'success',
      title: '任务历史已清空',
      content: `已删除 ${deleted} 条终态 Operation 记录。`,
      dedupeKey: 'operation:clear-history',
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '清空任务历史失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'operation:clear-history-error',
    })
  } finally {
    clearingHistory.value = false
  }
}

function formatDetails(details: Record<string, unknown> | undefined): string {
  if (!details || Object.keys(details).length === 0) return '-'
  return JSON.stringify(details, null, 2)
}

async function showDetails(operation: Operation): Promise<void> {
  let current = operation
  try {
    current = await operations.get(operation.id)
  } catch {
    // 保留列表中的最后已知状态，详情仍然可查看。
  }
  interactions.openDrawer({
    title: `任务详情 · ${typeLabel(current.type)}`,
    content: [
      `ID: ${current.id}`,
      `类型: ${typeLabel(current.type)}`,
      `目标: ${targetLabel(current.targetType)} / ${current.targetID}`,
      `状态: ${stateLabel(current.state)}`,
      `阶段: ${current.stage || '-'}`,
      `进度: ${Math.round(current.progress * 100)}%`,
      `消息: ${current.message || '-'}`,
      `错误码: ${current.errorCode ?? '-'}`,
      `错误详情:\n${formatDetails(current.errorDetails)}`,
      `重试来源: ${current.retryOf ?? '-'}`,
      `创建时间: ${current.createdAt}`,
      `开始时间: ${current.startedAt ?? '-'}`,
      `结束时间: ${current.finishedAt ?? '-'}`,
    ].join('\n'),
    width: 620,
  })
}

async function copyOperationID(operation: Operation): Promise<void> {
  try {
    await navigator.clipboard.writeText(operation.id)
    notifications.push({
      kind: 'success',
      title: 'Operation ID 已复制',
      content: operation.id,
      dedupeKey: `operation:copy:${operation.id}`,
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '复制 Operation ID 失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: `operation:copy-error:${operation.id}`,
    })
  }
}

function handleContextAction(key: string | number, operation: Operation): void {
  if (key === 'details') showDetails(operation)
  if (key === 'cancel') void confirmCancel(operation)
  if (key === 'retry') void confirmRetry(operation)
  if (key === 'delete') void confirmDelete(operation)
  if (key === 'copy-id') void copyOperationID(operation)
}

onMounted(() => {
  void operations.refresh()
})
</script>

<template>
  <section class="operations-page">
    <NFlex align="center" justify="space-between" wrap class="page-header">
      <NFlex align="center" wrap>
        <NInput
          v-model:value="keyword"
          clearable
          placeholder="搜索 ID、阶段、消息或错误码"
          style="width: 260px"
        />
        <NSelect
          v-model:value="typeFilter"
          clearable
          placeholder="任务类型"
          :options="typeOptions"
          style="width: 140px"
        />
        <NSelect
          v-model:value="targetFilter"
          clearable
          placeholder="目标类型"
          :options="targetOptions"
          style="width: 150px"
        />
        <NSelect
          v-model:value="stateFilter"
          clearable
          placeholder="状态"
          :options="stateOptions"
          style="width: 130px"
        />
        <NButton quaternary @click="clearFilters">清除筛选</NButton>
      </NFlex>
      <NFlex align="center">
        <NButton
          type="error"
          secondary
          :disabled="operations.history.length === 0"
          :loading="clearingHistory"
          @click="confirmClearHistory"
          >清空历史</NButton
        >
        <NButton :loading="operations.loading" @click="operations.refresh">刷新</NButton>
      </NFlex>
    </NFlex>

    <NTabs type="line" animated>
      <NTabPane :tab="`活动 (${filteredActive.length}/${operations.active.length})`" name="active">
        <AppDataTable
          :columns="columns"
          :data="filteredActive"
          :loading="operations.loading"
          :error="operations.activeError"
          :partial-message="operations.partialMessage"
          :context-options="contextOptions"
          empty-description="当前没有活动 Operation"
          @retry="operations.refresh"
          @context-action="handleContextAction"
          @open="showDetails"
        />
      </NTabPane>
      <NTabPane
        :tab="`历史 (${filteredHistory.length}/${operations.history.length})`"
        name="history"
      >
        <AppDataTable
          :columns="columns"
          :data="filteredHistory"
          :loading="operations.loading"
          :error="operations.historyError"
          :partial-message="operations.partialMessage"
          :context-options="contextOptions"
          empty-description="暂无 Operation 历史"
          @retry="operations.refresh"
          @context-action="handleContextAction"
          @open="showDetails"
        />
      </NTabPane>
    </NTabs>
  </section>
</template>

<style scoped>
.operations-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
}

.operation-summary,
.operation-state,
.operation-activity {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.operation-summary__secondary,
.operation-summary__meta,
.operation-activity span {
  color: var(--text-muted);
  font-size: 12px;
  overflow-wrap: anywhere;
}

.operation-activity strong {
  overflow-wrap: anywhere;
}
</style>
