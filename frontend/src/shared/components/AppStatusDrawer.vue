<script setup lang="ts">
import {
  NAlert,
  NButton,
  NDrawer,
  NDrawerContent,
  NEmpty,
  NFlex,
  NList,
  NListItem,
  NProgress,
  NTabPane,
  NTabs,
  NTag,
  NText,
  NThing,
} from 'naive-ui'
import { ref, watch } from 'vue'
import { useRouter } from 'vue-router'

import type {
  AlertEvent,
  Operation,
} from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import { ApplicationError } from '../../services/api-client'
import { formatMetricLabel } from '../monitoring/metric-labels'
import { useAlertNotificationStore } from '../../stores/alert-notifications'
import type { ErrorEntry } from '../../stores/error-center'
import { useErrorCenterStore } from '../../stores/error-center'
import { useInteractionStore } from '../../stores/interactions'
import { useLocaleStore } from '../../stores/locale'
import { useNotificationStore } from '../../stores/notifications'
import { useOperationsStore } from '../../stores/operations'

type StatusTab = 'operations' | 'alerts' | 'errors'

const props = defineProps<{ initialTab: StatusTab }>()
const show = defineModel<boolean>('show', { default: false })
const router = useRouter()
const operations = useOperationsStore()
const alerts = useAlertNotificationStore()
const errors = useErrorCenterStore()
const interactions = useInteractionStore()
const locale = useLocaleStore()
const notifications = useNotificationStore()
const activeTab = ref<StatusTab>(props.initialTab)
const expandedErrorID = ref<number | null>(null)

watch(
  () => props.initialTab,
  (value) => {
    activeTab.value = value
  },
)
watch(show, (visible) => {
  if (!visible) return
  activeTab.value = props.initialTab
  void operations.refresh()
  refreshAlerts()
})

/**
 * 刷新告警列表，失败时静默忽略。
 * @returns 无返回值
 */
function refreshAlerts(): void {
  void alerts.refresh().catch(() => undefined)
}

/**
 * 把 Operation 状态映射成标签配色。
 * @param state - Operation 状态
 * @returns naive-ui 标签的语义类型
 */
function stateType(state: string): 'default' | 'info' | 'success' | 'warning' | 'error' {
  if (state === 'running') return 'info'
  if (state === 'succeeded') return 'success'
  if (state === 'failed') return 'error'
  if (state === 'cancelled') return 'warning'
  return 'default'
}

/**
 * 二次确认后取消一条进行中的 Operation。
 * @param operation - 目标 Operation
 * @returns 取消完成后的 Promise
 */
async function cancelOperation(operation: Operation): Promise<void> {
  const confirmed = await interactions.confirm({
    title: '取消后台任务？',
    content: '取消请求会发送给正在运行的 Operation，已完成步骤不会自动回滚。',
    objectLabel: `${operation.type} · ${operation.id}`,
    impact: operation.message || operation.stage,
    positiveText: '确认取消',
    danger: true,
  })
  if (!confirmed) return
  try {
    await operations.cancel(operation.id)
  } catch (reason) {
    notifications.push({
      kind: 'error',
      title: '取消 Operation 失败',
      content: reason instanceof Error ? reason.message : String(reason),
      dedupeKey: `status-drawer:cancel:${operation.id}`,
    })
  }
}

/**
 * 关闭抽屉并跳转到 Operations 页面。
 * @returns 跳转完成后的 Promise
 */
async function openOperationsPage(): Promise<void> {
  show.value = false
  await router.push('/operations')
}

/**
 * 确认一条告警事件。
 * @param event - 目标告警事件
 * @returns 确认完成后的 Promise
 */
async function acknowledgeAlertEvent(event: AlertEvent): Promise<void> {
  try {
    await alerts.acknowledge(event.id)
  } catch (reason) {
    notifications.push({
      kind: 'error',
      title: '确认告警失败',
      content: reason instanceof Error ? reason.message : String(reason),
      dedupeKey: `status-drawer:alert:acknowledge:${event.id}`,
    })
  }
}

/**
 * 二次确认后删除一条告警事件。
 * @param event - 目标告警事件
 * @returns 删除完成后的 Promise
 */
