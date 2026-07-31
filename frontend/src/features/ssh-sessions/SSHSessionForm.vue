<script setup lang="ts">
import {
  NAlert,
  NButton,
  NCheckbox,
  NFlex,
  NForm,
  NInput,
  NInputNumber,
  NModal,
  NSelect,
  NSwitch,
} from 'naive-ui'
import { computed, reactive, ref, watch } from 'vue'

import type {
  SSHConnectionTestDTO,
  SSHSessionDTO,
  SSHSessionInput,
} from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/models'
import {
  createSSHSession,
  testSSHSessionInput,
  updateSSHSession,
} from '../../services/ssh-session-api'
import { hasMessage } from '../../locales/runtime'
import AppFormActions from '../../shared/components/AppFormActions.vue'
import AppFormField from '../../shared/components/AppFormField.vue'
import { useUnsavedGuard } from '../../composables/use-unsaved-guard'
import { useLocaleStore } from '../../stores/locale'

const props = defineProps<{
  show: boolean
  session: SSHSessionDTO | null
}>()

const emit = defineEmits<{
  'update:show': [show: boolean]
  saved: [session: SSHSessionDTO]
  failed: [error: unknown]
}>()

const locale = useLocaleStore()
const authTypes = ['password', 'private_key', 'agent']
const hostKeyPolicies = ['strict', 'trust_on_first_use']
const authOptions = computed(() =>
  authTypes.map((value) => ({ label: authTypeLabel(value), value })),
)
const hostKeyOptions = computed(() =>
  hostKeyPolicies.map((value) => ({
    label: locale.t(`sshForm.hostKey.${value}` as Parameters<typeof locale.t>[0]),
    value,
  })),
)

const draft = reactive<SSHSessionInput>(emptyInput())
const initial = reactive<SSHSessionInput>(emptyInput())
const submitting = reactive({ value: false })
const testing = ref(false)
const testResult = ref<SSHConnectionTestDTO | null>(null)
const testError = ref('')
let testRevision = 0
const dirty = computed(() => JSON.stringify(draft) !== JSON.stringify(initial))
const title = computed(() =>
  props.session ? locale.t('sshForm.titleEdit') : locale.t('sshForm.titleCreate'),
)
const exitGuardDirty = computed(() => props.show && dirty.value)

useUnsavedGuard(
  computed(() => `ssh-session-form:${props.session?.id ?? 'new'}`),
  computed(() => locale.t('sshForm.unsaved', { title: title.value })),
  exitGuardDirty,
)

watch(
  () => [props.show, props.session] as const,
  ([show, session]) => {
    if (!show) return
    const value = session ? fromSession(session) : emptyInput()
    Object.assign(draft, value)
    Object.assign(initial, value)
    clearTestResult()
  },
  { immediate: true },
)

watch(draft, clearTestResult)

/**
 * 校验并提交 SSH Session 的新建或编辑。
 * @returns 提交完成后的 Promise
 */
async function submit(): Promise<void> {
  if (submitting.value || testing.value) return
  submitting.value = true
  try {
    const saved = props.session
      ? await updateSSHSession(props.session.id, { ...draft })
      : await createSSHSession({ ...draft })
    clearSecrets()
    emit('saved', saved)
    emit('update:show', false)
  } catch (error) {
    emit('failed', error)
  } finally {
    submitting.value = false
  }
}

/**
 * 用表单当前内容测试 SSH 连通性。
 * @returns 测试完成后的 Promise
 */
async function testConnection(): Promise<void> {
  if (testing.value || submitting.value) return
  const revision = testRevision
  testing.value = true
  testResult.value = null
  testError.value = ''
  try {
    const result = await testSSHSessionInput(props.session?.id ?? '', { ...draft })
    if (revision === testRevision) testResult.value = result
  } catch (error) {
    if (revision === testRevision) {
      testError.value = error instanceof Error ? error.message : String(error)
    }
  } finally {
    testing.value = false
  }
}

/**
 * 关闭表单并清除内存中的口令与私钥口令。
 * @returns 无返回值
 */
function close(): void {
  if (submitting.value || testing.value) return
  clearSecrets()
  emit('update:show', false)
}

/**
 * 放弃修改，把表单恢复到打开时的内容。
 * @returns 无返回值
 */
function discard(): void {
  Object.assign(draft, initial)
}

/**
 * 作废上一次连通性测试结果。
 * @returns 无返回值
 */
function clearTestResult(): void {
  testRevision += 1
  testResult.value = null
  testError.value = ''
}

/**
 * 清除表单中缓存的口令与私钥口令。
 * @returns 无返回值
 */
function clearSecrets(): void {
  draft.secret = ''
  draft.passphrase = ''
  initial.secret = ''
  initial.passphrase = ''
}

/**
 * 把认证方式标识映射成本地化标签。
 * @param authType - 认证方式标识
 * @returns 本地化标签，未知方式原样返回
 */
function authTypeLabel(authType: string): string {
  const key = `sshForm.auth.${authType}`
  return hasMessage(key) ? locale.t(key) : authType
}

/**
 * 构造一份空白的 SSH Session 输入。
 * @returns 空白 SSH Session 输入
 */
