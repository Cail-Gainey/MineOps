<script setup lang="ts">
import { NButton, NFlex, NTag, NText, type DataTableColumns } from 'naive-ui'
import { h, onMounted, onUnmounted, ref } from 'vue'

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

const columns: DataTableColumns<ServerBackup> = [
  { title: '备份', key: 'name', minWidth: 280, ellipsis: { tooltip: true } },
  {
    title: '大小',
    key: 'size',
    width: 130,
    render: (row) => `${(row.size / 1024 / 1024).toFixed(1)} MiB`,
  },
  {
    title: 'SHA-256',
    key: 'sha256',
    minWidth: 260,
    render: (row) =>
      row.sha256
        ? row.sha256
        : h(NTag, { type: 'warning', bordered: false }, { default: () => '校验文件缺失' }),
  },
  {
    title: '修改时间',
    key: 'modifiedAt',
    width: 190,
    render: (row) => locale.formatDateTime(row.modifiedAt),
  },
  {
    title: '操作',
    key: 'actions',
    width: 100,
    render: (row) =>
      h(
        NButton,
        { size: 'small', type: 'warning', disabled: !row.sha256, onClick: () => void restore(row) },
        { default: () => '恢复' },
      ),
  },
]

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
    title: '创建服务器目录备份？',
    content: '仅允许已停止或就绪的服务器。符号链接、特殊文件和独立挂载根会被拒绝。',
    objectLabel: props.server.remotePath,
    impact: '备份将写入远程 ~/MineOps/Backup，并生成 SHA-256 校验文件。',
    positiveText: '创建备份',
  })
  if (!confirmed) return
  try {
    const operationID = await createMinecraftServerBackup(props.server.id)
    notifications.push({
      kind: 'info',
      title: '服务器备份任务已启动',
      content: operationID,
      dedupeKey: `server-backup:${operationID}`,
    })
  } catch (reason) {
    notifyError('启动服务器备份失败', reason)
  }
}

/**
 * 二次确认后用指定备份恢复 Server 目录。
 * @param backup - 目标备份
 * @returns 恢复发起后的 Promise
 */
async function restore(backup: ServerBackup): Promise<void> {
  const confirmed = await interactions.confirm({
    title: '从备份恢复服务器目录？',
    content:
      'MineOps 将先下载并完整校验归档，在远程暂存目录中安全解压，最后原子替换当前目录。仅允许已停止或就绪的服务器。',
    objectLabel: backup.name,
    impact: `${props.server.remotePath}\nSHA-256: ${backup.sha256}`,
    positiveText: '确认恢复',
    danger: true,
  })
  if (!confirmed) return
  try {
    const operationID = await restoreMinecraftServerBackup(props.server.id, backup.path)
    notifications.push({
      kind: 'warning',
      title: '服务器恢复任务已启动',
      content: operationID,
      dedupeKey: `server-restore:${operationID}`,
    })
  } catch (reason) {
    notifyError('启动服务器恢复失败', reason)
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
        <NText strong>服务器专属目录备份</NText>
        <NText depth="3" class="description"
          >远程 tar.gz + SHA-256；恢复前由 Go 完整检查路径、类型、数量和展开体积。</NText
        >
      </div>
      <NButton
        type="primary"
        :disabled="!['stopped', 'ready'].includes(server.state)"
        @click="create"
      >
        创建备份
      </NButton>
    </NFlex>
    <AppDataTable
      :columns="columns"
      :data="backups"
      :loading="loading"
      :error="error"
      :scroll-x="1040"
      empty-description="该服务器尚无 MineOps 管理的备份"
      @retry="load"
    />
  </NFlex>
</template>

<style scoped>
.description {
  display: block;
}
</style>
