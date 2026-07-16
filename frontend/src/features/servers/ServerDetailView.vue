<script setup lang="ts">
import { ChevronLeft, ChevronRight, Play, RotateCw, Square } from '@lucide/vue'
import {
  NAlert,
  NButton,
  NDescriptions,
  NDescriptionsItem,
  NFlex,
  NPageHeader,
  NSelect,
  NTabPane,
  NTabs,
  NTag,
  NText,
} from 'naive-ui'
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import type { MinecraftServer } from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import type { ServerInstallationStatus } from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/service/models'
import { ApplicationError } from '../../services/api-client'
import {
  getMinecraftServer,
  getCachedMinecraftServerInstallationStatus,
  inspectMinecraftServerInstallationStatus,
  listMinecraftServers,
} from '../../services/minecraft-server-api'
import {
  getLifecycleState,
  restartServer,
  startServer,
  stopServer,
} from '../../services/lifecycle-api'
import { subscribeOperationProgress } from '../../services/operation-api'
import AppIcon from '../../shared/components/AppIcon.vue'
import { useInteractionStore } from '../../stores/interactions'
import { useNotificationStore } from '../../stores/notifications'
import FilesView from '../files/FilesView.vue'
import MonitoringView from '../monitoring/MonitoringView.vue'
import PerformanceView from '../performance/PerformanceView.vue'
import ServerBackupsPanel from './ServerBackupsPanel.vue'
import ServerConsole from './ServerConsole.vue'
import ServerConfigurationPanel from './ServerConfigurationPanel.vue'
import ServerInstallationHistory from './ServerInstallationHistory.vue'
import ServerPlayersPanel from './ServerPlayersPanel.vue'

const route = useRoute()
const router = useRouter()
const notifications = useNotificationStore()
const interactions = useInteractionStore()
const server = ref<MinecraftServer | null>(null)
const serverList = ref<MinecraftServer[]>([])
const actionLoading = ref(false)
const detailTabs = new Set([
  'overview',
  'console',
  'players',
  'files',
  'configuration',
  'monitoring',
  'performance',
  'install-history',
  'backups',
])
const activeTab = ref(resolveDetailTab(route.query.tab))
const lifecycleError = ref('')
const installationStatus = ref<ServerInstallationStatus | null>(null)
const installationStatusError = ref('')
const installationChecking = ref(false)
const loadError = ref<unknown>(null)
const operationID = ref('')
let unsubscribeOperation: (() => void) | null = null
const serverOptions = computed(() =>
  serverList.value.map((item) => ({
    label: `${item.name} · ${serverTypeLabels[item.type] ?? item.type} ${item.version}`,
    value: item.id,
  })),
)
const currentServerIndex = computed(() =>
  server.value ? serverList.value.findIndex((item) => item.id === server.value?.id) : -1,
)
const previousServer = computed(() =>
  currentServerIndex.value > 0 ? (serverList.value[currentServerIndex.value - 1] ?? null) : null,
)
const nextServer = computed(() =>
  currentServerIndex.value >= 0 && currentServerIndex.value < serverList.value.length - 1
    ? (serverList.value[currentServerIndex.value + 1] ?? null)
    : null,
)
const lifecycleStateLabels: Record<string, string> = {
  creating: '创建中',
  installing: '安装中',
  ready: '就绪',
  starting: '启动中',
  running: '运行中',
  stopping: '停止中',
  stopped: '已停止',
  updating: '更新中',
  backing_up: '备份中',
  deleted: '已删除',
  failed: '失败',
}
const installationStatusLabels: Record<string, string> = {
  consistent: '一致',
  partial: '部分就绪',
  untracked: '未纳入安装记录',
  mismatch: '不一致',
}
const installationTaskStateLabels: Record<string, string> = {
  waiting: '等待中',
  running: '执行中',
  succeeded: '已成功',
  failed: '失败',
  cancelled: '已取消',
}
const serverTypeLabels: Record<string, string> = {
  vanilla: '原版（Vanilla）',
  paper: 'Paper',
  purpur: 'Purpur',
  spigot: 'Spigot',
  fabric: 'Fabric',
  forge: 'Forge',
  neoforge: 'NeoForge',
  quilt: 'Quilt',
  folia: 'Folia',
  velocity: 'Velocity',
  waterfall: 'Waterfall',
  bungeecord: 'BungeeCord',
}

