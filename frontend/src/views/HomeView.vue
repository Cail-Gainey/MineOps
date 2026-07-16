<script setup lang="ts">
import { Boxes, DatabaseZap, MonitorCog, ServerCog } from '@lucide/vue'
import { NAlert, NButton, NCard, NFlex, NGrid, NGridItem, NTag, NText } from 'naive-ui'
import { storeToRefs } from 'pinia'
import { defineAsyncComponent, onUnmounted, ref } from 'vue'

import { runBindingEcho, runCancellationWait } from '../services/binding-spike'
import { startEventSpike, stopEventSpike, subscribeEventSpike } from '../services/event-spike'
import { subscribeSecondInstance } from '../services/lifecycle-spike'
import { runSQLCipherSpike } from '../services/sqlcipher-spike'
import AppIcon from '../shared/components/AppIcon.vue'
import { useThemeStore, type ThemeMode } from '../stores/theme'
import { accentColours, type AccentName } from '../themes/tokens'

const themeStore = useThemeStore()
const { accent, mode } = storeToRefs(themeStore)
const TerminalSpike = defineAsyncComponent(
  () => import('../features/terminal-spike/TerminalSpike.vue'),
)
const MonacoSpike = defineAsyncComponent(() => import('../features/editor-spike/MonacoSpike.vue'))
const EChartsSpike = defineAsyncComponent(
  () => import('../features/monitoring-spike/EChartsSpike.vue'),
)

const themeModes: ThemeMode[] = ['light', 'dark', 'system']
const accentNames = Object.keys(accentColours) as AccentName[]
const bindingResult = ref('尚未执行')
const cancellationResult = ref('尚未执行')
const eventResult = ref('尚未执行')
const receivedBatches = ref(0)
const receivedItems = ref(0)
const secondInstanceResult = ref('等待第二实例启动')
const secondInstanceCount = ref(0)
const sqlCipherResult = ref('尚未执行')
const sqlCipherRunning = ref(false)
let pendingWait: ReturnType<typeof runCancellationWait> | null = null
let unsubscribeEventSpike: (() => void) | null = null
const unsubscribeSecondInstance = subscribeSecondInstance((data) => {
  secondInstanceCount.value++
  secondInstanceResult.value = `${data.workingDir} / ${data.args.length} 个参数`
})

async function verifyBinding(): Promise<void> {
  bindingResult.value = '执行中'
  try {
    const response = await runBindingEcho()
    bindingResult.value = response.accepted
      ? `${response.echo.name} / ${response.echo.tags.join(', ')}`
      : 'DTO 未被接受'
  } catch (error) {
    bindingResult.value = error instanceof Error ? error.message : String(error)
  }
}

async function startCancellation(): Promise<void> {
  cancellationResult.value = '等待取消'
  pendingWait = runCancellationWait(5_000)
  try {
    cancellationResult.value = await pendingWait
  } catch (error) {
    cancellationResult.value = error instanceof Error ? error.message : String(error)
  } finally {
    pendingWait = null
  }
}

function cancelWait(): void {
  pendingWait?.cancel('用户取消阶段 0 Binding Spike')
}

async function verifySQLCipher(): Promise<void> {
  sqlCipherRunning.value = true
  sqlCipherResult.value = '正在创建临时加密数据库'
  try {
    const result = await runSQLCipherSpike()
    const passed =
      result.journalMode === 'wal' &&
      result.busyTimeoutMillis === 5_000 &&
      result.foreignKeysEnabled &&
      result.transactionCommitted &&
      result.transactionRolledBack &&
      result.integrityCheckOK &&
      result.cipherIntegrityOK &&
      result.foreignKeyCheckOK &&
      result.encryptedHeader &&
      result.plaintextAbsent &&
      result.wrongKeyRejected &&
      result.correctKeyReopened &&
      result.automaticKeyRead &&
      result.keyFileModeSecure &&
      result.keyRotationSucceeded &&
      result.oldKeyRejected &&
      result.backupCreated &&
      result.backupRestored &&
      result.tamperDetected &&
      result.keyVersion === 2
    sqlCipherResult.value = `${passed ? '通过' : '未通过'} / ${result.cipherVersion} / key v${result.keyVersion} / backup ${result.backupBytes} bytes`
  } catch (error) {
    sqlCipherResult.value = error instanceof Error ? error.message : String(error)
  } finally {
    sqlCipherRunning.value = false
  }
}

async function startEvents(): Promise<void> {
  unsubscribeEventSpike?.()
  receivedBatches.value = 0
  receivedItems.value = 0
  unsubscribeEventSpike = subscribeEventSpike((batch) => {
    receivedBatches.value++
    receivedItems.value += batch.values.length
    eventResult.value = `批次 ${batch.batchID} / 序列 ${batch.firstSequence}-${batch.lastSequence}`
  })

  try {
    await startEventSpike()
    eventResult.value = '事件流运行中'
  } catch (error) {
    unsubscribeEventSpike?.()
    unsubscribeEventSpike = null
    eventResult.value = error instanceof Error ? error.message : String(error)
  }
}

