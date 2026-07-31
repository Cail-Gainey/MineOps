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
import { useNotificationStore } from '../../stores/notifications'
import { useSSHSessionsStore } from '../../stores/ssh-sessions'

const store = useJavaRuntimesStore()
const router = useRouter()
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
      ? 'SSH Session 列表刷新失败，继续使用已加载连接。'
      : 'SSH Session 列表不可用，暂时无法选择远程 Java 目标。'
    : '',
)

const majorOptions = [
  { label: '全部版本', value: 0 },
  ...[8, 11, 16, 17, 21, 24, 25].map((value) => ({ label: `Java ${value}`, value })),
]
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

const runtimeColumns: DataTableColumns<JavaRuntime> = [
  {
    title: '默认',
    key: 'default',
    width: 70,
    render: (row) =>
      row.default ? h(NTag, { type: 'success', bordered: false }, { default: () => '默认' }) : null,
  },
  { title: '版本', key: 'version', sorter: 'default', minWidth: 140 },
  { title: '主版本', key: 'majorVersion', width: 90 },
  { title: 'Vendor', key: 'vendor', minWidth: 160 },
  { title: '架构', key: 'architecture', width: 110 },
  {
    title: '来源',
    key: 'source',
    width: 120,
    render: (row) =>
      h(
        NTag,
        { type: row.managed ? 'info' : 'default', bordered: false },
        { default: () => row.source },
      ),
  },
  { title: '安装路径', key: 'installPath', minWidth: 260, ellipsis: { tooltip: true } },
  {
    title: '操作',
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
          { default: () => h(AppIcon, { icon: Star, label: '设为默认' }) },
        ),
        h(
          NButton,
          { quaternary: true, circle: true, type: 'error', onClick: () => void remove(row) },
          { default: () => h(AppIcon, { icon: Trash2, label: '删除注册' }) },
        ),
      ]),
  },
]

const candidateColumns: DataTableColumns<JavaCandidate> = [
  { title: '版本', key: 'info.version', render: (row) => row.info.version, minWidth: 140 },
  { title: '主版本', key: 'major', render: (row) => row.info.majorVersion, width: 90 },
  { title: 'Vendor', key: 'vendor', render: (row) => row.info.vendor, minWidth: 160 },
  { title: '架构', key: 'architecture', render: (row) => row.info.architecture, width: 110 },
  { title: '来源', key: 'source', width: 120 },
  { title: '可执行文件', key: 'executablePath', minWidth: 280, ellipsis: { tooltip: true } },
  {
    title: '操作',
    key: 'actions',
    width: 100,
    render: (row) =>
      h(
        NButton,
        { size: 'small', type: 'primary', onClick: () => void importCandidate(row) },
        { default: () => '注册' },
      ),
  },
]

const artifactColumns: DataTableColumns<JDKArtifact> = [
  { title: '版本', key: 'version', minWidth: 160 },
  { title: 'Vendor', key: 'vendor', minWidth: 160 },
  { title: '架构', key: 'architecture', width: 100 },
  { title: '格式', key: 'archiveType', width: 90 },
  {
    title: '大小',
    key: 'size',
    width: 110,
    render: (row) => `${(row.size / 1024 / 1024).toFixed(1)} MiB`,
  },
  { title: 'SHA-256', key: 'sha256', minWidth: 260, ellipsis: { tooltip: true } },
  {
    title: '操作',
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
          icon: () => h(AppIcon, { icon: Download, label: '安装' }),
          default: () => '安装',
        },
      ),
  },
]

/**
 * 重新加载当前 SSH Session 上的 Java 运行时列表。
 * @returns 刷新完成后的 Promise
 */
