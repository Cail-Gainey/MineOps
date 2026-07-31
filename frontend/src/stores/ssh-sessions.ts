import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import type { SSHSessionDTO } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/models'
import { deleteSSHSession, listSSHSessions, updateSSHSession } from '../services/ssh-session-api'

export const useSSHSessionsStore = defineStore('ssh-sessions', () => {
  const sessions = ref<SSHSessionDTO[]>([])
  const search = ref('')
  const group = ref('')
  const favouriteOnly = ref(false)
  const loading = ref(false)
  const error = ref<unknown>(null)
  const partialMessage = computed(() =>
    error.value && sessions.value.length
      ? 'SSH Sessions 刷新失败，继续展示上一次成功加载的结果。'
      : '',
  )
  const groups = computed(() =>
    [...new Set(sessions.value.map((session) => session.group).filter(Boolean))].sort(),
  )

  /**
   * 重新加载 SSH Session 列表。
   * @returns 刷新完成后的 Promise
   */
  async function refresh(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      sessions.value = await listSSHSessions(search.value, group.value, favouriteOnly.value)
    } catch (reason) {
      error.value = reason
      throw reason
    } finally {
      loading.value = false
    }
  }

  /**
   * 按 ID 把一个 SSH Session 合并进本地列表。
   * @param session - 目标 SSH Session
   * @returns 无返回值
   */
  function apply(session: SSHSessionDTO): void {
    const index = sessions.value.findIndex((item) => item.id === session.id)
    if (index >= 0) sessions.value[index] = session
  }

  /**
   * 切换 SSH Session 的收藏状态。
   * @param session - 目标 SSH Session
   * @returns 切换完成后的 Promise
   */
  async function toggleFavourite(session: SSHSessionDTO): Promise<void> {
    await updateSSHSession(session.id, {
      name: session.name,
      host: session.host,
      port: session.port,
      username: session.username,
      authType: session.authType,
      secret: '',
      passphrase: '',
      hostKeyPolicy: session.hostKeyPolicy,
      group: session.group,
      favourite: !session.favourite,
      remark: session.remark,
      connectTimeoutSec: session.connectTimeoutSec,
      handshakeTimeoutSec: session.handshakeTimeoutSec,
      keepAliveSec: session.keepAliveSec,
      compression: session.compression,
      overrideSettings: session.overrideSettings,
    })
    await refresh()
  }

  /**
   * 删除一个 SSH Session 并刷新列表。
   * @param id - SSH Session ID
   * @returns 删除完成后的 Promise
   */
  async function remove(id: string): Promise<void> {
    await deleteSSHSession(id)
    await refresh()
  }

  return {
    apply,
    error,
    favouriteOnly,
    group,
    groups,
    loading,
    partialMessage,
    refresh,
    remove,
    search,
    sessions,
    toggleFavourite,
  }
})
