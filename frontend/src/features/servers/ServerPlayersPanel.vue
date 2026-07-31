<script setup lang="ts">
import {
  NAlert,
  NButton,
  NDatePicker,
  NDescriptions,
  NDescriptionsItem,
  NDrawer,
  NDrawerContent,
  NFlex,
  NInput,
  NPagination,
  NSelect,
  NSpin,
  NTag,
  NText,
  type DataTableColumns,
} from 'naive-ui'
import { computed, h, onMounted, onUnmounted, ref, watch } from 'vue'

import type {
  PlayerOverview,
  PlayerSession,
} from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import { hasMessage } from '../../locales/runtime'
import AppDataTable from '../../shared/components/AppDataTable.vue'
import { useInteractionStore } from '../../stores/interactions'
import { useLocaleStore } from '../../stores/locale'
import { useNotificationStore } from '../../stores/notifications'
import { usePlayersStore, type PlayerManagementAction } from '../../stores/players'
import {
  currentPlayerDurationSeconds,
  formatPlayerDuration,
  playerRecentActivityAt,
  totalPlayerDurationSeconds,
  type PlayerFilter,
  type PlayerSort,
  type SortDirection,
} from './player-utils'

const props = defineProps<{
  serverId: string
  serverState: string
  active: boolean
}>()

const store = usePlayersStore()
const locale = useLocaleStore()
const notifications = useNotificationStore()
const interactions = useInteractionStore()
const nowMillis = ref(Date.now())
const banReason = ref('')
const banExpiresAt = ref<number | null>(null)
const canManage = computed(() => props.active && props.serverState === 'running')
const drawerVisible = computed(() => store.selectedPlayerID !== null)
const selected = computed(() => store.selectedPlayer)
const qualityMessages = computed(() => {
  const messages = new Set<string>()
  if (store.directoryError)
    messages.add(locale.t('players.directorySync', { message: store.directoryError }))
  const overallCollectorIssue = collectorStatusIssue(store.collectorStatus)
  if (overallCollectorIssue) messages.add(overallCollectorIssue)
  for (const player of store.players) {
    if (player.directoryError) {
      messages.add(
        locale.t('players.playerDirectoryError', {
          name: player.name,
          message: player.directoryError,
        }),
      )
    }
    if (player.accuracy === 'incomplete')
      messages.add(locale.t('players.incompleteDuration', { name: player.name }))
  }
  return [...messages]
})
const playerFilters: PlayerFilter[] = ['all', 'online', 'whitelist', 'operator', 'banned']
const playerSorts: PlayerSort[] = ['name', 'currentDuration', 'totalDuration', 'lastActivity']
const sortDirections: SortDirection[] = ['asc', 'desc']
const filterOptions = computed(() =>
  playerFilters.map((value) => ({
    label: locale.t(`players.filter.${value}` as Parameters<typeof locale.t>[0]),
    value,
  })),
)
const sortOptions = computed(() =>
  playerSorts.map((value) => ({
    label: locale.t(`players.sort.${value}` as Parameters<typeof locale.t>[0]),
    value,
  })),
)
const directionOptions = computed(() =>
  sortDirections.map((value) => ({
    label: locale.t(`players.direction.${value}` as Parameters<typeof locale.t>[0]),
    value,
  })),
)

let timerID = 0
let searchTimerID = 0

