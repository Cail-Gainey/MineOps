<script setup lang="ts">
import {
  NAlert,
  NButton,
  NCard,
  NEmpty,
  NFlex,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NSelect,
  NSpin,
  NSwitch,
  NTabPane,
  NTabs,
  NText,
} from 'naive-ui'
import { computed, onMounted, reactive, ref } from 'vue'

import type {
  MinecraftServer,
  RemoteTextDocument,
} from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import type {
  ServerPropertiesBackup,
  ServerPropertiesSnapshot,
  ServerPropertyUpdate,
} from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/service/models'
import { useUnsavedGuard } from '../../composables/use-unsaved-guard'
import { ApplicationError } from '../../services/api-client'
import { readRemoteText, saveRemoteText } from '../../services/file-api'
import {
  listMinecraftServerPropertyBackups,
  readMinecraftServerProperties,
  restoreMinecraftServerPropertyBackup,
  saveMinecraftServerProperties,
} from '../../services/minecraft-server-api'
import { useInteractionStore } from '../../stores/interactions'
import { useLocaleStore } from '../../stores/locale'
import { useNotificationStore } from '../../stores/notifications'
import RemoteTextEditor from '../files/RemoteTextEditor.vue'

interface StructuredPropertiesForm {
  motd: string
  serverPort: number
  maxPlayers: number
  gamemode: string
  difficulty: string
  onlineMode: boolean
  whiteList: boolean
  pvp: boolean
  viewDistance: number
  simulationDistance: number
  levelName: string
}

const props = defineProps<{ server: MinecraftServer; active: boolean }>()
const interactions = useInteractionStore()
const locale = useLocaleStore()
const notifications = useNotificationStore()
const snapshot = ref<ServerPropertiesSnapshot | null>(null)
const genericDocument = ref<RemoteTextDocument | null>(null)
const backups = ref<ServerPropertiesBackup[]>([])
const selectedBackupPath = ref('')
const mode = ref<'structured' | 'raw'>('structured')
const loading = ref(false)
const saving = ref(false)
const restoring = ref(false)
const error = ref<unknown>(null)
const backupError = ref<unknown>(null)
const conflictMessage = ref('')
const rawDirty = ref(false)
const isProxyConfiguration = computed(() =>
  ['velocity', 'waterfall', 'bungeecord'].includes(props.server.type),
)
const configurationFileName = computed(() => {
  if (props.server.type === 'velocity') return 'velocity.toml'
  if (props.server.type === 'waterfall' || props.server.type === 'bungeecord') return 'config.yml'
  return 'server.properties'
})

const form = reactive<StructuredPropertiesForm>({
  motd: 'A Minecraft Server',
  serverPort: 25565,
  maxPlayers: 20,
  gamemode: 'survival',
  difficulty: 'easy',
  onlineMode: true,
  whiteList: false,
  pvp: true,
  viewDistance: 10,
  simulationDistance: 10,
  levelName: 'world',
})
const initialForm = ref<StructuredPropertiesForm>({ ...form })
const structuredDirty = computed(() => JSON.stringify(form) !== JSON.stringify(initialForm.value))
const structuredValid = computed(
  () =>
    Number.isInteger(form.serverPort) &&
    form.serverPort >= 1 &&
    form.serverPort <= 65535 &&
    Number.isInteger(form.maxPlayers) &&
    form.maxPlayers >= 1 &&
    Number.isInteger(form.viewDistance) &&
    form.viewDistance >= 2 &&
    form.viewDistance <= 32 &&
    Number.isInteger(form.simulationDistance) &&
    form.simulationDistance >= 3 &&
    form.simulationDistance <= 32 &&
    form.levelName.trim().length > 0,
)
const document = computed(() =>
  isProxyConfiguration.value ? genericDocument.value : (snapshot.value?.document ?? null),
)
const propertiesPath = computed(
  () => `${props.server.remotePath.replace(/\/$/, '')}/${configurationFileName.value}`,
)
const gamemodeOptions = computed(() =>
  ['survival', 'creative', 'adventure', 'spectator'].map((value) => ({
    label: locale.t(`serverConfig.gamemode.${value}` as Parameters<typeof locale.t>[0]),
    value,
  })),
)
const difficultyOptions = computed(() =>
  ['peaceful', 'easy', 'normal', 'hard'].map((value) => ({
    label: locale.t(`serverConfig.difficulty.${value}` as Parameters<typeof locale.t>[0]),
    value,
  })),
)
const backupOptions = computed(() =>
  backups.value.map((backup) => ({
    label: locale.t('serverConfig.backupOption', {
      time: locale.formatDateTime(backup.modifiedAt),
      size: (backup.size / 1024).toFixed(1),
    }),
    value: backup.path,
  })),
)

