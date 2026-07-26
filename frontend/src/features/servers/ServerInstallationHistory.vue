<script setup lang="ts">
import { NButton, NProgress, NTag, type DataTableColumns } from 'naive-ui'
import { h, onMounted, ref } from 'vue'

import type { InstallationTask } from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import { getInstallation, listServerInstallations } from '../../services/installation-api'
import AppDataTable from '../../shared/components/AppDataTable.vue'
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
const installationStateLabels: Record<string, string> = {
  waiting: '等待中',
  running: '执行中',
  succeeded: '已成功',
  failed: '失败',
  cancelled: '已取消',
}
// 与 model.NewInstallationTask 的步骤名列表一一对应。
const installationStepLabels: Record<string, string> = {
  connect_ssh: '建立 SSH 连接',
  initialize_directories: '初始化远程目录',
  create_server_directory: '创建服务器目录',
  resolve_java: '检测 Java 运行时',
  install_java: '安装 OpenJDK',
  download_server: '下载服务端',
  install_server: '安装服务端',
  write_eula: '写入 EULA',
  configure_firewall: '配置防火墙',
  first_start: '首次启动检查',
  register_server: '注册服务器',
}
const installationStepStateLabels: Record<string, string> = {
  waiting: '等待中',
  running: '执行中',
  success: '成功',
  failed: '失败',
  skipped: '已跳过',
  cancelled: '已取消',
}

const columns: DataTableColumns<InstallationTask> = [
  { title: '安装任务', key: 'id', minWidth: 220, ellipsis: { tooltip: true } },
  {
    title: '状态',
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
        { default: () => installationStateLabels[row.state] ?? row.state },
      ),
  },
  {
    title: '步骤',
    key: 'currentStep',
    width: 150,
    render: (row) =>
      h(NProgress, {
        percentage: Math.min(100, Math.round((row.currentStep / installationStepCount) * 100)),
        showIndicator: false,
      }),
  },
  { title: '后台任务', key: 'operationID', minWidth: 220, ellipsis: { tooltip: true } },
  {
    title: '创建时间',
    key: 'createdAt',
    width: 190,
    render: (row) => locale.formatDateTime(row.createdAt),
  },
  {
    title: '详情',
    key: 'detail',
    width: 90,
    render: (row) =>
      h(
        NButton,
        { size: 'small', onClick: () => void showDetails(row) },
        { default: () => '查看' },
      ),
  },
]

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

async function showDetails(task: InstallationTask): Promise<void> {
  try {
    const aggregate = await getInstallation(task.id)
    interactions.openDrawer({
      title: `安装任务 ${task.id}`,
      content: aggregate.steps
        .map(
          (step) =>
            `${step.order}. ${installationStepLabels[step.name] ?? step.name} · ${installationStepStateLabels[step.state] ?? step.state} · 第 ${step.attempt} 次尝试\n${step.message || step.errorCode || ''}`,
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
    empty-description="该服务器尚无安装历史"
    @retry="load"
    @open="showDetails"
  />
</template>
