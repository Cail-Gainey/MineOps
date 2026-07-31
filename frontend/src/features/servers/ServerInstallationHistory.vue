<script setup lang="ts">
import { NButton, NProgress, NTag, type DataTableColumns } from 'naive-ui'
import { computed, h, onMounted, ref } from 'vue'

import type { InstallationTask } from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import { getInstallation, listServerInstallations } from '../../services/installation-api'
import AppDataTable from '../../shared/components/AppDataTable.vue'
import { hasMessage } from '../../locales/runtime'
import { useInteractionStore } from '../../stores/interactions'
import { useLocaleStore } from '../../stores/locale'

// 与 Go 侧 model.InstallationStepCount 保持一致:安装流程固定 11 步。
const installationStepCount = 11

const props = defineProps<{ serverID: string }>()
const interactions = useInteractionStore()
const locale = useLocaleStore()
const tasks = ref<InstallationTask[]>([])
const loading = ref(false)
const error = ref<unknown>(null)
/**
 * 把枚举值翻译成当前界面语言的标签。
 * @param prefix - 文案键前缀
 * @param value - 枚举值
 * @returns 本地化标签，未登记的值原样返回
 */
function enumLabel(prefix: string, value: string): string {
  const key = `${prefix}.${value}`
  return hasMessage(key) ? locale.t(key) : value
}

const columns = computed<DataTableColumns<InstallationTask>>(() => [
  {
    title: locale.t('installHistory.column.task'),
    key: 'id',
    minWidth: 220,
    ellipsis: { tooltip: true },
  },
  {
    title: locale.t('servers.column.state'),
    key: 'state',
    width: 110,
    render: (row) =>
      h(
        NTag,
        {
          type:
            row.state === 'succeeded'
              ? 'success'
              : row.state === 'failed'
                ? 'error'
                : row.state === 'cancelled'
                  ? 'warning'
                  : 'info',
          bordered: false,
        },
        { default: () => enumLabel('installHistory.state', row.state) },
      ),
  },
  {
    title: locale.t('installHistory.column.step'),
    key: 'currentStep',
    width: 150,
    render: (row) =>
      h(NProgress, {
        percentage: Math.min(100, Math.round((row.currentStep / installationStepCount) * 100)),
        showIndicator: false,
      }),
  },
  {
    title: locale.t('installHistory.column.operation'),
    key: 'operationID',
    minWidth: 220,
    ellipsis: { tooltip: true },
  },
  {
    title: locale.t('common.created'),
    key: 'createdAt',
    width: 190,
    render: (row) => locale.formatDateTime(row.createdAt),
  },
  {
    title: locale.t('installHistory.column.detail'),
    key: 'detail',
    width: 90,
    render: (row) =>
      h(
        NButton,
        { size: 'small', onClick: () => void showDetails(row) },
        { default: () => locale.t('installHistory.view') },
      ),
  },
])

/**
 * 加载该 Server 的安装历史列表。
 * @returns 加载完成后的 Promise
 */
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    tasks.value = await listServerInstallations(props.serverID)
  } catch (reason) {
    error.value = reason
  } finally {
    loading.value = false
  }
}

/**
 * 拉取安装任务的完整聚合状态并展开详情。
 * @param task - 目标安装任务
 * @returns 展开完成后的 Promise
 */
async function showDetails(task: InstallationTask): Promise<void> {
  try {
    const aggregate = await getInstallation(task.id)
    interactions.openDrawer({
      title: locale.t('installHistory.drawerTitle', { id: task.id }),
      content: aggregate.steps
        .map(
          (step) =>
            `${locale.t('installHistory.stepLine', {
              order: step.order,
              label: enumLabel('installHistory.step', step.name),
              state: enumLabel('installHistory.stepState', step.state),
              attempt: step.attempt,
            })}\n${step.message || step.errorCode || ''}`,
        )
        .join('\n\n'),
    })
  } catch (reason) {
    error.value = reason
  }
}

onMounted(() => void load())
</script>

<template>
  <AppDataTable
    :columns="columns"
    :data="tasks"
    :loading="loading"
    :error="error"
    :scroll-x="980"
    :empty-description="locale.t('installHistory.empty')"
    @retry="load"
    @open="showDetails"
  />
</template>