function resolveDetailTab(tab: unknown): string {
  const value = typeof tab === 'string' ? tab : ''
  return detailTabs.has(value) ? value : 'overview'
}

function lifecycleStateLabel(state: string): string {
  return lifecycleStateLabels[state] ?? state
}

function installationStatusLabel(state: string): string {
  return installationStatusLabels[state] ?? state
}

function installationTaskStateLabel(state: string): string {
  return installationTaskStateLabels[state] ?? state
}

async function loadServerNavigation(): Promise<void> {
  try {
    serverList.value = await listMinecraftServers()
  } catch (error) {
    notifications.push({
      kind: 'warning',
      title: '加载服务器快捷导航失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'server-detail:navigation:error',
    })
  }
}

function navigateToServer(serverID: string | null): void {
  if (!serverID || serverID === server.value?.id) return
  void router.push({
    name: 'server-detail',
    params: { serverID },
    query: route.query,
  })
}

/** Loads the newly registered Server so all following management features share its durable identity. */
async function refresh(probeLifecycle = true): Promise<void> {
  loadError.value = null
  try {
    server.value = await getMinecraftServer(String(route.params.serverID))
    if (probeLifecycle) {
      try {
        const lifecycle = await getLifecycleState(server.value.id)
        server.value.state = lifecycle.state
        lifecycleError.value = ''
      } catch (error) {
        loadError.value = error
        lifecycleError.value = error instanceof Error ? error.message : String(error)
      }
    } else {
      lifecycleError.value = ''
    }
  } catch (error) {
    notifications.push({
      kind: 'error',
      title: '加载服务器详情失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'server-detail:error',
    })
  }
}

async function refreshInstallationStatus(): Promise<void> {
  if (!server.value || installationChecking.value) return
  installationChecking.value = true
  try {
    installationStatus.value = await inspectMinecraftServerInstallationStatus(server.value.id, true)
    installationStatusError.value = ''
  } catch (error) {
    installationStatusError.value = error instanceof Error ? error.message : String(error)
  } finally {
    installationChecking.value = false
  }
}

async function start(): Promise<void> {
  if (!server.value) return
  actionLoading.value = true
  try {
    let firewallConfirmed = false
    let tmuxInstallConfirmed = false
    let acceptedOperationID = ''
    while (!acceptedOperationID) {
      try {
        acceptedOperationID = await startServer(
          server.value.id,
          firewallConfirmed,
          tmuxInstallConfirmed,
        )
      } catch (error) {
        if (
          error instanceof ApplicationError &&
          error.details.requiresTmuxInstallConfirmation === true &&
          !tmuxInstallConfirmed
        ) {
          const confirmed = await interactions.confirm({
            title: '远程服务器未安装 tmux，是否自动安装？',
            content: String(
              error.details.intent ??
                'MineOps 将安装 tmux，用于在 SSH 断开后保持服务器进程运行并提供实时日志。',
            ),
            objectLabel: `${error.details.os ?? 'Linux'} · ${error.details.packageManager ?? '系统包管理器'}`,
            impact: `${error.details.needsSudo === true ? '需要 sudo 权限。' : ''}${error.details.command ? ` 将执行：${String(error.details.command)}` : ''}`,
            positiveText: '安装 tmux 并继续',
          })
          if (!confirmed) return
          tmuxInstallConfirmed = true
          continue
        }
        if (
          error instanceof ApplicationError &&
          error.details.requiresFirewallConfirmation === true &&
          !firewallConfirmed
        ) {
          const confirmed = await interactions.confirm({
            title: '启动前放行防火墙端口？',
            content: String(error.details.intent ?? '将幂等放行 Minecraft TCP 端口。'),
            objectLabel: `${error.details.backend ?? 'firewall'} · TCP ${error.details.port ?? ''}`,
            impact: 'MineOps 会记录规则归属，仅在安全条件满足时回收旧端口。',
            positiveText: '确认放行并启动',
          })
          if (!confirmed) return
          firewallConfirmed = true
          continue
        }
        throw error
      }
    }
    operationID.value = acceptedOperationID
    server.value.state = 'starting'
    activeTab.value = 'console'
    lifecycleError.value = ''
  } catch (error) {
    notifyActionError('启动服务器失败', error)
  } finally {
    actionLoading.value = false
  }
}

