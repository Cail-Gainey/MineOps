<script setup lang="ts">
import {
  Activity,
  ArrowRight,
  CheckCircle2,
  Gauge,
  Layers,
  ListChecks,
  Plus,
  RefreshCw,
  Server,
  Star,
  TriangleAlert,
  Users,
  Zap,
} from '@lucide/vue'
import {
  NAlert,
  NButton,
  NCard,
  NEmpty,
  NFlex,
  NGrid,
  NGridItem,
  NProgress,
  NSpin,
  NStatistic,
  NTag,
  NText,
} from 'naive-ui'
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'

import type {
  MinecraftServer,
  Operation,
  PlayerOverview,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import { listMinecraftServers } from '../services/minecraft-server-api'
import { listPlayers, subscribePlayerEvents } from '../services/player-api'
import { useOperationsStore } from '../stores/operations'

interface DashboardOnlinePlayer {
  key: string
  serverID: string
  serverName: string
  player: PlayerOverview
}

const router = useRouter()
const operations = useOperationsStore()
const servers = ref<MinecraftServer[]>([])
const loading = ref(false)
const errors = ref<unknown[]>([])
const onlinePlayers = ref<DashboardOnlinePlayer[]>([])
const onlinePlayerCount = ref(0)
const onlinePlayerLoading = ref(false)
const onlinePlayerErrors = ref<unknown[]>([])
let playerRefreshTimer = 0
let onlinePlayerRefreshSequence = 0
let unsubscribePlayerEvents: (() => void) | null = null

const serverCount = computed(() => servers.value.length)
const runningServerCount = computed(
  () => servers.value.filter((server) => server.state === 'running').length,
)
const favouriteServerCount = computed(
  () => servers.value.filter((server) => server.favourite).length,
)
const serverTypeCount = computed(() => new Set(servers.value.map((server) => server.type)).size)
const onlinePlayerServerCount = computed(
  () => new Set(onlinePlayers.value.map((entry) => entry.serverID)).size,
)
const attentionServerCount = computed(
  () =>
    servers.value.filter((server) =>
      ['failed', 'creating', 'installing', 'starting', 'stopping'].includes(server.state),
    ).length,
)
const healthPercentage = computed(() => {
  if (serverCount.value === 0) return 0
  return Math.round((runningServerCount.value / serverCount.value) * 100)
})
const allSourcesFailed = computed(() => errors.value.length >= 2)
const permissionDenied = computed(() =>
  errors.value.some(
    (error) =>
      error &&
      typeof error === 'object' &&
      'code' in error &&
      String((error as { code: unknown }).code).includes('permission_denied'),
  ),
)
const errorTitle = computed(() => {
  if (permissionDenied.value && allSourcesFailed.value) return '概览权限不足'
  if (allSourcesFailed.value) return '概览加载失败'
  return '部分概览数据不可用'
})
const errorMessage = computed(() => {
  const messages = errors.value
    .map((error) => (error instanceof Error ? error.message : String(error)))
    .map((message) => message.trim())
    .filter(Boolean)
  return messages.length ? messages.join('；') : '部分数据接口未返回有效结果。'
})
const serverStatusRows = computed(() => [
  {
    label: '运行中',
    count: runningServerCount.value,
    percentage: serverCount.value
      ? Math.round((runningServerCount.value / serverCount.value) * 100)
      : 0,
    tone: 'success',
  },
  {
    label: '需要关注',
    count: attentionServerCount.value,
    percentage: serverCount.value
      ? Math.round((attentionServerCount.value / serverCount.value) * 100)
      : 0,
    tone: 'warning',
  },
  {
    label: '已停止',
    count: Math.max(serverCount.value - runningServerCount.value - attentionServerCount.value, 0),
    percentage: serverCount.value
      ? Math.round(
          (Math.max(serverCount.value - runningServerCount.value - attentionServerCount.value, 0) /
            serverCount.value) *
            100,
        )
      : 0,
    tone: 'neutral',
  },
])
const recentOperations = computed(() => {
  const seen = new Set<string>()
  return [...operations.active, ...operations.history]
    .filter((operation) => {
      if (seen.has(operation.id)) return false
      seen.add(operation.id)
      return true
    })
    .slice(0, 5)
})
const serverGroupRows = computed(() => {
  const grouped = new Map<string, number>()
  for (const server of servers.value) {
    const group = server.group?.trim() || '未分组'
    grouped.set(group, (grouped.get(group) ?? 0) + 1)
  }
  return [...grouped.entries()]
    .sort((left, right) => right[1] - left[1] || left[0].localeCompare(right[0]))
    .slice(0, 5)
    .map(([name, count]) => ({
      name,
      count,
      percentage: serverCount.value ? Math.round((count / serverCount.value) * 100) : 0,
    }))
})

const serverTypeSummary = computed(() => {
  const types = [...new Set(servers.value.map((server) => server.type))]
  return types.length ? types.slice(0, 3).join(' · ') : '尚未配置服务器类型'
})

/**
 * 把任务状态映射成标签配色。
 * @param operation - 目标任务
 * @returns naive-ui 标签的语义类型
 */
function operationType(operation: Operation): 'default' | 'info' | 'success' | 'warning' | 'error' {
  switch (operation.state) {
    case 'running':
      return 'info'
    case 'succeeded':
      return 'success'
    case 'failed':
      return 'error'
    case 'cancelled':
      return 'warning'
    default:
      return 'default'
  }
}

/**
 * 跳转到指定路径。
 * @param path - 目标路由路径
 * @returns 无返回值
 */
function navigate(path: string): void {
  void router.push(path)
}

/**
 * 跳转到某台 Server 详情页的玩家分页。
 * @param serverID - 目标 Server ID
 * @returns 无返回值
 */
function navigateToPlayers(serverID: string): void {
  void router.push({ name: 'server-detail', params: { serverID }, query: { tab: 'players' } })
}

/**
 * 批量刷新运行中 Server 的在线玩家数，丢弃过期请求的结果。
 * @param targetServers - 待刷新的 Server 列表
 * @returns 刷新完成后的 Promise
 */
async function refreshOnlinePlayers(targetServers: MinecraftServer[]): Promise<void> {
  const sequence = ++onlinePlayerRefreshSequence
  const runningServers = targetServers.filter((server) => server.state === 'running')
  if (!runningServers.length) {
    if (sequence === onlinePlayerRefreshSequence) {
      onlinePlayers.value = []
      onlinePlayerCount.value = 0
      onlinePlayerErrors.value = []
      onlinePlayerLoading.value = false
    }
    return
  }
  onlinePlayerLoading.value = true
  try {
    const results = await Promise.allSettled(
      runningServers.map(async (server) => ({
        server,
        result: await listPlayers({
          serverID: server.id,
          filter: 'online',
          sort: 'name',
          direction: 'asc',
          limit: 200,
        }),
      })),
    )
    const fulfilled = results.filter(
      (
        result,
      ): result is PromiseFulfilledResult<{
        server: MinecraftServer
        result: Awaited<ReturnType<typeof listPlayers>>
      }> => result.status === 'fulfilled',
    )
    if (sequence !== onlinePlayerRefreshSequence) return
    onlinePlayerErrors.value = results
      .filter((result): result is PromiseRejectedResult => result.status === 'rejected')
      .map((result) => result.reason)
    if (!fulfilled.length) return
    onlinePlayerCount.value = fulfilled.reduce(
      (total, entry) => total + entry.value.result.total,
      0,
    )
    onlinePlayers.value = fulfilled
      .flatMap(({ value: { server, result } }) =>
        result.players.map((player) => ({
          key: `${server.id}:${player.identityID}`,
          serverID: server.id,
          serverName: server.name,
          player,
        })),
      )
      .sort(
        (left, right) =>
          left.serverName.localeCompare(right.serverName) ||
          left.player.name.localeCompare(right.player.name),
      )
  } finally {
    if (sequence === onlinePlayerRefreshSequence) onlinePlayerLoading.value = false
  }
}

/**
 * 在 Server 处于运行中时防抖地安排一次在线玩家刷新。
 * @param serverID - 触发刷新的 Server ID
 * @returns 无返回值
 */
function scheduleOnlinePlayerRefresh(serverID: string): void {
  if (!servers.value.some((server) => server.id === serverID && server.state === 'running')) return
  window.clearTimeout(playerRefreshTimer)
  playerRefreshTimer = window.setTimeout(() => {
    void refreshOnlinePlayers(servers.value)
  }, 200)
}

/**
 * 重新加载仪表盘的 Server、任务与在线玩家数据。
 * @returns 刷新完成后的 Promise
 */
async function refresh(): Promise<void> {
  loading.value = true
  try {
    const [operationResult, serverResult] = await Promise.allSettled([
      operations.refresh(),
      listMinecraftServers(),
    ])
    if (serverResult.status === 'fulfilled') {
      servers.value = serverResult.value
      await refreshOnlinePlayers(serverResult.value)
    }
    const operationError =
      operationResult.status === 'rejected'
        ? operationResult.reason
        : (operations.activeError ?? operations.historyError)
    errors.value = [
      operationError,
      serverResult.status === 'rejected' ? serverResult.reason : null,
    ].filter(Boolean)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void refresh()
  unsubscribePlayerEvents = subscribePlayerEvents((event) => {
    if (event.type !== 'error') scheduleOnlinePlayerRefresh(event.serverID)
  })
})

onUnmounted(() => {
  window.clearTimeout(playerRefreshTimer)
  unsubscribePlayerEvents?.()
  unsubscribePlayerEvents = null
})
</script>

<template>
  <NSpin :show="loading">
    <section class="dashboard-page">
      <section class="dashboard-hero">
        <div class="hero-copy">
          <div class="hero-eyebrow"><span class="hero-eyebrow__dot" /> MineOps Control Center</div>
          <NText tag="h2">运维概览</NText>
          <NText depth="2" class="hero-description">
            快速掌握服务器健康度、后台任务与资源分组状态。
          </NText>
          <NFlex class="hero-actions" wrap>
            <NButton type="primary" @click="navigate('/servers')">
              <template #icon><Server :size="16" /></template>
              管理服务器
            </NButton>
            <NButton secondary @click="navigate('/monitoring')">
              <template #icon><Activity :size="16" /></template>
              查看监控
            </NButton>
          </NFlex>
        </div>
        <div class="health-summary">
          <NProgress
            type="circle"
            :percentage="healthPercentage"
            :status="
              healthPercentage >= 80 ? 'success' : healthPercentage > 0 ? 'warning' : 'default'
            "
            :stroke-width="10"
            :show-indicator="false"
            class="health-summary__ring"
          />
          <div class="health-summary__content">
            <NText depth="3">服务器健康度</NText>
            <NText tag="strong">{{ healthPercentage }}%</NText>
            <NText depth="3">{{ runningServerCount }} / {{ serverCount }} 运行中</NText>
          </div>
        </div>
      </section>

      <NAlert
        v-if="errors.length"
        :type="allSourcesFailed && !permissionDenied ? 'error' : 'warning'"
        :title="errorTitle"
      >
        <NFlex align="center" justify="space-between" :wrap="false">
          <span>{{ errorMessage }}</span>
          <NButton size="small" :loading="loading" @click="refresh">
            <template #icon><RefreshCw :size="14" /></template>
            重试
          </NButton>
        </NFlex>
      </NAlert>

      <NGrid cols="1 640:2 1000:3 1320:5" :x-gap="14" :y-gap="14" class="kpi-grid">
        <NGridItem>
          <NCard class="kpi-card kpi-card--accent">
            <div class="kpi-card__icon"><Server :size="19" /></div>
            <NStatistic label="托管服务器" :value="serverCount" />
            <NText depth="3" class="kpi-card__meta">{{ runningServerCount }} 台正在运行</NText>
          </NCard>
        </NGridItem>
        <NGridItem>
          <NCard class="kpi-card kpi-card--success">
            <div class="kpi-card__icon"><CheckCircle2 :size="19" /></div>
            <NStatistic label="在线实例" :value="runningServerCount" />
            <NText depth="3" class="kpi-card__meta">健康度 {{ healthPercentage }}%</NText>
          </NCard>
        </NGridItem>
        <NGridItem>
          <NCard class="kpi-card kpi-card--warning">
            <div class="kpi-card__icon"><Zap :size="19" /></div>
            <NStatistic label="活动任务" :value="operations.activeCount" />
            <NText depth="3" class="kpi-card__meta"
              >{{ operations.history.length }} 条历史记录</NText
            >
          </NCard>
        </NGridItem>
        <NGridItem>
          <NCard class="kpi-card kpi-card--info">
            <div class="kpi-card__icon"><Users :size="19" /></div>
            <NStatistic label="在线玩家" :value="onlinePlayerCount" />
            <NText depth="3" class="kpi-card__meta">
              {{
                onlinePlayerErrors.length
                  ? '部分服务器数据不可用'
                  : `分布在 ${onlinePlayerServerCount} 台服务器`
              }}
            </NText>
          </NCard>
        </NGridItem>
        <NGridItem>
          <NCard class="kpi-card kpi-card--info">
            <div class="kpi-card__icon"><Star :size="19" /></div>
            <NStatistic label="收藏服务器" :value="favouriteServerCount" />
            <NText depth="3" class="kpi-card__meta">覆盖 {{ serverTypeCount }} 种服务器类型</NText>
          </NCard>
        </NGridItem>
      </NGrid>

      <NCard class="dashboard-card online-players-card">
        <template #header>
          <div class="card-heading">
            <div>
              <NText tag="h3">当前在线玩家</NText>
              <NText depth="3">按运行中的服务器实时汇总</NText>
            </div>
            <NTag type="success" :bordered="false">{{ onlinePlayerCount }} 人在线</NTag>
          </div>
        </template>
        <NAlert
          v-if="onlinePlayerErrors.length"
          type="warning"
          title="部分在线玩家数据不可用"
          class="online-player-alert"
        >
          已加载可用服务器的数据；刷新后会继续尝试同步。
        </NAlert>
        <NSpin :show="onlinePlayerLoading">
          <NEmpty
            v-if="onlinePlayerCount === 0 && !onlinePlayerErrors.length"
            :description="runningServerCount ? '当前没有在线玩家' : '当前没有运行中的服务器'"
          />
          <NEmpty v-else-if="onlinePlayerCount === 0" description="暂时无法读取在线玩家" />
          <div v-else class="online-player-list">
            <button
              v-for="entry in onlinePlayers"
              :key="entry.key"
              type="button"
              class="online-player-row"
              @click="navigateToPlayers(entry.serverID)"
            >
              <span class="online-player-row__status" />
              <span class="online-player-row__main">
                <strong>{{ entry.player.name }}</strong>
                <small>{{ entry.serverName }}</small>
              </span>
              <ArrowRight :size="15" />
            </button>
          </div>
        </NSpin>
      </NCard>

      <div class="dashboard-grid dashboard-grid--primary">
        <NCard class="dashboard-card server-health-card">
          <template #header>
            <div class="card-heading">
              <div>
                <NText tag="h3">服务器健康分布</NText>
                <NText depth="3">按当前生命周期状态汇总</NText>
              </div>
              <NButton text type="primary" @click="navigate('/servers')">
                查看全部 <ArrowRight :size="15" />
              </NButton>
            </div>
          </template>
          <NEmpty v-if="serverCount === 0" description="还没有登记 Minecraft Server">
            <template #extra>
              <NButton type="primary" secondary @click="navigate('/servers')">
                <template #icon><Plus :size="16" /></template>
                添加第一台服务器
              </NButton>
            </template>
          </NEmpty>
          <div v-else class="status-list">
            <div v-for="row in serverStatusRows" :key="row.label" class="status-row">
              <div class="status-row__label">
                <span class="status-dot" :class="`status-dot--${row.tone}`" />
                <NText>{{ row.label }}</NText>
                <NText depth="3">{{ row.count }} 台</NText>
              </div>
              <NProgress
                type="line"
                :percentage="row.percentage"
                :show-indicator="false"
                :status="
                  row.tone === 'success'
                    ? 'success'
                    : row.tone === 'warning'
                      ? 'warning'
                      : 'default'
                "
                processing
              />
            </div>
          </div>
          <div v-if="attentionServerCount" class="health-callout">
            <TriangleAlert :size="16" />
            <NText>有 {{ attentionServerCount }} 台服务器需要检查</NText>
            <NButton text type="warning" @click="navigate('/servers')">去处理</NButton>
          </div>
        </NCard>

        <NCard class="dashboard-card quick-actions-card">
          <template #header>
            <div class="card-heading">
              <div>
                <NText tag="h3">快捷入口</NText>
                <NText depth="3">常用运维动作</NText>
              </div>
              <Gauge :size="20" class="card-heading__icon" />
            </div>
          </template>
          <div class="quick-actions">
            <button type="button" class="quick-action" @click="navigate('/servers')">
              <span class="quick-action__icon quick-action__icon--green"
                ><Server :size="18"
              /></span>
              <span><strong>服务器列表</strong><small>启动、停止与配置</small></span>
              <ArrowRight :size="16" />
            </button>
            <button type="button" class="quick-action" @click="navigate('/monitoring')">
              <span class="quick-action__icon quick-action__icon--blue"
                ><Activity :size="18"
              /></span>
              <span><strong>实时监控</strong><small>查看 CPU、内存与 TPS</small></span>
              <ArrowRight :size="16" />
            </button>
            <button type="button" class="quick-action" @click="navigate('/operations')">
              <span class="quick-action__icon quick-action__icon--amber"
                ><ListChecks :size="18"
              /></span>
              <span><strong>任务中心</strong><small>跟踪后台操作进度</small></span>
              <ArrowRight :size="16" />
            </button>
          </div>
        </NCard>
      </div>

      <div class="dashboard-grid dashboard-grid--secondary">
        <NCard class="dashboard-card activity-card">
          <template #header>
            <div class="card-heading">
              <div>
                <NText tag="h3">最近任务</NText>
                <NText depth="3">后台 Operation 活动</NText>
              </div>
              <NButton text type="primary" @click="navigate('/operations')">
                任务中心 <ArrowRight :size="15" />
              </NButton>
            </div>
          </template>
          <NEmpty v-if="recentOperations.length === 0" description="暂无后台任务" />
          <div v-else class="operation-list">
            <div v-for="operation in recentOperations" :key="operation.id" class="operation-row">
              <div class="operation-row__main">
                <div class="operation-row__title">
                  <NText strong>{{ operation.type }}</NText>
                  <NTag size="small" :type="operationType(operation)" :bordered="false">
                    {{ operation.state }}
                  </NTag>
                </div>
                <NText depth="3" class="operation-row__message">
                  {{ operation.message || operation.stage || operation.targetType }}
                </NText>
              </div>
              <div class="operation-row__progress">
                <NProgress
                  type="line"
                  :percentage="Math.round(operation.progress * 100)"
                  :status="operation.state === 'failed' ? 'error' : 'default'"
                  :show-indicator="false"
                />
                <NText depth="3">{{ Math.round(operation.progress * 100) }}%</NText>
              </div>
            </div>
          </div>
        </NCard>

        <NCard class="dashboard-card groups-card">
          <template #header>
            <div class="card-heading">
              <div>
                <NText tag="h3">服务器分组</NText>
                <NText depth="3">按业务用途快速浏览</NText>
              </div>
              <Layers :size="20" class="card-heading__icon" />
            </div>
          </template>
          <NEmpty v-if="serverGroupRows.length === 0" description="暂无服务器分组" />
          <div v-else class="group-list">
            <div v-for="group in serverGroupRows" :key="group.name" class="group-row">
              <span class="group-row__icon"><Layers :size="16" /></span>
              <div class="group-row__main">
                <div class="group-row__label">
                  <NText strong>{{ group.name }}</NText>
                  <NText depth="3">{{ group.count }} 台</NText>
                </div>
                <NProgress type="line" :percentage="group.percentage" :show-indicator="false" />
              </div>
            </div>
          </div>
          <div class="group-footer">
            <NTag type="info" :bordered="false">
              {{ serverTypeSummary }}
            </NTag>
            <NButton text type="primary" @click="navigate('/servers')">管理分组</NButton>
          </div>
        </NCard>
      </div>
    </section>
  </NSpin>
</template>

<style scoped>
.dashboard-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
}

