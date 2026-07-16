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

  /** Loads rules and incident history for one Server. */
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

  /** Creates one rule for the active Server. */
  async function create(rule: AlertRule): Promise<void> {
    await createAlertRule(rule)
    await refresh()
  }

  /** Saves enablement or threshold changes to one rule. */
  async function update(rule: AlertRule): Promise<void> {
    await updateAlertRule(rule)
    await refresh()
  }

  /** Deletes one rule and refreshes recovered incident state. */
  async function remove(ruleID: string): Promise<void> {
    await deleteAlertRule(ruleID)
    await refresh()
  }

  /** Acknowledges one incident and merges the returned durable state. */
  async function acknowledge(eventID: string): Promise<void> {
    const updated = await acknowledgeAlert(eventID)
    const index = events.value.findIndex((event) => event.id === updated.id)
    if (index >= 0) events.value[index] = updated
  }

  /** Permanently deletes one incident from the active Server history. */
  async function removeEvent(eventID: string): Promise<void> {
    await deleteAlertEvent(eventID)
    const index = events.value.findIndex((event) => event.id === eventID)
    if (index >= 0) events.value.splice(index, 1)
  }

  /** Starts the global durable Alert Event subscription. */
  function startSubscription(): void {
    unsubscribe?.()
    unsubscribe = subscribeAlertEvents((event) => {
      if (event.serverID !== serverID.value) return
      const index = events.value.findIndex((candidate) => candidate.id === event.id)
      if (index >= 0) events.value[index] = event
      else events.value.unshift(event)
    })
  }

  /** Stops the Alert Event subscription. */
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
