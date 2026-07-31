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
import { hasMessage } from '../../locales/runtime'
import { useLocaleStore } from '../../stores/locale'
import { useNotificationStore } from '../../stores/notifications'
import { useServerWizardStore } from '../../stores/server-wizard'
import { useSSHSessionsStore } from '../../stores/ssh-sessions'
import InstallationTerminal from './InstallationTerminal.vue'

const show = defineModel<boolean>('show', { default: false })
const wizard = useServerWizardStore()
const locale = useLocaleStore()
const sshSessions = useSSHSessionsStore()
const notifications = useNotificationStore()
const router = useRouter()
const replacementName = ref('')
const now = ref(Date.now())
const elapsedTimer = window.setInterval(() => (now.value = Date.now()), 1000)
const exitGuardDirty = computed(() => show.value && !wizard.task)
const firewallPolicies = ['automatic', 'prompt', 'disabled']
const firewallPolicyOptions = computed(() =>
  firewallPolicies.map((value) => ({
    label: locale.t(`servers.firewall.${value}` as Parameters<typeof locale.t>[0]),
    value,
  })),
)
const authTypeOptions = computed(() =>
  ['password', 'private_key', 'agent'].map((value) => ({
    label: locale.t(`sshForm.auth.${value}` as Parameters<typeof locale.t>[0]),
    value,
  })),
)

useUnsavedGuard(
  'server-wizard',
  computed(() => locale.t('wizard.unsavedGuard')),
  exitGuardDirty,
)

const distributionOptions = computed(() =>
  wizard.distributions.map((item) => ({
    label: `${item.displayName}${item.proxy ? locale.t('wizard.distributionProxy') : ''}${
      !item.catalogReady || !item.installerReady ? locale.t('wizard.distributionNotReady') : ''
    }`,
    value: item.type,
    disabled: !item.catalogReady || !item.installerReady,
  })),
)
const versionOptions = computed(() =>
  wizard.versions.map((item) => ({
    label: `${item.version}${
      item.build ? locale.t('wizard.versionBuild', { build: item.build }) : ''
    } · Java ${item.javaMajor}`,
    value: item.version,
  })),
)
const directoryConflict = computed(() =>
  wizard.steps.find((item) => item.name === 'create_server_directory' && item.state === 'failed'),
)
const remainingSteps = computed(
  () => wizard.steps.filter((item) => item.state !== 'success' && item.state !== 'skipped').length,
)
// 安装步骤按波次并行执行,同一时刻可能有多个步骤处于 running。
const runningSteps = computed(() => wizard.steps.filter((item) => item.state === 'running'))
const completedStepCount = computed(
  () => wizard.steps.filter((item) => item.state === 'success' || item.state === 'skipped').length,
)
const stepProgressText = computed(() => {
  const summary = locale.t('wizard.stepProgress', {
    done: completedStepCount.value,
    total: wizard.steps.length,
  })
  return runningSteps.value.length > 1
    ? locale.t('wizard.stepParallel', { summary, count: runningSteps.value.length })
    : summary
})
const elapsedText = computed(() => {
  const startedAt = wizard.task?.startedAt
  if (!startedAt) return locale.t('wizard.notStarted')
  const finishedAt = wizard.task?.finishedAt ? Date.parse(wizard.task.finishedAt) : now.value
  const seconds = Math.max(0, Math.floor((finishedAt - Date.parse(startedAt)) / 1000))
  const minutes = Math.floor(seconds / 60)
  return `${minutes}m ${seconds % 60}s`
})
const installationTerminalLines = computed(() => {
  if (!wizard.task) return []
  const lines = [locale.t('wizard.terminalHeader', { id: wizard.task.id })]
  for (const step of wizard.steps) {
    const logs = step.checkpoint?.logs
    if (!Array.isArray(logs)) continue
    for (const line of logs) lines.push(`[${step.order}. ${stepLabel(step.name)}] ${String(line)}`)
  }
  return lines
})
const installationTerminalStatus = computed(() => {
  if (!wizard.task) return locale.t('wizard.terminalWaiting')
  if (runningSteps.value.length > 0) {
    return runningSteps.value
      .map((step) =>
        locale.t('wizard.stepRunning', {
          label: stepLabel(step.name),
          message: step.message || locale.t('wizard.stepExecuting'),
        }),
      )
      .join(' ｜ ')
  }
  const current = wizard.steps.find((step) => step.order === wizard.task?.currentStep)
  if (!current) return locale.t('wizard.taskState', { state: wizard.task.state })
  return locale.t('wizard.stepSummary', {
    label: stepLabel(current.name),
    state: current.state,
    message: current.message || locale.t('wizard.stepWaiting'),
  })
})

