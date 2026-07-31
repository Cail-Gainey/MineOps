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
import { hasMessage } from '../../locales/runtime'
import { useInteractionStore } from '../../stores/interactions'
import { useLocaleStore } from '../../stores/locale'
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
const locale = useLocaleStore()
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
    label: `${item.name} · ${serverTypeLabel(item.type)} ${item.version}`,
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
/**
 * 把枚举值翻译成当前界面语言的标签。
 * @param prefix - 文案键前缀
 * @param value - 枚举值
 * @returns 本地化标签，未登记的值原样返回
 */
function enumLabel(prefix: string, value: string): string {
  const key = `${prefix}.${value}`
  return hasMessage(key) ? locale.t(key) : value
}

/**
 * 把服务端类型映射成本地化标签。
 * @param type - 服务端类型标识
 * @returns 本地化标签，未知类型原样返回
 */
function serverTypeLabel(type: string): string {
  return enumLabel('serverDetail.type', type)
}

/**
 * 校验路由传入的分页标识，非法时回落到概览。
 * @param tab - 路由中的分页标识
 * @returns 合法的分页标识
 */
function resolveDetailTab(tab: unknown): string {
  const value = typeof tab === 'string' ? tab : ''
  return detailTabs.has(value) ? value : 'overview'
}

/**
 * 把生命周期状态映射成本地化标签。
 * @param state - 生命周期状态标识
 * @returns 本地化标签，未知状态原样返回
 */
function lifecycleStateLabel(state: string): string {
  return enumLabel('serverDetail.lifecycle', state)
}

/**
 * 把安装完整性状态映射成本地化标签。
 * @param state - 安装状态标识
 * @returns 本地化标签，未知状态原样返回
 */
function installationStatusLabel(state: string): string {
  return enumLabel('serverDetail.installStatus', state)
}

/**
 * 把安装任务状态映射成本地化标签。
 * @param state - 安装任务状态标识
 * @returns 本地化标签，未知状态原样返回
 */
function installationTaskStateLabel(state: string): string {
  return enumLabel('serverDetail.taskState', state)
}

/**
 * 加载 Server 列表，供详情页顶部快速切换。
 * @returns 加载完成后的 Promise
 */
async function loadServerNavigation(): Promise<void> {
  try {
    serverList.value = await listMinecraftServers()
  } catch (error) {
    notifications.push({
      kind: 'warning',
      title: locale.t('serverDetail.navFailed'),
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'server-detail:navigation:error',
    })
  }
}

/**
 * 切换到另一台 Server 的详情页。
 * @param serverID - 目标 Server ID，为空或同一台时忽略
 * @returns 无返回值
 */
function navigateToServer(serverID: string | null): void {
  if (!serverID || serverID === server.value?.id) return
  void router.push({
    name: 'server-detail',
    params: { serverID },
    query: route.query,
  })
}

/**
 * 重新加载当前 Server 详情，可选择跳过生命周期探测。
 * @param probeLifecycle - 是否顺带探测运行状态
 * @returns 刷新完成后的 Promise
 */
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
      title: locale.t('serverDetail.loadFailed'),
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: 'server-detail:error',
    })
  }
}

/**
 * 重新探测当前 Server 的安装完整性。
 * @returns 探测完成后的 Promise
 */
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

/**
 * 启动当前 Server。
 * @returns 启动完成后的 Promise
 */
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
            title: locale.t('servers.tmuxTitle'),
            content: String(error.details.intent ?? locale.t('servers.tmuxContent')),
            objectLabel: `${error.details.os ?? 'Linux'} · ${error.details.packageManager ?? locale.t('servers.tmuxPackageManager')}`,
            impact: `${error.details.needsSudo === true ? locale.t('servers.tmuxSudo') : ''}${
              error.details.command
                ? locale.t('servers.tmuxCommand', { command: String(error.details.command) })
                : ''
            }`,
            positiveText: locale.t('servers.tmuxConfirmStart'),
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
            title: locale.t('servers.firewallTitle'),
            content: String(error.details.intent ?? locale.t('servers.firewallContent')),
            objectLabel: `${error.details.backend ?? 'firewall'} · TCP ${error.details.port ?? ''}`,
            impact: locale.t('servers.firewallImpact'),
            positiveText: locale.t('servers.firewallConfirm'),
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
    notifyActionError(locale.t('serverDetail.startFailed'), error)
  } finally {
    actionLoading.value = false
  }
}

/**
 * 停止当前 Server，可选择强制结束。
 * @param force - 是否强制结束进程
 * @returns 停止完成后的 Promise
 */