async function removeAlertEvent(event: AlertEvent): Promise<void> {
  const confirmed = await interactions.confirm({
    title: '删除活动告警？',
    content: '告警记录将永久删除；阈值规则保持启用，条件持续满足时可能再次触发。',
    objectLabel: formatMetricLabel(event.metric),
    impact: `当前值 ${event.latestValue.toFixed(2)} / 阈值 ${event.threshold.toFixed(2)}`,
    positiveText: '删除告警',
    danger: true,
  })
  if (!confirmed) return
  try {
    await alerts.remove(event.id)
    notifications.push({
      kind: 'success',
      title: '活动告警已删除',
      dedupeKey: `status-drawer:alert:delete:${event.id}`,
    })
  } catch (reason) {
    notifications.push({
      kind: 'error',
      title: '删除活动告警失败',
      content: reason instanceof Error ? reason.message : String(reason),
      dedupeKey: `status-drawer:alert:delete:error:${event.id}`,
    })
  }
}

/**
 * 关闭抽屉并跳转到该告警对应的指标趋势。
 * @param event - 目标告警事件
 * @returns 跳转完成后的 Promise
 */
async function openAlertMetric(event: AlertEvent): Promise<void> {
  show.value = false
  await router.push({
    path: '/monitoring',
    query: {
      serverID: event.serverID,
      metric: event.metric,
      rangeHours: '1',
      focus: event.triggeredAt,
    },
  })
  requestAnimationFrame(() => {
    document
      .getElementById('monitoring-metric-history')
      ?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  })
}

/**
 * 二次确认后清空错误中心的全部条目。
 * @returns 清空完成后的 Promise
 */
async function clearErrors(): Promise<void> {
  if (!errors.entries.length) return
  const confirmed = await interactions.confirm({
    title: '清空错误列表？',
    content: `将清除当前会话记录的 ${errors.entries.length} 条客户端错误。`,
    positiveText: '清空错误',
    danger: true,
  })
  if (confirmed) {
    errors.clear()
    expandedErrorID.value = null
  }
}

/**
 * 把一条错误记录展开成可展示的详情文本。
 * @param entry - 错误中心条目
 * @returns 详情文本
 */
function errorDetails(entry: ErrorEntry): string {
  const error = entry.error
  if (error instanceof ApplicationError) {
    return [
      `错误码: ${error.code}`,
      `技术信息: ${error.technicalMessage}`,
      `可重试: ${error.retryable ? '是' : '否'}`,
      `Details: ${JSON.stringify(error.details, null, 2)}`,
      error.stack ? `Stack:\n${error.stack}` : '',
    ]
      .filter(Boolean)
      .join('\n')
  }
  if (error instanceof Error) return error.stack || error.message
  try {
    return JSON.stringify(error, null, 2)
  } catch {
    return String(error)
  }
}
</script>