useUnsavedGuard(
  computed(() => `server-properties-form:${props.server.id}`),
  computed(() => locale.t('serverConfig.unsavedGuard', { name: props.server.name })),
  structuredDirty,
)

/**
 * 从配置键值表中读取数字，缺失或非法时返回兜底值。
 * @param values - 配置键值表
 * @param key - 配置键
 * @param fallback - 兜底值
 * @returns 解析后的数字
 */
function numberValue(values: Map<string, string>, key: string, fallback: number): number {
  const parsed = Number(values.get(key))
  return Number.isFinite(parsed) ? parsed : fallback
}

/**
 * 从配置键值表中读取布尔值，缺失时返回兜底值。
 * @param values - 配置键值表
 * @param key - 配置键
 * @param fallback - 兜底值
 * @returns 解析后的布尔值
 */
function booleanValue(values: Map<string, string>, key: string, fallback: boolean): boolean {
  const value = values.get(key)
  return value === undefined ? fallback : value.toLowerCase() === 'true'
}

/**
 * 把服务端返回的配置快照填充到结构化表单与原文编辑器。
 * @param value - 配置快照
 * @returns 无返回值
 */
function applySnapshot(value: ServerPropertiesSnapshot): void {
  if (!value.document) return
  snapshot.value = value
  const values = new Map(value.values.map((item) => [item.key, item.value]))
  Object.assign(form, {
    motd: values.get('motd') ?? 'A Minecraft Server',
    serverPort: numberValue(values, 'server-port', value.serverPort || 25565),
    maxPlayers: numberValue(values, 'max-players', 20),
    gamemode: values.get('gamemode') ?? 'survival',
    difficulty: values.get('difficulty') ?? 'easy',
    onlineMode: booleanValue(values, 'online-mode', true),
    whiteList: booleanValue(values, 'white-list', false),
    pvp: booleanValue(values, 'pvp', true),
    viewDistance: numberValue(values, 'view-distance', 10),
    simulationDistance: numberValue(values, 'simulation-distance', 10),
    levelName: values.get('level-name') ?? 'world',
  })
  initialForm.value = { ...form }
  conflictMessage.value = ''
}

/**
 * 加载 server.properties 的历史备份列表。
 * @returns 加载完成后的 Promise
 */
async function loadBackups(): Promise<void> {
  backupError.value = null
  try {
    backups.value = await listMinecraftServerPropertyBackups(props.server.id)
    if (!backups.value.some((backup) => backup.path === selectedBackupPath.value)) {
      selectedBackupPath.value = backups.value[0]?.path ?? ''
    }
  } catch (reason) {
    backupError.value = reason
  }
}

/**
 * 加载 server.properties 当前内容与备份列表。
 * @returns 加载完成后的 Promise
 */
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    if (isProxyConfiguration.value) {
      mode.value = 'raw'
      genericDocument.value = await readRemoteText(
        props.server.sshSessionID,
        propertiesPath.value,
        props.server.remotePath,
      )
      conflictMessage.value = ''
      backups.value = []
      selectedBackupPath.value = ''
    } else {
      applySnapshot(await readMinecraftServerProperties(props.server.id))
      await loadBackups()
    }
  } catch (reason) {
    error.value = reason
  } finally {
    loading.value = false
  }
}

/**
 * 重新加载配置；存在未保存修改时先二次确认。
 * @returns 重载完成后的 Promise
 */
async function requestReload(): Promise<void> {
  if (structuredDirty.value || rawDirty.value) {
    const confirmed = await interactions.confirm({
      title: locale.t('serverConfig.reloadTitle'),
      content: locale.t('serverConfig.reloadContent'),
      objectLabel: propertiesPath.value,
      positiveText: locale.t('serverConfig.reloadConfirm'),
      danger: true,
    })
    if (!confirmed) return
  }
  await load()
}

/**
 * 对比表单与原始快照，收集实际发生变化的配置项。
 * @returns 发生变化的配置项数组
 */
