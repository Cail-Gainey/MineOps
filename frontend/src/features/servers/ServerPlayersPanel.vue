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
  if (store.directoryError) messages.add(`目录同步：${store.directoryError}`)
  const overallCollectorIssue = collectorStatusIssue(store.collectorStatus)
  if (overallCollectorIssue) messages.add(overallCollectorIssue)
  for (const player of store.players) {
    if (player.directoryError) messages.add(`${player.name}：${player.directoryError}`)
    if (player.accuracy === 'incomplete') messages.add(`${player.name} 的累计在线时长不完整`)
  }
  return [...messages]
})
const filterOptions: Array<{ label: string; value: PlayerFilter }> = [
  { label: '全部玩家', value: 'all' },
  { label: '在线', value: 'online' },
  { label: '白名单', value: 'whitelist' },
  { label: 'OP', value: 'operator' },
  { label: '封禁', value: 'banned' },
]
const sortOptions: Array<{ label: string; value: PlayerSort }> = [
  { label: '玩家名称', value: 'name' },
  { label: '当前在线时长', value: 'currentDuration' },
  { label: '累计在线时长', value: 'totalDuration' },
  { label: '最近活动', value: 'lastActivity' },
]
const directionOptions: Array<{ label: string; value: SortDirection }> = [
  { label: '升序', value: 'asc' },
  { label: '降序', value: 'desc' },
]

let timerID = 0
let searchTimerID = 0

const columns = computed<DataTableColumns<PlayerOverview>>(() => {
  const currentNow = nowMillis.value
  return [
    {
      title: '玩家',
      key: 'player',
      minWidth: 200,
      render: (row) =>
        h('div', { class: 'player-name-cell' }, [
          h('div', { class: 'player-name-cell__title' }, [
            h(NText, { strong: true }, { default: () => row.name }),
            h(
              NTag,
              { size: 'small', type: row.online ? 'success' : 'default', bordered: false },
              { default: () => (row.online ? '在线' : '离线') },
            ),
          ]),
          h(
            NText,
            { depth: 3, class: 'player-name-cell__identity' },
            { default: () => (row.identityKind === 'uuid' ? '已验证玩家身份' : '昵称临时身份') },
          ),
          h(NFlex, { size: 4, wrap: true, class: 'player-name-cell__permissions' }, () => [
            ...(row.whitelisted
              ? [h(NTag, { size: 'small', bordered: false }, { default: () => '白名单' })]
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
                    { default: () => '封禁' },
                  ),
                ]
              : []),
            ...(!row.whitelisted && !row.operator && !row.banned
              ? [h(NText, { depth: 3 }, { default: () => '普通玩家' })]
              : []),
          ]),
        ]),
    },
    {
      title: '在线时长',
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
                  ? `当前 ${formatPlayerDuration(currentPlayerDurationSeconds(row, currentNow))}`
                  : '当前离线',
            },
          ),
          h(
            NText,
            { depth: 3 },
            {
              default: () =>
                `累计 ${formatPlayerDuration(totalPlayerDurationSeconds(row, currentNow))}`,
            },
          ),
        ]),
    },
    {
      title: '最近活动',
      key: 'lastActivity',
      minWidth: 164,
      render: (row) =>
        h('div', { class: 'player-stat-cell' }, [
          h(NText, null, { default: () => formatTimestamp(playerRecentActivityAt(row)) }),
          h(NText, { depth: 3 }, { default: () => `已完成 ${row.completedSessionCount} 次会话` }),
        ]),
    },
    {
      title: '操作',
      key: 'actions',
      width: 72,
      render: (row) =>
        h(
          NButton,
          { size: 'small', quaternary: true, onClick: () => void openPlayer(row) },
          { default: () => '详情' },
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
    notifyError('加载玩家详情失败', error)
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
      title: action === 'kick' ? '踢出该玩家？' : '封禁该玩家？',
      content:
        action === 'kick'
          ? '服务器将立即踢出该玩家，在线状态由后续离开事件更新。'
          : '服务器将封禁该玩家，并同步封禁名单的最终状态。',
      objectLabel: selected.value.name,
      impact: action === 'ban' ? banReason.value || '未填写封禁原因' : '玩家连接将被关闭',
      positiveText: action === 'kick' ? '确认踢出' : '确认封禁',
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
    notifyError('玩家管理操作失败', error)
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
    notifyError('玩家列表刷新失败', error)
  }
}

