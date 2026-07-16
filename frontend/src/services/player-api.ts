import { Events } from '@wailsio/runtime'

import {
  PlayerActionInput,
  PlayerQuery,
  PlayerSessionQuery,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import type {
  PlayerEvent,
  PlayerListResult,
  PlayerOverview,
  PlayerSession,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import {
  AddWhitelist,
  Ban,
  Detail,
  GrantOperator,
  Kick,
  List,
  Pardon,
  RecentSessions,
  RefreshDirectory,
  RemoveWhitelist,
  RevokeOperator,
  SynchronizeActivity,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/playerservice'
import { throwIfError } from './api-client'

export const playerEventName = 'mineops:player:event'

export interface PlayerListRequest {
  serverID: string
  search?: string
  filter?: string
  sort?: string
  direction?: string
  limit?: number
  offset?: number
}

export interface PlayerManagementRequest {
  serverID: string
  playerIdentityID: string
  reason?: string
  expiresAt?: string
}

/**
 * @param {PlayerListRequest} request - Bounded unified player list request.
 * @returns {Promise<PlayerListResult>} Unified rows and total count.
 */
export async function listPlayers(request: PlayerListRequest): Promise<PlayerListResult> {
  const result = await List(
    new PlayerQuery({
      serverID: request.serverID,
      search: request.search ?? '',
      filter: request.filter ?? 'all',
      sort: request.sort ?? 'name',
      direction: request.direction ?? 'asc',
      limit: request.limit ?? 100,
      offset: request.offset ?? 0,
    }),
  )
  throwIfError(result.error)
  if (!result.result) throw new Error('PlayerService 未返回玩家列表')
  return result.result
}

/**
 * @param {string} serverID - Durable Server identifier.
 * @param {string} playerIdentityID - Server-scoped player identity.
 * @returns {Promise<PlayerOverview>} Latest unified player overview.
 */
export async function getPlayerDetail(
  serverID: string,
  playerIdentityID: string,
): Promise<PlayerOverview> {
  const result = await Detail(serverID, playerIdentityID)
  throwIfError(result.error)
  if (!result.player) throw new Error('PlayerService 未返回玩家详情')
  return result.player
}

/**
 * @param {string} serverID - Durable Server identifier.
 * @param {string} playerIdentityID - Server-scoped player identity.
 * @param {number} limit - Bounded recent session limit.
 * @param {number} offset - Non-negative result offset.
 * @returns {Promise<PlayerSession[]>} Recent sessions in descending join order.
 */
export async function listRecentPlayerSessions(
  serverID: string,
  playerIdentityID: string,
  limit = 20,
  offset = 0,
): Promise<PlayerSession[]> {
  const result = await RecentSessions(
    new PlayerSessionQuery({ serverID, playerIdentityID, limit, offset }),
  )
  throwIfError(result.error)
  return result.sessions
}

/**
 * @param {string} serverID - Durable Server identifier.
 * @returns {Promise<PlayerOverview[]>} Player rows changed by directory synchronization.
 */
export async function refreshPlayerDirectory(serverID: string): Promise<PlayerOverview[]> {
  const result = await RefreshDirectory(serverID)
  throwIfError(result.error)
  if (!result.result) throw new Error('PlayerService 未返回目录同步后的玩家列表')
  return result.result.players
}

/**
 * @param {string} serverID - Durable Server identifier.
 * @returns {Promise<PlayerOverview[]>} Player rows changed by activity synchronization.
 */
export async function synchronizePlayerActivity(serverID: string): Promise<PlayerOverview[]> {
  const result = await SynchronizeActivity(serverID)
  throwIfError(result.error)
  if (!result.result) throw new Error('PlayerService 未返回活动同步后的玩家列表')
  return result.result.players
}

/**
 * @param {PlayerManagementRequest} request - Validated player action input.
 * @param {'addWhitelist' | 'removeWhitelist' | 'grantOperator' | 'revokeOperator' | 'ban' | 'pardon' | 'kick'} action - Management action.
 * @returns {Promise<PlayerOverview>} Refreshed unified player overview.
 */
export async function managePlayer(
  request: PlayerManagementRequest,
  action:
    | 'addWhitelist'
    | 'removeWhitelist'
    | 'grantOperator'
    | 'revokeOperator'
    | 'ban'
    | 'pardon'
    | 'kick',
): Promise<PlayerOverview> {
  const input = new PlayerActionInput({
    serverID: request.serverID,
    playerIdentityID: request.playerIdentityID,
    reason: request.reason ?? '',
    ...(request.expiresAt ? { expiresAt: request.expiresAt } : {}),
  })
  const calls = {
    addWhitelist: AddWhitelist,
    removeWhitelist: RemoveWhitelist,
    grantOperator: GrantOperator,
    revokeOperator: RevokeOperator,
    ban: Ban,
    pardon: Pardon,
    kick: Kick,
  }
  const result = await calls[action](input)
  throwIfError(result.error)
  if (!result.player) throw new Error('PlayerService 未返回操作后的玩家状态')
  return result.player
}

/**
 * @param {(event: PlayerEvent) => void} listener - Realtime player event listener.
 * @returns {() => void} Event unsubscribe callback.
 */
export function subscribePlayerEvents(listener: (event: PlayerEvent) => void): () => void {
  return Events.On(playerEventName, (event) => listener(event.data))
}
