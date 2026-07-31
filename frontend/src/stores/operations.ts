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

  /**
   * 取任务的开始时间毫秒值，缺失时回落到创建时间。
   * @param operation - 目标任务
   * @returns 毫秒时间戳
   */
  function operationStartedAt(operation: Operation): number {
    const timestamp = Date.parse(operation.startedAt ?? operation.createdAt)
    return Number.isFinite(timestamp) ? timestamp : 0
  }

  /**
   * 按开始时间倒序排列任务。
   * @param items - 任务数组
   * @returns 排序后的新数组
   */
  function sortByStartedAt(items: Operation[]): Operation[] {
    return [...items].sort((left, right) => {
      const timeDifference = operationStartedAt(right) - operationStartedAt(left)
      return timeDifference === 0 ? right.id.localeCompare(left.id) : timeDifference
    })
  }

  /**
   * 按 ID 把一条任务合并进活动列表或历史列表。
   * @param operation - 目标任务
   * @returns 无返回值
   */
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

  /**
   * 重新加载活动任务与历史任务。
   * @returns 刷新完成后的 Promise
   */
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

  /**
   * 取消一条任务并刷新列表。
   * @param id - 任务 ID
   * @returns 取消完成后的 Promise
   */
  async function cancel(id: string): Promise<void> {
    await cancelOperation(id)
    await refresh()
  }

  /**
   * 删除一条任务历史记录。
   * @param id - 任务 ID
   * @returns 删除完成后的 Promise
   */
  async function deleteHistory(id: string): Promise<void> {
    await deleteOperationHistory(id)
    history.value = history.value.filter((operation) => operation.id !== id)
  }

  /**
   * 清空全部任务历史。
   * @returns 删除的记录数
   */
  async function clearHistory(): Promise<number> {
    const deleted = await clearOperationHistory()
    history.value = []
    return deleted
  }

  /**
   * 按 ID 拉取一条任务并合并到本地状态。
   * @param id - 任务 ID
   * @returns 任务详情
   */
  async function get(id: string): Promise<Operation> {
    const operation = await getOperation(id)
    merge(operation)
    return operation
  }

  /**
   * 重试一条失败的安装任务。
   * @param operation - 目标任务
   * @returns 新任务的 Operation ID
   */
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

  /**
   * 订阅任务进度推送。
   * @returns 无返回值
   */
  function startSubscription(): void {
    unsubscribe?.()
    unsubscribe = subscribeOperationProgress(merge)
  }

  /**
   * 取消任务进度订阅。
   * @returns 无返回值
   */
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
