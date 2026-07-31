export type PlayerFilter = 'all' | 'online' | 'whitelist' | 'operator' | 'banned'
export type PlayerSort = 'name' | 'currentDuration' | 'totalDuration' | 'lastActivity'
export type SortDirection = 'asc' | 'desc'

export interface PlayerOverviewLike {
  identityID: string
  name: string
  normalizedName: string
  online: boolean
  currentJoinedAt: string | null | undefined
  totalDurationSeconds: number
  lastActivityAt: string | null | undefined
  whitelisted: boolean
  operator: boolean
  banned: boolean
}

export interface PlayerEventLike<Player extends PlayerOverviewLike> {
  playerIdentityID: string | undefined
  player: Player | null | undefined
}

/**
 * @param {number} seconds - Non-negative duration in seconds.
 * @returns {string} Compact localized duration text.
 */
export function formatPlayerDuration(seconds: number): string {
  const value = Number.isFinite(seconds) ? Math.max(0, Math.floor(seconds)) : 0
  const days = Math.floor(value / 86_400)
  const hours = Math.floor((value % 86_400) / 3_600)
  const minutes = Math.floor((value % 3_600) / 60)
  const remainingSeconds = value % 60
  if (days > 0) return `${days}天 ${hours}小时`
  if (hours > 0) return `${hours}小时 ${minutes}分`
  if (minutes > 0) return `${minutes}分 ${remainingSeconds}秒`
  return `${remainingSeconds}秒`
}

/**
 * @param {PlayerOverviewLike} player - Unified player overview.
 * @param {number} nowMillis - Current Unix time in milliseconds.
 * @returns {number} Current online duration without mutating persisted totals.
 */
export function currentPlayerDurationSeconds(
  player: PlayerOverviewLike,
  nowMillis = Date.now(),
): number {
  if (!player.online || !player.currentJoinedAt) return 0
  const joinedAt = effectiveCurrentJoinedAtMillis(player)
  if (!Number.isFinite(joinedAt)) return 0
  return Math.max(0, Math.floor((nowMillis - joinedAt) / 1_000))
}

/**
 * @param {PlayerOverviewLike} player - Unified player overview.
 * @returns {string | null} Latest trustworthy activity timestamp.
 */
export function playerRecentActivityAt(player: PlayerOverviewLike): string | null {
  const joinedAt = player.currentJoinedAt ? Date.parse(player.currentJoinedAt) : Number.NaN
  const lastActivityAt = player.lastActivityAt ? Date.parse(player.lastActivityAt) : Number.NaN
  if (
    Number.isFinite(joinedAt) &&
    (!Number.isFinite(lastActivityAt) || joinedAt > lastActivityAt)
  ) {
    return player.currentJoinedAt ?? null
  }
  return player.lastActivityAt ?? null
}

/**
 * @param {PlayerOverviewLike} player - Unified player overview.
 * @param {number} nowMillis - Current Unix time in milliseconds.
 * @returns {number} Settled duration plus the locally advancing open session.
 */
export function totalPlayerDurationSeconds(
  player: PlayerOverviewLike,
  nowMillis = Date.now(),
): number {
  return Math.max(0, player.totalDurationSeconds) + currentPlayerDurationSeconds(player, nowMillis)
}

/**
 * @param {PlayerOverviewLike[]} players - Unified player rows.
 * @param {string} search - Case-insensitive player name fragment.
 * @param {PlayerFilter} filter - Selected status filter.
 * @returns {PlayerOverviewLike[]} Filtered rows preserving input order.
 */
export function filterPlayerOverviews<Player extends PlayerOverviewLike>(
  players: Player[],
  search: string,
  filter: PlayerFilter,
): Player[] {
  const query = search.trim().toLocaleLowerCase()
  return players.filter((player) => {
    if (
      query &&
      !player.name.toLocaleLowerCase().includes(query) &&
      !player.normalizedName.toLocaleLowerCase().includes(query)
    ) {
      return false
    }
    if (filter === 'online') return player.online
    if (filter === 'whitelist') return player.whitelisted
    if (filter === 'operator') return player.operator
    if (filter === 'banned') return player.banned
    return true
  })
}