const columns = computed<DataTableColumns<PlayerOverview>>(() => {
  const currentNow = nowMillis.value
  return [
    {
      title: locale.t('players.column.player'),
      key: 'player',
      minWidth: 200,
      render: (row) =>
        h('div', { class: 'player-name-cell' }, [
          h('div', { class: 'player-name-cell__title' }, [
            h(NText, { strong: true }, { default: () => row.name }),
            h(
              NTag,
              { size: 'small', type: row.online ? 'success' : 'default', bordered: false },
              {
                default: () =>
                  row.online ? locale.t('players.online') : locale.t('players.offline'),
              },
            ),
          ]),
          h(
            NText,
            { depth: 3, class: 'player-name-cell__identity' },
            {
              default: () =>
                row.identityKind === 'uuid'
                  ? locale.t('players.identityVerified')
                  : locale.t('players.identityTemporary'),
            },
          ),
          h(NFlex, { size: 4, wrap: true, class: 'player-name-cell__permissions' }, () => [
            ...(row.whitelisted
              ? [
                  h(
                    NTag,
                    { size: 'small', bordered: false },
                    { default: () => locale.t('players.whitelisted') },
                  ),
                ]
              : []),
            ...(row.operator
              ? [
                  h(
                    NTag,
                    { size: 'small', type: 'warning', bordered: false },
                    { default: () => 'OP' },
                  ),
                ]
              : []),
            ...(row.banned
              ? [
                  h(
                    NTag,
                    { size: 'small', type: 'error', bordered: false },
                    { default: () => locale.t('players.banned') },
                  ),
                ]
              : []),
            ...(!row.whitelisted && !row.operator && !row.banned
              ? [h(NText, { depth: 3 }, { default: () => locale.t('players.regular') })]
              : []),
          ]),
        ]),
    },
    {
      title: locale.t('players.column.duration'),
      key: 'duration',
      minWidth: 176,
      render: (row) =>
        h('div', { class: 'player-stat-cell' }, [
          h(
            NText,
            { strong: row.online },
            {
              default: () =>
                row.online
                  ? locale.t('players.currentDuration', {
                      value: formatPlayerDuration(currentPlayerDurationSeconds(row, currentNow)),
                    })
                  : locale.t('players.currentOffline'),
            },
          ),
          h(
            NText,
            { depth: 3 },
            {
              default: () =>
                locale.t('players.totalDuration', {
                  value: formatPlayerDuration(totalPlayerDurationSeconds(row, currentNow)),
                }),
            },
          ),
        ]),
    },
    {
      title: locale.t('players.column.lastActivity'),
      key: 'lastActivity',
      minWidth: 164,
      render: (row) =>
        h('div', { class: 'player-stat-cell' }, [
          h(NText, null, { default: () => formatTimestamp(playerRecentActivityAt(row)) }),
          h(
            NText,
            { depth: 3 },
            {
              default: () =>
                locale.t('players.completedSessions', { count: row.completedSessionCount }),
            },
          ),
        ]),
    },
    {
      title: locale.t('common.actions'),
      key: 'actions',
      width: 72,
      render: (row) =>
        h(
          NButton,
          { size: 'small', quaternary: true, onClick: () => void openPlayer(row) },
          { default: () => locale.t('common.detail') },
        ),
    },
  ]
})

/**
 * @param {PlayerOverview} player - Unified player row opened from the table.
 * @returns {Promise<void>} Completes after detail loading starts.
 */
async function openPlayer(player: PlayerOverview): Promise<void> {
  banReason.value = player.banReason || ''
  const expiresAt = player.banExpiresAt ? Date.parse(player.banExpiresAt) : Number.NaN
  banExpiresAt.value = Number.isFinite(expiresAt) ? expiresAt : null
  try {
    await store.selectPlayer(player.identityID)
  } catch (error) {
    notifyError(locale.t('players.detailLoadFailed'), error)
  }
}

/**
 * 关闭玩家详情抽屉并清空封禁表单。
 * @returns 无返回值
 */
function closePlayer(): void {
  banReason.value = ''
  banExpiresAt.value = null
  void store.selectPlayer(null)
}

/**
 * @param {PlayerManagementAction} action - Player management action.
 * @param {string} success - Success notification text.
 * @returns {Promise<void>} Completes after backend refresh.
 */