function changedUpdates(): ServerPropertyUpdate[] {
  const candidates: ServerPropertyUpdate[] = [
    { key: 'motd', value: form.motd },
    { key: 'server-port', value: String(form.serverPort) },
    { key: 'max-players', value: String(form.maxPlayers) },
    { key: 'gamemode', value: form.gamemode },
    { key: 'difficulty', value: form.difficulty },
    { key: 'online-mode', value: String(form.onlineMode) },
    { key: 'white-list', value: String(form.whiteList) },
    { key: 'pvp', value: String(form.pvp) },
    { key: 'view-distance', value: String(form.viewDistance) },
    { key: 'simulation-distance', value: String(form.simulationDistance) },
    { key: 'level-name', value: form.levelName },
  ]
  const initial = initialForm.value
  const initialValues = new Map<string, string>([
    ['motd', initial.motd],
    ['server-port', String(initial.serverPort)],
    ['max-players', String(initial.maxPlayers)],
    ['gamemode', initial.gamemode],
    ['difficulty', initial.difficulty],
    ['online-mode', String(initial.onlineMode)],
    ['white-list', String(initial.whiteList)],
    ['pvp', String(initial.pvp)],
    ['view-distance', String(initial.viewDistance)],
    ['simulation-distance', String(initial.simulationDistance)],
    ['level-name', initial.levelName],
  ])
  return candidates.filter((update) => initialValues.get(update.key) !== update.value)
}

/**
 * 执行保存动作；后端要求同步防火墙时先确认再重试。
 * @param action - 接收确认结果并返回新快照的保存动作
 * @returns 保存后的配置快照
 */
async function runWithFirewallConfirmation(
  action: (confirmed: boolean) => Promise<ServerPropertiesSnapshot>,
): Promise<ServerPropertiesSnapshot | null> {
  try {
    return await action(false)
  } catch (reason) {
    if (
      !(reason instanceof ApplicationError) ||
      reason.details.requiresFirewallPortChangeConfirmation !== true
    ) {
      throw reason
    }
    const confirmed = await interactions.confirm({
      title: locale.t('serverConfig.firewallTitle'),
      content: locale.t('serverConfig.firewallContent', {
        port: String(reason.details.newPort ?? ''),
      }),
      objectLabel: `TCP ${reason.details.oldPort ?? ''} → ${reason.details.newPort ?? ''}`,
      impact:
        reason.details.removeOldPort === true
          ? locale.t('serverConfig.firewallRemoveOld')
          : locale.t('serverConfig.firewallKeepOld'),
      positiveText: locale.t('serverConfig.firewallConfirm'),
      danger: reason.details.removeOldPort === true,
    })
    return confirmed ? action(true) : null
  }
}

/**
 * 按结构化表单保存变更的配置项。
 * @returns 保存完成后的 Promise
 */
async function saveStructured(): Promise<void> {
  if (!document.value || !structuredDirty.value || !structuredValid.value || rawDirty.value) return
  saving.value = true
  conflictMessage.value = ''
  try {
    const saved = await runWithFirewallConfirmation((confirmed) =>
      saveMinecraftServerProperties(
        props.server.id,
        'structured',
        '',
        document.value!.versionToken,
        changedUpdates(),
        confirmed,
      ),
    )
    if (!saved) return
    applySnapshot(saved)
    await loadBackups()
    notifySuccess(locale.t('serverConfig.savedStructured'))
  } catch (reason) {
    handleSaveError(reason)
  } finally {
    saving.value = false
  }
}

/**
 * 按原文整体保存 server.properties。
 * @param content - 完整文件内容
 * @param versionToken - 读取时拿到的版本标识
 * @returns 保存完成后的 Promise
 */
async function saveRaw(content: string, versionToken: string): Promise<void> {
  if (structuredDirty.value) return
  saving.value = true
  conflictMessage.value = ''
  try {
    if (isProxyConfiguration.value) {
      genericDocument.value = await saveRemoteText(
        props.server.sshSessionID,
        propertiesPath.value,
        props.server.remotePath,
        content,
        versionToken,
      )
      rawDirty.value = false
      notifySuccess(locale.t('serverConfig.savedProxy'), false)
      return
    }
    const saved = await runWithFirewallConfirmation((confirmed) =>
      saveMinecraftServerProperties(props.server.id, 'raw', content, versionToken, [], confirmed),
    )
    if (!saved) return
    applySnapshot(saved)
    await loadBackups()
    notifySuccess(locale.t('serverConfig.savedRaw'))
  } catch (reason) {
    handleSaveError(reason)
  } finally {
    saving.value = false
  }
}

/**
 * 用选中的备份恢复 server.properties。
 * @returns 恢复完成后的 Promise
 */
