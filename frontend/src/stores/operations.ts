import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import type { Operation } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import {
  cancelOperation,
  clearOperationHistory,
  deleteOperationHistory,
  getOperation,
  listActiveOperations,
  listOperationHistory,
  subscribeOperationProgress,
} from '../services/operation-api'
import { listServerInstallations, retryInstallation } from '../services/installation-api'
import { useNotificationStore } from './notifications'

export const useOperationsStore = defineStore('operations', () => {
  const notifications = useNotificationStore()
  const active = ref<Operation[]>([])
  const history = ref<Operation[]>([])
  const loading = ref(false)
  const activeError = ref<unknown>(null)
  const historyError = ref<unknown>(null)
  const error = computed(() => (activeError.value && historyError.value ? activeError.value : null))
  const partialMessage = computed(() => {
    if (activeError.value && !historyError.value)
      return '活动 Operation 暂时不可用，历史记录仍可浏览。'
    if (historyError.value && !activeError.value)
      return 'Operation 历史暂时不可用，活动任务仍在实时更新。'
    return ''
  })
  let unsubscribe: (() => void) | null = null

  const activeCount = computed(() => active.value.length)

  function operationStartedAt(operation: Operation): number {
    const timestamp = Date.parse(operation.startedAt ?? operation.createdAt)
    return Number.isFinite(timestamp) ? timestamp : 0
  }

  function sortByStartedAt(items: Operation[]): Operation[] {
    return [...items].sort((left, right) => {
      const timeDifference = operationStartedAt(right) - operationStartedAt(left)
      return timeDifference === 0 ? right.id.localeCompare(left.id) : timeDifference
    })
  }

  function merge(operation: Operation): void {
    const previous =
      active.value.find((item) => item.id === operation.id) ??
      history.value.find((item) => item.id === operation.id)
    const isActive = operation.state === 'pending' || operation.state === 'running'
    active.value = isActive
      ? sortByStartedAt([operation, ...active.value.filter((item) => item.id !== operation.id)])
      : active.value.filter((item) => item.id !== operation.id)
    if (!isActive) {
      history.value = sortByStartedAt([
        operation,
        ...history.value.filter((item) => item.id !== operation.id),
      ])
    }
    if (previous?.state !== operation.state && !isActive) {
      notifications.push({
        kind:
          operation.state === 'succeeded'
            ? 'success'
            : operation.state === 'failed'
              ? 'error'
              : 'warning',
        title: `Operation ${operation.state}`,
        content: operation.message || `${operation.type} · ${operation.targetType}`,
        dedupeKey: `operation:state:${operation.id}:${operation.state}`,
      })
    }
  }

  async function refresh(): Promise<void> {
    loading.value = true
    activeError.value = null
    historyError.value = null
    try {
      const [activeResult, historyResult] = await Promise.allSettled([
        listActiveOperations(),
        listOperationHistory(),
      ])
      if (activeResult.status === 'fulfilled') active.value = sortByStartedAt(activeResult.value)
      else activeError.value = activeResult.reason
      if (historyResult.status === 'fulfilled') history.value = sortByStartedAt(historyResult.value)
      else historyError.value = historyResult.reason
    } finally {
      loading.value = false
    }
  }

  async function cancel(id: string): Promise<void> {
    await cancelOperation(id)
    await refresh()
  }

  /** Deletes one terminal Operation from durable history. */
  async function deleteHistory(id: string): Promise<void> {
    await deleteOperationHistory(id)
    history.value = history.value.filter((operation) => operation.id !== id)
  }

  /** Clears all terminal Operation history and returns the deleted row count. */
  async function clearHistory(): Promise<number> {
    const deleted = await clearOperationHistory()
    history.value = []
    return deleted
  }

  /** Loads the latest durable state for one Operation. */
  async function get(id: string): Promise<Operation> {
    const operation = await getOperation(id)
    merge(operation)
    return operation
  }

  /** Retries a checkpointed installation Operation through its owning Installation Task. */
  async function retry(operation: Operation): Promise<string> {
    if (
      operation.type !== 'install' ||
      (operation.state !== 'failed' && operation.state !== 'cancelled')
    ) {
      throw new Error('当前 Operation 不支持安全重试')
    }
    const tasks = await listServerInstallations(operation.targetID, 200)
    const task = tasks.find((item) => item.operationID === operation.id)
    if (!task) {
      throw new Error('未找到该 Operation 对应的 Installation Task，可能已被后续重试替代')
    }
    const started = await retryInstallation(task.id)
    await refresh()
    return started.operationID
  }

  function startSubscription(): void {
    unsubscribe?.()
    unsubscribe = subscribeOperationProgress(merge)
  }

  function stopSubscription(): void {
    unsubscribe?.()
    unsubscribe = null
  }

  return {
    active,
    activeError,
    activeCount,
    cancel,
    clearHistory,
    deleteHistory,
    error,
    get,
    history,
    historyError,
    loading,
    partialMessage,
    refresh,
    retry,
    startSubscription,
    stopSubscription,
  }
})
