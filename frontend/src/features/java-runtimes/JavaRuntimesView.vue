<script setup lang="ts">
import { CheckCircle2, Download, Search, Star, Trash2 } from '@lucide/vue'
import {
  NButton,
  NCard,
  NAlert,
  NFlex,
  NInput,
  NSelect,
  NTag,
  NText,
  type DataTableColumns,
} from 'naive-ui'
import { computed, h, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'

import type { JavaRuntime } from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import type { JavaCandidate } from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/service/models'
import type { JDKArtifact } from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/port/models'
import { ApplicationError } from '../../services/api-client'
import { listJDKArtifacts, recommendJavaRuntime } from '../../services/java-runtime-api'
import AppDataTable from '../../shared/components/AppDataTable.vue'
import AppIcon from '../../shared/components/AppIcon.vue'
import { useInteractionStore } from '../../stores/interactions'
import { useJavaRuntimesStore } from '../../stores/java-runtimes'
import { useLocaleStore } from '../../stores/locale'
import { useNotificationStore } from '../../stores/notifications'
import { useSSHSessionsStore } from '../../stores/ssh-sessions'

const store = useJavaRuntimesStore()
const router = useRouter()
const locale = useLocaleStore()
const sshSessions = useSSHSessionsStore()
const interactions = useInteractionStore()
const notifications = useNotificationStore()
const manualPath = ref('')
const serverType = ref('paper')
const minecraftVersion = ref('1.21.1')
const catalogMajor = ref(21)
const catalogArchitecture = ref('x64')
const catalogLoading = ref(false)
const artifacts = ref<JDKArtifact[]>([])
const sessionError = ref<unknown>(null)
const catalogError = ref<unknown>(null)
const partialMessage = computed(() =>
  sessionError.value
    ? sshSessions.sessions.length
      ? locale.t('java.sessionRefreshFailed')
      : locale.t('java.sessionUnavailable')
    : '',
)

const majorOptions = computed(() => [
  { label: locale.t('java.majorAll'), value: 0 },
  ...[8, 11, 16, 17, 21, 24, 25].map((value) => ({
    label: locale.t('java.majorVersion', { value }),
    value,
  })),
])
const serverTypeOptions = [
  'vanilla',
  'paper',
  'purpur',
  'spigot',
  'fabric',
  'forge',
  'neoforge',
  'quilt',
  'folia',
  'velocity',
].map((value) => ({ label: value, value }))
const architectureOptions = [
  { label: 'Linux x64', value: 'x64' },
  { label: 'Linux ARM64', value: 'aarch64' },
]

const runtimeColumns = computed<DataTableColumns<JavaRuntime>>(() => [
  {
    title: locale.t('java.column.default'),
    key: 'default',
    width: 70,
    render: (row) =>
      row.default
        ? h(
            NTag,
            { type: 'success', bordered: false },
            { default: () => locale.t('java.tagDefault') },
          )
        : null,
  },
  { title: locale.t('java.column.version'), key: 'version', sorter: 'default', minWidth: 140 },
  { title: locale.t('java.column.major'), key: 'majorVersion', width: 90 },
  { title: 'Vendor', key: 'vendor', minWidth: 160 },
  { title: locale.t('java.column.architecture'), key: 'architecture', width: 110 },
  {
    title: locale.t('java.column.source'),
    key: 'source',
    width: 120,
    render: (row) =>
      h(
        NTag,
        { type: row.managed ? 'info' : 'default', bordered: false },
        { default: () => row.source },
      ),
  },
  {
    title: locale.t('java.column.installPath'),
    key: 'installPath',
    minWidth: 260,
    ellipsis: { tooltip: true },
  },
  {
    title: locale.t('common.actions'),
    key: 'actions',
    width: 140,
    render: (row) =>
      h(NFlex, { wrap: false }, () => [
        h(
          NButton,
          {
            quaternary: true,
            circle: true,
            disabled: row.default,
            onClick: () => void setDefault(row),
          },
          { default: () => h(AppIcon, { icon: Star, label: locale.t('java.setDefault') }) },
        ),
        h(
          NButton,
          { quaternary: true, circle: true, type: 'error', onClick: () => void remove(row) },
          {
            default: () => h(AppIcon, { icon: Trash2, label: locale.t('java.removeRegistration') }),
          },
        ),
      ]),
  },
])

const candidateColumns = computed<DataTableColumns<JavaCandidate>>(() => [
  {
    title: locale.t('java.column.version'),
    key: 'info.version',
    render: (row) => row.info.version,
    minWidth: 140,
  },
  {
    title: locale.t('java.column.major'),
    key: 'major',
    render: (row) => row.info.majorVersion,
    width: 90,
  },
  { title: 'Vendor', key: 'vendor', render: (row) => row.info.vendor, minWidth: 160 },
  {
    title: locale.t('java.column.architecture'),
    key: 'architecture',
    render: (row) => row.info.architecture,
    width: 110,
  },
  { title: locale.t('java.column.source'), key: 'source', width: 120 },
  {
    title: locale.t('java.column.executable'),
    key: 'executablePath',
    minWidth: 280,
    ellipsis: { tooltip: true },
  },
  {
    title: locale.t('common.actions'),
    key: 'actions',
    width: 100,
    render: (row) =>
      h(
        NButton,
        { size: 'small', type: 'primary', onClick: () => void importCandidate(row) },
        { default: () => locale.t('java.register') },
      ),
  },
])

const artifactColumns = computed<DataTableColumns<JDKArtifact>>(() => [
  { title: locale.t('java.column.version'), key: 'version', minWidth: 160 },
  { title: 'Vendor', key: 'vendor', minWidth: 160 },
  { title: locale.t('java.column.architecture'), key: 'architecture', width: 100 },
  { title: locale.t('java.column.format'), key: 'archiveType', width: 90 },
  {
    title: locale.t('java.column.size'),
    key: 'size',
    width: 110,
    render: (row) => locale.t('java.sizeMiB', { value: (row.size / 1024 / 1024).toFixed(1) }),
  },
  { title: 'SHA-256', key: 'sha256', minWidth: 260, ellipsis: { tooltip: true } },
  {
    title: locale.t('common.actions'),
    key: 'actions',
    width: 110,
    render: (row) =>
      h(
        NButton,
        {
          size: 'small',
          type: 'primary',
          loading: store.installing,
          disabled: !store.sshSessionID || store.installing,
          onClick: () => void installArtifact(row),
        },
        {
          icon: () => h(AppIcon, { icon: Download, label: locale.t('java.install') }),
          default: () => locale.t('java.install'),
        },
      ),
  },
])

/**
 * 重新加载当前 SSH Session 上的 Java 运行时列表。
 * @returns 刷新完成后的 Promise
 */
async function refresh(): Promise<void> {
  try {
    await store.refresh()
  } catch (error) {
    notifyError(locale.t('java.loadFailed'), error)
  }
}

/**
 * 加载可选的 SSH Session 列表。
 * @returns 加载完成后的 Promise
 */
async function refreshSessions(): Promise<void> {
  sessionError.value = null
  try {
    await sshSessions.refresh()
    if (!store.sshSessionID && sshSessions.sessions[0]) {
      store.sshSessionID = sshSessions.sessions[0].id
    }
    if (store.sshSessionID) await refresh()
  } catch (error) {
    sessionError.value = error
  }
}

/**
 * 切换目标 SSH Session 并清空上一台主机的探测结果。
 * @param value - SSH Session ID
 * @returns 切换完成后的 Promise
 */
async function selectSSHSession(value: string): Promise<void> {
  store.sshSessionID = value
  store.candidates = []
  await refresh()
}

/**
 * 在远端主机上探测可用的 Java 候选。
 * @returns 探测完成后的 Promise
 */
async function discover(): Promise<void> {
  try {
    await store.discover()
    notifications.push({
      kind: 'success',
      title: locale.t('java.discoverSucceeded'),
      content: locale.t('java.discoverSucceededContent', { count: store.candidates.length }),
      dedupeKey: `java:discover:${store.sshSessionID}`,
    })
  } catch (error) {
    notifyError(locale.t('java.discoverFailed'), error)
  }
}

/**
 * 把探测到的 Java 候选登记为运行时。
 * @param candidate - Java 候选
 * @returns 登记完成后的 Promise
 */
async function importCandidate(candidate: JavaCandidate): Promise<void> {
  try {
    const runtime = await store.importPath(candidate.executablePath)
    notifications.push({
      kind: 'success',
      title: locale.t('java.registered'),
      content: `${runtime.version} · ${runtime.installPath}`,
      dedupeKey: `java:import:${runtime.id}`,
    })
  } catch (error) {
    notifyError(locale.t('java.registerFailed'), error)
  }
}

/**
 * 按手工填写的路径登记 Java 运行时。
 * @returns 登记完成后的 Promise
 */
async function importManual(): Promise<void> {
  if (!manualPath.value.trim()) return
  try {
    const runtime = await store.importPath(manualPath.value.trim())
    manualPath.value = ''
    notifications.push({
      kind: 'success',
      title: locale.t('java.registered'),
      content: `${runtime.version} · ${runtime.installPath}`,
      dedupeKey: `java:import:${runtime.id}`,
    })
  } catch (error) {
    notifyError(locale.t('java.registerFailed'), error)
  }
}

/**
 * 把某个 Java 运行时设为该主机的默认项。
 * @param runtime - 目标 Java 运行时
 * @returns 设置完成后的 Promise
 */
async function setDefault(runtime: JavaRuntime): Promise<void> {
  try {
    await store.setDefault(runtime.id)
    notifications.push({
      kind: 'success',
      title: locale.t('java.defaultUpdated'),
      content: runtime.version,
      dedupeKey: `java:default:${runtime.id}`,
    })
  } catch (error) {
    notifyError(locale.t('java.defaultFailed'), error)
  }
}

/**
 * 二次确认后删除一条 Java 运行时登记。
 * @param runtime - 目标 Java 运行时
 * @returns 删除完成后的 Promise
 */
async function remove(runtime: JavaRuntime): Promise<void> {
  const confirmed = await interactions.confirm({
    title: locale.t('java.deleteTitle'),
    content: locale.t('java.deleteContent'),
    objectLabel: `${runtime.version} · ${runtime.installPath}`,
    positiveText: locale.t('java.removeRegistration'),
    danger: true,
  })
  if (!confirmed) return
  try {
    await store.remove(runtime.id)
    notifications.push({
      kind: 'success',
      title: locale.t('java.deleted'),
      dedupeKey: `java:deleted:${runtime.id}`,
    })
  } catch (error) {
    notifyError(locale.t('java.deleteFailed'), error)
  }
}

/**
 * 按服务端类型与版本推荐合适的 Java 运行时。
 * @returns 推荐完成后的 Promise
 */
async function recommend(): Promise<void> {
  try {
    const runtime = await recommendJavaRuntime(
      store.sshSessionID,
      serverType.value,
      minecraftVersion.value,
    )
    interactions.openDrawer({
      title: locale.t('java.recommendTitle'),
      content: [
        `Server: ${serverType.value} ${minecraftVersion.value}`,
        `Java: ${runtime.version}`,
        `Vendor: ${runtime.vendor}`,
        `Architecture: ${runtime.architecture}`,
        `Path: ${runtime.installPath}`,
      ].join('\n'),
    })
  } catch (error) {
    notifyError(locale.t('java.recommendFailed'), error)
  }
}

/**
 * 加载可下载安装的 JDK 构件列表。
 * @returns 加载完成后的 Promise
 */
async function loadArtifacts(): Promise<void> {
  catalogLoading.value = true
  catalogError.value = null
  try {
    artifacts.value = await listJDKArtifacts(catalogMajor.value, catalogArchitecture.value)
  } catch (error) {
    catalogError.value = error
    notifyError(locale.t('java.catalogLoadFailed'), error)
  } finally {
    catalogLoading.value = false
  }
}

/**
 * 二次确认后在远端安装一个 JDK 构件。
 * @param artifact - 目标 JDK 构件
 * @returns 安装发起后的 Promise
 */
async function installArtifact(artifact: JDKArtifact): Promise<void> {
  const confirmed = await interactions.confirm({
    title: locale.t('java.installTitle', { major: artifact.majorVersion }),
    content: locale.t('java.installContent'),
    objectLabel: `${artifact.vendor} ${artifact.version} · Linux ${artifact.architecture}`,
    impact: locale.t('java.installImpact'),
    positiveText: locale.t('java.installConfirm'),
  })
  if (!confirmed) return
  try {
    const operationID = await store.install(artifact.majorVersion, artifact.architecture)
    notifications.push({
      kind: 'info',
      title: locale.t('java.installStarted'),
      content: operationID,
      dedupeKey: `java:install:${operationID}`,
    })
  } catch (error) {
    notifyError(locale.t('java.installStartFailed'), error)
  }
}

/**
 * 推送一条错误通知，并展开远端返回的详细原因。
 * @param title - 通知标题
 * @param error - 捕获到的错误
 * @returns 无返回值
 */
function notifyError(title: string, error: unknown): void {
  const message = error instanceof Error ? error.message : String(error)
  const remoteError =
    error instanceof ApplicationError && typeof error.details.stderr === 'string'
      ? error.details.stderr.trim()
      : ''
  notifications.push({
    kind: 'error',
    title,
    content: remoteError ? locale.t('java.remoteError', { message, detail: remoteError }) : message,
    dedupeKey: `java:error:${title}`,
  })
}

onMounted(async () => {
  if (sshSessions.sessions.length) {
    if (!store.sshSessionID && sshSessions.sessions[0]) {
      store.sshSessionID = sshSessions.sessions[0].id
    }
    if (store.sshSessionID) await refresh()
    return
  }
  await refreshSessions()
})
</script>

<template>
  <NFlex vertical :size="16">
    <NAlert
      v-if="sessionError && !sshSessions.sessions.length"
      :type="
        sessionError instanceof ApplicationError && sessionError.code.includes('permission_denied')
          ? 'warning'
          : 'error'
      "
      :title="
        sessionError instanceof ApplicationError && sessionError.code.includes('permission_denied')
          ? locale.t('java.sessionPermission')
          : locale.t('java.sessionLoadFailed')
      "
    >
      <NFlex align="center" justify="space-between">
        <span>{{
          sessionError instanceof Error ? sessionError.message : String(sessionError)
        }}</span>
        <NButton size="small" :loading="sshSessions.loading" @click="refreshSessions">
          {{ locale.t('common.retry') }}
        </NButton>
      </NFlex>
    </NAlert>
    <NAlert
      v-else-if="!sshSessions.sessions.length && !sshSessions.loading"
      type="info"
      :title="locale.t('java.noSessionTitle')"
    >
      <NFlex align="center" justify="space-between">
        <span>{{ locale.t('java.noSessionContent') }}</span>
        <NButton size="small" type="primary" @click="router.push('/ssh-sessions')">
          {{ locale.t('java.createSession') }}
        </NButton>
      </NFlex>
    </NAlert>
    <NCard>
      <NFlex :wrap="false">
        <NSelect
          :value="store.sshSessionID"
          :disabled="!sshSessions.sessions.length"
          :placeholder="locale.t('java.selectSession')"
          :options="
            sshSessions.sessions.map((session) => ({ label: session.name, value: session.id }))
          "
          @update:value="selectSSHSession"
        />
        <NSelect
          v-model:value="store.majorVersion"
          :options="majorOptions"
          style="width: 160px"
          @update:value="refresh"
        />
        <NButton :loading="store.loading" @click="refresh">
          {{ locale.t('common.refresh') }}
        </NButton>
        <NButton
          type="primary"
          :loading="store.discovering"
          :disabled="!store.sshSessionID"
          @click="discover"
        >
          <template #icon>
            <AppIcon :icon="Search" :label="locale.t('java.discoverIcon')" />
          </template>
          {{ locale.t('java.discoverButton') }}
        </NButton>
      </NFlex>
    </NCard>

    <NCard :title="locale.t('java.registeredCard')">
      <NAlert v-if="partialMessage" type="warning" :title="locale.t('java.partialTitle')">{{
        partialMessage
      }}</NAlert>
      <AppDataTable
        :columns="runtimeColumns"
        :data="store.runtimes"
        :loading="store.loading"
        :error="store.runtimeError"
        :empty-description="locale.t('java.emptyRuntimes')"
        @retry="refresh"
      />
    </NCard>

    <NCard
      v-if="store.candidates.length || store.discoveryError"
      :title="locale.t('java.candidatesCard')"
    >
      <AppDataTable
        :columns="candidateColumns"
        :data="store.candidates"
        :error="store.discoveryError"
        @retry="discover"
      />
    </NCard>

    <NCard :title="locale.t('java.manualCard')">
      <NFlex :wrap="false">
        <NInput
          v-model:value="manualPath"
          :disabled="!store.sshSessionID"
          :placeholder="locale.t('java.manualPlaceholder')"
        />
        <NButton type="primary" :disabled="!store.sshSessionID" @click="importManual">
          <template #icon>
            <AppIcon :icon="Download" :label="locale.t('java.register')" />
          </template>
          {{ locale.t('java.manualSubmit') }}
        </NButton>
      </NFlex>
    </NCard>

    <NCard :title="locale.t('java.compatCard')">
      <NFlex :wrap="false">
        <NSelect v-model:value="serverType" :options="serverTypeOptions" />
        <NInput v-model:value="minecraftVersion" :placeholder="locale.t('java.minecraftVersion')" />
        <NButton type="primary" :disabled="!store.sshSessionID" @click="recommend">
          <template #icon>
            <AppIcon :icon="CheckCircle2" :label="locale.t('java.recommendIcon')" />
          </template>
          {{ locale.t('java.recommendButton') }}
        </NButton>
      </NFlex>
    </NCard>

    <NCard :title="locale.t('java.catalogCard')">
      <NFlex :wrap="false" class="catalog-toolbar">
        <NSelect
          v-model:value="catalogMajor"
          :options="majorOptions.filter((option) => option.value !== 0 && option.value !== 16)"
        />
        <NSelect v-model:value="catalogArchitecture" :options="architectureOptions" />
        <NButton :loading="catalogLoading" @click="loadArtifacts">
          {{ locale.t('java.queryTemurin') }}
        </NButton>
      </NFlex>
      <NAlert
        v-if="store.installOperationID"
        :type="
          store.installOperation?.state === 'succeeded'
            ? 'success'
            : store.installOperation?.state === 'failed'
              ? 'error'
              : store.installOperation?.state === 'cancelled'
                ? 'warning'
                : 'info'
        "
        :title="locale.t('java.installOperation')"
        class="catalog-operation"
      >
        <NFlex vertical :size="6">
          <NFlex align="center" :size="8">
            <NTag :type="store.installing ? 'info' : 'default'" size="small" :bordered="false">
              {{ store.installOperation?.state ?? 'pending' }}
            </NTag>
            <NText>{{ store.installOperationID }}</NText>
          </NFlex>
          <NText>
            {{ store.installOperation?.message || locale.t('java.installPending') }}
          </NText>
          <NText v-if="store.installOperation" depth="3">
            {{
              locale.t('java.installStage', {
                stage: store.installOperation.stage || 'pending',
                progress: Math.round(store.installOperation.progress * 100),
              })
            }}
          </NText>
        </NFlex>
      </NAlert>
      <NAlert
        v-else-if="store.installError"
        type="error"
        :title="locale.t('java.installFailedTitle')"
        class="catalog-operation"
      >
        {{
          store.installError instanceof Error
            ? store.installError.message
            : String(store.installError)
        }}
      </NAlert>
      <AppDataTable
        :columns="artifactColumns"
        :data="artifacts"
        :loading="catalogLoading"
        :error="catalogError"
        :empty-description="locale.t('java.emptyArtifacts')"
        @retry="loadArtifacts"
      />
      <NText depth="3">{{ locale.t('java.catalogNote') }}</NText>
    </NCard>
  </NFlex>
</template>

<style scoped>
.catalog-toolbar {
  margin-bottom: 16px;
}

.catalog-operation {
  margin-bottom: 16px;
}
</style>
