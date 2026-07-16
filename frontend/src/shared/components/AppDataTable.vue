<script setup lang="ts" generic="Row extends Record<string, unknown>">
import {
  NAlert,
  NButton,
  NDataTable,
  NDropdown,
  NEmpty,
  NFlex,
  NSpin,
  type DataTableColumns,
  type DataTableRowKey,
  type DropdownOption,
} from 'naive-ui'
import { computed, ref } from 'vue'

import { useLocaleStore } from '../../stores/locale'

const props = withDefaults(
  defineProps<{
    columns: DataTableColumns<Row>
    data: Row[]
    rowKey?: (row: Row) => DataTableRowKey
    loading?: boolean
    // eslint-disable-next-line vue/require-default-prop
    error?: unknown
    partialMessage?: string
    emptyDescription?: string
    checkedRowKeys?: DataTableRowKey[]
    // eslint-disable-next-line vue/require-default-prop
    contextOptions?: DropdownOption[] | ((row: Row) => DropdownOption[])
    // eslint-disable-next-line vue/require-default-prop
    maxHeight?: number | string
    // eslint-disable-next-line vue/require-default-prop
    scrollX?: number
  }>(),
  {
    rowKey: (row: Row) => row.id as DataTableRowKey,
    loading: false,
    emptyDescription: '',
    checkedRowKeys: () => [],
    partialMessage: '',
  },
)

const locale = useLocaleStore()

const emit = defineEmits<{
  retry: []
  'update:checkedRowKeys': [keys: DataTableRowKey[]]
  contextAction: [key: string | number, row: Row]
  open: [row: Row]
}>()

const contextVisible = ref(false)
const contextX = ref(0)
const contextY = ref(0)
const contextRow = ref<Row | null>(null)
const currentContextOptions = ref<DropdownOption[]>([])
const tableOptionalProps = computed(() => ({
  ...(props.maxHeight === undefined ? {} : { maxHeight: props.maxHeight }),
  ...(props.scrollX === undefined ? {} : { scrollX: props.scrollX }),
}))

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error ?? '数据加载失败')
}

function permissionDenied(error: unknown): boolean {
  return Boolean(
    error &&
    typeof error === 'object' &&
    'code' in error &&
    String((error as { code: unknown }).code).includes('permission_denied'),
  )
}

function rowProps(row: Row): Record<string, unknown> {
  return {
    onDblclick: () => emit('open', row),
    onContextmenu: (event: MouseEvent) => {
      if (!props.contextOptions) return
      event.preventDefault()
      contextVisible.value = false
      contextRow.value = row
      currentContextOptions.value =
        typeof props.contextOptions === 'function'
          ? props.contextOptions(row)
          : props.contextOptions
      contextX.value = event.clientX
      contextY.value = event.clientY
      requestAnimationFrame(() => {
        contextVisible.value = currentContextOptions.value.length > 0
      })
    },
  }
}

function selectContextAction(key: string | number): void {
  contextVisible.value = false
  if (contextRow.value) emit('contextAction', key, contextRow.value)
}
</script>

<template>
  <NAlert
    v-if="permissionDenied(error) && !data.length"
    type="warning"
    :title="locale.t('table.permissionDenied')"
    class="table-state"
  >
    {{ locale.t('table.permissionReadMessage') }}
  </NAlert>
  <NAlert
    v-else-if="error && !data.length"
    type="error"
    :title="locale.t('table.loadFailed')"
    class="table-state"
  >
    <NFlex align="center" justify="space-between">
      <span>{{ errorMessage(error) }}</span>
      <NButton size="small" @click="$emit('retry')">{{ locale.t('common.retry') }}</NButton>
    </NFlex>
  </NAlert>
  <NAlert
    v-if="partialMessage || (error && data.length)"
    type="warning"
    :title="locale.t('table.partialUnavailable')"
    class="table-state"
  >
    {{ partialMessage || errorMessage(error) }}
  </NAlert>
  <NSpin v-if="!error || data.length" :show="loading">
    <NDataTable
      v-if="data.length || loading"
      :columns="columns"
      :data="data"
      :row-key="rowKey"
      :checked-row-keys="checkedRowKeys"
      :row-props="rowProps"
      v-bind="tableOptionalProps"
      striped
      :flex-height="maxHeight !== undefined"
      @update:checked-row-keys="$emit('update:checkedRowKeys', $event)"
    />
    <NEmpty v-else :description="emptyDescription || locale.t('table.empty')" class="table-state">
      <template #extra><slot name="empty-action" /></template>
    </NEmpty>
  </NSpin>
  <NDropdown
    placement="bottom-start"
    trigger="manual"
    :x="contextX"
    :y="contextY"
    :options="currentContextOptions"
    :show="contextVisible"
    :on-clickoutside="() => (contextVisible = false)"
    @select="selectContextAction"
  />
</template>

<style scoped>
.table-state {
  margin: 16px 0;
}
</style>
