<script setup lang="ts">
import { NButton, NFlex, NTag, NText, type DataTableColumns } from 'naive-ui'
import { computed, h, onMounted, onUnmounted, ref } from 'vue'

import type { MinecraftServer } from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import type { ServerBackup } from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/service/models'
import {
  createMinecraftServerBackup,
  listMinecraftServerBackups,
  restoreMinecraftServerBackup,
} from '../../services/minecraft-server-api'
import { subscribeOperationProgress } from '../../services/operation-api'
import AppDataTable from '../../shared/components/AppDataTable.vue'
import { useInteractionStore } from '../../stores/interactions'
import { useLocaleStore } from '../../stores/locale'
import { useNotificationStore } from '../../stores/notifications'

const props = defineProps<{ server: MinecraftServer }>()
const interactions = useInteractionStore()
const locale = useLocaleStore()
const notifications = useNotificationStore()
const backups = ref<ServerBackup[]>([])
const loading = ref(false)
const error = ref<unknown>(null)
let unsubscribeOperation: (() => void) | null = null

const columns = computed<DataTableColumns<ServerBackup>>(() => [
  {
    title: locale.t('serverBackups.column.backup'),
    key: 'name',
    minWidth: 280,
    ellipsis: { tooltip: true },
  },
  {
    title: locale.t('serverBackups.column.size'),
    key: 'size',
    width: 130,
    render: (row) =>
      locale.t('serverBackups.sizeMiB', { value: (row.size / 1024 / 1024).toFixed(1) }),
  },
  {
    title: 'SHA-256',
    key: 'sha256',
    minWidth: 260,
    render: (row) =>
      row.sha256
        ? row.sha256
        : h(
            NTag,
            { type: 'warning', bordered: false },
            { default: () => locale.t('serverBackups.checksumMissing') },
          ),
  },
  {
    title: locale.t('serverBackups.column.modifiedAt'),
    key: 'modifiedAt',
    width: 190,
    render: (row) => locale.formatDateTime(row.modifiedAt),
  },
  {
    title: locale.t('common.actions'),
    key: 'actions',
    width: 100,
    render: (row) =>
      h(
        NButton,
        { size: 'small', type: 'warning', disabled: !row.sha256, onClick: () => void restore(row) },
        { default: () => locale.t('serverBackups.restore') },
      ),
  },
])

/**
 * 加载该 Server 的目录备份列表。
 * @returns 加载完成后的 Promise
 */
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    backups.value = await listMinecraftServerBackups(props.server.id)
  } catch (reason) {
    error.value = reason
  } finally {
    loading.value = false
  }
}

/**
 * 二次确认后创建一份 Server 目录备份。
 * @returns 创建发起后的 Promise
 */
async function create(): Promise<void> {
  const confirmed = await interactions.confirm({
    title: locale.t('serverBackups.createTitle'),
    content: locale.t('serverBackups.createContent'),
    objectLabel: props.server.remotePath,
    impact: locale.t('serverBackups.createImpact'),
    positiveText: locale.t('serverBackups.createConfirm'),
  })
  if (!confirmed) return
  try {
    const operationID = await createMinecraftServerBackup(props.server.id)
    notifications.push({
      kind: 'info',
      title: locale.t('serverBackups.createStarted'),
      content: operationID,
      dedupeKey: `server-backup:${operationID}`,
    })
  } catch (reason) {
    notifyError(locale.t('serverBackups.createFailed'), reason)
  }
}

/**
 * 二次确认后用指定备份恢复 Server 目录。
 * @param backup - 目标备份
 * @returns 恢复发起后的 Promise
 */
async function restore(backup: ServerBackup): Promise<void> {
  const confirmed = await interactions.confirm({
    title: locale.t('serverBackups.restoreTitle'),
    content: locale.t('serverBackups.restoreContent'),
    objectLabel: backup.name,
    impact: `${props.server.remotePath}\nSHA-256: ${backup.sha256}`,
    positiveText: locale.t('serverBackups.restoreConfirm'),
    danger: true,
  })
  if (!confirmed) return
  try {
    const operationID = await restoreMinecraftServerBackup(props.server.id, backup.path)
    notifications.push({
      kind: 'warning',
      title: locale.t('serverBackups.restoreStarted'),
      content: operationID,
      dedupeKey: `server-restore:${operationID}`,
    })
  } catch (reason) {
    notifyError(locale.t('serverBackups.restoreFailed'), reason)
  }
}

/**
 * 推送一条备份操作错误通知。
 * @param title - 通知标题
 * @param reason - 捕获到的错误
 * @returns 无返回值
 */
function notifyError(title: string, reason: unknown): void {
  notifications.push({
    kind: 'error',
    title,
    content: reason instanceof Error ? reason.message : String(reason),
    dedupeKey: `server-backup:error:${title}`,
  })
}

onMounted(() => {
  unsubscribeOperation = subscribeOperationProgress((operation) => {
    if (
      operation.targetID === props.server.id &&
      (operation.type === 'backup' || operation.type === 'restore') &&
      !['pending', 'running'].includes(operation.state)
    ) {
      void load()
    }
  })
  void load()
})
onUnmounted(() => unsubscribeOperation?.())
</script>

<template>
  <NFlex vertical :size="12">
    <NFlex align="center" justify="space-between" wrap>
      <div>
        <NText strong>{{ locale.t('serverBackups.title') }}</NText>
        <NText depth="3" class="description">{{ locale.t('serverBackups.description') }}</NText>
      </div>
      <NButton
        type="primary"
        :disabled="!['stopped', 'ready'].includes(server.state)"
        @click="create"
      >
        {{ locale.t('serverBackups.createConfirm') }}
      </NButton>
    </NFlex>
    <AppDataTable
      :columns="columns"
      :data="backups"
      :loading="loading"
      :error="error"
      :scroll-x="1040"
      :empty-description="locale.t('serverBackups.empty')"
      @retry="load"
    />
  </NFlex>
</template>

<style scoped>
.description {
  display: block;
}
</style>
