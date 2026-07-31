import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { translate } from '../locales/runtime'

import type { AlertEvent } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import {
  acknowledgeAlert,
  deleteAlertEvent,
  listAlertEvents,
  subscribeAlertEvents,
} from '../services/alert-api'
import { formatMetricLabel } from '../shared/monitoring/metric-labels'
import { useNotificationStore } from './notifications'
import { useSettingsStore } from './settings'

const rememberedAlertMaximum = 1000
const rememberedAlertLifetime = 24 * 60 * 60 * 1000

export const useAlertNotificationStore = defineStore('alert-notifications', () => {
  const notifications = useNotificationStore()
  const settings = useSettingsStore()
  const activeEvents = ref<AlertEvent[]>([])
  const loading = ref(false)
  const error = ref<unknown>(null)
  const activeCount = computed(() => activeEvents.value.length)
  const rememberedActiveEvents = new Map<string, number>()
  let unsubscribe: (() => void) | null = null
  let audioContext: AudioContext | null = null

  /**
   * 记录该告警已通知过，用于抑制重复弹窗。
   * @param eventID - 告警事件 ID
   * @returns 无返回值
   */
  function remember(eventID: string): void {
    const now = Date.now()
    rememberedActiveEvents.set(eventID, now)
    if (rememberedActiveEvents.size <= rememberedAlertMaximum) return
    for (const [id, createdAt] of rememberedActiveEvents) {
      if (now - createdAt > rememberedAlertLifetime) rememberedActiveEvents.delete(id)
    }
    while (rememberedActiveEvents.size > rememberedAlertMaximum) {
      const oldest = rememberedActiveEvents.keys().next().value
      if (typeof oldest !== 'string') break
      rememberedActiveEvents.delete(oldest)
    }
  }

  /**
   * 判断当前时刻是否落在静默时段内，支持跨零点区间。
   * @param now - 当前时刻
   * @param start - 静默开始时刻
   * @param end - 静默结束时刻
   * @returns 处于静默时段时返回 true
   */
  function isQuietTime(now: Date, start: string, end: string): boolean {
    if (!start || !end || start === end) return false
    const currentMinutes = now.getHours() * 60 + now.getMinutes()
    const startMinutes = Number(start.slice(0, 2)) * 60 + Number(start.slice(3, 5))
    const endMinutes = Number(end.slice(0, 2)) * 60 + Number(end.slice(3, 5))
    return startMinutes < endMinutes
      ? currentMinutes >= startMinutes && currentMinutes < endMinutes
      : currentMinutes >= startMinutes || currentMinutes < endMinutes
  }

  /**
   * 播放一次告警提示音。
   * @returns 播放完成后的 Promise
   */
  async function playAlertSound(): Promise<void> {
    try {
      const context = audioContext ?? new AudioContext()
      audioContext = context
      if (context.state === 'suspended') await context.resume()
      const oscillator = context.createOscillator()
      const gain = context.createGain()
      const startedAt = context.currentTime
      oscillator.type = 'sine'
      oscillator.frequency.setValueAtTime(880, startedAt)
      oscillator.frequency.exponentialRampToValueAtTime(660, startedAt + 0.22)
      gain.gain.setValueAtTime(0.0001, startedAt)
      gain.gain.exponentialRampToValueAtTime(0.12, startedAt + 0.02)
      gain.gain.exponentialRampToValueAtTime(0.0001, startedAt + 0.24)
      oscillator.connect(gain)
      gain.connect(context.destination)
      oscillator.start(startedAt)
      oscillator.stop(startedAt + 0.25)
    } catch {
      // WebView 未授权音频播放时静默跳过，不影响告警事件和桌面通知。
    }
  }

  /**
   * 按事件 ID 增量合并活跃告警列表。
   * @param event - 告警事件
   * @returns 无返回值
   */
  function mergeAlertEvent(event: AlertEvent): void {
    const index = activeEvents.value.findIndex((candidate) => candidate.id === event.id)
    if (event.state !== 'active') {
      if (index >= 0) activeEvents.value.splice(index, 1)
      return
    }
    if (index >= 0) activeEvents.value[index] = event
    else activeEvents.value.unshift(event)
  }

  /**
   * 合并告警并按静默时段与去重规则决定是否提醒。
   * @param event - 告警事件
   * @returns 无返回值
   */
  function handleAlert(event: AlertEvent): void {
    mergeAlertEvent(event)
    if (event.state !== 'active' || rememberedActiveEvents.has(event.id)) return
    remember(event.id)
    if (event.acknowledgedAt) return
    const monitoring = settings.committed?.monitoring
    if (!monitoring || monitoring.alertNotifications.length === 0) return
    if (isQuietTime(new Date(), monitoring.quietHoursStart, monitoring.quietHoursEnd)) return
    const channels = new Set(monitoring.alertNotifications)
    if (channels.has('desktop')) {
      notifications.push({
        kind: 'warning',
        title: translate('store.alertNotificationTitle', {
          metric: formatMetricLabel(event.metric),
        }),
        content: translate('store.alertNotificationContent', {
          value: event.latestValue.toFixed(2),
          threshold: event.threshold.toFixed(2),
          server: event.serverID,
        }),
        dedupeKey: `alert:active:${event.id}`,
        duration: 8000,
      })
    }
    if (channels.has('sound')) void playAlertSound()
  }

  /**
   * 加载所有服务器当前活动的阈值告警。
   * @returns 活动告警刷新完成后的 Promise
   */
  async function refresh(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      activeEvents.value = await listAlertEvents('', 'active')
    } catch (reason) {
      error.value = reason
      throw reason
    } finally {
      loading.value = false
    }
  }

  /**
   * 确认一个活动告警并合并服务端返回状态。
   * @param eventID - 告警事件 ID
   * @returns 告警确认完成后的 Promise
   */
  async function acknowledge(eventID: string): Promise<void> {
    const event = await acknowledgeAlert(eventID)
    mergeAlertEvent(event)
  }

  /**
   * 永久删除一个告警事件并从底栏活动列表移除。
   * @param eventID - 告警事件 ID
   * @returns 告警删除完成后的 Promise
   */
  async function remove(eventID: string): Promise<void> {
    await deleteAlertEvent(eventID)
    const index = activeEvents.value.findIndex((event) => event.id === eventID)
    if (index >= 0) activeEvents.value.splice(index, 1)
  }

  /**
   * 订阅告警事件推送。
   * @returns 无返回值
   */
  function start(): void {
    if (unsubscribe) return
    unsubscribe = subscribeAlertEvents(handleAlert)
    void refresh().catch(() => undefined)
  }

  /**
   * 取消告警事件订阅。
   * @returns 无返回值
   */
  function stop(): void {
    unsubscribe?.()
    unsubscribe = null
    if (audioContext) void audioContext.close().catch(() => undefined)
    audioContext = null
  }

  return { acknowledge, activeCount, activeEvents, error, loading, refresh, remove, start, stop }
})