function emptyInput(): SSHSessionInput {
  return {
    name: '',
    host: '',
    port: 22,
    username: 'root',
    authType: 'private_key',
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
 * 把已有 SSH Session 转换成表单输入，不带密文。
 * @param session - 源 SSH Session
 * @returns 表单输入内容
 */
function fromSession(session: SSHSessionDTO): SSHSessionInput {
  return {
    name: session.name,
    host: session.host,
    port: session.port,
    username: session.username,
    authType: session.authType,
    secret: '',
    passphrase: '',
    hostKeyPolicy: session.hostKeyPolicy,
    group: session.group,
    favourite: session.favourite,
    remark: session.remark,
    connectTimeoutSec: session.connectTimeoutSec,
    handshakeTimeoutSec: session.handshakeTimeoutSec,
    keepAliveSec: session.keepAliveSec,
    compression: session.compression,
    overrideSettings: session.overrideSettings,
  }
}
</script>

<template>
  <NModal
    :show="show"
    preset="card"
    :title="title"
    :mask-closable="false"
    :close-on-esc="!submitting.value && !testing"
    style="width: min(720px, calc(100vw - 32px))"
    @close="close"
    @esc="close"
  >
    <NForm label-placement="left" label-width="150">
      <AppFormField :label="locale.t('sshForm.name')" required>
        <NInput v-model:value="draft.name" :placeholder="locale.t('sshForm.namePlaceholder')" />
      </AppFormField>
      <AppFormField :label="locale.t('sshForm.host')" required>
        <NInput v-model:value="draft.host" :placeholder="locale.t('sshForm.hostPlaceholder')" />
      </AppFormField>
      <AppFormField :label="locale.t('sshForm.port')" required>
        <NInputNumber v-model:value="draft.port" :min="1" :max="65535" />
      </AppFormField>
      <AppFormField :label="locale.t('sshForm.username')" required>
        <NInput v-model:value="draft.username" />
      </AppFormField>
      <AppFormField :label="locale.t('sshForm.authType')" required>
        <NSelect v-model:value="draft.authType" :options="authOptions" />
      </AppFormField>
      <AppFormField
        v-if="draft.authType === 'password'"
        :label="locale.t('sshForm.password')"
        :required="!session?.hasCredential"
        :help="locale.t('sshForm.passwordHelp')"
      >
        <NInput v-model:value="draft.secret" type="password" show-password-on="click" />
      </AppFormField>
      <template v-if="draft.authType === 'private_key'">
        <AppFormField
          :label="locale.t('sshForm.privateKey')"
          :required="!session?.hasCredential"
          :help="locale.t('sshForm.privateKeyHelp')"
        >
          <NInput v-model:value="draft.secret" type="textarea" :rows="5" />
        </AppFormField>
        <AppFormField :label="locale.t('sshForm.passphrase')">
          <NInput v-model:value="draft.passphrase" type="password" show-password-on="click" />
        </AppFormField>
      </template>
      <AppFormField
        :label="locale.t('sshForm.hostKeyPolicy')"
        required
        :help="locale.t('sshForm.hostKeyPolicyHelp')"
      >
        <NSelect v-model:value="draft.hostKeyPolicy" :options="hostKeyOptions" />
      </AppFormField>
      <AppFormField :label="locale.t('sshForm.override')" :help="locale.t('sshForm.overrideHelp')">
        <NSwitch v-model:value="draft.overrideSettings" />
      </AppFormField>
      <AppFormField :label="locale.t('sshForm.group')">
        <NInput v-model:value="draft.group" />
      </AppFormField>
      <AppFormField :label="locale.t('sshForm.favourite')">
        <NCheckbox v-model:checked="draft.favourite">
          {{ locale.t('sshForm.favouritePin') }}
        </NCheckbox>
      </AppFormField>
      <AppFormField v-if="draft.overrideSettings" :label="locale.t('sshForm.connectTimeout')">
        <NInputNumber v-model:value="draft.connectTimeoutSec" :min="1" :max="300" />
      </AppFormField>
      <AppFormField v-if="draft.overrideSettings" :label="locale.t('sshForm.handshakeTimeout')">
        <NInputNumber v-model:value="draft.handshakeTimeoutSec" :min="1" :max="300" />
      </AppFormField>
      <AppFormField v-if="draft.overrideSettings" :label="locale.t('sshForm.keepAlive')">
        <NInputNumber v-model:value="draft.keepAliveSec" :min="0" :max="3600" />
      </AppFormField>
      <AppFormField v-if="draft.overrideSettings" :label="locale.t('sshForm.compression')">
        <NSwitch v-model:value="draft.compression" />
      </AppFormField>
      <AppFormField :label="locale.t('sshForm.remark')">
        <NInput v-model:value="draft.remark" type="textarea" :rows="3" />
      </AppFormField>
    </NForm>
    <NAlert
      v-if="testResult"
      type="success"
      :title="locale.t('sshForm.testSuccessTitle')"
      class="test-result"
    >
      {{
        locale.t('sshForm.testSuccessContent', {
          version: testResult.serverVersion,
          address: testResult.remoteAddress,
          auth: authTypeLabel(testResult.authType),
          duration: testResult.connectDurationMs,
        })
      }}
    </NAlert>
    <NAlert
      v-else-if="testError"
      type="error"
      :title="locale.t('sshForm.testFailedTitle')"
      class="test-result"
    >
      {{ testError }}
    </NAlert>
    <template #footer>
      <NFlex align="center" justify="space-between">
        <NButton :loading="testing" :disabled="submitting.value" @click="testConnection">
          {{ locale.t('sshForm.testConnection') }}
        </NButton>
        <AppFormActions
          class="form-actions"
          :dirty="dirty"
          :submitting="submitting.value"
          :disabled="testing"
          :submit-text="locale.t('sshForm.submit')"
          @discard="discard"
          @submit="submit"
        />
      </NFlex>
    </template>
  </NModal>
</template>

<style scoped>
.test-result {
  margin-top: 16px;
}

.form-actions {
  flex: 1;
  margin-left: 16px;
}
</style>
