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

  /**
   * 重新加载 Minecraft Server 列表。
   * @returns 刷新完成后的 Promise
   */
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

  /**
   * 软删除一台 Server 并刷新列表。
   * @param id - Server ID
   * @returns 删除完成后的 Promise
   */
  async function softDelete(id: string): Promise<void> {
    await softDeleteMinecraftServer(id)
    await refresh()
  }

  /**
   * 恢复一台已软删除的 Server 并刷新列表。
   * @param id - Server ID
   * @returns 恢复完成后的 Promise
   */
  async function restore(id: string): Promise<void> {
    await restoreMinecraftServer(id)
    await refresh()
  }

  /**
   * 更新 Server 的可编辑元数据,不改动 SSH 绑定与远端路径。
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
