import { defineStore } from 'pinia'
import { ref } from 'vue'

export interface WorkspaceTab {
  id: string
  title: string
}

export const useWorkspaceStore = defineStore('workspace', () => {
  const activeTabID = ref<string | null>(null)
  const tabs = ref<WorkspaceTab[]>([])

  return { activeTabID, tabs }
})
