import { defineStore } from 'pinia'
import { ref } from 'vue'

export type NotificationKind = 'success' | 'warning' | 'error' | 'info'

export interface AppNotification {
  id: number
  kind: NotificationKind
  title: string
  content?: string
  dedupeKey: string
  createdAt: number
  duration: number
}

export interface NotificationInput {
  kind: NotificationKind
  title: string
  content?: string
  dedupeKey?: string
  duration?: number
}

export const useNotificationStore = defineStore('notifications', () => {
  const pending = ref<AppNotification[]>([])
  const recent = new Map<string, number>()
  let nextID = 0

  /**
   * 将通知加入统一队列，并在短时间窗口内按业务键去重。
   * @param input - 通知类型、文案、去重键与显示时长
   * @returns 通知已入队时返回 true，被去重时返回 false
   */
  function push(input: NotificationInput): boolean {
    const now = Date.now()
    const dedupeKey = input.dedupeKey ?? `${input.kind}:${input.title}:${input.content ?? ''}`
    const lastShownAt = recent.get(dedupeKey) ?? 0
    if (now - lastShownAt < 3000) return false

    recent.set(dedupeKey, now)
    pending.value.push({
      id: ++nextID,
      kind: input.kind,
      title: input.title,
      ...(input.content === undefined ? {} : { content: input.content }),
      dedupeKey,
      createdAt: now,
      duration: input.duration ?? (input.kind === 'error' ? 6000 : 3500),
    })
    if (recent.size > 200) {
      for (const [key, createdAt] of recent) {
        if (now - createdAt > 60_000) recent.delete(key)
      }
    }
    return true
  }

  /**
   * 从待展示队列中移除已消费通知。
   * @param id - 通知标识
   * @returns void
   */
  function consume(id: number): void {
    pending.value = pending.value.filter((item) => item.id !== id)
  }

  return { consume, pending, push }
})