/**
 * 重新加载白名单、OP 与封禁名录。
 * @returns 刷新完成后的 Promise
 */
async function refreshDirectory(): Promise<void> {
  try {
    await store.refreshDirectory()
    notifications.push({ kind: 'success', title: '玩家目录已同步', content: '' })
  } catch (error) {
    notifyError('玩家目录同步失败', error)
  }
}

/**
 * 触发一次玩家活动数据的远端同步。
 * @returns 同步完成后的 Promise
 */
async function synchronizeActivity(): Promise<void> {
  try {
    await store.synchronizeActivity()
    notifications.push({ kind: 'success', title: '玩家活动已同步', content: '' })
  } catch (error) {
    notifyError('玩家活动同步失败', error)
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
  return '0秒'
}

/**
 * 把数据精确度标识映射成中文标签。
 * @param value - 精确度标识
 * @returns 中文标签
 */
function accuracyLabel(value: string): string {
  const labels: Record<string, string> = {
    exact: '精确',
    server_boundary: '边界结算',
    estimated: '估算',
    incomplete: '不完整',
    reconstructed: '已重建',
  }
  return labels[value] ?? (value || '未知')
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
    const labels: Record<string, string> = {
      incomplete: '玩家活动采集存在缺失，累计在线时长可能不完整',
      estimated: '玩家活动包含异常退出后的估算结算',
      reconstructed: '玩家在线状态由当前启动周期证据重建',
      pending: '玩家活动采集器尚未完成首次同步',
    }
    if (['exact', 'server_boundary', 'running', 'healthy', 'ok'].includes(status)) return ''
    return labels[status] ?? `玩家活动采集状态异常：${status}`
  }
  if (typeof status !== 'object') return ''
  const value = status as Record<string, unknown>
  if (typeof value.lastError === 'string' && value.lastError) return value.lastError
  const dropped = Number(value.droppedEvents ?? value.droppedRecords ?? value.droppedCount ?? 0)
  if (Number.isFinite(dropped) && dropped > 0) return `Spool 已丢弃 ${dropped} 条事件`
  if (
    typeof value.state === 'string' &&
    !['exact', 'server_boundary', 'running', 'healthy', 'ok'].includes(value.state)
  ) {
    return `玩家活动采集状态异常：${value.state}`
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
    <NAlert v-if="serverState !== 'running'" type="info" title="服务器当前未运行">
      玩家历史、累计在线时长和权限状态仍可查看；白名单、OP、封禁、解除封禁与踢出
      操作仅在服务器运行时可用。
    </NAlert>
    <NAlert v-if="qualityMessages.length" type="warning" title="玩家数据质量提示">
      <div v-for="message in qualityMessages" :key="message">{{ message }}</div>
    </NAlert>
    <NFlex align="center" wrap class="player-toolbar">
      <NInput v-model:value="store.search" clearable placeholder="搜索玩家名称" />
      <NSelect v-model:value="store.filter" :options="filterOptions" class="toolbar-select" />
      <NSelect v-model:value="store.sort" :options="sortOptions" class="toolbar-select" />
      <NSelect
        v-model:value="store.direction"
        :options="directionOptions"
        class="direction-select"
      />
      <NButton :loading="store.loading" @click="refreshList">刷新列表</NButton>
      <NButton :loading="store.actionLoading" @click="synchronizeActivity">同步活动</NButton>
      <NButton :loading="store.actionLoading" @click="refreshDirectory">同步目录</NButton>
    </NFlex>

    <AppDataTable
      :columns="columns"
      :data="store.players"
      :loading="store.loading"
      :error="store.listError"
      :partial-message="store.partialMessage"
      :row-key="(row) => row.identityID"
      class="player-table"
      empty-description="当前筛选条件下没有玩家"
      @retry="refreshList"
      @open="openPlayer"
    />

    <NFlex justify="space-between" align="center" wrap>
      <NText depth="3">共 {{ store.total }} 名玩家</NText>
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
    <NDrawerContent :title="selected?.name || '玩家详情'" closable>
      <NSpin :show="store.detailLoading">
        <NAlert
          v-if="store.detailError"
          type="error"
          title="玩家详情加载失败"
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
            <NDescriptionsItem label="UUID">{{ selected.uuid || '尚未解析' }}</NDescriptionsItem>
            <NDescriptionsItem label="身份来源">{{
              selected.identityKind === 'uuid' ? 'UUID 已验证' : '昵称临时身份'
            }}</NDescriptionsItem>
            <NDescriptionsItem label="首次活动">{{
              formatTimestamp(selected.firstActivityAt)
            }}</NDescriptionsItem>
            <NDescriptionsItem label="最近活动">{{
              formatTimestamp(playerRecentActivityAt(selected))
            }}</NDescriptionsItem>
            <NDescriptionsItem label="累计在线时长">{{
              formatPlayerDuration(totalPlayerDurationSeconds(selected, nowMillis))
            }}</NDescriptionsItem>
            <NDescriptionsItem label="当前在线时长">
              {{
                selected.online
                  ? formatPlayerDuration(currentPlayerDurationSeconds(selected, nowMillis))
                  : '—'
              }}
            </NDescriptionsItem>
            <NDescriptionsItem label="最长会话">{{
              formatPlayerDuration(selected.longestSessionSeconds)
            }}</NDescriptionsItem>
            <NDescriptionsItem label="平均会话">{{
              formatPlayerDuration(selected.averageSessionSeconds)
            }}</NDescriptionsItem>
            <NDescriptionsItem label="完整会话">{{
              selected.completedSessionCount
            }}</NDescriptionsItem>
            <NDescriptionsItem label="统计准确性">
              <NTag :type="accuracyTagType(selected.accuracy)" :bordered="false">{{
                accuracyLabel(selected.accuracy)
              }}</NTag>
            </NDescriptionsItem>
            <NDescriptionsItem label="封禁原因">{{ selected.banReason || '—' }}</NDescriptionsItem>
            <NDescriptionsItem label="封禁来源">{{ selected.banSource || '—' }}</NDescriptionsItem>
            <NDescriptionsItem label="封禁到期">{{
              formatTimestamp(selected.banExpiresAt)
            }}</NDescriptionsItem>
            <NDescriptionsItem label="目录同步错误">{{
              selected.directoryError || '无'
            }}</NDescriptionsItem>
          </NDescriptions>

          <NFlex vertical :size="8" class="drawer-section">
            <NText strong>玩家管理</NText>
            <NFlex>
              <NButton
                v-if="!selected.whitelisted"
                :disabled="!canManage"
                :loading="store.actionLoading"
                @click="runAction('addWhitelist', '玩家已加入白名单')"
                >加入白名单</NButton
              >
              <NButton
                v-else
                :disabled="!canManage"
                :loading="store.actionLoading"
                @click="runAction('removeWhitelist', '玩家已移出白名单')"
                >移出白名单</NButton
              >
              <NButton
                v-if="!selected.operator"
                :disabled="!canManage"
                :loading="store.actionLoading"
                @click="runAction('grantOperator', '玩家已设为 OP')"
                >设为 OP</NButton
              >
              <NButton
                v-else
                :disabled="!canManage"
                :loading="store.actionLoading"
                @click="runAction('revokeOperator', '玩家 OP 已取消')"
                >取消 OP</NButton
              >
              <NButton
                type="warning"
                :disabled="!canManage || !selected.online"
                :loading="store.actionLoading"
                @click="runAction('kick', '玩家已被踢出')"
                >踢出</NButton
              >
              <NButton
                v-if="selected.banned"
                :disabled="!canManage"
                :loading="store.actionLoading"
                @click="runAction('pardon', '玩家封禁已取消')"
                >解除封禁</NButton
              >
            </NFlex>
            <NInput
              v-model:value="banReason"
              :disabled="!canManage || selected.banned"
              placeholder="封禁原因（可选）"
            />
            <NDatePicker
              v-model:value="banExpiresAt"
              type="datetime"
              format="yyyy-MM-dd HH:mm:ss"
              time-picker-format="HH:mm:ss"
              :time-picker-props="{ format: 'HH:mm:ss' }"
              clearable
              :disabled="!canManage || selected.banned"
              placeholder="选择封禁到期时间（可选）"
              class="ban-expiration-picker"
            />
            <NButton
              v-if="!selected.banned"
              type="error"
              :disabled="!canManage"
              :loading="store.actionLoading"
              @click="runAction('ban', '玩家已被封禁')"
              >封禁</NButton
            >
          </NFlex>

          <div class="drawer-section">
            <NText strong>最近会话</NText>
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
            <NText v-else depth="3">暂无会话记录</NText>
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