async function refresh(): Promise<void> {
  try {
    await store.refresh()
  } catch (error) {
    notifyError('加载 Java Runtimes 失败', error)
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
      title: 'Java 发现完成',
      content: `已验证 ${store.candidates.length} 个候选`,
      dedupeKey: `java:discover:${store.sshSessionID}`,
    })
  } catch (error) {
    notifyError('远程 Java 发现失败', error)
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
      title: 'Java Runtime 已注册',
      content: `${runtime.version} · ${runtime.installPath}`,
      dedupeKey: `java:import:${runtime.id}`,
    })
  } catch (error) {
    notifyError('注册 Java Runtime 失败', error)
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
      title: 'Java Runtime 已注册',
      content: `${runtime.version} · ${runtime.installPath}`,
      dedupeKey: `java:import:${runtime.id}`,
    })
  } catch (error) {
    notifyError('注册 Java Runtime 失败', error)
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
      title: '默认 Java Runtime 已更新',
      content: runtime.version,
      dedupeKey: `java:default:${runtime.id}`,
    })
  } catch (error) {
    notifyError('设置默认 Java Runtime 失败', error)
  }
}

/**
 * 二次确认后删除一条 Java 运行时登记。
 * @param runtime - 目标 Java 运行时
 * @returns 删除完成后的 Promise
 */