.dashboard-hero {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  min-height: 196px;
  padding: 28px 32px;
  overflow: hidden;
  border: 1px solid color-mix(in srgb, var(--accent-primary) 28%, var(--border-default));
  border-radius: 18px;
  background:
    radial-gradient(
      circle at 88% 22%,
      color-mix(in srgb, var(--accent-primary) 24%, transparent),
      transparent 36%
    ),
    linear-gradient(
      135deg,
      color-mix(in srgb, var(--accent-primary) 11%, var(--surface-panel)),
      var(--surface-panel)
    );
  box-shadow: 0 14px 35px color-mix(in srgb, var(--accent-primary) 9%, transparent);
}

.hero-copy {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 10px;
}

.hero-copy h2 {
  margin: 0;
  font-size: clamp(26px, 3vw, 34px);
  letter-spacing: -0.03em;
}

.hero-eyebrow {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--accent-primary);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.hero-eyebrow__dot,
.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--accent-primary);
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--accent-primary) 14%, transparent);
}

.hero-description {
  max-width: 560px;
  line-height: 1.6;
}

.hero-actions {
  margin-top: 6px;
}

.health-summary {
  position: relative;
  display: flex;
  align-items: center;
  gap: 16px;
  min-width: 190px;
  padding: 16px 20px;
  border: 1px solid color-mix(in srgb, var(--accent-primary) 20%, var(--border-default));
  border-radius: 14px;
  background: color-mix(in srgb, var(--surface-panel) 76%, transparent);
}