async function stop(force = false): Promise<void> {
  if (!server.value) return
  if (force) {
    const confirmed = await interactions.confirm({
      title: '强制停止服务器？',
      content: '将跳过或升级优雅 stop，向受控进程组发送 TERM/KILL。可能造成世界数据未完整保存。',
      objectLabel: server.value.name,
      positiveText: '强制停止',
      danger: true,
    })
    if (!confirmed) return
  }
  actionLoading.value = true
  try {
    operationID.value = await stopServer(server.value.id, force)
    server.value.state = 'stopping'
    lifecycleError.value = ''
  } catch (error) {
    notifyActionError(force ? '强制停止服务器失败' : '停止服务器失败', error)
  } finally {
    actionLoading.value = false
  }
}

async function restart(): Promise<void> {
  if (!server.value) return
  actionLoading.value = true
  try {
    let tmuxInstallConfirmed = false
    while (true) {
      try {
        operationID.value = await restartServer(server.value.id, tmuxInstallConfirmed)
        break
      } catch (error) {
        if (
          error instanceof ApplicationError &&
          error.details.requiresTmuxInstallConfirmation === true &&
          !tmuxInstallConfirmed
        ) {
          const confirmed = await interactions.confirm({
            title: '远程服务器未安装 tmux，是否自动安装？',
            content: String(
              error.details.intent ??
                'MineOps 将安装 tmux，用于在 SSH 断开后保持服务器进程运行并提供实时日志。',
            ),
            objectLabel: `${error.details.os ?? 'Linux'} · ${error.details.packageManager ?? '系统包管理器'}`,
            impact: `${error.details.needsSudo === true ? '需要 sudo 权限。' : ''}${error.details.command ? ` 将执行：${String(error.details.command)}` : ''}`,
            positiveText: '安装 tmux 并重启',
          })
          if (!confirmed) return
          tmuxInstallConfirmed = true
          continue
        }
        throw error
      }
    }
    server.value.state = 'stopping'
    activeTab.value = 'console'
    lifecycleError.value = ''
  } catch (error) {
    notifyActionError('重启服务器失败', error)
  } finally {
    actionLoading.value = false
  }
}

function notifyActionError(title: string, error: unknown): void {
  notifications.push({
    kind: 'error',
    title,
    content: error instanceof Error ? error.message : String(error),
    dedupeKey: `server-lifecycle:${title}`,
  })
}

onMounted(async () => {
  unsubscribeOperation = subscribeOperationProgress((operation) => {
    if (operation.targetID === String(route.params.serverID)) {
      operationID.value = operation.id
      if (['pending', 'running'].includes(operation.state)) {
        if (server.value && operation.type === 'start') {
          server.value.state = 'starting'
          activeTab.value = 'console'
        }
        if (server.value && operation.type === 'stop') server.value.state = 'stopping'
        if (server.value && operation.type === 'restart') {
          server.value.state = operation.stage === 'start' ? 'starting' : 'stopping'
          activeTab.value = 'console'
        }
        lifecycleError.value = ''
        return
      }
      void refresh(false)
      if (operation.type === 'install') void refreshInstallationStatus()
    }
  })
  await Promise.all([refresh(), loadServerNavigation()])
  if (server.value) {
    installationStatus.value = getCachedMinecraftServerInstallationStatus(server.value.id)
  }
})

onUnmounted(() => unsubscribeOperation?.())

watch(activeTab, (tab) => {
  const routeTab = typeof route.query.tab === 'string' ? route.query.tab : ''
  const nextRouteTab = tab === 'overview' ? '' : tab
  if (routeTab === nextRouteTab) return
  void router.replace({
    query: {
      ...route.query,
      tab: nextRouteTab || undefined,
    },
  })
})

watch(
  () => route.query.tab,
  (tab) => {
    const nextTab = resolveDetailTab(tab)
    if (activeTab.value !== nextTab) activeTab.value = nextTab
  },
)

