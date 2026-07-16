import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import type { MinecraftServer } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import type { MinecraftServerInput } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/models'
import {
  listMinecraftServers,
  restoreMinecraftServer,
  softDeleteMinecraftServer,
  updateMinecraftServer,
} from '../services/minecraft-server-api'

export const useMinecraftServersStore = defineStore('minecraft-servers', () => {
  const servers = ref<MinecraftServer[]>([])
  const search = ref('')
  const sshSessionID = ref('')
  const group = ref('')
  const state = ref('')
  const includeDeleted = ref(false)
  const loading = ref(false)
  const error = ref<unknown>(null)
  const groups = computed(() =>
    [...new Set(servers.value.map((server) => server.group).filter(Boolean))].sort(),
  )

  /** Loads Minecraft Servers using current filters. */
  async function refresh(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      servers.value = await listMinecraftServers(
        search.value,
        sshSessionID.value,
        group.value,
        '',
        state.value,
        includeDeleted.value,
      )
    } catch (reason) {
      error.value = reason
      throw reason
    } finally {
      loading.value = false
    }
  }

  /** Soft-deletes one Server and reloads the list. */
  async function softDelete(id: string): Promise<void> {
    await softDeleteMinecraftServer(id)
    await refresh()
  }

  /** Restores one soft-deleted Server and reloads the list. */
  async function restore(id: string): Promise<void> {
    await restoreMinecraftServer(id)
    await refresh()
  }

  /**
   * Updates editable Server metadata without changing its SSH binding or remote path.
   * @param {string} id - Durable Minecraft Server identifier.
   * @param {MinecraftServerInput} input - Complete update command with immutable identity fields preserved.
   * @returns {Promise<MinecraftServer>} The updated Server record.
   */
  async function update(id: string, input: MinecraftServerInput): Promise<MinecraftServer> {
    loading.value = true
    error.value = null
    try {
      const updated = await updateMinecraftServer(id, input)
      await refresh()
      return updated
    } catch (reason) {
      error.value = reason
      throw reason
    } finally {
      loading.value = false
    }
  }

  return {
    error,
    group,
    groups,
    includeDeleted,
    loading,
    refresh,
    restore,
    search,
    servers,
    softDelete,
    sshSessionID,
    state,
    update,
  }
})
