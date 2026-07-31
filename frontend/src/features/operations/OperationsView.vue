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
import { hasMessage } from '../../locales/runtime'
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

const operationTypes = [
  'install',
  'download',
  'upload',
  'extract',
  'start',
  'stop',
  'restart',
  'backup',
  'restore',
  'update',
  'delete',
  'profile',
]
const operationTargets = [
  'server',
  'ssh_session',
  'file',
  'java_runtime',
  'agent',
  'settings',
  'spark',
]
const operationStates = ['pending', 'running', 'succeeded', 'failed', 'cancelled']

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
 * 把任务类型映射成本地化标签。
 * @param value - 任务类型标识
 * @returns 本地化标签，未知类型原样返回
 */
function typeLabel(value: string): string {
  return enumLabel('operations.type', value)
}

/**
 * 把任务目标类型映射成本地化标签。
 * @param value - 目标类型标识
 * @returns 本地化标签，未知类型原样返回
 */
function targetLabel(value: string): string {
  return enumLabel('operations.target', value)
}

/**
 * 把任务状态映射成本地化标签。
 * @param value - 任务状态标识
 * @returns 本地化标签，未知状态原样返回
 */
function stateLabel(value: string): string {
  return enumLabel('operations.state', value)
}

const typeOptions = computed(() =>
  operationTypes.map((value) => ({ label: typeLabel(value), value })),
)
const targetOptions = computed(() =>
  operationTargets.map((value) => ({ label: targetLabel(value), value })),
)
const stateOptions = computed(() =>
  operationStates.map((value) => ({ label: stateLabel(value), value })),
)

/**
 * 判断一条任务是否满足当前的关键字与筛选条件。
 * @param operation - 待判断的任务
 * @returns 满足全部筛选条件时返回 true
 */
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

/**
 * 清空关键字与全部筛选条件。
 * @returns 无返回值
 */
function clearFilters(): void {
  keyword.value = ''
  typeFilter.value = null
  targetFilter.value = null
  stateFilter.value = null
}

/**
 * 判断某条任务是否有进行中的操作。
 * @param id - 任务 ID
 * @returns 有进行中操作时返回 true
 */
function operationActionPending(id: string): boolean {
  return pendingActionIDs.value.has(id)
}

/**
 * 标记或清除某条任务的操作进行中状态。
 * @param id - 任务 ID
 * @param pending - 是否进行中
 * @returns 无返回值
 */
function setOperationActionPending(id: string, pending: boolean): void {
  const next = new Set(pendingActionIDs.value)
  if (pending) next.add(id)
  else next.delete(id)
  pendingActionIDs.value = next
}

/**
 * 判断一条任务是否可以重试。
 * @param operation - 待判断的任务
 * @returns 可重试时返回 true
 */
function canRetry(operation: Operation): boolean {
  return (
    operation.type === 'install' &&
    (operation.state === 'failed' || operation.state === 'cancelled')
  )
}

/**
 * 把任务状态映射成标签配色。
 * @param state - 任务状态标识
 * @returns naive-ui 标签的语义类型
 */
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

const columns = computed<DataTableColumns<Operation>>(() => [
  {
    title: locale.t('operations.column.task'),
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
    title: locale.t('operations.column.progress'),
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
    title: locale.t('operations.column.activity'),
    key: 'message',
    render: (row) =>
      h('div', { class: 'operation-activity' }, [
        h('strong', row.stage || locale.t('operations.pendingStage')),
        h('span', row.message || locale.t('operations.noMessage')),
      ]),
  },
  {
    title: locale.t('common.actions'),
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
              { default: () => locale.t('common.detail') },
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
                  { default: () => locale.t('common.cancel') },
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
                  { default: () => locale.t('operations.retry') },
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
                  { default: () => locale.t('common.delete') },
                )
              : null,
          ],
        },
      ),
  },
])

/**
 * 按任务当前状态构建右键菜单项。
 * @param row - 当前行的任务
 * @returns 下拉菜单项数组
 */
function contextOptions(row: Operation): DropdownOption[] {
  const options: DropdownOption[] = [
    { label: locale.t('operations.context.details'), key: 'details' },
  ]
  if (row.state === 'pending' || row.state === 'running') {
    options.push(
      { type: 'divider', key: 'divider' },
      { label: locale.t('operations.context.cancel'), key: 'cancel' },
    )
  }
  if (canRetry(row)) {
    options.push(
      { type: 'divider', key: 'retry-divider' },
      { label: locale.t('operations.context.retry'), key: 'retry' },
    )
  }
  if (row.state !== 'pending' && row.state !== 'running') {
    options.push(
      { type: 'divider', key: 'delete-divider' },
      { label: locale.t('operations.context.delete'), key: 'delete' },
    )
  }
  options.push({ label: locale.t('operations.context.copyID'), key: 'copy-id' })
  return options
}

