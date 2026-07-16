<script setup lang="ts">
import { NAlert, NButton, NCard, NEmpty, NFlex, NInput, NSpin, NTabPane, NTabs } from 'naive-ui'
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'

import { getSSHSession } from '../../services/ssh-session-api'
import { useNotificationStore } from '../../stores/notifications'
import { useTerminalTabsStore } from '../../stores/terminal-tabs'
import TerminalPane from './TerminalPane.vue'

const route = useRoute()
const tabs = useTerminalTabsStore()
const notifications = useNotificationStore()
const renameDraft = ref('')
const routeLoading = ref(false)
const routeError = ref<unknown>(null)
let routeRequestGeneration = 0
const routePermissionDenied = computed(
  () =>
    routeError.value &&
    typeof routeError.value === 'object' &&
    'code' in routeError.value &&
    String((routeError.value as { code: unknown }).code).includes('permission_denied'),
)

async function openRouteSession(): Promise<void> {
  const sshSessionID = String(route.params.sshSessionID ?? '').trim()
  const generation = ++routeRequestGeneration
  routeError.value = null
  if (!sshSessionID) return

  // 路由参数是 Terminal 打开目标的唯一事实源，加载完成前不激活缓存标签。
  tabs.activeID = ''
  routeLoading.value = true
  try {
    const session = await getSSHSession(sshSessionID)
    if (generation !== routeRequestGeneration) return
    tabs.open(session)
  } catch (error) {
    if (generation !== routeRequestGeneration) return
    routeError.value = error
    notifications.push({
      kind: 'error',
      title: '打开 Terminal 标签失败',
      content: error instanceof Error ? error.message : String(error),
      dedupeKey: `terminal:open-tab:${sshSessionID}`,
    })
  } finally {
    if (generation === routeRequestGeneration) routeLoading.value = false
  }
}

watch(
  () => route.params.sshSessionID,
  () => void openRouteSession(),
  { immediate: true },
)
watch(
  () => tabs.activeTab?.title,
  (title) => {
    renameDraft.value = title ?? ''
  },
  { immediate: true },
)

function renameActiveTab(): void {
  if (tabs.activeID) tabs.rename(tabs.activeID, renameDraft.value)
}
</script>

<template>
  <NCard
    class="terminal-workspace"
    content-style="height: 100%; display: flex; flex-direction: column;"
  >
    <template v-if="tabs.activeTab" #header>
      <NFlex align="center" justify="end">
        <NFlex :wrap="false">
          <NInput v-model:value="renameDraft" size="small" @keyup.enter="renameActiveTab" />
          <NButton size="small" @click="renameActiveTab">重命名标签</NButton>
        </NFlex>
      </NFlex>
    </template>
    <NAlert
      v-if="routeError"
      :type="routePermissionDenied ? 'warning' : 'error'"
      :title="routePermissionDenied ? '权限不足' : '打开 Terminal 失败'"
      class="route-error"
    >
      <NFlex align="center" justify="space-between">
        <span>{{ routeError instanceof Error ? routeError.message : String(routeError) }}</span>
        <NButton size="small" @click="openRouteSession">重试</NButton>
      </NFlex>
    </NAlert>
    <NSpin :show="routeLoading" class="workspace-content">
      <NEmpty v-if="!tabs.tabs.length" description="请从 SSH Sessions 页面打开 Terminal" />
      <NTabs
        v-else
        v-model:value="tabs.activeID"
        type="card"
        closable
        class="terminal-tabs"
        @close="tabs.close"
      >
        <NTabPane v-for="tab in tabs.tabs" :key="tab.id" :name="tab.id" :tab="tab.title">
          <TerminalPane :ssh-session-id="tab.sshSessionID" :active="tabs.activeID === tab.id" />
        </NTabPane>
      </NTabs>
    </NSpin>
  </NCard>
</template>

<style scoped>
.terminal-workspace {
  height: 100%;
  min-height: 480px;
}

.route-error {
  margin-bottom: 12px;
}

.workspace-content {
  flex: 1;
  min-height: 0;
}

.workspace-content :deep(.n-spin-content) {
  height: 100%;
}

.terminal-tabs,
.terminal-tabs :deep(.n-tabs-pane-wrapper),
.terminal-tabs :deep(.n-tab-pane) {
  height: 100%;
}
</style>
