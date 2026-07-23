<script setup lang="ts">
import {
  NAlert,
  NButton,
  NCheckbox,
  NDescriptions,
  NDescriptionsItem,
  NDynamicTags,
  NFlex,
  NForm,
  NInput,
  NInputNumber,
  NModal,
  NProgress,
  NRadioButton,
  NRadioGroup,
  NSelect,
  NStep,
  NSteps,
  NTag,
  NText,
} from 'naive-ui'
import { computed, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'

import AppFormActions from '../../shared/components/AppFormActions.vue'
import AppFormField from '../../shared/components/AppFormField.vue'
import { useUnsavedGuard } from '../../composables/use-unsaved-guard'
import { useNotificationStore } from '../../stores/notifications'
import { useServerWizardStore } from '../../stores/server-wizard'
import { useSSHSessionsStore } from '../../stores/ssh-sessions'

const show = defineModel<boolean>('show', { default: false })
const wizard = useServerWizardStore()
const sshSessions = useSSHSessionsStore()
const notifications = useNotificationStore()
const router = useRouter()
const replacementName = ref('')
const now = ref(Date.now())
const elapsedTimer = window.setInterval(() => (now.value = Date.now()), 1000)
const exitGuardDirty = computed(() => show.value && !wizard.task)
const firewallPolicyOptions = [
  { label: '自动管理', value: 'automatic' },
  { label: '操作前确认', value: 'prompt' },
  { label: '不管理', value: 'disabled' },
]

useUnsavedGuard('server-wizard', 'Minecraft Server Wizard 有未提交配置', exitGuardDirty)

const distributionOptions = computed(() =>
  wizard.distributions.map((item) => ({
    label: `${item.displayName}${item.proxy ? ' · Proxy' : ''}${!item.catalogReady || !item.installerReady ? ' · 尚未开放安装' : ''}`,
    value: item.type,
    disabled: !item.catalogReady || !item.installerReady,
  })),
)
const versionOptions = computed(() =>
  wizard.versions.map((item) => ({
    label: `${item.version}${item.build ? ` · build ${item.build}` : ''} · Java ${item.javaMajor}`,
    value: item.version,
  })),
)
const directoryConflict = computed(() =>
  wizard.steps.find((item) => item.name === 'create_server_directory' && item.state === 'failed'),
)
const remainingSteps = computed(
  () => wizard.steps.filter((item) => item.state !== 'success' && item.state !== 'skipped').length,
)
const elapsedText = computed(() => {
  const startedAt = wizard.task?.startedAt
  if (!startedAt) return '尚未开始'
  const finishedAt = wizard.task?.finishedAt ? Date.parse(wizard.task.finishedAt) : now.value
  const seconds = Math.max(0, Math.floor((finishedAt - Date.parse(startedAt)) / 1000))
  const minutes = Math.floor(seconds / 60)
  return `${minutes}m ${seconds % 60}s`
})

async function initialize(): Promise<void> {
  try {
    await wizard.initialize()
  } catch (error) {
    notifyError('初始化 Server Wizard 失败', error)
  }
}

async function next(): Promise<void> {
  try {
    await wizard.next()
  } catch (error) {
    notifyError('无法进入下一步', error)
  }
}

async function testSSH(): Promise<void> {
  try {
    await wizard.testSelectedSSH()
    notifications.push({
      kind: 'success',
      title: 'SSH 连接测试成功',
      content: wizard.connectionEvidence,
      dedupeKey: `wizard:ssh:${wizard.selectedSSHSessionID}`,
    })
  } catch (error) {
    notifyError('SSH 连接测试失败', error)
  }
}

async function submit(): Promise<void> {
  try {
    await wizard.submit()
    notifications.push({
      kind: 'info',
      title: '安装任务已启动',
      content: `Operation ${wizard.operationID}`,
      dedupeKey: `installation:${wizard.operationID}`,
    })
  } catch (error) {
    notifyError('启动安装失败', error)
  }
}

async function cancel(): Promise<void> {
  try {
    await wizard.cancel()
  } catch (error) {
    notifyError('取消安装失败', error)
  }
}

async function retry(): Promise<void> {
  try {
    await wizard.retry()
  } catch (error) {
    notifyError('重试安装失败', error)
  }
}

function clearInstallationState(): void {
  replacementName.value = ''
  wizard.resetDraft()
}

async function resolveDirectoryConflict(action: 'backup' | 'rename' | 'cancel'): Promise<void> {
  try {
    await wizard.resolveDirectoryConflict(action, replacementName.value)
  } catch (error) {
    notifyError('处理 Server 目录冲突失败', error)
  }
}

function notifyError(title: string, error: unknown): void {
  notifications.push({
    kind: 'error',
    title,
    content: error instanceof Error ? error.message : String(error),
    dedupeKey: `wizard:error:${title}`,
  })
}

function stepLabel(name: string): string {
  return (
    {
      connect_ssh: '建立 SSH 连接',
      initialize_directories: '初始化 MineOps 目录',
      create_server_directory: '创建服务器目录',
      resolve_java: '检测 Java Runtime',
      install_java: '安装 OpenJDK',
      download_server: '下载服务端',
      install_server: '安装服务端',
      write_eula: '写入 EULA',
      configure_firewall: '放行防火墙端口',
      first_start: '首次启动与正常停止',
      register_server: '注册 Server',
    }[name] ?? name
  )
}

function stepTagType(state: string): 'default' | 'info' | 'success' | 'warning' | 'error' {
  if (state === 'success' || state === 'skipped') return 'success'
  if (state === 'running') return 'info'
  if (state === 'failed') return 'error'
  if (state === 'cancelled') return 'warning'
  return 'default'
}

watch(show, (visible) => {
  if (visible) void initialize()
})

watch(
  () => wizard.server.type,
  (distribution, previous) => {
    if (distribution && distribution !== previous)
      void wizard
        .loadVersions(distribution)
        .catch((error) => notifyError('加载版本目录失败', error))
  },
)

onUnmounted(() => window.clearInterval(elapsedTimer))

watch(
  () => wizard.server.name,
  () => {
    if (!wizard.server.remotePath || wizard.server.remotePath.includes('/MineOps/Servers/'))
      wizard.server.remotePath = ''
  },
)

watch(
  () => wizard.task?.state,
  (state) => {
    if (state === 'succeeded' && wizard.task?.serverID) {
      show.value = false
      void router.push({ name: 'server-detail', params: { serverID: wizard.task.serverID } })
    }
  },
)
</script>

<template>
  <NModal
    v-model:show="show"
    preset="card"
    title="创建并安装 Minecraft Server"
    :mask-closable="false"
    style="width: min(920px, calc(100vw - 32px))"
  >
    <template #default>
      <template v-if="wizard.task">
        <NFlex vertical :size="18">
          <NFlex align="center" justify="space-between">
            <div>
              <NText tag="h3">安装任务 {{ wizard.task.id }}</NText>
              <NText depth="3">关闭此窗口不会取消后台任务，重新打开后会从 SQLite 恢复状态。</NText>
              <div>
                <NText depth="3"
                  >当前步骤 {{ wizard.task.currentStep }}/{{ wizard.steps.length }} · 剩余
                  {{ remainingSteps }} · 已用时 {{ elapsedText }}</NText
                >
              </div>
            </div>
            <NTag
              :type="
                wizard.task.state === 'succeeded'
                  ? 'success'
                  : wizard.task.state === 'failed'
                    ? 'error'
                    : 'info'
              "
            >
              {{ wizard.task.state }}
            </NTag>
          </NFlex>
          <NProgress
            type="line"
            :percentage="Math.round(wizard.overallProgress * 100)"
            :status="
              wizard.task.state === 'failed'
                ? 'error'
                : wizard.task.state === 'succeeded'
                  ? 'success'
                  : 'default'
            "
          />
          <div v-for="item in wizard.steps" :key="item.id" class="installation-step">
            <NFlex align="center" justify="space-between">
              <div>
                <NText strong>{{ item.order }}. {{ stepLabel(item.name) }}</NText>
                <div>
                  <NText depth="3">{{ item.message || '等待执行' }}</NText>
                </div>
              </div>
              <NFlex align="center">
                <NText depth="3">尝试 {{ item.attempt }}</NText>
                <NTag :type="stepTagType(item.state)" :bordered="false">{{ item.state }}</NTag>
              </NFlex>
            </NFlex>
            <NProgress :percentage="Math.round(item.progress * 100)" :show-indicator="false" />
            <NAlert v-if="item.errorCode" type="error" :title="item.errorCode">{{
              item.message
            }}</NAlert>
            <pre
              v-if="Array.isArray(item.checkpoint?.logs) && item.checkpoint.logs.length"
              class="installation-log"
              >{{ item.checkpoint.logs.join('\n') }}</pre>
            <pre v-if="item.errorDetails" class="installation-log error-details">{{
              JSON.stringify(item.errorDetails, null, 2)
            }}</pre>
          </div>
          <NAlert v-if="directoryConflict" type="warning" title="Server 目录已存在">
            <NFlex vertical>
              <NText
                >MineOps
                不会静默删除现有目录。请选择备份原目录后继续、输入新名称，或取消安装。</NText
              >
              <NFlex wrap>
                <NButton type="warning" @click="resolveDirectoryConflict('backup')"
                  >备份原目录后继续</NButton
                >
                <NInput v-model:value="replacementName" placeholder="新的 Server 名称" />
                <NButton
                  :disabled="!replacementName.trim()"
                  @click="resolveDirectoryConflict('rename')"
                  >更换名称并重试</NButton
                >
                <NButton type="error" @click="resolveDirectoryConflict('cancel')">取消安装</NButton>
              </NFlex>
            </NFlex>
          </NAlert>
        </NFlex>
      </template>

      <template v-else>
        <NSteps :current="wizard.currentStep" size="small">
          <NStep title="选择 SSH" description="已有会话或新建并测试" />
          <NStep title="服务器配置" description="动态类型、版本和启动参数" />
          <NStep title="确认安装" description="摘要、EULA 与十一阶段任务" />
        </NSteps>

        <NForm
          v-if="wizard.currentStep === 1"
          class="wizard-form"
          label-placement="left"
          label-width="170"
        >
          <AppFormField label="SSH 来源">
            <NRadioGroup v-model:value="wizard.sshMode">
              <NRadioButton value="existing">选择已有会话</NRadioButton>
              <NRadioButton value="new">新建会话</NRadioButton>
            </NRadioGroup>
          </AppFormField>
          <NAlert v-if="!sshSessions.sessions.length" type="info" title="尚未配置 SSH 会话">
            可直接在此填写 SSH 信息；点击下一步后会先保存并测试连接，再继续创建服务器。
          </NAlert>
          <template v-if="wizard.sshMode === 'existing'">
            <AppFormField label="SSH 会话" required>
              <NSelect
                v-model:value="wizard.selectedSSHSessionID"
                filterable
                :options="
                  sshSessions.sessions.map((session) => ({
                    label: `${session.name} · ${session.username}@${session.host}:${session.port}`,
                    value: session.id,
                  }))
                "
              />
            </AppFormField>
            <AppFormField label="连接测试">
              <NFlex align="center">
                <NButton @click="testSSH">测试 SSH</NButton>
                <NText v-if="wizard.connectionEvidence" depth="3">{{
                  wizard.connectionEvidence
                }}</NText>
              </NFlex>
            </AppFormField>
          </template>
          <template v-else>
            <AppFormField label="会话名称" required
              ><NInput v-model:value="wizard.newSSH.name" placeholder="例如：生产服宿主机"
            /></AppFormField>
            <AppFormField label="主机" required
              ><NInput v-model:value="wizard.newSSH.host" placeholder="主机名或 IP 地址"
            /></AppFormField>
            <AppFormField label="端口" required
              ><NInputNumber v-model:value="wizard.newSSH.port" :min="1" :max="65535"
            /></AppFormField>
            <AppFormField label="用户名" required
              ><NInput v-model:value="wizard.newSSH.username" placeholder="请输入用户名"
            /></AppFormField>
            <AppFormField label="认证方式" required>
              <NSelect
                v-model:value="wizard.newSSH.authType"
                :options="[
                  { label: '密码', value: 'password' },
                  { label: '私钥', value: 'private_key' },
                  { label: 'SSH Agent', value: 'agent' },
                ]"
              />
            </AppFormField>
            <AppFormField
              v-if="wizard.newSSH.authType !== 'agent'"
              :label="wizard.newSSH.authType === 'private_key' ? '私钥' : '密码'"
              required
            >
              <NInput
                v-model:value="wizard.newSSH.secret"
                :type="wizard.newSSH.authType === 'private_key' ? 'textarea' : 'password'"
                :placeholder="
                  wizard.newSSH.authType === 'private_key' ? '请粘贴私钥内容' : '请输入密码'
                "
                v-bind="
                  wizard.newSSH.authType === 'private_key'
                    ? { autosize: { minRows: 5, maxRows: 10 } }
                    : {}
                "
              />
            </AppFormField>
            <AppFormField v-if="wizard.newSSH.authType === 'private_key'" label="私钥口令"
              ><NInput
                v-model:value="wizard.newSSH.passphrase"
                type="password"
                placeholder="请输入私钥口令"
            /></AppFormField>
          </template>
        </NForm>

        <NForm
          v-else-if="wizard.currentStep === 2"
          class="wizard-form"
          label-placement="left"
          label-width="170"
        >
          <AppFormField label="Server 名称" required
            ><NInput v-model:value="wizard.server.name"
          /></AppFormField>
          <AppFormField label="服务端类型" required
            ><NSelect v-model:value="wizard.server.type" :options="distributionOptions"
          /></AppFormField>
          <AppFormField label="Minecraft 版本" required
            ><NSelect
              v-model:value="wizard.server.version"
              filterable
              :loading="wizard.versionsLoading"
              :disabled="!wizard.server.type || wizard.versionsLoading"
              :placeholder="wizard.versionsLoading ? '正在加载版本…' : '选择 Minecraft 版本'"
              :options="versionOptions"
          /></AppFormField>
          <AppFormField
            label="远程目录"
            help="留空时根据 SSH 用户生成 /home/{user}/MineOps/Servers/{safe-name}。"
            ><NInput v-model:value="wizard.server.remotePath"
          /></AppFormField>
          <AppFormField label="Xms / Xmx MiB">
            <NFlex
              ><NInputNumber
                v-model:value="wizard.server.launchProfile.xmsMiB"
                :min="64" /><NInputNumber
                v-model:value="wizard.server.launchProfile.xmxMiB"
                :min="64"
            /></NFlex>
          </AppFormField>
          <AppFormField label="JVM 参数"
            ><NDynamicTags v-model:value="wizard.server.launchProfile.jvmArguments"
          /></AppFormField>
          <AppFormField label="Server 参数"
            ><NDynamicTags v-model:value="wizard.server.launchProfile.serverArguments"
          /></AppFormField>
          <AppFormField label="防火墙策略">
            <NSelect
              v-model:value="wizard.server.firewallPolicy"
              :options="firewallPolicyOptions"
            />
          </AppFormField>
          <AppFormField label="分组"><NInput v-model:value="wizard.server.group" /></AppFormField>
          <AppFormField label="标签"
            ><NDynamicTags v-model:value="wizard.server.tags"
          /></AppFormField>
        </NForm>

        <NFlex v-else class="wizard-form" vertical :size="16">
          <NDescriptions bordered :columns="2" label-placement="left">
            <NDescriptionsItem label="SSH"
              >{{ wizard.selectedSession?.name }} · {{ wizard.selectedSession?.username }}@{{
                wizard.selectedSession?.host
              }}</NDescriptionsItem
            >
            <NDescriptionsItem label="服务端"
              >{{ wizard.server.type }} {{ wizard.server.version }}</NDescriptionsItem
            >
            <NDescriptionsItem label="Java 要求"
              >Java {{ wizard.selectedVersion?.javaMajor || '自动解析' }}</NDescriptionsItem
            >
            <NDescriptionsItem label="内存"
              >Xms {{ wizard.server.launchProfile.xmsMiB }} MiB / Xmx
              {{ wizard.server.launchProfile.xmxMiB }} MiB</NDescriptionsItem
            >
            <NDescriptionsItem label="目录">{{
              wizard.server.remotePath || '提交时生成默认路径'
            }}</NDescriptionsItem>
            <NDescriptionsItem label="分组 / 标签"
              >{{ wizard.server.group || '未分组' }} ·
              {{ wizard.server.tags.join(', ') || '无标签' }}</NDescriptionsItem
            >
          </NDescriptions>
          <NAlert type="warning" title="Minecraft EULA 提示">
            提交安装即确认 MineOps 将在远程服务器目录幂等写入
            <code>eula=true</code>。安装会创建持久化 Operation 和十一阶段
            InstallationTask，关闭窗口不会取消任务。
          </NAlert>
          <NCheckbox v-model:checked="wizard.server.eulaAccepted"
            >我理解并确认自动写入 eula=true</NCheckbox
          >
        </NFlex>
      </template>
    </template>

    <template #footer>
      <NFlex v-if="wizard.task" justify="space-between">
        <NButton @click="show = false">关闭窗口</NButton>
        <NFlex>
          <NButton v-if="wizard.running" type="warning" @click="cancel">取消任务</NButton>
          <NButton
            v-if="wizard.task.state === 'failed' || wizard.task.state === 'cancelled'"
            @click="clearInstallationState"
            >清除安装状态</NButton
          >
          <NButton
            v-if="wizard.task.state === 'failed' || wizard.task.state === 'cancelled'"
            type="primary"
            @click="retry"
            >从失败步骤重试</NButton
          >
        </NFlex>
      </NFlex>
      <AppFormActions
        v-else
        :dirty="true"
        :submitting="wizard.loading"
        :show-discard="wizard.currentStep > 1"
        :submit-text="wizard.currentStep === 3 ? '创建并开始安装' : '下一步'"
        discard-text="上一步"
        @discard="wizard.currentStep--"
        @submit="wizard.currentStep === 3 ? submit() : next()"
      />
    </template>
  </NModal>
</template>

<style scoped>
.wizard-form {
  margin-top: 24px;
}

.installation-step {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px;
  border: 1px solid var(--border-default);
  border-radius: 8px;
}

.installation-log {
  max-height: 180px;
  margin: 10px 0 0;
  padding: 10px;
  overflow: auto;
  border-radius: 6px;
  background: rgba(15, 23, 42, 0.8);
  color: #d1fae5;
  font:
    12px/1.5 ui-monospace,
    SFMono-Regular,
    Menlo,
    monospace;
  white-space: pre-wrap;
}

.error-details {
  color: #fecaca;
}
</style>