/**
 * 二次确认后取消一条进行中的任务。
 * @param operation - 目标任务
 * @returns 取消完成后的 Promise
 */
async function confirmCancel(operation: Operation): Promise<void> {
  const confirmed = await interactions.confirm({
    title: locale.t('operations.cancelTitle'),
    content: locale.t('operations.cancelContent'),
    objectLabel: `${typeLabel(operation.type)} · ${operation.id}`,
    impact: operation.message || operation.stage,
    positiveText: locale.t('operations.cancelConfirm'),
    danger: true,
  })
  if (!confirmed) return
  setOperationActionPending(operation.id, true)
  try {
    await operations.cancel(operation.id)
    notifications.push({
      kind: 'warning',
      title: locale.t('operations.cancelSubmitted'),
      content: operation.message,
      dedupeKey: `operation:cancel:${operation.id}`,
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: locale.t('operations.cancelFailed'),
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: `operation:cancel-error:${operation.id}`,
    })
  } finally {
    setOperationActionPending(operation.id, false)
  }
}

/**
 * 二次确认后重试一条失败的安装任务。
 * @param operation - 目标任务
 * @returns 重试完成后的 Promise
 */
async function confirmRetry(operation: Operation): Promise<void> {
  const confirmed = await interactions.confirm({
    title: locale.t('operations.retryTitle'),
    content: locale.t('operations.retryContent'),
    objectLabel: `${typeLabel(operation.type)} · ${operation.targetID}`,
    impact: operation.message || operation.errorCode || operation.stage,
    positiveText: locale.t('operations.retryConfirm'),
  })
  if (!confirmed) return
  setOperationActionPending(operation.id, true)
  try {
    const replacementID = await operations.retry(operation)
    notifications.push({
      kind: 'success',
      title: locale.t('operations.retryStarted'),
      content: locale.t('operations.retryStartedContent', { id: replacementID }),
      dedupeKey: `operation:retry:${operation.id}:${replacementID}`,
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: locale.t('operations.retryFailed'),
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: `operation:retry-error:${operation.id}`,
    })
  } finally {
    setOperationActionPending(operation.id, false)
  }
}

/**
 * 二次确认后删除一条任务记录。
 * @param operation - 目标任务
 * @returns 删除完成后的 Promise
 */
async function confirmDelete(operation: Operation): Promise<void> {
  const confirmed = await interactions.confirm({
    title: locale.t('operations.deleteTitle'),
    content: locale.t('operations.deleteContent'),
    objectLabel: `${typeLabel(operation.type)} · ${operation.id}`,
    impact: operation.message || operation.errorCode || operation.stage,
    positiveText: locale.t('operations.deleteConfirm'),
    danger: true,
  })
  if (!confirmed) return
  setOperationActionPending(operation.id, true)
  try {
    await operations.deleteHistory(operation.id)
    notifications.push({
      kind: 'success',
      title: locale.t('operations.deleted'),
      content: operation.id,
      dedupeKey: `operation:delete:${operation.id}`,
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: locale.t('operations.deleteFailed'),
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: `operation:delete-error:${operation.id}`,
    })
  } finally {
    setOperationActionPending(operation.id, false)
  }
}

/**
 * 二次确认后清空全部任务历史。
 * @returns 清空完成后的 Promise
 */
async function confirmClearHistory(): Promise<void> {
  if (!operations.history.length) return
  const confirmed = await interactions.confirm({
    title: locale.t('operations.clearTitle'),
    content: locale.t('operations.clearContent'),
    objectLabel: locale.t('operations.clearObject', { count: operations.history.length }),
    impact: locale.t('operations.clearImpact'),
    positiveText: locale.t('operations.clearConfirm'),
    danger: true,
  })
  if (!confirmed) return
  clearingHistory.value = true
  try {
    const deleted = await operations.clearHistory()
    notifications.push({
      kind: 'success',
      title: locale.t('operations.cleared'),
      content: locale.t('operations.clearedContent', { count: deleted }),
      dedupeKey: 'operation:clear-history',
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: locale.t('operations.clearFailed'),
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'operation:clear-history-error',
    })
  } finally {
    clearingHistory.value = false
  }
}

/**
 * 把任务详情对象格式化成可读的 JSON 文本。
 * @param details - 任务详情，可为空
 * @returns JSON 文本，无内容时返回短横线
 */
function formatDetails(details: Record<string, unknown> | undefined): string {
  if (!details || Object.keys(details).length === 0) return '-'
  return JSON.stringify(details, null, 2)
}