.health-summary__ring {
  width: 76px;
  height: 76px;
}

.health-summary__content {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.health-summary__content strong {
  font-size: 26px;
  line-height: 1.1;
}

.kpi-grid :deep(.n-card__content) {
  position: relative;
  min-height: 124px;
  padding: 18px 20px;
}

.kpi-card {
  overflow: hidden;
  border-radius: 14px;
}

.kpi-card::after {
  position: absolute;
  right: -24px;
  bottom: -32px;
  width: 100px;
  height: 100px;
  border-radius: 50%;
  background: color-mix(in srgb, var(--accent-primary) 7%, transparent);
  content: '';
}

.kpi-card--success {
  --kpi-colour: var(--status-success);
}
.kpi-card--warning {
  --kpi-colour: var(--status-warning);
}
.kpi-card--info {
  --kpi-colour: var(--status-info);
}
.kpi-card--accent {
  --kpi-colour: var(--accent-primary);
}

.kpi-card__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  margin-bottom: 12px;
  border-radius: 10px;
  color: var(--kpi-colour);
  background: color-mix(in srgb, var(--kpi-colour) 13%, transparent);
}

.kpi-card :deep(.n-statistic-value__content) {
  color: var(--text-primary);
  font-size: 25px;
  font-weight: 700;
}

