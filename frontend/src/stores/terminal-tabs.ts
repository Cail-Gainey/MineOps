import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import type { SSHSessionDTO } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/models'

export interface TerminalTab {
  id: string
  sshSessionID: string
  title: string
  target: string
}

export const useTerminalTabsStore = defineStore('terminal-tabs', () => {
  const tabs = ref<TerminalTab[]>([])
  const activeID = ref('')
  const activeTab = computed(() => tabs.value.find((tab) => tab.id === activeID.value) ?? null)

  /** Opens or activates a Terminal tab for one SSH Session. */
  function open(session: SSHSessionDTO): TerminalTab {
    const existing = tabs.value.find((tab) => tab.sshSessionID === session.id)
    if (existing) {
      existing.title = session.name
      existing.target = `${session.username}@${session.host}:${session.port}`
      activeID.value = existing.id
      return existing
    }
    const tab: TerminalTab = {
      id: crypto.randomUUID(),
      sshSessionID: session.id,
      title: session.name,
      target: `${session.username}@${session.host}:${session.port}`,
    }
    tabs.value.push(tab)
    activeID.value = tab.id
    return tab
  }

  /** Closes one tab and selects the nearest remaining tab. */
  function close(id: string): void {
    const index = tabs.value.findIndex((tab) => tab.id === id)
    if (index < 0) return
    tabs.value.splice(index, 1)
    if (activeID.value === id) {
      activeID.value = tabs.value[Math.min(index, tabs.value.length - 1)]?.id ?? ''
    }
  }

  /** Closes every Terminal tab that references one removed SSH Session. */
  function closeForSSHSession(sshSessionID: string): void {
    const ids = tabs.value.filter((tab) => tab.sshSessionID === sshSessionID).map((tab) => tab.id)
    for (const id of ids) close(id)
  }

  /** Renames a Terminal tab without changing its SSH Session identity. */
  function rename(id: string, title: string): void {
    const tab = tabs.value.find((item) => item.id === id)
    if (tab && title.trim()) tab.title = title.trim()
  }

  return { activeID, activeTab, close, closeForSSHSession, open, rename, tabs }
})