async function remove(runtime: JavaRuntime): Promise<void> {
  const confirmed = await interactions.confirm({
    title: '删除 Java Runtime 注册？',
    content:
      '仅删除 MineOps 中的注册记录，不会删除远程 Java 文件。存在 Server 引用时操作会被拒绝。',
    objectLabel: `${runtime.version} · ${runtime.installPath}`,
    positiveText: '删除注册',
    danger: true,
  })
  if (!confirmed) return
  try {
    await store.remove(runtime.id)
    notifications.push({
      kind: 'success',
      title: 'Java Runtime 注册已删除',
      dedupeKey: `java:deleted:${runtime.id}`,
    })
  } catch (error) {
    notifyError('删除 Java Runtime 失败', error)
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
      title: 'Java Runtime 推荐结果',
      content: [
        `Server: ${serverType.value} ${minecraftVersion.value}`,
        `Java: ${runtime.version}`,
        `Vendor: ${runtime.vendor}`,
        `Architecture: ${runtime.architecture}`,
        `Path: ${runtime.installPath}`,
      ].join('\n'),
    })
  } catch (error) {
    notifyError('没有可推荐的 Java Runtime', error)
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
    notifyError('加载 JDK Catalog 失败', error)
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
    title: `安装 Java ${artifact.majorVersion}？`,
    content:
      'MineOps 将把经过批准的 JDK 归档下载到远程 ~/MineOps/Downloads，通过 SSH stdout 拉取归档并由本地 Go 代码执行路径、链接、体积和 SHA-256 安全检查，然后在远程 staging 目录解压并原子发布。',
    objectLabel: `${artifact.vendor} ${artifact.version} · Linux ${artifact.architecture}`,
    impact: '安装成功后会自动验证 bin/java，并注册为当前 SSH Session 可复用的托管 Runtime。',
    positiveText: '启动安装',
  })
  if (!confirmed) return
  try {
    const operationID = await store.install(artifact.majorVersion, artifact.architecture)
    notifications.push({
      kind: 'info',
      title: 'Java 安装 Operation 已启动',
      content: operationID,
      dedupeKey: `java:install:${operationID}`,
    })
  } catch (error) {
    notifyError('启动 Java 安装失败', error)
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
    content: remoteError ? `${message}\n远程错误：${remoteError}` : message,
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
          ? 'SSH Session 权限不足'
          : 'SSH Session 加载失败'
      "
    >
      <NFlex align="center" justify="space-between">
        <span>{{
          sessionError instanceof Error ? sessionError.message : String(sessionError)
        }}</span>
        <NButton size="small" :loading="sshSessions.loading" @click="refreshSessions">重试</NButton>
      </NFlex>
    </NAlert>
    <NAlert
      v-else-if="!sshSessions.sessions.length && !sshSessions.loading"
      type="info"
      title="尚无 SSH Session"
    >
      <NFlex align="center" justify="space-between">
        <span>Java Runtime 必须绑定一个已保存的 SSH Session 后才能发现、注册或安装。</span>
        <NButton size="small" type="primary" @click="router.push('/ssh-sessions')">
          创建 SSH Session
        </NButton>
      </NFlex>
    </NAlert>
    <NCard>
      <NFlex :wrap="false">
        <NSelect
          :value="store.sshSessionID"
          :disabled="!sshSessions.sessions.length"
          placeholder="选择 SSH Session"
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
        <NButton :loading="store.loading" @click="refresh">刷新</NButton>
        <NButton
          type="primary"
          :loading="store.discovering"
          :disabled="!store.sshSessionID"
          @click="discover"
        >
          <template #icon><AppIcon :icon="Search" label="发现" /></template>
          发现远程 Java
        </NButton>
      </NFlex>
    </NCard>

    <NCard title="已注册 Runtime">
      <NAlert v-if="partialMessage" type="warning" title="部分连接状态不可用">{{
        partialMessage
      }}</NAlert>
      <AppDataTable
        :columns="runtimeColumns"
        :data="store.runtimes"
        :loading="store.loading"
        :error="store.runtimeError"
        empty-description="当前 SSH Session 尚未注册 Java Runtime"
        @retry="refresh"
      />
    </NCard>

    <NCard v-if="store.candidates.length || store.discoveryError" title="发现候选">
      <AppDataTable
        :columns="candidateColumns"
        :data="store.candidates"
        :error="store.discoveryError"
        @retry="discover"
      />
    </NCard>

    <NCard title="手动注册">
      <NFlex :wrap="false">
        <NInput
          v-model:value="manualPath"
          :disabled="!store.sshSessionID"
          placeholder="远程 Java 可执行文件绝对路径，例如 /usr/bin/java"
        />
        <NButton type="primary" :disabled="!store.sshSessionID" @click="importManual">
          <template #icon><AppIcon :icon="Download" label="注册" /></template>
          验证并注册
        </NButton>
      </NFlex>
    </NCard>

    <NCard title="兼容性推荐">
      <NFlex :wrap="false">
        <NSelect v-model:value="serverType" :options="serverTypeOptions" />
        <NInput v-model:value="minecraftVersion" placeholder="Minecraft 版本" />
        <NButton type="primary" :disabled="!store.sshSessionID" @click="recommend">
          <template #icon><AppIcon :icon="CheckCircle2" label="推荐" /></template>
          推荐 Runtime
        </NButton>
      </NFlex>
    </NCard>

    <NCard title="OpenJDK Catalog">
      <NFlex :wrap="false" class="catalog-toolbar">
        <NSelect
          v-model:value="catalogMajor"
          :options="majorOptions.filter((option) => option.value !== 0 && option.value !== 16)"
        />
        <NSelect v-model:value="catalogArchitecture" :options="architectureOptions" />
        <NButton :loading="catalogLoading" @click="loadArtifacts">查询 Eclipse Temurin</NButton>
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
        title="Java 安装 Operation"
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
            {{ store.installOperation?.message || 'Operation 已创建，正在等待进度事件。' }}
          </NText>
          <NText v-if="store.installOperation" depth="3">
            阶段 {{ store.installOperation.stage || 'pending' }} ·
            {{ Math.round(store.installOperation.progress * 100) }}%
          </NText>
        </NFlex>
      </NAlert>
      <NAlert
        v-else-if="store.installError"
        type="error"
        title="Java 安装启动失败"
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
        empty-description="选择版本和架构后查询已批准的 JDK Artifact"
        @retry="loadArtifacts"
      />
      <NText depth="3"
        >Catalog 仅展示经过批准的 Artifact；下载、摘要校验和远程安装由 Java 安装 Operation
        执行。</NText
      >
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