async function runAction(action: PlayerManagementAction, success: string): Promise<void> {
  if (!canManage.value || !selected.value) return
  if (action === 'kick' || action === 'ban') {
    const confirmed = await interactions.confirm({
      title: action === 'kick' ? locale.t('players.kickTitle') : locale.t('players.banTitle'),
      content: action === 'kick' ? locale.t('players.kickContent') : locale.t('players.banContent'),
      objectLabel: selected.value.name,
      impact:
        action === 'ban'
          ? banReason.value || locale.t('players.noBanReason')
          : locale.t('players.kickImpact'),
      positiveText:
        action === 'kick' ? locale.t('players.kickConfirm') : locale.t('players.banConfirm'),
      danger: action === 'ban',
    })
    if (!confirmed) return
  }
  try {
    await store.runAction(action, {
      reason: banReason.value,
      expiresAt: banExpiresAt.value === null ? '' : new Date(banExpiresAt.value).toISOString(),
    })
    notifications.push({
      kind: 'success',
      title: success,
      content: selected.value?.name ?? '',
      dedupeKey: `player:${props.serverId}:${selected.value?.identityID}:${action}`,
    })
  } catch (error) {
    notifyError(locale.t('players.actionFailed'), error)
  }
}

/**
 * 重新加载玩家总览列表。
 * @returns 刷新完成后的 Promise
 */
async function refreshList(): Promise<void> {
  try {
    await store.refresh()
  } catch (error) {
    notifyError(locale.t('players.listRefreshFailed'), error)
  }
}

/**
 * 重新加载白名单、OP 与封禁名录。
 * @returns 刷新完成后的 Promise
 */
async function refreshDirectory(): Promise<void> {
  try {
    await store.refreshDirectory()
    notifications.push({ kind: 'success', title: locale.t('players.directorySynced'), content: '' })
  } catch (error) {
    notifyError(locale.t('players.directorySyncFailed'), error)
  }
}

/**
 * 触发一次玩家活动数据的远端同步。
 * @returns 同步完成后的 Promise
 */
async function synchronizeActivity(): Promise<void> {
  try {
    await store.synchronizeActivity()
    notifications.push({ kind: 'success', title: locale.t('players.activitySynced'), content: '' })
  } catch (error) {
    notifyError(locale.t('players.activitySyncFailed'), error)
  }
}

/**
 * 推送一条玩家面板错误通知。
 * @param title - 通知标题
 * @param error - 捕获到的错误
 * @returns 无返回值
 */
function notifyError(title: string, error: unknown): void {
  notifications.push({
    kind: 'error',
    title,
    content: error instanceof Error ? error.message : String(error),
    dedupeKey: `player:${props.serverId}:${title}`,
  })
}

/**
 * 按当前时间格式渲染时间文本。
 * @param value - ISO 时间字符串，可为空
 * @returns 本地时间文本，空值返回破折号
 */
function formatTimestamp(value?: string | null): string {
  return value ? locale.formatDateTime(value) : '—'
}

/**
 * 把一次会话的时长格式化成可读文本。
 * @param session - 玩家会话
 * @returns 时长文本
 */
function sessionDuration(session: PlayerSession): string {
  if (session.durationSeconds > 0) return formatPlayerDuration(session.durationSeconds)
  if (!session.leftAt) {
    return formatPlayerDuration(
      Math.max(0, Math.floor((nowMillis.value - Date.parse(session.joinedAt)) / 1_000)),
    )
  }
  return locale.t('players.zeroSeconds')
}

/**
 * 把数据精确度标识映射成本地化标签。
 * @param value - 精确度标识
 * @returns 本地化标签，未知取值回退为原值或“未知”
 */
function accuracyLabel(value: string): string {
  const key = `players.accuracy.${value}`
  if (hasMessage(key)) return locale.t(key)
  return value || locale.t('common.unknown')
}

/**
 * 把数据精确度标识映射成标签配色。
 * @param value - 精确度标识
 * @returns naive-ui 标签的语义类型
 */
function accuracyTagType(value: string): 'default' | 'success' | 'warning' | 'error' | 'info' {
  if (value === 'exact') return 'success'
  if (value === 'incomplete') return 'error'
  if (value === 'estimated' || value === 'server_boundary' || value === 'reconstructed') {
    return 'warning'
  }
  return 'default'
}

/**
 * 从采集器状态中提取需要展示的异常说明。
 * @param status - 采集器状态，结构不固定
 * @returns 异常说明文本，无异常时为空串
 */
