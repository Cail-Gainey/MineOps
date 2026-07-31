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
const backupOptions = computed(() =>
  backups.value.map((backup) => ({
    label: `${locale.formatDateTime(backup.modifiedAt)} · ${(backup.size / 1024).toFixed(1)} KiB`,
    value: backup.path,
  })),
)

useUnsavedGuard(
  computed(() => `server-properties-form:${props.server.id}`),
  computed(() => `Server ${props.server.name} 的结构化配置有未保存修改`),
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
      title: '重新读取远端配置？',
      content: '表单模式或原文模式存在未保存修改，重新读取后将丢弃这些修改。',
      objectLabel: propertiesPath.value,
      positiveText: '放弃修改并重新读取',
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
      title: '同步防火墙端口并保存配置？',
      content: `将先放行 TCP ${reason.details.newPort ?? ''}，再原子保存配置。`,
      objectLabel: `TCP ${reason.details.oldPort ?? ''} → ${reason.details.newPort ?? ''}`,
      impact:
        reason.details.removeOldPort === true
          ? '保存后仅在规则由 MineOps 创建、没有其他 Server 引用且当前未使用时移除原端口。'
          : '原端口规则将保留。',
      positiveText: '确认同步并保存',
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
    notifySuccess('结构化配置已保存')
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
      notifySuccess('代理配置已保存', false)
      return
    }
    const saved = await runWithFirewallConfirmation((confirmed) =>
      saveMinecraftServerProperties(props.server.id, 'raw', content, versionToken, [], confirmed),
    )
    if (!saved) return
    applySnapshot(saved)
    await loadBackups()
    notifySuccess('原文配置已保存')
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
    title: '恢复 server.properties 备份？',
    content: '恢复前会再次检查远端版本；当前配置也会先进入有限备份，然后原子替换。',
    objectLabel: backup.name,
    impact: propertiesPath.value,
    positiveText: '确认恢复',
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
    notifySuccess('配置备份已恢复')
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
    title: '保存 server.properties 失败',
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
      ? '保存前版本已进入远程有限备份，最多保留 10 份。'
      : '远端配置已原子保存。',
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
    title: '请在“文件”页签另存配置文件',
    content: `“配置”页签固定编辑 ${configurationFileName.value}，其他文件请使用完整的“文件”工作区。`,
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
            placeholder="选择历史备份"
            style="width: 320px"
          />
          <NButton
            v-if="!isProxyConfiguration"
            type="warning"
            :disabled="!selectedBackupPath || !document || structuredDirty || rawDirty"
            :loading="restoring"
            @click="restoreSelected"
          >
            恢复备份
          </NButton>
          <NButton :loading="loading" @click="requestReload">重新读取</NButton>
        </NFlex>
      </NFlex>

      <NAlert v-if="error" type="error" title="读取服务器配置失败">
        <NFlex align="center" justify="space-between">
          <span>{{ error instanceof Error ? error.message : String(error) }}</span>
          <NButton size="small" @click="load">重试</NButton>
        </NFlex>
      </NAlert>
      <NAlert v-if="backupError" type="warning" title="配置已加载，但备份列表不可用">
        {{ backupError instanceof Error ? backupError.message : String(backupError) }}
      </NAlert>
      <NAlert v-if="isProxyConfiguration" type="info" title="代理配置使用原文模式">
        {{ configurationFileName }} 由对应代理服务端生成，MineOps 会进行冲突检测和原子保存。
      </NAlert>
      <NAlert v-if="snapshot?.validationNotice" type="warning" title="配置值需要修正">
        {{ snapshot.validationNotice }}
      </NAlert>
      <NAlert v-if="conflictMessage" type="warning" title="远端配置已变化">
        {{ conflictMessage }} 请重新读取；MineOps 不会静默覆盖远端修改。
      </NAlert>
      <NAlert v-if="structuredDirty && rawDirty" type="warning" title="两个编辑模式均有修改">
        为避免一个模式覆盖另一个模式，请重新读取并只保留一个模式的修改。
      </NAlert>
      <NAlert v-if="structuredDirty && !structuredValid" type="warning" title="表单值无效">
        请检查端口、玩家数、视距、模拟距离和世界名称后再保存。
      </NAlert>

      <NTabs v-if="document" v-model:value="mode" type="segment" animated>
        <NTabPane
          v-if="!isProxyConfiguration"
          name="structured"
          tab="表单模式"
          display-directive="show"
        >
          <NCard size="small">
            <NForm label-placement="top">
              <div class="form-grid">
                <NFormItem label="服务器描述（MOTD）" class="wide-field">
                  <NInput v-model:value="form.motd" maxlength="256" />
                </NFormItem>
                <NFormItem label="服务端口">
                  <NInputNumber v-model:value="form.serverPort" :min="1" :max="65535" />
                </NFormItem>
                <NFormItem label="最大玩家数">
                  <NInputNumber v-model:value="form.maxPlayers" :min="1" :max="100000" />
                </NFormItem>
                <NFormItem label="游戏模式">
                  <NSelect
                    v-model:value="form.gamemode"
                    :options="[
                      { label: '生存', value: 'survival' },
                      { label: '创造', value: 'creative' },
                      { label: '冒险', value: 'adventure' },
                      { label: '旁观', value: 'spectator' },
                    ]"
                  />
                </NFormItem>
                <NFormItem label="难度">
                  <NSelect
                    v-model:value="form.difficulty"
                    :options="[
                      { label: '和平', value: 'peaceful' },
                      { label: '简单', value: 'easy' },
                      { label: '普通', value: 'normal' },
                      { label: '困难', value: 'hard' },
                    ]"
                  />
                </NFormItem>
                <NFormItem label="视距">
                  <NInputNumber v-model:value="form.viewDistance" :min="2" :max="32" />
                </NFormItem>
                <NFormItem label="模拟距离">
                  <NInputNumber v-model:value="form.simulationDistance" :min="3" :max="32" />
                </NFormItem>
                <NFormItem label="世界名称">
                  <NInput v-model:value="form.levelName" maxlength="128" />
                </NFormItem>
                <NFormItem label="正版验证"><NSwitch v-model:value="form.onlineMode" /></NFormItem>
                <NFormItem label="白名单"><NSwitch v-model:value="form.whiteList" /></NFormItem>
                <NFormItem label="玩家对战（PVP）"><NSwitch v-model:value="form.pvp" /></NFormItem>
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
                  保存表单修改
                </NButton>
              </NFlex>
            </NForm>
          </NCard>
        </NTabPane>
        <NTabPane
          name="raw"
          :tab="isProxyConfiguration ? '代理配置' : '原文模式'"
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

      <NEmpty v-else-if="!error && !loading" description="配置文件尚未加载">
        <template #extra><NButton type="primary" @click="load">读取配置</NButton></template>
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
