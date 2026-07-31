import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

export interface ErrorEntry {
  id: number
  message: string
  source: string
  occurredAt: Date
  error: unknown
}

export const useErrorCenterStore = defineStore('error-center', () => {
  const entries = ref<ErrorEntry[]>([])
  let nextID = 0

  const latest = computed(() => entries.value[0] ?? null)

  /**
   * 把一个异常记入错误中心。
   * @param error - 捕获到的错误
   * @param source - 错误来源标识
   * @returns 无返回值
   */
  function capture(error: unknown, source: string): void {
    const message = error instanceof Error ? error.message : String(error)
    entries.value = [
      { id: ++nextID, message, source, occurredAt: new Date(), error },
      ...entries.value,
    ].slice(0, 100)
  }

  /**
   * 移除一条错误记录。
   * @param id - 错误记录 ID
   * @returns 无返回值
   */
  function dismiss(id: number): void {
    entries.value = entries.value.filter((entry) => entry.id !== id)
  }

  /**
   * 清空全部错误记录。
   * @returns 无返回值
   */
  function clear(): void {
    entries.value = []
  }

  return { capture, clear, dismiss, entries, latest }
})