/**
 * 拉取最新任务状态并打开详情抽屉。
 * @param operation - 目标任务
 * @returns 打开完成后的 Promise
 */
async function showDetails(operation: Operation): Promise<void> {
  let current = operation
  try {
    current = await operations.get(operation.id)
  } catch {
    // 保留列表中的最后已知状态，详情仍然可查看。
  }
  interactions.openDrawer({
    title: locale.t('operations.detailsTitle', { type: typeLabel(current.type) }),
    content: [
      locale.t('operations.details.id', { value: current.id }),
      locale.t('operations.details.type', { value: typeLabel(current.type) }),
      locale.t('operations.details.target', {
        type: targetLabel(current.targetType),
        id: current.targetID,
      }),
      locale.t('operations.details.state', { value: stateLabel(current.state) }),
      locale.t('operations.details.stage', { value: current.stage || '-' }),
      locale.t('operations.details.progress', { value: Math.round(current.progress * 100) }),
      locale.t('operations.details.message', { value: current.message || '-' }),
      locale.t('operations.details.errorCode', { value: current.errorCode ?? '-' }),
      locale.t('operations.details.errorDetails', { value: formatDetails(current.errorDetails) }),
      locale.t('operations.details.retryOf', { value: current.retryOf ?? '-' }),
      locale.t('operations.details.createdAt', { value: locale.formatDateTime(current.createdAt) }),
      locale.t('operations.details.startedAt', {
        value: current.startedAt ? locale.formatDateTime(current.startedAt) : '-',
      }),
      locale.t('operations.details.finishedAt', {
        value: current.finishedAt ? locale.formatDateTime(current.finishedAt) : '-',
      }),
    ].join('\n'),
    width: 620,
  })
}

/**
 * 把任务 ID 复制到剪贴板。
 * @param operation - 目标任务
 * @returns 复制完成后的 Promise
 */
async function copyOperationID(operation: Operation): Promise<void> {
  try {
    await navigator.clipboard.writeText(operation.id)
    notifications.push({
      kind: 'success',
      title: locale.t('operations.copySucceeded'),
      content: operation.id,
      dedupeKey: `operation:copy:${operation.id}`,
    })
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: locale.t('operations.copyFailed'),
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: `operation:copy-error:${operation.id}`,
    })
  }
}

/**
 * 分发右键菜单选中的动作。
 * @param key - 菜单项 key
 * @param operation - 当前行的任务
 * @returns 无返回值
 */
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
          :placeholder="locale.t('operations.searchPlaceholder')"
          style="width: 260px"
        />
        <NSelect
          v-model:value="typeFilter"
          clearable
          :placeholder="locale.t('operations.filterType')"
          :options="typeOptions"
          style="width: 140px"
        />
        <NSelect
          v-model:value="targetFilter"
          clearable
          :placeholder="locale.t('operations.filterTarget')"
          :options="targetOptions"
          style="width: 150px"
        />
        <NSelect
          v-model:value="stateFilter"
          clearable
          :placeholder="locale.t('operations.filterState')"
          :options="stateOptions"
          style="width: 130px"
        />
        <NButton quaternary @click="clearFilters">
          {{ locale.t('operations.clearFilters') }}
        </NButton>
      </NFlex>
      <NFlex align="center">
        <NButton
          type="error"
          secondary
          :disabled="operations.history.length === 0"
          :loading="clearingHistory"
          @click="confirmClearHistory"
        >
          {{ locale.t('operations.clearHistory') }}
        </NButton>
        <NButton :loading="operations.loading" @click="operations.refresh">
          {{ locale.t('common.refresh') }}
        </NButton>
      </NFlex>
    </NFlex>

    <NTabs type="line" animated>
      <NTabPane
        :tab="
          locale.t('operations.tabActive', {
            filtered: filteredActive.length,
            total: operations.active.length,
          })
        "
        name="active"
      >
        <AppDataTable
          :columns="columns"
          :data="filteredActive"
          :loading="operations.loading"
          :error="operations.activeError"
          :partial-message="operations.partialMessage"
          :context-options="contextOptions"
          :empty-description="locale.t('operations.emptyActive')"
          @retry="operations.refresh"
          @context-action="handleContextAction"
          @open="showDetails"
        />
      </NTabPane>
      <NTabPane
        :tab="
          locale.t('operations.tabHistory', {
            filtered: filteredHistory.length,
            total: operations.history.length,
          })
        "
        name="history"
      >
        <AppDataTable
          :columns="columns"
          :data="filteredHistory"
          :loading="operations.loading"
          :error="operations.historyError"
          :partial-message="operations.partialMessage"
          :context-options="contextOptions"
          :empty-description="locale.t('operations.emptyHistory')"
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