/**
 * 加载向导初始数据，失败时推送错误通知。
 * @returns 初始化完成后的 Promise
 */
async function initialize(): Promise<void> {
  try {
    await wizard.initialize()
  } catch (error) {
    notifyError(locale.t('wizard.initFailed'), error)
  }
}

/**
 * 进入向导下一步，校验失败时推送错误通知。
 * @returns 跳转完成后的 Promise
 */
async function next(): Promise<void> {
  try {
    await wizard.next()
  } catch (error) {
    notifyError(locale.t('wizard.nextFailed'), error)
  }
}

/**
 * 测试当前选中的 SSH Session 连通性。
 * @returns 测试完成后的 Promise
 */
async function testSSH(): Promise<void> {
  try {
    await wizard.testSelectedSSH()
    notifications.push({
      kind: 'success',
      title: locale.t('wizard.sshTestSucceeded'),
      content: wizard.connectionEvidence,
      dedupeKey: `wizard:ssh:${wizard.selectedSSHSessionID}`,
    })
  } catch (error) {
    notifyError(locale.t('wizard.sshTestFailed'), error)
  }
}

/**
 * 提交向导，创建 Server 并启动安装。
 * @returns 提交完成后的 Promise
 */
async function submit(): Promise<void> {
  try {
    await wizard.submit()
    notifications.push({
      kind: 'info',
      title: locale.t('wizard.installStarted'),
      content: `Operation ${wizard.operationID}`,
      dedupeKey: `installation:${wizard.operationID}`,
    })
  } catch (error) {
    notifyError(locale.t('wizard.installStartFailed'), error)
  }
}

/**
 * 取消进行中的安装任务。
 * @returns 取消完成后的 Promise
 */
async function cancel(): Promise<void> {
  try {
    await wizard.cancel()
  } catch (error) {
    notifyError(locale.t('wizard.cancelFailed'), error)
  }
}

/**
 * 重试失败的安装任务。
 * @returns 重试完成后的 Promise
 */
async function retry(): Promise<void> {
  try {
    await wizard.retry()
  } catch (error) {
    notifyError(locale.t('wizard.retryFailed'), error)
  }
}

/**
 * 清空安装状态与草稿，回到向导初始状态。
 * @returns 无返回值
 */
function clearInstallationState(): void {
  replacementName.value = ''
  wizard.resetDraft()
}

/**
 * 处理安装目录冲突：备份、改名或取消。
 * @param action - 冲突处理方式
 * @returns 处理完成后的 Promise
 */
async function resolveDirectoryConflict(action: 'backup' | 'rename' | 'cancel'): Promise<void> {
  try {
    await wizard.resolveDirectoryConflict(action, replacementName.value)
  } catch (error) {
    notifyError(locale.t('wizard.conflictFailed'), error)
  }
}

/**
 * 推送一条向导错误通知。
 * @param title - 通知标题
 * @param error - 捕获到的错误
 * @returns 无返回值
 */
function notifyError(title: string, error: unknown): void {
  notifications.push({
    kind: 'error',
    title,
    content: error instanceof Error ? error.message : String(error),
    dedupeKey: `wizard:error:${title}`,
  })
}

/**
 * 把安装步骤标识映射成本地化标签。
 * @param name - 安装步骤标识
 * @returns 本地化标签，未登记的步骤原样返回
 */
function stepLabel(name: string): string {
  const key = `wizard.step.${name}`
  return hasMessage(key) ? locale.t(key) : name
}

