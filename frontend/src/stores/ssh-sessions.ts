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

  /** Loads SSH Sessions using the current search and filter state. */
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

  /** Replaces one cached row after a targeted backend refresh, such as host spec collection. */
  function apply(session: SSHSessionDTO): void {
    const index = sessions.value.findIndex((item) => item.id === session.id)
    if (index >= 0) sessions.value[index] = session
  }

  /** Toggles a session favourite flag while retaining its current credential. */
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

  /** Deletes one SSH Session and reloads the current result set. */
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