function collectorStatusIssue(status: unknown): string {
  if (!status) return ''
  if (typeof status === 'string') {
    if (['exact', 'server_boundary', 'running', 'healthy', 'ok'].includes(status)) return ''
    const key = `players.collector.${status}`
    return hasMessage(key) ? locale.t(key) : locale.t('players.collectorUnknown', { state: status })
  }
  if (typeof status !== 'object') return ''
  const value = status as Record<string, unknown>
  if (typeof value.lastError === 'string' && value.lastError) return value.lastError
  const dropped = Number(value.droppedEvents ?? value.droppedRecords ?? value.droppedCount ?? 0)
  if (Number.isFinite(dropped) && dropped > 0)
    return locale.t('players.spoolDropped', { count: dropped })
  if (
    typeof value.state === 'string' &&
    !['exact', 'server_boundary', 'running', 'healthy', 'ok'].includes(value.state)
  ) {
    return locale.t('players.collectorUnknown', { state: value.state })
  }
  return ''
}

watch(
  () => props.serverId,
  (serverID) => {
    store.setServer(serverID)
    if (props.active) void refreshList()
  },
)

watch(
  [
    () => store.filter,
    () => store.sort,
    () => store.direction,
    () => store.page,
    () => store.pageSize,
  ],
  () => {
    if (props.active) void refreshList()
  },
)

watch(
  () => store.search,
  () => {
    window.clearTimeout(searchTimerID)
    searchTimerID = window.setTimeout(() => {
      store.page = 1
      if (props.active) void refreshList()
    }, 250)
  },
)

onMounted(() => {
  store.setServer(props.serverId)
  store.startSubscription()
  timerID = window.setInterval(() => {
    nowMillis.value = Date.now()
  }, 1_000)
  if (props.active) void refreshList()
})

onUnmounted(() => {
  window.clearInterval(timerID)
  window.clearTimeout(searchTimerID)
  store.stopSubscription()
})
</script>