<template>
  <NDrawer
    v-model:show="show"
    :width="640"
    placement="right"
    @update:show="(visible) => !visible && (expandedErrorID = null)"
  >
    <NDrawerContent title="状态中心" closable>
      <NTabs v-model:value="activeTab" type="line" animated>
        <NTabPane :tab="`活动任务 (${operations.active.length})`" name="operations">
          <NFlex vertical :size="12">
            <NAlert v-if="operations.activeError" type="error" title="活动任务加载失败">
              {{
                operations.activeError instanceof Error
                  ? operations.activeError.message
                  : String(operations.activeError)
              }}
            </NAlert>
            <NAlert v-else-if="operations.partialMessage" type="warning">
              {{ operations.partialMessage }}
            </NAlert>
            <NList v-if="operations.active.length" bordered hoverable>
              <NListItem v-for="operation in operations.active" :key="operation.id">
                <NThing>
                  <template #header>
                    <NFlex align="center" :wrap="false">
                      <NText strong>{{ operation.type }}</NText>
                      <NTag size="small" :type="stateType(operation.state)">{{
                        operation.state
                      }}</NTag>
                    </NFlex>
                  </template>
                  <template #header-extra>
                    <NButton size="small" type="warning" @click="cancelOperation(operation)">
                      取消
                    </NButton>
                  </template>
                  <NFlex vertical :size="6">
                    <NText depth="3">
                      {{ operation.targetType }} · {{ operation.stage || '等待阶段' }}
                    </NText>
                    <NProgress
                      :percentage="Math.round(operation.progress * 100)"
                      :show-indicator="true"
                      processing
                    />
                    <NText>{{ operation.message || '等待进度更新…' }}</NText>
                  </NFlex>
                </NThing>
              </NListItem>
            </NList>
            <NEmpty v-else-if="!operations.loading" description="当前没有活动任务" />
            <NFlex justify="end">
              <NButton :loading="operations.loading" @click="operations.refresh">刷新</NButton>
              <NButton type="primary" @click="openOperationsPage">前往任务中心</NButton>
            </NFlex>
          </NFlex>
        </NTabPane>

        <NTabPane :tab="`告警 (${alerts.activeCount})`" name="alerts">
          <NFlex vertical :size="12">
            <NAlert v-if="alerts.error" type="error" title="活动告警加载失败">
              {{ alerts.error instanceof Error ? alerts.error.message : String(alerts.error) }}
            </NAlert>
            <NList v-if="alerts.activeEvents.length" bordered hoverable>
              <NListItem v-for="event in alerts.activeEvents" :key="event.id">
                <NThing>
                  <template #header>
                    <NFlex align="center" :wrap="false">
                      <NText type="error" strong>{{ formatMetricLabel(event.metric) }}</NText>
                      <NTag size="small" :type="event.acknowledgedAt ? 'default' : 'error'">
                        {{ event.acknowledgedAt ? '已确认' : '未确认' }}
                      </NTag>
                    </NFlex>
                  </template>
                  <template #header-extra>
                    <NFlex :wrap="false">
                      <NButton
                        v-if="!event.acknowledgedAt"
                        size="tiny"
                        type="primary"
                        @click="acknowledgeAlertEvent(event)"
                      >
                        确认
                      </NButton>
                      <NButton size="tiny" @click="openAlertMetric(event)">查看指标</NButton>
                      <NButton size="tiny" type="error" quaternary @click="removeAlertEvent(event)">
                        删除
                      </NButton>
                    </NFlex>
                  </template>
                  <NFlex vertical :size="6">
                    <NText>
                      当前值 {{ event.latestValue.toFixed(2) }} / 阈值
                      {{ event.threshold.toFixed(2) }}
                    </NText>
                    <NText depth="3">服务器：{{ event.serverID }}</NText>
                    <NText depth="3">触发：{{ locale.formatDateTime(event.triggeredAt) }}</NText>
                  </NFlex>
                </NThing>
              </NListItem>
            </NList>
            <NEmpty v-else-if="!alerts.loading" description="当前没有活动告警" />
            <NFlex justify="end">
              <NButton :loading="alerts.loading" @click="refreshAlerts">刷新</NButton>
            </NFlex>
          </NFlex>
        </NTabPane>

        <NTabPane :tab="`错误 (${errors.entries.length})`" name="errors">
          <NFlex vertical :size="12">
            <NFlex justify="end">
              <NButton
                type="error"
                secondary
                :disabled="!errors.entries.length"
                @click="clearErrors"
              >
                清空全部
              </NButton>
            </NFlex>
            <NList v-if="errors.entries.length" bordered hoverable>
              <NListItem v-for="entry in errors.entries" :key="entry.id">
                <NThing>
                  <template #header>
                    <NText type="error" strong>{{ entry.source }}</NText>
                  </template>
                  <template #header-extra>
                    <NFlex :wrap="false">
                      <NButton
                        size="tiny"
                        @click="expandedErrorID = expandedErrorID === entry.id ? null : entry.id"
                      >
                        {{ expandedErrorID === entry.id ? '收起' : '详情' }}
                      </NButton>
                      <NButton size="tiny" quaternary @click="errors.dismiss(entry.id)">
                        忽略
                      </NButton>
                    </NFlex>
                  </template>
                  <NFlex vertical :size="6">
                    <NText>{{ entry.message }}</NText>
                    <NText depth="3">{{ locale.formatDateTime(entry.occurredAt) }}</NText>
                    <pre v-if="expandedErrorID === entry.id" class="error-details">{{
                      errorDetails(entry)
                    }}</pre>
                  </NFlex>
                </NThing>
              </NListItem>
            </NList>
            <NEmpty v-else description="当前没有错误" />
          </NFlex>
        </NTabPane>
      </NTabs>
    </NDrawerContent>
  </NDrawer>
</template>

<style scoped>
.error-details {
  max-height: 280px;
  margin: 4px 0 0;
  overflow: auto;
  padding: 10px;
  border: 1px solid var(--border-default);
  border-radius: 8px;
  background: color-mix(in srgb, var(--surface-canvas) 88%, transparent);
  color: var(--text-secondary);
  font:
    12px/1.5 ui-monospace,
    SFMono-Regular,
    Menlo,
    Consolas,
    monospace;
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
