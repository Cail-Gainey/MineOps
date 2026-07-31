import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import type {
  PlayerEvent,
  PlayerOverview,
  PlayerSession,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import {
  getPlayerDetail,
  listPlayers,
  listRecentPlayerSessions,
  managePlayer,
  refreshPlayerDirectory,
  subscribePlayerEvents,
  synchronizePlayerActivity,
  type PlayerManagementRequest,
} from '../services/player-api'
import {
  filterPlayerOverviews,
  mergePlayerOverviewEvent,
  resolveSelectedPlayer,
  sortPlayerOverviews,
  type PlayerFilter,
  type PlayerSort,
  type SortDirection,
} from '../features/servers/player-utils'

export type PlayerManagementAction =
  | 'addWhitelist'
  | 'removeWhitelist'
  | 'grantOperator'
  | 'revokeOperator'
  | 'ban'
  | 'pardon'
  | 'kick'

export const usePlayersStore = defineStore('players', () => {
  const serverID = ref('')
  const players = ref<PlayerOverview[]>([])
  const total = ref(0)
  const collectorStatus = ref('')
  const directoryError = ref('')
  const search = ref('')
  const filter = ref<PlayerFilter>('all')
  const sort = ref<PlayerSort>('name')
  const direction = ref<SortDirection>('asc')
  const page = ref(1)
  const pageSize = ref(50)
  const selectedPlayerID = ref<string | null>(null)
  const detailPlayer = ref<PlayerOverview | null>(null)
  const sessions = ref<PlayerSession[]>([])
  const loading = ref(false)
  const detailLoading = ref(false)
  const actionLoading = ref(false)
  const listError = ref<unknown>(null)
  const detailError = ref<unknown>(null)
  const eventError = ref('')
  let unsubscribe: (() => void) | null = null
  let refreshSequence = 0
  let eventRefreshTimer = 0
  const liveJoinedAt = new Map<string, string>()

  const selectedPlayer = computed(
    () => detailPlayer.value ?? resolveSelectedPlayer(players.value, selectedPlayerID.value),
  )
  const pageCount = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))
  const partialMessage = computed(() => {
    if (eventError.value) return eventError.value
    if (listError.value && players.value.length)
      return '玩家列表刷新失败，继续显示最近一次成功数据。'
    return ''
  })

  /**
   * @param {string} id - Durable Server identifier.
   * @returns {void}
   */
  function setServer(id: string): void {
    if (serverID.value === id) return
    serverID.value = id
    players.value = []
    total.value = 0
    collectorStatus.value = ''
    directoryError.value = ''
    page.value = 1
    selectedPlayerID.value = null
    detailPlayer.value = null
    sessions.value = []
    listError.value = null
    detailError.value = null
    eventError.value = ''
    liveJoinedAt.clear()
  }

  /**
   * @returns {Promise<void>} Completes after the bounded player page is refreshed.
   */
  async function refresh(): Promise<void> {
    const targetServerID = serverID.value
    if (!targetServerID) {
      players.value = []
      total.value = 0
      return
    }
    const sequence = ++refreshSequence
    loading.value = true
    listError.value = null
    try {
      const result = await listPlayers({
        serverID: targetServerID,
        search: search.value,
        filter: filter.value,
        sort: backendSort(sort.value),
        direction: direction.value,
        limit: pageSize.value,
        offset: (page.value - 1) * pageSize.value,
      })
      if (sequence !== refreshSequence || serverID.value !== targetServerID) return
      for (const player of result.players) applyLiveJoinOverride(player)
      players.value = result.players
      total.value = result.total
      collectorStatus.value = result.collectorStatus ?? ''
      directoryError.value = result.directoryError ?? ''
      if (page.value > pageCount.value) page.value = pageCount.value
      if (selectedPlayerID.value) {
        const current = resolveSelectedPlayer(players.value, selectedPlayerID.value)
        if (current) detailPlayer.value = current
      }
    } catch (reason) {
      if (sequence === refreshSequence) listError.value = reason
      throw reason
    } finally {
      if (sequence === refreshSequence) loading.value = false
    }
  }

  /**
   * @param {string | null} identityID - Nullable player identity selected by the table.
   * @returns {Promise<void>} Completes after detail and bounded sessions are loaded.
   */
  async function selectPlayer(identityID: string | null): Promise<void> {
    selectedPlayerID.value = identityID
    detailPlayer.value = resolveSelectedPlayer(players.value, identityID)
    sessions.value = []
    detailError.value = null
    if (!identityID || !serverID.value) return
    const targetServerID = serverID.value
    detailLoading.value = true
    try {
      const [detail, recentSessions] = await Promise.all([
        getPlayerDetail(targetServerID, identityID),
        listRecentPlayerSessions(targetServerID, identityID, 20, 0),
      ])
      if (selectedPlayerID.value !== identityID || serverID.value !== targetServerID) return
      detailPlayer.value = detail
      sessions.value = recentSessions
      upsertPlayers([detail])
    } catch (reason) {
      if (selectedPlayerID.value === identityID) detailError.value = reason
      throw reason
    } finally {
      if (selectedPlayerID.value === identityID) detailLoading.value = false
    }
  }

  /**
   * @returns {Promise<void>} Completes after Minecraft player files are synchronized.
   */
  async function refreshDirectory(): Promise<void> {
    if (!serverID.value || actionLoading.value) return
    actionLoading.value = true
    try {
      upsertPlayers(await refreshPlayerDirectory(serverID.value))
      await refresh()
    } finally {
      actionLoading.value = false
    }
  }

  /**
   * @returns {Promise<void>} Completes after remote activity claims are synchronized.
   */
  async function synchronizeActivity(): Promise<void> {
    if (!serverID.value || actionLoading.value) return
    actionLoading.value = true
    try {
      upsertPlayers(await synchronizePlayerActivity(serverID.value))
      await refresh()
    } finally {
      actionLoading.value = false
    }
  }

  /**
   * @param {PlayerManagementAction} action - Supported whitelist, operator, ban, pardon, or kick action.
   * @param {Partial<PlayerManagementRequest>} options - Optional Ban reason and expiration.
   * @returns {Promise<void>} Completes after the action and unified state refresh.
   */
  async function runAction(
    action: PlayerManagementAction,
    options: Partial<PlayerManagementRequest> = {},
  ): Promise<void> {
    const identityID = selectedPlayerID.value
    if (!serverID.value || !identityID || actionLoading.value) return
    actionLoading.value = true
    try {
      const updated = await managePlayer(
        {
          serverID: serverID.value,
          playerIdentityID: identityID,
          reason: options.reason ?? '',
          expiresAt: options.expiresAt ?? '',
        },
        action,
      )
      detailPlayer.value = updated
      upsertPlayers([updated])
      await refresh()
      if (selectedPlayerID.value === identityID) {
        detailPlayer.value = await getPlayerDetail(serverID.value, identityID)
      }
    } finally {
      actionLoading.value = false
    }
  }

  /**
   * @returns {void}
   */
  function startSubscription(): void {
    unsubscribe?.()
    unsubscribe = subscribePlayerEvents(mergeEvent)
  }

  /**
   * @returns {void}
   */
  function stopSubscription(): void {
    unsubscribe?.()
    unsubscribe = null
    window.clearTimeout(eventRefreshTimer)
  }

  /**
   * 把一条玩家事件合并进当前 Server 的玩家状态。
   * @param event - 玩家进出事件
   * @returns 无返回值
   */
  function mergeEvent(event: PlayerEvent): void {
    if (event.serverID !== serverID.value) return
    if (event.type === 'error') {
      eventError.value = event.message || '玩家活动同步发生错误'
      return
    }
    eventError.value = ''
    const eventPlayerID = event.playerIdentityID ?? event.player?.identityID ?? ''
    if (eventPlayerID && event.type === 'join') {
      liveJoinedAt.set(eventPlayerID, event.emittedAt || new Date().toISOString())
    }
    if (eventPlayerID && ['leave', 'reconciled'].includes(event.type)) {
      liveJoinedAt.delete(eventPlayerID)
    }
    if (!event.player) {
      scheduleEventRefresh(eventPlayerID)
      return
    }
    const existed = players.value.some((player) => player.identityID === eventPlayerID)
    const merged = mergePlayerOverviewEvent<PlayerOverview>(players.value, event)
    const filtered = filterPlayerOverviews(merged, search.value, filter.value)
    players.value = sortPlayerOverviews(filtered, sort.value, direction.value).slice(
      0,
      pageSize.value,
    )
    if (event.player) {
      const remainsVisible = players.value.some((player) => player.identityID === eventPlayerID)
      if (!existed && remainsVisible) total.value += 1
      if (existed && !remainsVisible) total.value = Math.max(0, total.value - 1)
    }
    if (event.player && eventPlayerID === selectedPlayerID.value) {
      detailPlayer.value = event.player
    }
  }

  /**
   * 防抖地安排一次玩家事件列表刷新。
   * @param playerIdentityID - 触发刷新的玩家身份 ID
   * @returns 无返回值
   */
  function scheduleEventRefresh(playerIdentityID: string): void {
    window.clearTimeout(eventRefreshTimer)
    eventRefreshTimer = window.setTimeout(() => {
      void refresh()
        .then(async () => {
          if (playerIdentityID && selectedPlayerID.value === playerIdentityID) {
            await selectPlayer(playerIdentityID)
          }
        })
        .catch((error) => {
          eventError.value = error instanceof Error ? error.message : String(error)
        })
    }, 150)
  }

  /**
   * 按身份 ID 增量合并玩家总览列表。
   * @param updatedPlayers - 新的玩家总览数组
   * @returns 无返回值
   */
  function upsertPlayers(updatedPlayers: PlayerOverview[]): void {
    for (const player of updatedPlayers) {
      applyLiveJoinOverride(player)
      players.value = mergePlayerOverviewEvent<PlayerOverview>(players.value, {
        playerIdentityID: player.identityID,
        player,
      })
      if (selectedPlayerID.value === player.identityID) detailPlayer.value = player
    }
  }

  /**
   * 用实时事件记录的加入时间覆盖后端返回值，避免在线时长跳变。
   * @param player - 待修正的玩家总览
   * @returns 无返回值
   */
  function applyLiveJoinOverride(player: PlayerOverview): void {
    const joinedAt = liveJoinedAt.get(player.identityID)
    if (!joinedAt || !player.online) return
    const backendJoinedAt = player.currentJoinedAt ? Date.parse(player.currentJoinedAt) : Number.NaN
    const realtimeJoinedAt = Date.parse(joinedAt)
    if (!Number.isFinite(realtimeJoinedAt)) return
    if (!Number.isFinite(backendJoinedAt) || realtimeJoinedAt > backendJoinedAt) {
      player.currentJoinedAt = joinedAt
    }
    const lastActivityAt = player.lastActivityAt ? Date.parse(player.lastActivityAt) : Number.NaN
    if (!Number.isFinite(lastActivityAt) || realtimeJoinedAt > lastActivityAt) {
      player.lastActivityAt = joinedAt
    }
  }

  return {
    actionLoading,
    collectorStatus,
    detailError,
    detailLoading,
    directoryError,
    direction,
    filter,
    listError,
    loading,
    page,
    pageCount,
    pageSize,
    partialMessage,
    players,
    refresh,
    refreshDirectory,
    runAction,
    search,
    selectedPlayer,
    selectedPlayerID,
    selectPlayer,
    serverID,
    sessions,
    setServer,
    sort,
    startSubscription,
    stopSubscription,
    synchronizeActivity,
    total,
  }
})

/**
 * 把前端排序字段名转换成后端识别的字段名。
 * @param sort - 前端排序字段
 * @returns 后端排序字段名
 */
function backendSort(sort: PlayerSort): string {
  if (sort === 'currentDuration') return 'current_duration'
  if (sort === 'totalDuration') return 'total_duration'
  if (sort === 'lastActivity') return 'last_activity'
  return 'name'
}