<template>
  <NFlex vertical :size="16">
    <NAlert
      v-if="serverState !== 'running'"
      type="info"
      :title="locale.t('players.serverStoppedTitle')"
    >
      {{ locale.t('players.serverStoppedContent') }}
    </NAlert>
    <NAlert v-if="qualityMessages.length" type="warning" :title="locale.t('players.qualityTitle')">
      <div v-for="message in qualityMessages" :key="message">{{ message }}</div>
    </NAlert>
    <NFlex align="center" wrap class="player-toolbar">
      <NInput
        v-model:value="store.search"
        clearable
        :placeholder="locale.t('players.searchPlaceholder')"
      />
      <NSelect v-model:value="store.filter" :options="filterOptions" class="toolbar-select" />
      <NSelect v-model:value="store.sort" :options="sortOptions" class="toolbar-select" />
      <NSelect
        v-model:value="store.direction"
        :options="directionOptions"
        class="direction-select"
      />
      <NButton :loading="store.loading" @click="refreshList">
        {{ locale.t('players.refreshList') }}
      </NButton>
      <NButton :loading="store.actionLoading" @click="synchronizeActivity">
        {{ locale.t('players.syncActivity') }}
      </NButton>
      <NButton :loading="store.actionLoading" @click="refreshDirectory">
        {{ locale.t('players.syncDirectory') }}
      </NButton>
    </NFlex>

    <AppDataTable
      :columns="columns"
      :data="store.players"
      :loading="store.loading"
      :error="store.listError"
      :partial-message="store.partialMessage"
      :row-key="(row) => row.identityID"
      class="player-table"
      :empty-description="locale.t('players.empty')"
      @retry="refreshList"
      @open="openPlayer"
    />

    <NFlex justify="space-between" align="center" wrap>
      <NText depth="3">{{ locale.t('players.total', { count: store.total }) }}</NText>
      <NPagination
        v-model:page="store.page"
        v-model:page-size="store.pageSize"
        :page-count="store.pageCount"
        :page-sizes="[20, 50, 100]"
        show-size-picker
      />
    </NFlex>
  </NFlex>

  <NDrawer
    :show="drawerVisible"
    :width="620"
    placement="right"
    @update:show="(show) => !show && closePlayer()"
  >
    <NDrawerContent :title="selected?.name || locale.t('players.detailTitle')" closable>
      <NSpin :show="store.detailLoading">
        <NAlert
          v-if="store.detailError"
          type="error"
          :title="locale.t('players.detailFailedTitle')"
          class="drawer-section"
        >
          {{
            store.detailError instanceof Error
              ? store.detailError.message
              : String(store.detailError)
          }}
        </NAlert>
        <template v-if="selected">
          <NDescriptions bordered :columns="2" label-placement="left" class="drawer-section">
            <NDescriptionsItem label="UUID">{{
              selected.uuid || locale.t('players.uuidUnresolved')
            }}</NDescriptionsItem>
            <NDescriptionsItem :label="locale.t('players.identitySource')">{{
              selected.identityKind === 'uuid'
                ? locale.t('players.identityUUID')
                : locale.t('players.identityTemporary')
            }}</NDescriptionsItem>
            <NDescriptionsItem :label="locale.t('players.firstActivity')">{{
              formatTimestamp(selected.firstActivityAt)
            }}</NDescriptionsItem>
            <NDescriptionsItem :label="locale.t('players.lastActivity')">{{
              formatTimestamp(playerRecentActivityAt(selected))
            }}</NDescriptionsItem>
            <NDescriptionsItem :label="locale.t('players.totalOnline')">{{
              formatPlayerDuration(totalPlayerDurationSeconds(selected, nowMillis))
            }}</NDescriptionsItem>
            <NDescriptionsItem :label="locale.t('players.currentOnline')">
              {{
                selected.online
                  ? formatPlayerDuration(currentPlayerDurationSeconds(selected, nowMillis))
                  : '—'
              }}
            </NDescriptionsItem>
            <NDescriptionsItem :label="locale.t('players.longestSession')">{{
              formatPlayerDuration(selected.longestSessionSeconds)
            }}</NDescriptionsItem>
            <NDescriptionsItem :label="locale.t('players.averageSession')">{{
              formatPlayerDuration(selected.averageSessionSeconds)
            }}</NDescriptionsItem>
            <NDescriptionsItem :label="locale.t('players.completeSessions')">{{
              selected.completedSessionCount
            }}</NDescriptionsItem>
            <NDescriptionsItem :label="locale.t('players.accuracyLabel')">
              <NTag :type="accuracyTagType(selected.accuracy)" :bordered="false">{{
                accuracyLabel(selected.accuracy)
              }}</NTag>
            </NDescriptionsItem>
            <NDescriptionsItem :label="locale.t('players.banReason')">{{
              selected.banReason || '—'
            }}</NDescriptionsItem>
            <NDescriptionsItem :label="locale.t('players.banSource')">{{
              selected.banSource || '—'
            }}</NDescriptionsItem>
            <NDescriptionsItem :label="locale.t('players.banExpires')">{{
              formatTimestamp(selected.banExpiresAt)
            }}</NDescriptionsItem>
            <NDescriptionsItem :label="locale.t('players.directoryError')">{{
              selected.directoryError || locale.t('common.none')
            }}</NDescriptionsItem>
          </NDescriptions>

          <NFlex vertical :size="8" class="drawer-section">
            <NText strong>{{ locale.t('players.management') }}</NText>
            <NFlex>
              <NButton
                v-if="!selected.whitelisted"
                :disabled="!canManage"
                :loading="store.actionLoading"
                @click="runAction('addWhitelist', locale.t('players.addedWhitelist'))"
              >
                {{ locale.t('players.addWhitelist') }}
              </NButton>
              <NButton
                v-else
                :disabled="!canManage"
                :loading="store.actionLoading"
                @click="runAction('removeWhitelist', locale.t('players.removedWhitelist'))"
              >
                {{ locale.t('players.removeWhitelist') }}
              </NButton>
              <NButton
                v-if="!selected.operator"
                :disabled="!canManage"
                :loading="store.actionLoading"
                @click="runAction('grantOperator', locale.t('players.grantedOperator'))"
              >
                {{ locale.t('players.grantOperator') }}
              </NButton>
              <NButton
                v-else
                :disabled="!canManage"
                :loading="store.actionLoading"
                @click="runAction('revokeOperator', locale.t('players.revokedOperator'))"
              >
                {{ locale.t('players.revokeOperator') }}
              </NButton>
              <NButton
                type="warning"
                :disabled="!canManage || !selected.online"
                :loading="store.actionLoading"
                @click="runAction('kick', locale.t('players.kicked'))"
              >
                {{ locale.t('players.kick') }}
              </NButton>
              <NButton
                v-if="selected.banned"
                :disabled="!canManage"
                :loading="store.actionLoading"
                @click="runAction('pardon', locale.t('players.pardoned'))"
              >
                {{ locale.t('players.pardon') }}
              </NButton>
            </NFlex>
            <NInput
              v-model:value="banReason"
              :disabled="!canManage || selected.banned"
              :placeholder="locale.t('players.banReasonPlaceholder')"
            />
            <NDatePicker
              v-model:value="banExpiresAt"
              type="datetime"
              format="yyyy-MM-dd HH:mm:ss"
              time-picker-format="HH:mm:ss"
              :time-picker-props="{ format: 'HH:mm:ss' }"
              clearable
              :disabled="!canManage || selected.banned"
              :placeholder="locale.t('players.banExpiresPlaceholder')"
              class="ban-expiration-picker"
            />
            <NButton
              v-if="!selected.banned"
              type="error"
              :disabled="!canManage"
              :loading="store.actionLoading"
              @click="runAction('ban', locale.t('players.bannedNotice'))"
            >
              {{ locale.t('players.ban') }}
            </NButton>
          </NFlex>

          <div class="drawer-section">
            <NText strong>{{ locale.t('players.recentSessions') }}</NText>
            <div v-if="store.sessions.length" class="session-list">
              <div v-for="session in store.sessions" :key="session.id" class="session-row">
                <div>
                  <NText strong>{{ formatTimestamp(session.joinedAt) }}</NText>
                  <div>
                    <NText depth="3">{{ session.closeReason || session.state }}</NText>
                  </div>
                </div>
                <div class="session-row__duration">
                  <NText>{{ sessionDuration(session) }}</NText>
                  <div>
                    <NTag
                      size="small"
                      :type="accuracyTagType(session.accuracy)"
                      :bordered="false"
                      >{{ accuracyLabel(session.accuracy) }}</NTag
                    >
                  </div>
                </div>
              </div>
            </div>
            <NText v-else depth="3">{{ locale.t('players.noSessions') }}</NText>
          </div>
        </template>
      </NSpin>
    </NDrawerContent>
  </NDrawer>
</template>

<style scoped>
.player-toolbar :deep(.n-input) {
  flex: 1 1 240px;
  min-width: 200px;
}

.toolbar-select {
  width: 160px;
}

.direction-select {
  width: 96px;
}

.ban-expiration-picker {
  width: 100%;
}

.player-name-cell__title {
  display: flex;
  gap: 8px;
  align-items: center;
}

.player-name-cell__identity {
  display: block;
  max-width: 280px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.player-name-cell__permissions {
  margin-top: 6px;
}

.player-stat-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.player-table {
  min-width: 0;
}

.drawer-section + .drawer-section {
  margin-top: 20px;
}

.session-list {
  margin-top: 8px;
  border: 1px solid var(--border-default);
  border-radius: 8px;
}

.session-row {
  display: flex;
  gap: 16px;
  align-items: center;
  justify-content: space-between;
  padding: 12px;
}

.session-row + .session-row {
  border-top: 1px solid var(--border-default);
}

.session-row__duration {
  min-width: 112px;
  text-align: right;
}

@media (max-width: 900px) {
  .toolbar-select,
  .direction-select {
    flex: 1 1 140px;
    width: auto;
  }
}
</style>
