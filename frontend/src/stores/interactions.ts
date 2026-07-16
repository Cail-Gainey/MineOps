import { defineStore } from 'pinia'
import { ref } from 'vue'

export interface ConfirmRequest {
  title: string
  content: string
  objectLabel?: string
  impact?: string
  positiveText?: string
  negativeText?: string
  danger?: boolean
}

export interface DrawerRequest {
  title: string
  content: string
  width?: number
  dirty?: boolean
}

interface PendingConfirm extends ConfirmRequest {
  resolve: (confirmed: boolean) => void
}

export const useInteractionStore = defineStore('interactions', () => {
  const confirmRequest = ref<PendingConfirm | null>(null)
  const drawerRequest = ref<DrawerRequest | null>(null)

  /**
   * 打开全局确认对话框；已有弹层时拒绝嵌套。
   * @param request - 对象、影响和按钮文案等确认信息
   * @returns 用户确认时为 true，否则为 false
   */
  function confirm(request: ConfirmRequest): Promise<boolean> {
    if (confirmRequest.value || drawerRequest.value) return Promise.resolve(false)
    return new Promise((resolve) => {
      confirmRequest.value = { ...request, resolve }
    })
  }

  /**
   * 结束当前确认对话框。
   * @param confirmed - 用户是否确认
   * @returns void
   */
  function resolveConfirm(confirmed: boolean): void {
    const request = confirmRequest.value
    confirmRequest.value = null
    request?.resolve(confirmed)
  }

  /**
   * 打开用于长详情的全局抽屉；禁止与其他弹层嵌套。
   * @param request - 抽屉标题、正文、宽度和脏状态
   * @returns 是否成功打开
   */
  function openDrawer(request: DrawerRequest): boolean {
    if (confirmRequest.value || drawerRequest.value) return false
    drawerRequest.value = request
    return true
  }

  /**
   * 关闭详情抽屉；脏状态时先展示关闭确认。
   * @param force - 是否跳过脏状态确认
   * @returns 抽屉是否已关闭
   */
  async function closeDrawer(force = false): Promise<boolean> {
    const request = drawerRequest.value
    if (!request) return true
    if (request.dirty && !force) {
      drawerRequest.value = null
      const discard = await confirm({
        title: '放弃未保存修改？',
        content: '关闭后，当前抽屉中的未保存修改将丢失。',
        positiveText: '放弃并关闭',
        danger: true,
      })
      if (!discard) drawerRequest.value = request
      return discard
    }
    drawerRequest.value = null
    return true
  }

  return {
    closeDrawer,
    confirm,
    confirmRequest,
    drawerRequest,
    openDrawer,
    resolveConfirm,
  }
})