async function stopEvents(): Promise<void> {
  try {
    const status = await stopEventSpike()
    eventResult.value = `已停止：后端 ${status.emittedBatches} 批 / ${status.emittedItems} 项`
  } catch (error) {
    eventResult.value = error instanceof Error ? error.message : String(error)
  } finally {
    unsubscribeEventSpike?.()
    unsubscribeEventSpike = null
  }
}

onUnmounted(() => {
  pendingWait?.cancel('页面卸载')
  unsubscribeEventSpike?.()
  unsubscribeSecondInstance()
  void stopEventSpike()
})
</script>

<template>
  <main class="baseline-shell">
    <header class="baseline-header">
      <div>
        <NText tag="h1" class="title">MineOps</NText>
        <NText depth="3">Go + Wails v3 + Vue 3 阶段 0 技术基线</NText>
      </div>
      <NFlex align="center">
        <NButton
          v-for="themeMode in themeModes"
          :key="themeMode"
          size="small"
          :type="mode === themeMode ? 'primary' : 'default'"
          @click="mode = themeMode"
        >
          {{ themeMode }}
        </NButton>
        <NButton
          v-for="accentName in accentNames"
          :key="accentName"
          size="small"
          :type="accent === accentName ? 'primary' : 'default'"
          @click="accent = accentName"
        >
          {{ accentName }}
        </NButton>
      </NFlex>
    </header>

    <NGrid cols="1 720:2" :x-gap="16" :y-gap="16">
      <NGridItem>
        <NCard title="Desktop Baseline">
          <template #header-extra>
            <AppIcon :icon="MonitorCog" label="桌面基线" />
          </template>
          <NFlex vertical>
            <NTag type="success">Wails Desktop Host</NTag>
            <NTag type="success">Vue TypeScript Strict</NTag>
            <NTag type="success">Naive UI Providers</NTag>
          </NFlex>
        </NCard>
      </NGridItem>
      <NGridItem>
        <NCard title="Architecture Baseline">
          <template #header-extra>
            <AppIcon :icon="Boxes" label="架构基线" />
          </template>
          <NFlex vertical>
            <NTag type="info"><AppIcon :icon="ServerCog" label="服务层" /> Service</NTag>
            <NTag type="info"><AppIcon :icon="DatabaseZap" label="仓储层" /> Repository</NTag>
            <NTag type="info">MVVM Feature Modules</NTag>
          </NFlex>
        </NCard>
      </NGridItem>
      <NGridItem>
        <NCard title="Wails Binding Spike">
          <NFlex vertical>
            <NFlex>
              <NButton type="primary" @click="verifyBinding">验证 DTO</NButton>
              <NButton @click="startCancellation">启动可取消调用</NButton>
              <NButton type="warning" :disabled="pendingWait === null" @click="cancelWait">
                取消调用
              </NButton>
            </NFlex>
            <NAlert title="DTO 回显" type="info">{{ bindingResult }}</NAlert>
            <NAlert title="取消结果" type="info">{{ cancellationResult }}</NAlert>
          </NFlex>
        </NCard>
      </NGridItem>
      <NGridItem>
        <NCard title="Wails Event Spike">
          <NFlex vertical>
            <NFlex>
              <NButton type="primary" @click="startEvents">启动批量事件</NButton>
              <NButton type="warning" @click="stopEvents">停止并取消订阅</NButton>
            </NFlex>
            <NAlert title="事件状态" type="info">{{ eventResult }}</NAlert>
            <NText>前端收到 {{ receivedBatches }} 批，共 {{ receivedItems }} 项</NText>
          </NFlex>
        </NCard>
      </NGridItem>
      <NGridItem>
        <NCard title="Wails Lifecycle Spike">
          <NFlex vertical>
            <NTag type="success">单实例锁已启用</NTag>
            <NText>第二实例触发 {{ secondInstanceCount }} 次</NText>
            <NAlert title="最近一次启动" type="info">{{ secondInstanceResult }}</NAlert>
          </NFlex>
        </NCard>
      </NGridItem>
      <NGridItem>
        <NCard title="SQLCipher + GORM Spike">
          <NFlex vertical>
            <NButton type="primary" :loading="sqlCipherRunning" @click="verifySQLCipher">
              运行加密数据库验证
            </NButton>
            <NAlert title="SQLCipher 结果" type="info">{{ sqlCipherResult }}</NAlert>
          </NFlex>
        </NCard>
      </NGridItem>
      <NGridItem span="1 720:2">
        <NCard title="xterm.js Spike">
          <TerminalSpike />
        </NCard>
      </NGridItem>
      <NGridItem span="1 720:2">
        <NCard title="Monaco Editor Spike">
          <MonacoSpike />
        </NCard>
      </NGridItem>
      <NGridItem span="1 720:2">
        <NCard title="ECharts Monitoring Spike">
          <EChartsSpike />
        </NCard>
      </NGridItem>
    </NGrid>
  </main>
</template>

<style scoped>
.baseline-shell {
  min-height: 100vh;
  box-sizing: border-box;
  padding: 72px 32px 32px;
}

.baseline-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  margin-bottom: 24px;
}

.title {
  display: block;
  margin: 0 0 8px;
  font-size: 32px;
  font-weight: 700;
}

@media (max-width: 720px) {
  .baseline-header {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