.kpi-card :deep(.n-statistic-label) {
  color: var(--text-muted);
  font-size: 12px;
}

.kpi-card__meta {
  display: block;
  margin-top: 4px;
  font-size: 12px;
}

.dashboard-grid {
  display: grid;
  gap: 16px;
  min-width: 0;
}

.dashboard-grid--primary,
.dashboard-grid--secondary {
  grid-template-columns: minmax(0, 1.35fr) minmax(300px, 0.9fr);
}

.dashboard-card {
  min-width: 0;
  border-radius: 14px;
}

.online-players-card :deep(.n-card__content) {
  min-width: 0;
}

.online-player-alert {
  margin-bottom: 14px;
}

.online-player-list {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(190px, 1fr));
  gap: 10px;
  min-width: 0;
}

.online-player-row {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  padding: 11px 12px;
  color: var(--text-primary);
  text-align: left;
  border: 1px solid var(--border-default);
  border-radius: 10px;
  background: color-mix(in srgb, var(--surface-panel) 86%, var(--accent-primary) 4%);
  cursor: pointer;
  transition:
    border-color 0.16s ease,
    background 0.16s ease,
    transform 0.16s ease;
}

.online-player-row:hover {
  border-color: color-mix(in srgb, var(--status-success) 45%, var(--border-default));
  background: color-mix(in srgb, var(--status-success) 8%, var(--surface-panel));
  transform: translateY(-1px);
}

