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

  function capture(error: unknown, source: string): void {
    const message = error instanceof Error ? error.message : String(error)
    entries.value = [
      { id: ++nextID, message, source, occurredAt: new Date(), error },
      ...entries.value,
    ].slice(0, 100)
  }

  function dismiss(id: number): void {
    entries.value = entries.value.filter((entry) => entry.id !== id)
  }

  function clear(): void {
    entries.value = []
  }

  return { capture, clear, dismiss, entries, latest }
})
