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
import AppFormActions from '../../shared/components/AppFormActions.vue'
import AppFormField from '../../shared/components/AppFormField.vue'
import { useUnsavedGuard } from '../../composables/use-unsaved-guard'

const props = defineProps<{
  show: boolean
  session: SSHSessionDTO | null
}>()

const emit = defineEmits<{
  'update:show': [show: boolean]
  saved: [session: SSHSessionDTO]
  failed: [error: unknown]
}>()

const authOptions = [
  { label: '密码', value: 'password' },
  { label: '私钥', value: 'private_key' },
  { label: 'SSH Agent', value: 'agent' },
]
const hostKeyOptions = [
  { label: '严格校验（推荐）', value: 'strict' },
  { label: '首次确认后信任', value: 'trust_on_first_use' },
]

const draft = reactive<SSHSessionInput>(emptyInput())
const initial = reactive<SSHSessionInput>(emptyInput())
const submitting = reactive({ value: false })
const testing = ref(false)
const testResult = ref<SSHConnectionTestDTO | null>(null)
const testError = ref('')
let testRevision = 0
const dirty = computed(() => JSON.stringify(draft) !== JSON.stringify(initial))
const title = computed(() => (props.session ? '编辑 SSH Session' : '新建 SSH Session'))
const exitGuardDirty = computed(() => props.show && dirty.value)

useUnsavedGuard(
  computed(() => `ssh-session-form:${props.session?.id ?? 'new'}`),
  computed(() => `${title.value}有未提交修改`),
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

function close(): void {
  if (submitting.value || testing.value) return
  clearSecrets()
  emit('update:show', false)
}

function discard(): void {
  Object.assign(draft, initial)
}

function clearTestResult(): void {
  testRevision += 1
  testResult.value = null
  testError.value = ''
}

function clearSecrets(): void {
  draft.secret = ''
  draft.passphrase = ''
  initial.secret = ''
  initial.passphrase = ''
}

function authTypeLabel(authType: string): string {
  return authOptions.find((option) => option.value === authType)?.label ?? authType
}

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
      <AppFormField label="名称" required>
        <NInput v-model:value="draft.name" placeholder="例如：生产服宿主机" />
      </AppFormField>
      <AppFormField label="主机" required>
        <NInput v-model:value="draft.host" placeholder="主机名或 IP 地址" />
      </AppFormField>
      <AppFormField label="端口" required>
        <NInputNumber v-model:value="draft.port" :min="1" :max="65535" />
      </AppFormField>
      <AppFormField label="用户名" required>
        <NInput v-model:value="draft.username" />
      </AppFormField>
      <AppFormField label="认证方式" required>
        <NSelect v-model:value="draft.authType" :options="authOptions" />
      </AppFormField>
      <AppFormField
        v-if="draft.authType === 'password'"
        label="密码"
        :required="!session?.hasCredential"
        help="编辑时留空表示保留原凭据。"
      >
        <NInput v-model:value="draft.secret" type="password" show-password-on="click" />
      </AppFormField>
      <template v-if="draft.authType === 'private_key'">
        <AppFormField
          label="私钥"
          :required="!session?.hasCredential"
          help="支持 OpenSSH/PEM；编辑时留空表示保留原凭据。"
        >
          <NInput v-model:value="draft.secret" type="textarea" :rows="5" />
        </AppFormField>
        <AppFormField label="私钥口令">
          <NInput v-model:value="draft.passphrase" type="password" show-password-on="click" />
        </AppFormField>
      </template>
      <AppFormField label="主机密钥策略" required help="不提供忽略全部主机指纹的选项。">
        <NSelect v-model:value="draft.hostKeyPolicy" :options="hostKeyOptions" />
      </AppFormField>
      <AppFormField label="连接级覆盖" help="关闭时继承 Settings 中的 SSH 全局默认值。">
        <NSwitch v-model:value="draft.overrideSettings" />
      </AppFormField>
      <AppFormField label="分组">
        <NInput v-model:value="draft.group" />
      </AppFormField>
      <AppFormField label="收藏">
        <NCheckbox v-model:checked="draft.favourite">置顶显示</NCheckbox>
      </AppFormField>
      <AppFormField v-if="draft.overrideSettings" label="连接超时（秒）">
        <NInputNumber v-model:value="draft.connectTimeoutSec" :min="1" :max="300" />
      </AppFormField>
      <AppFormField v-if="draft.overrideSettings" label="握手超时（秒）">
        <NInputNumber v-model:value="draft.handshakeTimeoutSec" :min="1" :max="300" />
      </AppFormField>
      <AppFormField v-if="draft.overrideSettings" label="KeepAlive（秒）">
        <NInputNumber v-model:value="draft.keepAliveSec" :min="0" :max="3600" />
      </AppFormField>
      <AppFormField v-if="draft.overrideSettings" label="压缩">
        <NSwitch v-model:value="draft.compression" />
      </AppFormField>
      <AppFormField label="备注">
        <NInput v-model:value="draft.remark" type="textarea" :rows="3" />
      </AppFormField>
    </NForm>
    <NAlert v-if="testResult" type="success" title="SSH 连接测试通过" class="test-result">
      SSH 握手、主机密钥、认证和命令通道均成功。远端版本：{{
        testResult.serverVersion
      }}；远端地址：{{ testResult.remoteAddress }}；认证方式：{{
        authTypeLabel(testResult.authType)
      }}；总耗时：{{ testResult.connectDurationMs }}
      ms。
    </NAlert>
    <NAlert v-else-if="testError" type="error" title="SSH 连接测试失败" class="test-result">
      {{ testError }}
    </NAlert>
    <template #footer>
      <NFlex align="center" justify="space-between">
        <NButton :loading="testing" :disabled="submitting.value" @click="testConnection">
          测试连接
        </NButton>
        <AppFormActions
          class="form-actions"
          :dirty="dirty"
          :submitting="submitting.value"
          :disabled="testing"
          submit-text="保存 SSH Session"
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
