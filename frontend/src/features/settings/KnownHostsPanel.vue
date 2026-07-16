<script setup lang="ts">
import { NButton, NFlex, NInput, NTag, NText, type DataTableColumns } from 'naive-ui'
import { h, onMounted, ref } from 'vue'

import type { KnownHostDTO } from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/models'
import { deleteKnownHost, listKnownHosts } from '../../services/known-host-api'
import AppDataTable from '../../shared/components/AppDataTable.vue'
import { useInteractionStore } from '../../stores/interactions'
import { useNotificationStore } from '../../stores/notifications'

const interactions = useInteractionStore()
const notifications = useNotificationStore()
const rows = ref<KnownHostDTO[]>([])
const search = ref('')
const loading = ref(false)
const error = ref<unknown>(null)

const columns: DataTableColumns<KnownHostDTO> = [
  {
    title: '状态',
    key: 'active',
    width: 90,
    render: (row) =>
      h(
        NTag,
        { type: row.active ? 'success' : 'default', bordered: false },
        { default: () => (row.active ? '当前' : '历史') },
      ),
  },
  { title: '主机', key: 'host', minWidth: 180, render: (row) => `${row.host}:${row.port}` },
  { title: '算法', key: 'algorithm', minWidth: 150 },
  { title: 'SHA256 指纹', key: 'fingerprint', minWidth: 260, ellipsis: { tooltip: true } },
  { title: '首次信任', key: 'firstSeenAt', minWidth: 180 },
  { title: '最近使用', key: 'lastSeenAt', minWidth: 180 },
  {
    title: '操作',
    key: 'actions',
    width: 90,
    render: (row) =>
      h(
        NButton,
        { size: 'small', type: 'error', secondary: true, onClick: () => void remove(row) },
        { default: () => '删除' },
      ),
  },
]

async function refresh(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    rows.value = await listKnownHosts(search.value)
  } catch (reason) {
    error.value = reason
  } finally {
    loading.value = false
  }
}

async function remove(row: KnownHostDTO): Promise<void> {
  const confirmed = await interactions.confirm({
    title: row.active ? '删除当前受信任指纹？' : '删除指纹历史？',
    content: row.active
      ? '下次连接时将重新进入首次信任流程。请在可信渠道核对新指纹。'
      : '该操作只删除历史审计记录，不改变当前受信任指纹。',
    objectLabel: `${row.host}:${row.port} · ${row.fingerprint}`,
    positiveText: '删除记录',
    danger: true,
  })
  if (!confirmed) return
  try {
    await deleteKnownHost(row.id)
    await refresh()
    notifications.push({
      kind: 'success',
      title: 'Known Host 记录已删除',
      dedupeKey: `known-host:deleted:${row.id}`,
    })
  } catch (reason) {
    notifications.push({
      kind: 'error',
      title: '删除 Known Host 失败',
      content: reason instanceof Error ? reason.message : String(reason),
      dedupeKey: `known-host:delete-error:${row.id}`,
    })
  }
}

onMounted(refresh)
</script>

<template>
  <section class="known-hosts">
    <NFlex align="center" justify="space-between">
      <div>
        <NText tag="h3">Known Hosts</NText>
        <NText depth="3">当前指纹与变更历史均保存在 SQLCipher；指纹变化默认拒绝连接。</NText>
      </div>
      <NButton :loading="loading" @click="refresh">刷新</NButton>
    </NFlex>
    <NInput
      v-model:value="search"
      clearable
      placeholder="搜索主机或 SHA256 指纹"
      class="known-host-search"
      @keyup.enter="refresh"
      @clear="refresh"
    />
    <AppDataTable
      :columns="columns"
      :data="rows"
      :loading="loading"
      :error="error"
      empty-description="尚无受信任主机指纹"
      @retry="refresh"
    />
  </section>
</template>

<style scoped>
.known-hosts {
  margin-top: 24px;
  padding-top: 20px;
  border-top: 1px solid var(--border-default);
}

.known-host-search {
  max-width: 420px;
  margin: 16px 0;
}
</style>