watch(
  () => route.params.serverID,
  async (serverID, previousServerID) => {
    if (!serverID || serverID === previousServerID) return
    installationStatus.value = null
    installationStatusError.value = ''
    lifecycleError.value = ''
    loadError.value = null
    operationID.value = ''
    await refresh()
    if (server.value) {
      installationStatus.value = getCachedMinecraftServerInstallationStatus(server.value.id)
    }
  },
)
</script>

<template>
  <section class="server-detail-page">
    <NPageHeader
      :title="server?.name ?? '服务器详情'"
      :subtitle="
        server
          ? `${serverTypeLabels[server.type] ?? server.type} · Minecraft ${server.version}`
          : ''
      "
      @back="router.push({ name: 'servers' })"
    >
      <template #extra>
        <NFlex v-if="server" align="center" wrap>
          <NSelect
            :value="server.id"
            :options="serverOptions"
            filterable
            placeholder="快捷切换服务器"
            style="width: 300px"
            @update:value="navigateToServer"
          />
          <NButton
            quaternary
            circle
            :disabled="!previousServer"
            @click="navigateToServer(previousServer?.id ?? null)"
          >
            <template #icon><AppIcon :icon="ChevronLeft" label="上一台服务器" /></template>
          </NButton>
          <NButton
            quaternary
            circle
            :disabled="!nextServer"
            @click="navigateToServer(nextServer?.id ?? null)"
          >
            <template #icon><AppIcon :icon="ChevronRight" label="下一台服务器" /></template>
          </NButton>
          <NButton
            type="primary"
            :loading="actionLoading"
            :disabled="!['stopped', 'ready', 'failed'].includes(server.state)"
            @click="start"
          >
            <template #icon><AppIcon :icon="Play" label="启动" /></template>
            启动
          </NButton>
          <NButton
            :loading="actionLoading"
            :disabled="!['running', 'starting', 'failed'].includes(server.state)"
            @click="stop(false)"
          >
            <template #icon><AppIcon :icon="Square" label="停止" /></template>
            停止
          </NButton>
          <NButton :loading="actionLoading" :disabled="server.state !== 'running'" @click="restart">
            <template #icon><AppIcon :icon="RotateCw" label="重启" /></template>
            重启
          </NButton>
          <NButton
            type="error"
            secondary
            :loading="actionLoading"
            :disabled="!['running', 'starting', 'stopping', 'failed'].includes(server.state)"
            @click="stop(true)"
            >强制停止</NButton
          >
          <NTag
            :type="
              server.state === 'running'
                ? 'success'
                : server.state === 'failed'
                  ? 'error'
                  : server.state === 'starting' || server.state === 'stopping'
                    ? 'warning'
                    : 'info'
            "
            :bordered="false"
          >
            {{ lifecycleStateLabel(server.state) }}
          </NTag>
        </NFlex>
      </template>
    </NPageHeader>

    <NAlert
      v-if="loadError && !server"
      :type="
        loadError instanceof ApplicationError && loadError.code.includes('permission_denied')
          ? 'warning'
          : 'error'
      "
      :title="
        loadError instanceof ApplicationError && loadError.code.includes('permission_denied')
          ? '权限不足'
          : '服务器详情加载失败'
      "
    >
      <NFlex align="center" justify="space-between">
        <span>{{ loadError instanceof Error ? loadError.message : String(loadError) }}</span>
        <NButton size="small" @click="() => refresh()">重试</NButton>
      </NFlex>
    </NAlert>

    <NAlert v-if="lifecycleError" type="warning" title="远程状态探测失败">{{
      lifecycleError
    }}</NAlert>
    <NAlert v-if="installationStatusError" type="warning" title="安装一致性检查失败">
      {{ installationStatusError }}
    </NAlert>
    <NText v-if="operationID" depth="3">当前后台任务：{{ operationID }}</NText>

    <NTabs v-if="server" v-model:value="activeTab" type="line" animated>
      <NTabPane name="overview" tab="概览">
        <NAlert
          v-if="installationStatus"
          :type="
            installationStatus.state === 'consistent'
              ? 'success'
              : installationStatus.state === 'mismatch'
                ? 'error'
                : 'warning'
          "
          :title="`安装一致性：${installationStatusLabel(installationStatus.state)}`"
          class="installation-status"
        >
          <div v-if="installationStatus.issues.length">
            问题：{{ installationStatus.issues.join('；') }}
          </div>
          <div v-if="installationStatus.warnings.length">
            提示：{{ installationStatus.warnings.join('；') }}
          </div>
          <div v-if="!installationStatus.issues.length && !installationStatus.warnings.length">
            远程启动文件、配置、EULA、Java 运行时和最近安装检查点一致。
          </div>
        </NAlert>
        <NDescriptions bordered :columns="2" label-placement="left">
          <NDescriptionsItem label="服务器 ID">{{ server.id }}</NDescriptionsItem>
          <NDescriptionsItem label="SSH 会话 ID">{{ server.sshSessionID }}</NDescriptionsItem>
          <NDescriptionsItem label="服务端类型">{{
            serverTypeLabels[server.type] ?? server.type
          }}</NDescriptionsItem>
          <NDescriptionsItem label="Minecraft 版本">{{ server.version }}</NDescriptionsItem>
          <NDescriptionsItem label="Java 运行时 ID">{{
            server.javaRuntimeID || '未绑定'
          }}</NDescriptionsItem>
          <NDescriptionsItem label="远程目录">{{ server.remotePath }}</NDescriptionsItem>
          <NDescriptionsItem label="内存配置"
            >Xms {{ server.launchProfile.xmsMiB }} MiB / Xmx
            {{ server.launchProfile.xmxMiB }} MiB</NDescriptionsItem
          >
          <NDescriptionsItem label="分组">{{ server.group || '未分组' }}</NDescriptionsItem>
          <NDescriptionsItem label="标签">{{
            server.tags.join(', ') || '无标签'
          }}</NDescriptionsItem>
          <NDescriptionsItem v-if="installationStatus" label="启动文件">
            {{ installationStatus.artifactPath }} ·
            {{ (installationStatus.artifactSize / 1024 / 1024).toFixed(1) }} MiB
          </NDescriptionsItem>
          <NDescriptionsItem v-if="installationStatus" label="最近安装任务">
            {{ installationStatus.latestTaskID || '远程导入/无记录' }}
            {{ installationTaskStateLabel(installationStatus.latestTaskState || '') }}
          </NDescriptionsItem>
        </NDescriptions>
      </NTabPane>
      <NTabPane name="console" tab="控制台">
        <ServerConsole
          :server-id="server.id"
          :server-state="server.state"
          :active="activeTab === 'console'"
        />
      </NTabPane>
      <NTabPane name="players" tab="玩家">
        <ServerPlayersPanel
          v-if="activeTab === 'players'"
          :server-id="server.id"
          :server-state="server.state"
          :active="activeTab === 'players'"
        />
      </NTabPane>
      <NTabPane name="files" tab="文件">
        <FilesView
          v-if="activeTab === 'files'"
          :locked-session-id="server.sshSessionID"
          :initial-path="server.remotePath"
          embedded
        />
      </NTabPane>
      <NTabPane name="configuration" tab="配置">
        <ServerConfigurationPanel
          v-if="activeTab === 'configuration'"
          :server="server"
          :active="activeTab === 'configuration'"
        />
      </NTabPane>
      <NTabPane name="monitoring" tab="监控">
        <MonitoringView v-if="activeTab === 'monitoring'" :locked-server-i-d="server.id" embedded />
      </NTabPane>
      <NTabPane name="performance" tab="性能">
        <PerformanceView
          v-if="activeTab === 'performance'"
          :locked-server-i-d="server.id"
          embedded
        />
      </NTabPane>
      <NTabPane name="install-history" tab="安装历史">
        <ServerInstallationHistory v-if="activeTab === 'install-history'" :server-i-d="server.id" />
      </NTabPane>
      <NTabPane name="backups" tab="备份">
        <ServerBackupsPanel v-if="activeTab === 'backups'" :server="server" />
      </NTabPane>
    </NTabs>
  </section>
</template>

<style scoped>
.server-detail-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
}

.installation-status {
  margin-bottom: 12px;
}
</style>