async function stop(force = false): Promise<void> {
  if (!server.value) return
  if (force) {
    const confirmed = await interactions.confirm({
      title: locale.t('serverDetail.forceStopTitle'),
      content: locale.t('serverDetail.forceStopContent'),
      objectLabel: server.value.name,
      positiveText: locale.t('serverDetail.forceStopConfirm'),
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
    notifyActionError(
      force ? locale.t('serverDetail.forceStopFailed') : locale.t('serverDetail.stopFailed'),
      error,
    )
  } finally {
    actionLoading.value = false
  }
}

/**
 * 重启当前 Server。
 * @returns 重启完成后的 Promise
 */
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
            title: locale.t('servers.tmuxTitle'),
            content: String(error.details.intent ?? locale.t('servers.tmuxContent')),
            objectLabel: `${error.details.os ?? 'Linux'} · ${error.details.packageManager ?? locale.t('servers.tmuxPackageManager')}`,
            impact: `${error.details.needsSudo === true ? locale.t('servers.tmuxSudo') : ''}${
              error.details.command
                ? locale.t('servers.tmuxCommand', { command: String(error.details.command) })
                : ''
            }`,
            positiveText: locale.t('servers.tmuxConfirmRestart'),
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
    notifyActionError(locale.t('serverDetail.restartFailed'), error)
  } finally {
    actionLoading.value = false
  }
}