/**
 * @param {PlayerOverviewLike[]} players - Unified player rows.
 * @param {PlayerSort} sort - Supported player sort field.
 * @param {SortDirection} direction - Sort direction.
 * @param {number} nowMillis - Current Unix time in milliseconds.
 * @returns {PlayerOverviewLike[]} Stable sorted copy.
 */
export function sortPlayerOverviews<Player extends PlayerOverviewLike>(
  players: Player[],
  sort: PlayerSort,
  direction: SortDirection,
  nowMillis = Date.now(),
): Player[] {
  const multiplier = direction === 'asc' ? 1 : -1
  return players
    .map((player, index) => ({ player, index }))
    .sort((left, right) => {
      let result = 0
      if (sort === 'name') result = left.player.name.localeCompare(right.player.name)
      if (sort === 'currentDuration') {
        result =
          currentPlayerDurationSeconds(left.player, nowMillis) -
          currentPlayerDurationSeconds(right.player, nowMillis)
      }
      if (sort === 'totalDuration') {
        result = left.player.totalDurationSeconds - right.player.totalDurationSeconds
      }
      if (sort === 'lastActivity') {
        result =
          timestampValue(left.player.lastActivityAt) - timestampValue(right.player.lastActivityAt)
      }
      return result === 0 ? left.index - right.index : result * multiplier
    })
    .map(({ player }) => player)
}

/**
 * @param {PlayerOverviewLike[]} players - Current unified player rows.
 * @param {PlayerEventLike} event - Server-scoped realtime player event.
 * @returns {PlayerOverviewLike[]} Rows with the event player inserted or replaced.
 */
export function mergePlayerOverviewEvent<Player extends PlayerOverviewLike>(
  players: Player[],
  event: PlayerEventLike<Player>,
): Player[] {
  if (!event.player) return players
  const identityID = event.playerIdentityID ?? event.player.identityID
  const index = players.findIndex((player) => player.identityID === identityID)
  if (index < 0) return [event.player, ...players]
  const next = [...players]
  next[index] = event.player
  return next
}

/**
 * @param {PlayerOverviewLike[]} players - Current unified player rows.
 * @param {string | null} identityID - Nullable selected player identity.
 * @returns {PlayerOverviewLike | null} Existing selection or null when unavailable.
 */
export function resolveSelectedPlayer<Player extends PlayerOverviewLike>(
  players: Player[],
  identityID: string | null,
): Player | null {
  if (!identityID) return null
  return players.find((player) => player.identityID === identityID) ?? null
}

/**
 * 把 ISO 时间字符串解析成毫秒数。
 * @param value - ISO 时间字符串，可为空
 * @returns 毫秒时间戳，无法解析时为 0
 */
function timestampValue(value?: string | null): number {
  if (!value) return 0
  const timestamp = Date.parse(value)
  return Number.isFinite(timestamp) ? timestamp : 0
}

/**
 * 取玩家本次实际加入时间的毫秒值，优先使用当前在线记录。
 * @param player - 玩家总览
 * @returns 毫秒时间戳，未在线时为 0
 */
function effectiveCurrentJoinedAtMillis(player: PlayerOverviewLike): number {
  const joinedAt = player.currentJoinedAt ? Date.parse(player.currentJoinedAt) : Number.NaN
  const lastActivityAt = player.lastActivityAt ? Date.parse(player.lastActivityAt) : Number.NaN
  if (!Number.isFinite(joinedAt)) return Number.NaN
  if (Number.isFinite(lastActivityAt) && lastActivityAt > joinedAt) return lastActivityAt
  return joinedAt
}