.online-player-row__status {
  flex: 0 0 auto;
  width: 9px;
  height: 9px;
  border-radius: 50%;
  background: var(--status-success);
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--status-success) 14%, transparent);
}

.online-player-row__main {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.online-player-row__main strong,
.online-player-row__main small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.online-player-row__main small {
  color: var(--text-muted);
}

.online-player-row > :last-child {
  flex: 0 0 auto;
  color: var(--text-muted);
}

.card-heading {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.card-heading > div {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.card-heading h3 {
  margin: 0;
  font-size: 16px;
}

.card-heading__icon {
  color: var(--accent-primary);
}

.card-heading__icon--success {
  color: var(--status-success);
}

.card-heading :deep(.n-button) {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}

.status-list {
  display: flex;
  flex-direction: column;
  gap: 20px;
  padding: 8px 0 4px;
}

.status-row {
  display: flex;
  flex-direction: column;
  gap: 9px;
}

.status-row__label {
  display: flex;
  align-items: center;
  gap: 9px;
}

.status-row__label > :last-child {
  margin-left: auto;
}

.status-dot--success {
  background: var(--status-success);
}
.status-dot--warning {
  background: var(--status-warning);
}
.status-dot--neutral {
  background: var(--text-muted);
}

.health-callout {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 22px;
  padding: 10px 12px;
  border-radius: 10px;
  color: var(--status-warning);
  background: color-mix(in srgb, var(--status-warning) 10%, transparent);
}

.health-callout :deep(.n-button) {
  margin-left: auto;
}

.quick-actions {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.quick-action {
  display: grid;
  grid-template-columns: 36px minmax(0, 1fr) auto;
  align-items: center;
  gap: 12px;
  width: 100%;
  padding: 10px;
  border: 1px solid transparent;
  border-radius: 10px;
  color: inherit;
  background: transparent;
  cursor: pointer;
  text-align: left;
  transition:
    border-color 150ms ease,
    background 150ms ease,
    transform 150ms ease;
}

.quick-action:hover {
  border-color: var(--border-default);
  background: color-mix(in srgb, var(--accent-primary) 7%, transparent);
  transform: translateX(2px);
}

.quick-action > span:nth-child(2) {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}

.quick-action small {
  overflow: hidden;
  color: var(--text-muted);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.quick-action > svg {
  color: var(--text-muted);
}

.quick-action__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border-radius: 10px;
}

.quick-action__icon--green {
  color: var(--status-success);
  background: color-mix(in srgb, var(--status-success) 13%, transparent);
}
.quick-action__icon--blue {
  color: var(--status-info);
  background: color-mix(in srgb, var(--status-info) 13%, transparent);
}
.quick-action__icon--amber {
  color: var(--status-warning);
  background: color-mix(in srgb, var(--status-warning) 13%, transparent);
}

.operation-list {
  display: flex;
  flex-direction: column;
}

.operation-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 11px 0;
  border-bottom: 1px solid var(--border-default);
}

.operation-row:last-child {
  border-bottom: 0;
}

.operation-row__main,
.operation-row__title {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.operation-row__main {
  flex-direction: column;
  align-items: flex-start;
}

.operation-row__message {
  max-width: 420px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.operation-row__progress {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 126px;
  flex: 0 0 126px;
}

.operation-row__progress :deep(.n-progress) {
  flex: 1;
}

.operation-row__progress > :last-child {
  width: 32px;
  font-size: 11px;
  text-align: right;
}

.group-list {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.group-row {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.group-row__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  flex: 0 0 30px;
  border-radius: 8px;
  color: var(--accent-primary);
  background: color-mix(in srgb, var(--accent-primary) 12%, transparent);
}

.group-row__main {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
  flex: 1;
}

.group-row__label {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.group-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-top: 18px;
  padding-top: 14px;
  border-top: 1px solid var(--border-default);
}

@media (max-width: 760px) {
  .dashboard-hero {
    align-items: flex-start;
    flex-direction: column;
    padding: 22px;
  }

  .health-summary {
    width: 100%;
  }

  .dashboard-grid--primary,
  .dashboard-grid--secondary {
    grid-template-columns: 1fr;
  }

  .operation-row {
    align-items: flex-start;
    flex-direction: column;
    gap: 8px;
  }

  .operation-row__progress {
    width: 100%;
    flex-basis: auto;
  }
}
</style>