/**
 * 推送一条生命周期操作错误通知。
 * @param title - 通知标题
 * @param error - 捕获到的错误
 * @returns 无返回值
 */
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
      :title="server?.name ?? locale.t('serverDetail.title')"
      :subtitle="
        server
          ? locale.t('serverDetail.subtitle', {
              type: serverTypeLabel(server.type),
              version: server.version,
            })
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
            :placeholder="locale.t('serverDetail.switchPlaceholder')"
            style="width: 300px"
            @update:value="navigateToServer"
          />
          <NButton
            quaternary
            circle
            :disabled="!previousServer"
            @click="navigateToServer(previousServer?.id ?? null)"
          >
            <template #icon>
              <AppIcon :icon="ChevronLeft" :label="locale.t('serverDetail.previousServer')" />
            </template>
          </NButton>
          <NButton
            quaternary
            circle
            :disabled="!nextServer"
            @click="navigateToServer(nextServer?.id ?? null)"
          >
            <template #icon>
              <AppIcon :icon="ChevronRight" :label="locale.t('serverDetail.nextServer')" />
            </template>
          </NButton>
          <NButton
            type="primary"
            :loading="actionLoading"
            :disabled="!['stopped', 'ready', 'failed'].includes(server.state)"
            @click="start"
          >
            <template #icon>
              <AppIcon :icon="Play" :label="locale.t('serverDetail.start')" />
            </template>
            {{ locale.t('serverDetail.start') }}
          </NButton>
          <NButton
            :loading="actionLoading"
            :disabled="!['running', 'starting', 'failed'].includes(server.state)"
            @click="stop(false)"
          >
            <template #icon>
              <AppIcon :icon="Square" :label="locale.t('serverDetail.stop')" />
            </template>
            {{ locale.t('serverDetail.stop') }}
          </NButton>
          <NButton :loading="actionLoading" :disabled="server.state !== 'running'" @click="restart">
            <template #icon>
              <AppIcon :icon="RotateCw" :label="locale.t('serverDetail.restart')" />
            </template>
            {{ locale.t('serverDetail.restart') }}
          </NButton>
          <NButton
            type="error"
            secondary
            :loading="actionLoading"
            :disabled="!['running', 'starting', 'stopping', 'failed'].includes(server.state)"
            @click="stop(true)"
          >
            {{ locale.t('serverDetail.forceStop') }}
          </NButton>
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
          ? locale.t('table.permissionDenied')
          : locale.t('serverDetail.detailLoadFailed')
      "
    >
      <NFlex align="center" justify="space-between">
        <span>{{ loadError instanceof Error ? loadError.message : String(loadError) }}</span>
        <NButton size="small" @click="() => refresh()">{{ locale.t('common.retry') }}</NButton>
      </NFlex>
    </NAlert>

    <NAlert
      v-if="lifecycleError"
      type="warning"
      :title="locale.t('serverDetail.lifecycleProbeFailed')"
      >{{ lifecycleError }}</NAlert
    >
    <NAlert
      v-if="installationStatusError"
      type="warning"
      :title="locale.t('serverDetail.installCheckFailed')"
    >
      {{ installationStatusError }}
    </NAlert>
    <NText v-if="operationID" depth="3">{{
      locale.t('serverDetail.currentOperation', { id: operationID })
    }}</NText>

    <NTabs v-if="server" v-model:value="activeTab" type="line" animated>
      <NTabPane name="overview" :tab="locale.t('serverDetail.tab.overview')">
        <NAlert
          v-if="installationStatus"
          :type="
            installationStatus.state === 'consistent'
              ? 'success'
              : installationStatus.state === 'mismatch'
                ? 'error'
                : 'warning'
          "
          :title="
            locale.t('serverDetail.installConsistency', {
              state: installationStatusLabel(installationStatus.state),
            })
          "
          class="installation-status"
        >
          <div v-if="installationStatus.issues.length">
            {{
              locale.t('serverDetail.issues', {
                items: installationStatus.issues.join(locale.t('common.listSeparator')),
              })
            }}
          </div>
          <div v-if="installationStatus.warnings.length">
            {{
              locale.t('serverDetail.warnings', {
                items: installationStatus.warnings.join(locale.t('common.listSeparator')),
              })
            }}
          </div>
          <div v-if="!installationStatus.issues.length && !installationStatus.warnings.length">
            {{ locale.t('serverDetail.consistentHint') }}
          </div>
        </NAlert>
        <NDescriptions bordered :columns="2" label-placement="left">
          <NDescriptionsItem :label="locale.t('serverDetail.serverID')">{{
            server.id
          }}</NDescriptionsItem>
          <NDescriptionsItem :label="locale.t('serverDetail.sshSessionID')">{{
            server.sshSessionID
          }}</NDescriptionsItem>
          <NDescriptionsItem :label="locale.t('serverDetail.serverType')">{{
            serverTypeLabel(server.type)
          }}</NDescriptionsItem>
          <NDescriptionsItem :label="locale.t('serverDetail.mcVersion')">{{
            server.version
          }}</NDescriptionsItem>
          <NDescriptionsItem :label="locale.t('serverDetail.javaRuntimeID')">{{
            server.javaRuntimeID || locale.t('serverDetail.javaUnbound')
          }}</NDescriptionsItem>
          <NDescriptionsItem :label="locale.t('serverDetail.remoteDir')">{{
            server.remotePath
          }}</NDescriptionsItem>
          <NDescriptionsItem :label="locale.t('serverDetail.memory')">{{
            locale.t('wizard.summaryMemoryValue', {
              xms: server.launchProfile.xmsMiB,
              xmx: server.launchProfile.xmxMiB,
            })
          }}</NDescriptionsItem>
          <NDescriptionsItem :label="locale.t('serverDetail.group')">{{
            server.group || locale.t('wizard.ungrouped')
          }}</NDescriptionsItem>
          <NDescriptionsItem :label="locale.t('serverDetail.tags')">{{
            server.tags.join(', ') || locale.t('wizard.noTags')
          }}</NDescriptionsItem>
          <NDescriptionsItem v-if="installationStatus" :label="locale.t('serverDetail.artifact')">
            {{
              locale.t('serverDetail.artifactValue', {
                path: installationStatus.artifactPath,
                size: (installationStatus.artifactSize / 1024 / 1024).toFixed(1),
              })
            }}
          </NDescriptionsItem>
          <NDescriptionsItem v-if="installationStatus" :label="locale.t('serverDetail.latestTask')">
            {{ installationStatus.latestTaskID || locale.t('serverDetail.noTaskRecord') }}
            {{ installationTaskStateLabel(installationStatus.latestTaskState || '') }}
          </NDescriptionsItem>
        </NDescriptions>
      </NTabPane>
      <NTabPane name="console" :tab="locale.t('serverDetail.tab.console')">
        <ServerConsole
          :server-id="server.id"
          :server-state="server.state"
          :active="activeTab === 'console'"
        />
      </NTabPane>
      <NTabPane name="players" :tab="locale.t('serverDetail.tab.players')">
        <ServerPlayersPanel
          v-if="activeTab === 'players'"
          :server-id="server.id"
          :server-state="server.state"
          :active="activeTab === 'players'"
        />
      </NTabPane>
      <NTabPane name="files" :tab="locale.t('serverDetail.tab.files')">
        <FilesView
          v-if="activeTab === 'files'"
          :locked-session-id="server.sshSessionID"
          :initial-path="server.remotePath"
          embedded
        />
      </NTabPane>
      <NTabPane name="configuration" :tab="locale.t('serverDetail.tab.configuration')">
        <ServerConfigurationPanel
          v-if="activeTab === 'configuration'"
          :server="server"
          :active="activeTab === 'configuration'"
        />
      </NTabPane>
      <NTabPane name="monitoring" :tab="locale.t('serverDetail.tab.monitoring')">
        <MonitoringView v-if="activeTab === 'monitoring'" :locked-server-i-d="server.id" embedded />
      </NTabPane>
      <NTabPane name="performance" :tab="locale.t('serverDetail.tab.performance')">
        <PerformanceView
          v-if="activeTab === 'performance'"
          :locked-server-i-d="server.id"
          embedded
        />
      </NTabPane>
      <NTabPane name="install-history" :tab="locale.t('serverDetail.tab.installHistory')">
        <ServerInstallationHistory v-if="activeTab === 'install-history'" :server-i-d="server.id" />
      </NTabPane>
      <NTabPane name="backups" :tab="locale.t('serverDetail.tab.backups')">
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
