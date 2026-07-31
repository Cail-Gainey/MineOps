import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'
import { translate } from '../locales/runtime'

import type {
  MinecraftServerInput,
  SSHSessionInput,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/models'
import type {
  InstallationStep,
  InstallationTask,
  MinecraftServer,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import type {
  ServerDistribution,
  ServerVersion,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/port/models'
import {
  cancelInstallation,
  getInstallation,
  listServerDistributions,
  resolveServerVersions,
  resolveInstallationDirectoryConflict,
  retryInstallation,
  startInstallation,
} from '../services/installation-api'
import { ApplicationError } from '../services/api-client'
import {
  createMinecraftServer,
  getMinecraftServer,
  listMinecraftServers,
  updateMinecraftServer,
} from '../services/minecraft-server-api'
import { createSSHSession, testSSHSessionConnection } from '../services/ssh-session-api'
import { subscribeOperationProgress } from '../services/operation-api'
import { useSSHSessionsStore } from './ssh-sessions'
import { useSettingsStore } from './settings'

const storageKey = 'mineops:server-wizard:v1'

export const useServerWizardStore = defineStore('server-wizard', () => {
  const sshSessions = useSSHSessionsStore()
  const settings = useSettingsStore()
  const currentStep = ref(1)
  const sshMode = ref<'existing' | 'new'>('existing')
  const selectedSSHSessionID = ref('')
  const newSSH = ref<SSHSessionInput>(emptySSHInput())
  const server = ref<MinecraftServerInput>(emptyServerInput())
  const distributions = ref<ServerDistribution[]>([])
  const versions = ref<ServerVersion[]>([])
  const createdServer = ref<MinecraftServer | null>(null)
  const task = ref<InstallationTask | null>(null)
  const steps = ref<InstallationStep[]>([])
  const taskID = ref('')
  const operationID = ref('')
  const loading = ref(false)
  const versionsLoading = ref(false)
  const error = ref<unknown>(null)
  const connectionEvidence = ref('')
  const pendingServerID = ref('')
  const versionCache = new Map<string, ServerVersion[]>()
  const versionRequests = new Map<string, Promise<ServerVersion[]>>()
  let versionRequestGeneration = 0
  let taskRefreshGeneration = 0

  const selectedSession = computed(() =>
    sshSessions.sessions.find((session) => session.id === selectedSSHSessionID.value),
  )
  const selectedVersion = computed(() =>
    versions.value.find((item) => item.version === server.value.version),
  )
  const overallProgress = computed(() => {
    if (!steps.value.length) return 0
    return steps.value.reduce((sum, step) => sum + step.progress, 0) / steps.value.length
  })
  const running = computed(() => task.value?.state === 'running' || task.value?.state === 'waiting')

  /**
   * 加载向导所需的 SSH Session、发行版与 Java 运行时选项。
   * @returns 初始化完成后的 Promise
   */
  async function initialize(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      const hadPersistedDraft = localStorage.getItem(storageKey) !== null
      restoreDraft()
      if (!hadPersistedDraft && settings.committed?.firewall.defaultPolicy) {
        server.value.firewallPolicy = settings.committed.firewall.defaultPolicy
      }
      if (!sshSessions.sessions.length) await sshSessions.refresh()
      if (pendingServerID.value && !createdServer.value) {
        try {
          createdServer.value = await getMinecraftServer(pendingServerID.value)
        } catch (reason) {
          if (!(reason instanceof ApplicationError) || reason.code !== 'io.not_found') throw reason
          pendingServerID.value = ''
        }
      }
      distributions.value = await listServerDistributions()
      if (!sshSessions.sessions.length) {
        sshMode.value = 'new'
        selectedSSHSessionID.value = ''
      } else if (
        !sshSessions.sessions.some((session) => session.id === selectedSSHSessionID.value)
      ) {
        selectedSSHSessionID.value = sshSessions.sessions[0]?.id ?? ''
      }
      const preferred = distributions.value.find(
        (item) => item.type === server.value.type && item.catalogReady && item.installerReady,
      )
      const fallback = distributions.value.find((item) => item.catalogReady && item.installerReady)
      server.value.type = preferred?.type ?? fallback?.type ?? ''
      if (server.value.type) await loadVersions(server.value.type)
      if (taskID.value) await refreshTask()
    } catch (reason) {
      error.value = reason
      throw reason
    } finally {
      loading.value = false
    }
  }

  /**
   * 加载指定发行版可安装的版本列表。
   * @param distribution - 发行版标识，默认取当前草稿的服务端类型
   * @returns 加载完成后的 Promise
   */
  async function loadVersions(distribution = server.value.type): Promise<void> {
    const targetDistribution = distribution.trim()
    const generation = ++versionRequestGeneration
    if (!targetDistribution) {
      versions.value = []
      server.value.version = ''
      versionsLoading.value = false
      return
    }
    const cached = versionCache.get(targetDistribution)
    versions.value = cached ? [...cached] : []
    if (!cached?.some((item) => item.version === server.value.version)) {
      server.value.version = cached?.[0]?.version ?? ''
    }
    versionsLoading.value = !cached
    try {
      let request = versionRequests.get(targetDistribution)
      if (!request) {
        request = resolveServerVersions(targetDistribution).finally(() => {
          versionRequests.delete(targetDistribution)
        })
        versionRequests.set(targetDistribution, request)
      }
      const resolved = await request
      if (generation !== versionRequestGeneration || server.value.type !== targetDistribution) {
        return
      }
      versionCache.set(targetDistribution, [...resolved])
      versions.value = [...resolved]
      if (!resolved.some((item) => item.version === server.value.version)) {
        server.value.version = resolved[0]?.version ?? ''
      }
    } catch (reason) {
      if (generation !== versionRequestGeneration || server.value.type !== targetDistribution) {
        return
      }
      if (cached?.length) return
      throw reason
    } finally {
      if (generation === versionRequestGeneration) versionsLoading.value = false
    }
  }

  /**
   * 测试当前选中的 SSH Session 连通性。
   * @returns 测试完成后的 Promise
   */
  async function testSelectedSSH(): Promise<void> {
    const id = selectedSSHSessionID.value
    if (!id) throw new Error(translate('store.selectSSHSession'))
    const evidence = await testSSHSessionConnection(id)
    connectionEvidence.value = `${evidence.serverVersion} · ${evidence.remoteAddress} · ${evidence.connectDurationMs}ms`
  }

  /**
   * 用向导内填写的信息新建 SSH Session 并立即测试连通性。
   * @returns 创建并测试完成后的 Promise
   */
  async function createAndTestSSH(): Promise<void> {
    const created = await createSSHSession({ ...newSSH.value })
    newSSH.value.secret = ''
    newSSH.value.passphrase = ''
    await sshSessions.refresh()
    selectedSSHSessionID.value = created.id
    sshMode.value = 'existing'
    await testSelectedSSH()
  }

  /**
   * 校验当前步骤后进入下一步。
   * @returns 跳转完成后的 Promise
   */
  async function next(): Promise<void> {
    if (currentStep.value === 1) {
      if (sshMode.value === 'new') await createAndTestSSH()
      if (!selectedSSHSessionID.value) throw new Error(translate('store.selectSSHSession'))
      server.value.sshSessionID = selectedSSHSessionID.value
      currentStep.value = 2
      return
    }
    if (currentStep.value === 2) {
      validateServerDraft()
      currentStep.value = 3
    }
  }

  /**
   * 校验草稿、创建 Server 并启动安装任务。
   * @returns 提交完成后的 Promise
   */
  async function submit(): Promise<void> {
    validateServerDraft()
    if (!server.value.eulaAccepted) throw new Error(translate('store.confirmEULA'))
    loading.value = true
    error.value = null
    try {
      server.value.sshSessionID = selectedSSHSessionID.value
      const input: MinecraftServerInput = {
        sshSessionID: server.value.sshSessionID,
        javaRuntimeID: server.value.javaRuntimeID,
        name: server.value.name,
        type: server.value.type,
        version: server.value.version,
        remotePath: server.value.remotePath,
        group: server.value.group,
        tags: [...server.value.tags],
        favourite: server.value.favourite,
        launchProfile: {
          xmsMiB: server.value.launchProfile.xmsMiB,
          xmxMiB: server.value.launchProfile.xmxMiB,
          jvmArguments: [...server.value.launchProfile.jvmArguments],
          jarPath: server.value.launchProfile.jarPath,
          workingDirectory: server.value.launchProfile.workingDirectory,
          serverArguments: [...server.value.launchProfile.serverArguments],
        },
        firewallPolicy: server.value.firewallPolicy,
        eulaAccepted: server.value.eulaAccepted,
      }
      if (!createdServer.value) {
        const existing = await listMinecraftServers('', input.sshSessionID)
        createdServer.value =
          existing.find(
            (item) =>
              item.name === input.name &&
              item.remotePath === input.remotePath &&
              (item.state === 'creating' || item.state === 'failed'),
          ) ?? null
      }
      createdServer.value = createdServer.value
        ? await updateMinecraftServer(createdServer.value.id, input)
        : await createMinecraftServer(input)
      pendingServerID.value = createdServer.value.id
      const started = await startInstallation(createdServer.value.id)
      taskID.value = started.taskID
      operationID.value = started.operationID
      await refreshTask()
    } catch (reason) {
      error.value = reason
      throw reason
    } finally {
      loading.value = false
    }
  }

  /**
   * 拉取当前安装任务的最新聚合状态。
   * @returns 刷新完成后的 Promise
   */
  async function refreshTask(): Promise<void> {
    const requestedTaskID = taskID.value
    if (!requestedTaskID) return
    const refreshGeneration = ++taskRefreshGeneration
    const aggregate = await getInstallation(requestedTaskID)
    if (refreshGeneration !== taskRefreshGeneration || taskID.value !== requestedTaskID) return
    task.value = aggregate.task
    steps.value = aggregate.steps
  }

  /**
   * 取消进行中的安装任务。
   * @returns 取消完成后的 Promise
   */
  async function cancel(): Promise<void> {
    if (!operationID.value) return
    await cancelInstallation(operationID.value)
    await refreshTask()
  }

  /**
   * 重试失败的安装任务。
   * @returns 重试完成后的 Promise
   */
  async function retry(): Promise<void> {
    if (!taskID.value) return
    const started = await retryInstallation(taskID.value)
    operationID.value = started.operationID
    await refreshTask()
  }

  /**
   * 处理安装目录冲突：备份、改名或取消。
   * @param action - 冲突处理方式
   * @param newName - 改名时的新目录名
   * @returns 处理完成后的 Promise
   */
  async function resolveDirectoryConflict(
    action: 'backup' | 'rename' | 'cancel',
    newName = '',
  ): Promise<void> {
    if (!taskID.value) return
    const started = await resolveInstallationDirectoryConflict(taskID.value, action, newName)
    operationID.value = started.operationID
    await refreshTask()
  }

  /**
   * 把向导恢复到初始步骤与空白草稿。
   * @returns 无返回值
   */
  function resetDraft(): void {
    currentStep.value = 1
    sshMode.value = sshSessions.sessions.length ? 'existing' : 'new'
    selectedSSHSessionID.value = sshSessions.sessions[0]?.id ?? ''
    newSSH.value = emptySSHInput()
    server.value = emptyServerInput()
    createdServer.value = null
    pendingServerID.value = ''
    task.value = null
    steps.value = []
    taskID.value = ''
    operationID.value = ''
    taskRefreshGeneration++
    connectionEvidence.value = ''
    sessionStorage.removeItem(storageKey)
  }

  /**
   * 校验 Server 草稿的必填项与格式，不合法时抛出异常。
   * @returns 无返回值
   */
  function validateServerDraft(): void {
    const name = server.value.name.trim()
    if (!selectedSSHSessionID.value || !name || !server.value.type || !server.value.version) {
      throw new Error(translate('store.wizardRequiredFields'))
    }
    if (
      server.value.launchProfile.xmsMiB <= 0 ||
      server.value.launchProfile.xmsMiB > server.value.launchProfile.xmxMiB
    ) {
      throw new Error(translate('store.wizardMemoryRange'))
    }
    if (!server.value.remotePath) {
      const username = selectedSession.value?.username ?? 'minecraft'
      const home = username === 'root' ? '/root' : `/home/${username}`
      server.value.remotePath = `${home}/MineOps/Servers/${safeDirectoryName(name)}`
    }
    if (
      ['velocity', 'waterfall', 'bungeecord'].includes(server.value.type) &&
      JSON.stringify(server.value.launchProfile.serverArguments) === JSON.stringify(['nogui'])
    ) {
      server.value.launchProfile.serverArguments = []
    }
    server.value.launchProfile.workingDirectory = server.value.remotePath
  }

  /**
   * 从会话存储恢复上次未完成的向导草稿。
   * @returns 无返回值
   */
  function restoreDraft(): void {
    const payload = sessionStorage.getItem(storageKey)
    if (!payload) return
    try {
      const stored = JSON.parse(payload) as Record<string, unknown>
      currentStep.value = Number(stored.currentStep) || 1
      sshMode.value = stored.sshMode === 'new' ? 'new' : 'existing'
      selectedSSHSessionID.value = String(stored.selectedSSHSessionID ?? '')
      newSSH.value = {
        ...emptySSHInput(),
        ...(stored.newSSH as Partial<SSHSessionInput>),
        secret: '',
        passphrase: '',
      }
      server.value = { ...emptyServerInput(), ...(stored.server as Partial<MinecraftServerInput>) }
      taskID.value = String(stored.taskID ?? '')
      operationID.value = String(stored.operationID ?? '')
      pendingServerID.value = String(stored.pendingServerID ?? '')
    } catch {
      sessionStorage.removeItem(storageKey)
    }
  }

  watch(
    () => server.value.type,
    (type, previousType) => {
      const previousDefaults = defaultServerArguments(previousType)
      if (
        server.value.launchProfile.serverArguments.length === 0 ||
        JSON.stringify(server.value.launchProfile.serverArguments) ===
          JSON.stringify(previousDefaults)
      ) {
        server.value.launchProfile.serverArguments = defaultServerArguments(type)
      }
    },
  )

  watch(
    [
      currentStep,
      sshMode,
      selectedSSHSessionID,
      newSSH,
      server,
      taskID,
      operationID,
      pendingServerID,
    ],
    () => {
      sessionStorage.setItem(
        storageKey,
        JSON.stringify({
          currentStep: currentStep.value,
          sshMode: sshMode.value,
          selectedSSHSessionID: selectedSSHSessionID.value,
          newSSH: { ...newSSH.value, secret: '', passphrase: '' },
          server: server.value,
          taskID: taskID.value,
          operationID: operationID.value,
          pendingServerID: pendingServerID.value,
        }),
      )
    },
    { deep: true },
  )

  subscribeOperationProgress((operation) => {
    const matchesActiveOperation = operation.id === operationID.value
    const matchesPendingInstallation =
      Boolean(pendingServerID.value) &&
      operation.type === 'install' &&
      operation.targetID === pendingServerID.value
    if (matchesActiveOperation || matchesPendingInstallation) void refreshTask()
  })

  return {
    cancel,
    connectionEvidence,
    createAndTestSSH,
    createdServer,
    currentStep,
    distributions,
    error,
    initialize,
    loadVersions,
    loading,
    newSSH,
    next,
    operationID,
    overallProgress,
    refreshTask,
    resetDraft,
    resolveDirectoryConflict,
    retry,
    running,
    selectedSession,
    selectedSSHSessionID,
    selectedVersion,
    server,
    sshMode,
    steps,
    submit,
    task,
    taskID,
    testSelectedSSH,
    versions,
    versionsLoading,
  }
})

/**
 * 构造一份空白的 SSH Session 输入。
 * @returns 空白 SSH Session 输入
 */
function emptySSHInput(): SSHSessionInput {
  return {
    name: '',
    host: '',
    port: 22,
    username: '',
    authType: 'password',
    secret: '',
    passphrase: '',
    hostKeyPolicy: 'strict',
    group: '',
    favourite: false,
    remark: '',
    connectTimeoutSec: 10,
    handshakeTimeoutSec: 15,
    keepAliveSec: 30,
    compression: false,
    overrideSettings: false,
  }
}

/**
 * 构造一份空白的 Server 输入。
 * @returns 空白 Server 输入
 */
function emptyServerInput(): MinecraftServerInput {
  return {
    sshSessionID: '',
    javaRuntimeID: '',
    name: '',
    type: '',
    version: '',
    remotePath: '',
    group: '',
    tags: [],
    favourite: false,
    launchProfile: {
      xmsMiB: 1024,
      xmxMiB: 2048,
      jvmArguments: [],
      jarPath: 'server.jar',
      workingDirectory: '',
      serverArguments: ['nogui'],
    },
    firewallPolicy: 'prompt',
    eulaAccepted: false,
  }
}

/**
 * 把名称转换成可安全用作目录名的字符串。
 * @param value - 原始名称
 * @returns 过滤掉非法字符后的目录名
 */
function safeDirectoryName(value: string): string {
  return (
    value
      .trim()
      .replace(/[^A-Za-z0-9._-]+/g, '-')
      .replace(/^[._-]+|[._-]+$/g, '')
      .slice(0, 80) || 'server'
  )
}

/**
 * 按服务端类型给出默认的 JVM 启动参数。
 * @param serverType - 服务端类型
 * @returns 默认启动参数数组，代理端返回空数组
 */
function defaultServerArguments(serverType: string): string[] {
  return ['velocity', 'waterfall', 'bungeecord'].includes(serverType) ? [] : ['nogui']
}
