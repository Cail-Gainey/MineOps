import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useLayoutStore = defineStore('layout', () => {
  const sidebarCollapsed = ref(false)
  const sidebarVisible = ref(true)
  const sidebarWidth = ref(232)
  const topBarVisible = ref(true)
  const bottomBarVisible = ref(true)

  /**
   * 折叠或展开侧栏。
   * @returns 无返回值
   */
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
