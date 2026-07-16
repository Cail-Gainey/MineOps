import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useLayoutStore = defineStore('layout', () => {
  const sidebarCollapsed = ref(false)
  const sidebarVisible = ref(true)
  const sidebarWidth = ref(232)
  const topBarVisible = ref(true)
  const bottomBarVisible = ref(true)

  function toggleSidebar(): void {
    sidebarCollapsed.value = !sidebarCollapsed.value
  }

  return {
    bottomBarVisible,
    sidebarCollapsed,
    sidebarVisible,
    sidebarWidth,
    toggleSidebar,
    topBarVisible,
  }
})
