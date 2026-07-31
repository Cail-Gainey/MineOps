<script setup lang="ts">
import { NButton, NFlex, NInput, NTag, NText, type DataTableColumns } from 'naive-ui'
import { computed, h, onMounted, ref } from 'vue'

import type { KnownHostDTO } from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/models'
import { deleteKnownHost, listKnownHosts } from '../../services/known-host-api'
import AppDataTable from '../../shared/components/AppDataTable.vue'
import { useInteractionStore } from '../../stores/interactions'
import { useLocaleStore } from '../../stores/locale'
import { useNotificationStore } from '../../stores/notifications'

const interactions = useInteractionStore()
const locale = useLocaleStore()
const notifications = useNotificationStore()
const rows = ref<KnownHostDTO[]>([])
const search = ref('')
const loading = ref(false)
const error = ref<unknown>(null)

const columns = computed<DataTableColumns<KnownHostDTO>>(() => [
  {
    title: locale.t('knownHosts.column.state'),
    key: 'active',
    width: 90,
    render: (row) =>
      h(
        NTag,
        { type: row.active ? 'success' : 'default', bordered: false },
        {
          default: () =>
            row.active ? locale.t('knownHosts.current') : locale.t('knownHosts.historical'),
        },
      ),
  },
  {
    title: locale.t('knownHosts.column.host'),
    key: 'host',
    minWidth: 180,
    render: (row) => `${row.host}:${row.port}`,
  },
  { title: locale.t('knownHosts.column.algorithm'), key: 'algorithm', minWidth: 150 },
  {
    title: locale.t('knownHosts.column.fingerprint'),
    key: 'fingerprint',
    minWidth: 260,
    ellipsis: { tooltip: true },
  },
  { title: locale.t('knownHosts.column.firstSeenAt'), key: 'firstSeenAt', minWidth: 180 },
  { title: locale.t('knownHosts.column.lastSeenAt'), key: 'lastSeenAt', minWidth: 180 },
  {
    title: locale.t('common.actions'),
    key: 'actions',
    width: 90,
    render: (row) =>
      h(
        NButton,
        { size: 'small', type: 'error', secondary: true, onClick: () => void remove(row) },
        { default: () => locale.t('common.delete') },
      ),
  },
])

/**
 * 重新加载已信任的主机密钥列表。
 * @returns 刷新完成后的 Promise
 */
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

/**
 * 二次确认后删除一条主机密钥记录。
 * @param row - 目标 Known Host
 * @returns 删除完成后的 Promise
 */
async function remove(row: KnownHostDTO): Promise<void> {
  const confirmed = await interactions.confirm({
    title: row.active
      ? locale.t('knownHosts.deleteActiveTitle')
      : locale.t('knownHosts.deleteHistoryTitle'),
    content: row.active
      ? locale.t('knownHosts.deleteActiveContent')
      : locale.t('knownHosts.deleteHistoryContent'),
    objectLabel: `${row.host}:${row.port} · ${row.fingerprint}`,
    positiveText: locale.t('knownHosts.deleteConfirm'),
    danger: true,
  })
  if (!confirmed) return
  try {
    await deleteKnownHost(row.id)
    await refresh()
    notifications.push({
      kind: 'success',
      title: locale.t('knownHosts.deleted'),
      dedupeKey: `known-host:deleted:${row.id}`,
    })
  } catch (reason) {
    notifications.push({
      kind: 'error',
      title: locale.t('knownHosts.deleteFailed'),
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
        <NText depth="3">{{ locale.t('knownHosts.description') }}</NText>
      </div>
      <NButton :loading="loading" @click="refresh">{{ locale.t('common.refresh') }}</NButton>
    </NFlex>
    <NInput
      v-model:value="search"
      clearable
      :placeholder="locale.t('knownHosts.searchPlaceholder')"
      class="known-host-search"
      @keyup.enter="refresh"
      @clear="refresh"
    />
    <AppDataTable
      :columns="columns"
      :data="rows"
      :loading="loading"
      :error="error"
      :empty-description="locale.t('knownHosts.empty')"
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