/**
 * 把安装步骤状态映射成标签配色。
 * @param state - 安装步骤状态
 * @returns naive-ui 标签的语义类型
 */
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
        .catch((error) => notifyError(locale.t('wizard.loadVersionsFailed'), error))
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
    :title="locale.t('wizard.title')"
    :mask-closable="false"
    style="width: min(920px, calc(100vw - 32px))"
  >
    <template #default>
      <template v-if="wizard.task">
        <NFlex vertical :size="18">
          <NFlex align="center" justify="space-between">
            <div>
              <NText tag="h3">{{ locale.t('wizard.taskTitle', { id: wizard.task.id }) }}</NText>
              <NText depth="3">{{ locale.t('wizard.taskHint') }}</NText>
              <div>
                <NText depth="3">
                  {{
                    locale.t('wizard.taskMeta', {
                      progress: stepProgressText,
                      remaining: remainingSteps,
                      elapsed: elapsedText,
                    })
                  }}
                </NText>
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
          <InstallationTerminal
            :task-id="wizard.task.id"
            :lines="installationTerminalLines"
            :status="installationTerminalStatus"
          />
          <div v-for="item in wizard.steps" :key="item.id" class="installation-step">
            <NFlex align="center" justify="space-between">
              <div>
                <NText strong>{{ item.order }}. {{ stepLabel(item.name) }}</NText>
                <div>
                  <NText depth="3">{{ item.message || locale.t('wizard.stepWaiting') }}</NText>
                </div>
              </div>
              <NFlex align="center">
                <NText depth="3">{{ locale.t('wizard.attempt', { count: item.attempt }) }}</NText>
                <NTag :type="stepTagType(item.state)" :bordered="false">{{ item.state }}</NTag>
              </NFlex>
            </NFlex>
            <NProgress :percentage="Math.round(item.progress * 100)" :show-indicator="false" />
            <NAlert v-if="item.errorCode" type="error" :title="item.errorCode">{{
              item.message
            }}</NAlert>
            <pre v-if="item.errorDetails" class="installation-error-details">{{
              JSON.stringify(item.errorDetails, null, 2)
            }}</pre>
          </div>
          <NAlert v-if="directoryConflict" type="warning" :title="locale.t('wizard.conflictTitle')">
            <NFlex vertical>
              <NText>{{ locale.t('wizard.conflictContent') }}</NText>
              <NFlex wrap>
                <NButton type="warning" @click="resolveDirectoryConflict('backup')">
                  {{ locale.t('wizard.conflictBackup') }}
                </NButton>
                <NInput
                  v-model:value="replacementName"
                  :placeholder="locale.t('wizard.conflictNamePlaceholder')"
                />
                <NButton
                  :disabled="!replacementName.trim()"
                  @click="resolveDirectoryConflict('rename')"
                >
                  {{ locale.t('wizard.conflictRename') }}
                </NButton>
                <NButton type="error" @click="resolveDirectoryConflict('cancel')">
                  {{ locale.t('wizard.conflictCancel') }}
                </NButton>
              </NFlex>
            </NFlex>
          </NAlert>
        </NFlex>
      </template>

      <template v-else>
        <NSteps :current="wizard.currentStep" size="small">
          <NStep
            :title="locale.t('wizard.step1Title')"
            :description="locale.t('wizard.step1Desc')"
          />
          <NStep
            :title="locale.t('wizard.step2Title')"
            :description="locale.t('wizard.step2Desc')"
          />
          <NStep
            :title="locale.t('wizard.step3Title')"
            :description="locale.t('wizard.step3Desc')"
          />
        </NSteps>

        <NForm
          v-if="wizard.currentStep === 1"
          class="wizard-form"
          label-placement="left"
          label-width="170"
        >
          <AppFormField :label="locale.t('wizard.sshSource')">
            <NRadioGroup v-model:value="wizard.sshMode">
              <NRadioButton value="existing">{{ locale.t('wizard.sshExisting') }}</NRadioButton>
              <NRadioButton value="new">{{ locale.t('wizard.sshNew') }}</NRadioButton>
            </NRadioGroup>
          </AppFormField>
          <NAlert
            v-if="!sshSessions.sessions.length"
            type="info"
            :title="locale.t('wizard.noSessionTitle')"
          >
            {{ locale.t('wizard.noSessionContent') }}
          </NAlert>
          <template v-if="wizard.sshMode === 'existing'">
            <AppFormField :label="locale.t('wizard.sshSession')" required>
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
            <AppFormField :label="locale.t('wizard.connectionTest')">
              <NFlex align="center">
                <NButton @click="testSSH">{{ locale.t('wizard.testSSH') }}</NButton>
                <NText v-if="wizard.connectionEvidence" depth="3">{{
                  wizard.connectionEvidence
                }}</NText>
              </NFlex>
            </AppFormField>
          </template>
          <template v-else>
            <AppFormField :label="locale.t('wizard.sessionName')" required>
              <NInput
                v-model:value="wizard.newSSH.name"
                :placeholder="locale.t('sshForm.namePlaceholder')"
              />
            </AppFormField>
            <AppFormField :label="locale.t('sshForm.host')" required>
              <NInput
                v-model:value="wizard.newSSH.host"
                :placeholder="locale.t('sshForm.hostPlaceholder')"
              />
            </AppFormField>
            <AppFormField :label="locale.t('sshForm.port')" required>
              <NInputNumber v-model:value="wizard.newSSH.port" :min="1" :max="65535" />
            </AppFormField>
            <AppFormField :label="locale.t('sshForm.username')" required>
              <NInput
                v-model:value="wizard.newSSH.username"
                :placeholder="locale.t('wizard.usernamePlaceholder')"
              />
            </AppFormField>
            <AppFormField :label="locale.t('sshForm.authType')" required>
              <NSelect v-model:value="wizard.newSSH.authType" :options="authTypeOptions" />
            </AppFormField>
            <AppFormField
              v-if="wizard.newSSH.authType !== 'agent'"
              :label="
                wizard.newSSH.authType === 'private_key'
                  ? locale.t('sshForm.privateKey')
                  : locale.t('sshForm.password')
              "
              required
            >
              <NInput
                v-model:value="wizard.newSSH.secret"
                :type="wizard.newSSH.authType === 'private_key' ? 'textarea' : 'password'"
                :placeholder="
                  wizard.newSSH.authType === 'private_key'
                    ? locale.t('wizard.secretPlaceholderKey')
                    : locale.t('wizard.secretPlaceholderPassword')
                "
                v-bind="
                  wizard.newSSH.authType === 'private_key'
                    ? { autosize: { minRows: 5, maxRows: 10 } }
                    : {}
                "
              />
            </AppFormField>
            <AppFormField
              v-if="wizard.newSSH.authType === 'private_key'"
              :label="locale.t('sshForm.passphrase')"
            >
              <NInput
                v-model:value="wizard.newSSH.passphrase"
                type="password"
                :placeholder="locale.t('wizard.passphrasePlaceholder')"
              />
            </AppFormField>
          </template>
        </NForm>

        <NForm
          v-else-if="wizard.currentStep === 2"
          class="wizard-form"
          label-placement="left"
          label-width="170"
        >
          <AppFormField :label="locale.t('wizard.serverName')" required>
            <NInput v-model:value="wizard.server.name" />
          </AppFormField>
          <AppFormField :label="locale.t('wizard.serverType')" required>
            <NSelect v-model:value="wizard.server.type" :options="distributionOptions" />
          </AppFormField>
          <AppFormField :label="locale.t('wizard.mcVersion')" required>
            <NSelect
              v-model:value="wizard.server.version"
              filterable
              :loading="wizard.versionsLoading"
              :disabled="!wizard.server.type || wizard.versionsLoading"
              :placeholder="
                wizard.versionsLoading
                  ? locale.t('wizard.versionsLoading')
                  : locale.t('wizard.selectVersion')
              "
              :options="versionOptions"
            />
          </AppFormField>
          <AppFormField
            :label="locale.t('servers.form.remoteDir')"
            :help="locale.t('wizard.remoteDirHelp')"
          >
            <NInput v-model:value="wizard.server.remotePath" />
          </AppFormField>
          <AppFormField :label="locale.t('wizard.memory')">
            <NFlex
              ><NInputNumber
                v-model:value="wizard.server.launchProfile.xmsMiB"
                :min="64" /><NInputNumber
                v-model:value="wizard.server.launchProfile.xmxMiB"
                :min="64"
            /></NFlex>
          </AppFormField>
          <AppFormField :label="locale.t('servers.form.jvmArgs')">
            <NDynamicTags v-model:value="wizard.server.launchProfile.jvmArguments" />
          </AppFormField>
          <AppFormField :label="locale.t('servers.form.serverArgs')">
            <NDynamicTags v-model:value="wizard.server.launchProfile.serverArguments" />
          </AppFormField>
          <AppFormField :label="locale.t('servers.form.firewall')">
            <NSelect
              v-model:value="wizard.server.firewallPolicy"
              :options="firewallPolicyOptions"
            />
          </AppFormField>
          <AppFormField :label="locale.t('servers.form.group')">
            <NInput v-model:value="wizard.server.group" />
          </AppFormField>
          <AppFormField :label="locale.t('servers.form.tags')">
            <NDynamicTags v-model:value="wizard.server.tags" />
          </AppFormField>
        </NForm>

        <NFlex v-else class="wizard-form" vertical :size="16">
          <NDescriptions bordered :columns="2" label-placement="left">
            <NDescriptionsItem label="SSH"
              >{{ wizard.selectedSession?.name }} · {{ wizard.selectedSession?.username }}@{{
                wizard.selectedSession?.host
              }}</NDescriptionsItem
            >
            <NDescriptionsItem :label="locale.t('wizard.summaryServer')">
              {{ wizard.server.type }} {{ wizard.server.version }}
            </NDescriptionsItem>
            <NDescriptionsItem :label="locale.t('wizard.summaryJava')">
              Java {{ wizard.selectedVersion?.javaMajor || locale.t('wizard.javaAuto') }}
            </NDescriptionsItem>
            <NDescriptionsItem :label="locale.t('wizard.summaryMemory')">
              {{
                locale.t('wizard.summaryMemoryValue', {
                  xms: wizard.server.launchProfile.xmsMiB,
                  xmx: wizard.server.launchProfile.xmxMiB,
                })
              }}
            </NDescriptionsItem>
            <NDescriptionsItem :label="locale.t('wizard.summaryDirectory')">{{
              wizard.server.remotePath || locale.t('wizard.defaultPathHint')
            }}</NDescriptionsItem>
            <NDescriptionsItem :label="locale.t('wizard.summaryGroupTags')">
              {{ wizard.server.group || locale.t('wizard.ungrouped') }} ·
              {{ wizard.server.tags.join(', ') || locale.t('wizard.noTags') }}
            </NDescriptionsItem>
          </NDescriptions>
          <NAlert type="warning" :title="locale.t('wizard.eulaTitle')">
            {{ locale.t('wizard.eulaContentBefore') }}
            <code>eula=true</code>{{ locale.t('wizard.eulaContentAfter') }}
          </NAlert>
          <NCheckbox v-model:checked="wizard.server.eulaAccepted">
            {{ locale.t('wizard.eulaCheckbox') }}
          </NCheckbox>
        </NFlex>
      </template>
    </template>

    <template #footer>
      <NFlex v-if="wizard.task" justify="space-between">
        <NButton @click="show = false">{{ locale.t('wizard.closeWindow') }}</NButton>
        <NFlex>
          <NButton v-if="wizard.running" type="warning" @click="cancel">
            {{ locale.t('wizard.cancelTask') }}
          </NButton>
          <NButton
            v-if="wizard.task.state === 'failed' || wizard.task.state === 'cancelled'"
            @click="clearInstallationState"
          >
            {{ locale.t('wizard.clearState') }}
          </NButton>
          <NButton
            v-if="wizard.task.state === 'failed' || wizard.task.state === 'cancelled'"
            type="primary"
            @click="retry"
          >
            {{ locale.t('wizard.retryFromFailure') }}
          </NButton>
        </NFlex>
      </NFlex>
      <AppFormActions
        v-else
        :dirty="true"
        :submitting="wizard.loading"
        :show-discard="wizard.currentStep > 1"
        :submit-text="
          wizard.currentStep === 3 ? locale.t('wizard.submitInstall') : locale.t('wizard.nextStep')
        "
        :discard-text="locale.t('wizard.previousStep')"
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

.installation-error-details {
  max-height: 180px;
  margin: 10px 0 0;
  padding: 10px;
  overflow: auto;
  border-radius: 6px;
  background: rgba(15, 23, 42, 0.8);
  color: #fecaca;
  font:
    12px/1.5 ui-monospace,
    SFMono-Regular,
    Menlo,
    monospace;
  white-space: pre-wrap;
}
</style>