async function restoreSelected(): Promise<void> {
  if (!document.value || !selectedBackupPath.value) return
  const backup = backups.value.find((item) => item.path === selectedBackupPath.value)
  if (!backup) return
  const confirmed = await interactions.confirm({
    title: locale.t('serverConfig.restoreTitle'),
    content: locale.t('serverConfig.restoreContent'),
    objectLabel: backup.name,
    impact: propertiesPath.value,
    positiveText: locale.t('serverConfig.restoreConfirm'),
    danger: true,
  })
  if (!confirmed) return
  restoring.value = true
  conflictMessage.value = ''
  try {
    const restored = await runWithFirewallConfirmation((firewallConfirmed) =>
      restoreMinecraftServerPropertyBackup(
        props.server.id,
        backup.path,
        document.value!.versionToken,
        firewallConfirmed,
      ),
    )
    if (!restored) return
    applySnapshot(restored)
    await loadBackups()
    notifySuccess(locale.t('serverConfig.restored'))
  } catch (reason) {
    handleSaveError(reason)
  } finally {
    restoring.value = false
  }
}

/**
 * 区分版本冲突与其他失败，给出对应提示。
 * @param reason - 保存过程抛出的错误
 * @returns 无返回值
 */
function handleSaveError(reason: unknown): void {
  if (reason instanceof ApplicationError && reason.code === 'validation.conflict') {
    conflictMessage.value = reason.message
    return
  }
  notifications.push({
    kind: 'error',
    title: locale.t('serverConfig.saveFailed'),
    content: reason instanceof Error ? reason.message : String(reason),
    dedupeKey: `server-properties:error:${props.server.id}`,
  })
}

/**
 * 推送保存成功通知，并说明是否已保留备份。
 * @param title - 通知标题
 * @param retainedBackup - 是否已保留改动前的备份
 * @returns 无返回值
 */
function notifySuccess(title: string, retainedBackup = true): void {
  notifications.push({
    kind: 'success',
    title,
    content: retainedBackup
      ? locale.t('serverConfig.retainedBackup')
      : locale.t('serverConfig.atomicSaved'),
    dedupeKey: `server-properties:saved:${props.server.id}`,
  })
}

/**
 * 提示另存为不会改动当前 Server 的生效配置。
 * @returns 无返回值
 */
function explainSaveAs(): void {
  notifications.push({
    kind: 'info',
    title: locale.t('serverConfig.saveAsTitle'),
    content: locale.t('serverConfig.saveAsContent', { file: configurationFileName.value }),
    dedupeKey: `server-properties:save-as:${props.server.id}`,
  })
}

onMounted(() => {
  if (props.active) void load()
})
</script>

