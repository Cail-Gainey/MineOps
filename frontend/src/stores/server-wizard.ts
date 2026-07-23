import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'

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

  /** Initializes sessions, dynamic distributions, persisted draft, and task recovery. */
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

  /** Loads provider versions when the selected dynamic distribution changes. */
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

  /** Tests the selected persisted SSH Session and records handshake evidence. */
  async function testSelectedSSH(): Promise<void> {
    const id = selectedSSHSessionID.value
    if (!id) throw new Error('请选择 SSH Session')
    const evidence = await testSSHSessionConnection(id)
    connectionEvidence.value = `${evidence.serverVersion} · ${evidence.remoteAddress} · ${evidence.connectDurationMs}ms`
  }

  /** Creates a write-only credential SSH Session, synchronizes the list, and tests it. */
  async function createAndTestSSH(): Promise<void> {
    const created = await createSSHSession({ ...newSSH.value })
    newSSH.value.secret = ''
    newSSH.value.passphrase = ''
    await sshSessions.refresh()
    selectedSSHSessionID.value = created.id
    sshMode.value = 'existing'
    await testSelectedSSH()
  }

  /** Advances the three-step wizard after validating the active step. */
  async function next(): Promise<void> {
    if (currentStep.value === 1) {
      if (sshMode.value === 'new') await createAndTestSSH()
      if (!selectedSSHSessionID.value) throw new Error('请选择 SSH Session')
      server.value.sshSessionID = selectedSSHSessionID.value
      currentStep.value = 2
      return
    }
    if (currentStep.value === 2) {
      validateServerDraft()
      currentStep.value = 3
    }
  }

  /** Creates the temporary Server and starts the durable installation Operation. */
  async function submit(): Promise<void> {
    validateServerDraft()
    if (!server.value.eulaAccepted) throw new Error('请先确认 Minecraft EULA 自动写入行为')
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

  /** Refreshes the durable task so closing and reopening the Wizard does not lose state. */
  async function refreshTask(): Promise<void> {
    if (!taskID.value) return
    const aggregate = await getInstallation(taskID.value)
    task.value = aggregate.task
    steps.value = aggregate.steps
  }

  /** Requests cancellation while retaining completed checkpoints. */
  async function cancel(): Promise<void> {
    if (!operationID.value) return
    await cancelInstallation(operationID.value)
    await refreshTask()
  }

  /** Starts a replacement Operation for incomplete steps only. */
  async function retry(): Promise<void> {
    if (!taskID.value) return
    const started = await retryInstallation(taskID.value)
    operationID.value = started.operationID
    await refreshTask()
  }

  /** Applies backup, rename, or cancel to the failed directory decision step. */
  async function resolveDirectoryConflict(
    action: 'backup' | 'rename' | 'cancel',
    newName = '',
  ): Promise<void> {
    if (!taskID.value) return
    const started = await resolveInstallationDirectoryConflict(taskID.value, action, newName)
    operationID.value = started.operationID
    await refreshTask()
  }

  /** Resets the draft while leaving an already-started backend task untouched. */
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
    connectionEvidence.value = ''
    sessionStorage.removeItem(storageKey)
  }

  function validateServerDraft(): void {
    const name = server.value.name.trim()
    if (!selectedSSHSessionID.value || !name || !server.value.type || !server.value.version) {
      throw new Error('SSH、名称、服务端类型和版本不能为空')
    }
    if (
      server.value.launchProfile.xmsMiB <= 0 ||
      server.value.launchProfile.xmsMiB > server.value.launchProfile.xmxMiB
    ) {
      throw new Error('Xms 必须大于 0 且不能超过 Xmx')
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
    if (operation.id === operationID.value) void refreshTask()
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

function safeDirectoryName(value: string): string {
  return (
    value
      .trim()
      .replace(/[^A-Za-z0-9._-]+/g, '-')
      .replace(/^[._-]+|[._-]+$/g, '')
      .slice(0, 80) || 'server'
  )
}

function defaultServerArguments(serverType: string): string[] {
  return ['velocity', 'waterfall', 'bungeecord'].includes(serverType) ? [] : ['nogui']
}
