import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import { AlertRule } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import type { AlertEvent } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import {
  acknowledgeAlert,
  createAlertRule,
  deleteAlertEvent,
  deleteAlertRule,
  listAlertEvents,
  listAlertRules,
  subscribeAlertEvents,
  updateAlertRule,
} from '../services/alert-api'

export const useAlertsStore = defineStore('alerts', () => {
  const serverID = ref('')
  const rules = ref<AlertRule[]>([])
  const events = ref<AlertEvent[]>([])
  const loading = ref(false)
  const rulesError = ref<unknown>(null)
  const eventsError = ref<unknown>(null)
  const error = computed(() => (rulesError.value && eventsError.value ? rulesError.value : null))
  const partialMessage = computed(() =>
    rulesError.value
      ? '告警规则暂时不可用，事件历史仍可浏览。'
      : eventsError.value
        ? '告警事件暂时不可用，规则仍可管理。'
        : '',
  )
  let unsubscribe: (() => void) | null = null

  const activeEvents = computed(() => events.value.filter((event) => event.state === 'active'))
  const historicalEvents = computed(() => events.value.filter((event) => event.state !== 'active'))

  /**
   * 重新加载指定 Server 的阈值规则与告警事件。
   * @param selectedServerID - 目标 Server ID，默认沿用当前选中项
   * @returns 刷新完成后的 Promise
   */
  async function refresh(selectedServerID = serverID.value): Promise<void> {
    serverID.value = selectedServerID
    loading.value = true
    rulesError.value = null
    eventsError.value = null
    try {
      if (!serverID.value) {
        rules.value = []
        events.value = []
        return
      }
      const [rulesResult, eventsResult] = await Promise.allSettled([
        listAlertRules(serverID.value),
        listAlertEvents(serverID.value),
      ])
      if (rulesResult.status === 'fulfilled') rules.value = rulesResult.value
      else rulesError.value = rulesResult.reason
      if (eventsResult.status === 'fulfilled') events.value = eventsResult.value
      else eventsError.value = eventsResult.reason
    } finally {
      loading.value = false
    }
  }

  /**
   * 创建一条阈值规则并刷新列表。
   * @param rule - 待创建的规则
   * @returns 创建完成后的 Promise
   */
  async function create(rule: AlertRule): Promise<void> {
    await createAlertRule(rule)
    await refresh()
  }

  /**
   * 更新一条阈值规则并刷新列表。
   * @param rule - 含 ID 的规则内容
   * @returns 更新完成后的 Promise
   */
  async function update(rule: AlertRule): Promise<void> {
    await updateAlertRule(rule)
    await refresh()
  }

  /**
   * 删除一条阈值规则并刷新列表。
   * @param ruleID - 规则 ID
   * @returns 删除完成后的 Promise
   */
  async function remove(ruleID: string): Promise<void> {
    await deleteAlertRule(ruleID)
    await refresh()
  }

  /**
   * 确认一条告警事件并就地更新列表。
   * @param eventID - 事件 ID
   * @returns 确认完成后的 Promise
   */
  async function acknowledge(eventID: string): Promise<void> {
    const updated = await acknowledgeAlert(eventID)
    const index = events.value.findIndex((event) => event.id === updated.id)
    if (index >= 0) events.value[index] = updated
  }

  /**
   * 删除一条告警事件并就地更新列表。
   * @param eventID - 事件 ID
   * @returns 删除完成后的 Promise
   */
  async function removeEvent(eventID: string): Promise<void> {
    await deleteAlertEvent(eventID)
    const index = events.value.findIndex((event) => event.id === eventID)
    if (index >= 0) events.value.splice(index, 1)
  }

  /**
   * 订阅告警事件推送。
   * @returns 无返回值
   */
  function startSubscription(): void {
    unsubscribe?.()
    unsubscribe = subscribeAlertEvents((event) => {
      if (event.serverID !== serverID.value) return
      const index = events.value.findIndex((candidate) => candidate.id === event.id)
      if (index >= 0) events.value[index] = event
      else events.value.unshift(event)
    })
  }

  /**
   * 取消告警事件订阅。
   * @returns 无返回值
   */
  function stopSubscription(): void {
    unsubscribe?.()
    unsubscribe = null
  }

  return {
    acknowledge,
    activeEvents,
    create,
    error,
    events,
    historicalEvents,
    partialMessage,
    rulesError,
    eventsError,
    loading,
    refresh,
    remove,
    removeEvent,
    rules,
    serverID,
    startSubscription,
    stopSubscription,
    update,
  }
})