<template>
  <NSpin :show="loading">
    <NFlex vertical :size="12" class="configuration-panel">
      <NFlex align="center" justify="space-between" wrap>
        <div>
          <NText strong>{{ configurationFileName }}</NText>
          <NText depth="3" class="path-text">{{ propertiesPath }}</NText>
        </div>
        <NFlex>
          <NSelect
            v-if="!isProxyConfiguration"
            v-model:value="selectedBackupPath"
            :options="backupOptions"
            :disabled="!backups.length"
            :placeholder="locale.t('serverConfig.selectBackup')"
            style="width: 320px"
          />
          <NButton
            v-if="!isProxyConfiguration"
            type="warning"
            :disabled="!selectedBackupPath || !document || structuredDirty || rawDirty"
            :loading="restoring"
            @click="restoreSelected"
          >
            {{ locale.t('serverConfig.restoreBackup') }}
          </NButton>
          <NButton :loading="loading" @click="requestReload">
            {{ locale.t('serverConfig.reload') }}
          </NButton>
        </NFlex>
      </NFlex>

      <NAlert v-if="error" type="error" :title="locale.t('serverConfig.loadFailed')">
        <NFlex align="center" justify="space-between">
          <span>{{ error instanceof Error ? error.message : String(error) }}</span>
          <NButton size="small" @click="load">{{ locale.t('common.retry') }}</NButton>
        </NFlex>
      </NAlert>
      <NAlert v-if="backupError" type="warning" :title="locale.t('serverConfig.backupListFailed')">
        {{ backupError instanceof Error ? backupError.message : String(backupError) }}
      </NAlert>
      <NAlert
        v-if="isProxyConfiguration"
        type="info"
        :title="locale.t('serverConfig.proxyModeTitle')"
      >
        {{ locale.t('serverConfig.proxyModeContent', { file: configurationFileName }) }}
      </NAlert>
      <NAlert
        v-if="snapshot?.validationNotice"
        type="warning"
        :title="locale.t('serverConfig.validationTitle')"
      >
        {{ snapshot.validationNotice }}
      </NAlert>
      <NAlert v-if="conflictMessage" type="warning" :title="locale.t('serverConfig.conflictTitle')">
        {{ locale.t('serverConfig.conflictContent', { message: conflictMessage }) }}
      </NAlert>
      <NAlert
        v-if="structuredDirty && rawDirty"
        type="warning"
        :title="locale.t('serverConfig.bothDirtyTitle')"
      >
        {{ locale.t('serverConfig.bothDirtyContent') }}
      </NAlert>
      <NAlert
        v-if="structuredDirty && !structuredValid"
        type="warning"
        :title="locale.t('serverConfig.invalidTitle')"
      >
        {{ locale.t('serverConfig.invalidContent') }}
      </NAlert>

      <NTabs v-if="document" v-model:value="mode" type="segment" animated>
        <NTabPane
          v-if="!isProxyConfiguration"
          name="structured"
          :tab="locale.t('serverConfig.tabStructured')"
          display-directive="show"
        >
          <NCard size="small">
            <NForm label-placement="top">
              <div class="form-grid">
                <NFormItem :label="locale.t('serverConfig.motd')" class="wide-field">
                  <NInput v-model:value="form.motd" maxlength="256" />
                </NFormItem>
                <NFormItem :label="locale.t('serverConfig.port')">
                  <NInputNumber v-model:value="form.serverPort" :min="1" :max="65535" />
                </NFormItem>
                <NFormItem :label="locale.t('serverConfig.maxPlayers')">
                  <NInputNumber v-model:value="form.maxPlayers" :min="1" :max="100000" />
                </NFormItem>
                <NFormItem :label="locale.t('serverConfig.gamemode')">
                  <NSelect v-model:value="form.gamemode" :options="gamemodeOptions" />
                </NFormItem>
                <NFormItem :label="locale.t('serverConfig.difficulty')">
                  <NSelect v-model:value="form.difficulty" :options="difficultyOptions" />
                </NFormItem>
                <NFormItem :label="locale.t('serverConfig.viewDistance')">
                  <NInputNumber v-model:value="form.viewDistance" :min="2" :max="32" />
                </NFormItem>
                <NFormItem :label="locale.t('serverConfig.simulationDistance')">
                  <NInputNumber v-model:value="form.simulationDistance" :min="3" :max="32" />
                </NFormItem>
                <NFormItem :label="locale.t('serverConfig.levelName')">
                  <NInput v-model:value="form.levelName" maxlength="128" />
                </NFormItem>
                <NFormItem :label="locale.t('serverConfig.onlineMode')">
                  <NSwitch v-model:value="form.onlineMode" />
                </NFormItem>
                <NFormItem :label="locale.t('serverConfig.whiteList')">
                  <NSwitch v-model:value="form.whiteList" />
                </NFormItem>
                <NFormItem :label="locale.t('serverConfig.pvp')">
                  <NSwitch v-model:value="form.pvp" />
                </NFormItem>
              </div>
              <NFlex justify="end">
                <NButton
                  type="primary"
                  :disabled="
                    !structuredDirty || !structuredValid || rawDirty || Boolean(conflictMessage)
                  "
                  :loading="saving"
                  @click="saveStructured"
                >
                  {{ locale.t('serverConfig.saveStructured') }}
                </NButton>
              </NFlex>
            </NForm>
          </NCard>
        </NTabPane>
        <NTabPane
          name="raw"
          :tab="
            isProxyConfiguration
              ? locale.t('serverConfig.tabProxy')
              : locale.t('serverConfig.tabRaw')
          "
          display-directive="show"
        >
          <RemoteTextEditor
            :document="document"
            :saving="saving"
            :conflict-message="conflictMessage"
            :save-blocked="structuredDirty"
            :close-blocked="structuredDirty"
            @save="saveRaw"
            @save-as="explainSaveAs"
            @reload="requestReload"
            @close="isProxyConfiguration ? (genericDocument = null) : (snapshot = null)"
            @dirty-change="rawDirty = $event"
          />
        </NTabPane>
      </NTabs>

      <NEmpty v-else-if="!error && !loading" :description="locale.t('serverConfig.notLoaded')">
        <template #extra>
          <NButton type="primary" @click="load">{{ locale.t('serverConfig.loadButton') }}</NButton>
        </template>
      </NEmpty>
    </NFlex>
  </NSpin>
</template>

<style scoped>
.configuration-panel {
  min-height: 560px;
}

.path-text {
  display: block;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(190px, 1fr));
  gap: 0 16px;
}

.wide-field {
  grid-column: 1 / -1;
}
</style>
