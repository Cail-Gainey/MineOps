<script setup lang="ts">
import {
  NButton,
  NAlert,
  NCard,
  NEmpty,
  NFlex,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NModal,
  NSelect,
  NSwitch,
  NTable,
  NTabPane,
  NTabs,
  NTag,
  NText,
} from 'naive-ui'
import { storeToRefs } from 'pinia'
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'

import { AlertRule } from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import type { AlertEvent } from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import { useAlertsStore } from '../../stores/alerts'
import { useInteractionStore } from '../../stores/interactions'
import { useLocaleStore } from '../../stores/locale'
import { useNotificationStore } from '../../stores/notifications'
import { useSettingsStore } from '../../stores/settings'
import { formatMetricLabel } from '../../shared/monitoring/metric-labels'

const props = defineProps<{ serverID: string }>()
const alerts = useAlertsStore()
const interactions = useInteractionStore()
const locale = useLocaleStore()
const notifications = useNotificationStore()
const router = useRouter()
const settings = useSettingsStore()
const { activeEvents, eventsError, historicalEvents, loading, partialMessage, rules, rulesError } =
  storeToRefs(alerts)
const showRuleModal = ref(false)
const editingRuleID = ref('')
const draft = reactive({
  name: '',
  metric: 'host.cpu',
  comparison: 'greater_than_or_equal',
  threshold: 90,
  durationSeconds: 60,
  cooldownSeconds: settings.committed?.monitoring.alertCooldownSeconds ?? 300,
  enabled: true,
})

const metricOptions = [
  'host.cpu',
  'host.memory',
  'host.disk.used',
  'process.cpu',
  'process.rss',
  'minecraft.tps',
  'minecraft.mspt',
].map((value) => ({
  label: formatMetricLabel(value),
  value,
}))
const comparisonLabels: Record<string, string> = {
  greater_than: '大于',
  greater_than_or_equal: '大于等于',
  less_than: '小于',
  less_than_or_equal: '小于等于',
}
const comparisonOptions = [
  { label: '大于', value: 'greater_than' },
  { label: '大于等于', value: 'greater_than_or_equal' },
  { label: '小于', value: 'less_than' },
  { label: '小于等于', value: 'less_than_or_equal' },
]
const modalTitle = computed(() => (editingRuleID.value ? '编辑阈值规则' : '新建阈值规则'))

function resetDraft(): void {
  editingRuleID.value = ''
  Object.assign(draft, {
    name: '',
    metric: 'host.cpu',
    comparison: 'greater_than_or_equal',
    threshold: 90,
    durationSeconds: 60,
    cooldownSeconds: settings.committed?.monitoring.alertCooldownSeconds ?? 300,
    enabled: true,
  })
}

function openCreate(): void {
  resetDraft()
  showRuleModal.value = true
}

function openEdit(rule: AlertRule): void {
  editingRuleID.value = rule.id
  Object.assign(draft, {
    name: rule.name,
    metric: rule.metric,
    comparison: rule.comparison,
    threshold: rule.threshold,
    durationSeconds: rule.durationSeconds,
    cooldownSeconds: rule.cooldownSeconds,
    enabled: rule.enabled,
  })
  showRuleModal.value = true
}

async function saveRule(): Promise<void> {
  try {
    const existing = rules.value.find((rule) => rule.id === editingRuleID.value)
    const rule = new AlertRule({
      ...existing,
      id: editingRuleID.value,
      serverID: props.serverID,
      name: draft.name.trim(),
      metric: draft.metric,
      comparison: draft.comparison,
      threshold: draft.threshold,
      durationSeconds: draft.durationSeconds,
      cooldownSeconds: draft.cooldownSeconds,
      enabled: draft.enabled,
    })
    if (editingRuleID.value) await alerts.update(rule)
    else await alerts.create(rule)
    showRuleModal.value = false
    notifications.push({
      kind: 'success',
      title: editingRuleID.value ? '阈值规则已更新' : '阈值规则已创建',
      dedupeKey: 'alert:rule:save',
    })
  } catch (reason) {
    notifications.push({
      kind: 'error',
      title: '保存阈值规则失败',
      content: reason instanceof Error ? reason.message : String(reason),
      dedupeKey: 'alert:rule:save:error',
    })
  }
}

async function toggleRule(rule: AlertRule, enabled: boolean): Promise<void> {
  await alerts.update(new AlertRule({ ...rule, enabled }))
}

async function removeRule(rule: AlertRule): Promise<void> {
  const confirmed = await interactions.confirm({
    title: '删除阈值规则？',
    content: '活动事件会先转为恢复状态，历史事件继续保留。',
    objectLabel: rule.name,
    impact: `${rule.metric} ${rule.comparison} ${rule.threshold}`,
    positiveText: '删除规则',
    danger: true,
  })
  if (confirmed) await alerts.remove(rule.id)
}

async function removeEvent(event: AlertEvent): Promise<void> {
  const confirmed = await interactions.confirm({
    title: '删除告警记录？',
    content:
      event.state === 'active'
        ? '将永久删除当前活动告警；阈值规则保持启用，条件持续满足时可能再次触发。'
        : '将永久删除这条告警历史记录。',
    objectLabel: formatMetricLabel(event.metric),
    impact: `当前值 ${event.latestValue.toFixed(2)} / 阈值 ${event.threshold.toFixed(2)}`,
    positiveText: '删除记录',
    danger: true,
  })
  if (!confirmed) return
  try {
    await alerts.removeEvent(event.id)
    notifications.push({
      kind: 'success',
      title: '告警记录已删除',
      dedupeKey: `alert:event:delete:${event.id}`,
    })
  } catch (reason) {
    notifications.push({
      kind: 'error',
      title: '删除告警记录失败',
      content: reason instanceof Error ? reason.message : String(reason),
      dedupeKey: `alert:event:delete:error:${event.id}`,
    })
  }
}

async function jumpToMetric(metric: string, timestamp: string): Promise<void> {
  await router.push({
    path: '/monitoring',
    query: { serverID: props.serverID, metric, rangeHours: '1', focus: timestamp },
  })
  await nextTick()
  document
    .getElementById('monitoring-metric-history')
    ?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

watch(
  () => props.serverID,
  async (serverID) => {
    if (serverID) await alerts.refresh(serverID)
  },
)

onMounted(async () => {
  alerts.startSubscription()
  if (props.serverID) await alerts.refresh(props.serverID)
})
onUnmounted(() => alerts.stopSubscription())
</script>

<template>
  <NCard title="阈值告警">
    <template #header-extra>
      <NButton type="primary" size="small" @click="openCreate">新建规则</NButton>
    </template>
    <NAlert v-if="partialMessage" type="warning" title="部分告警数据不可用">{{
      partialMessage
    }}</NAlert>
    <NTabs type="line" animated>
      <NTabPane name="active" :tab="`活动 (${activeEvents.length})`">
        <NTable v-if="activeEvents.length" size="small" striped>
          <thead>
            <tr>
              <th>指标</th>
              <th>触发值</th>
              <th>开始</th>
              <th>确认</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="event in activeEvents" :key="event.id">
              <td>
                <NTag type="error">{{ formatMetricLabel(event.metric) }}</NTag>
              </td>
              <td>{{ event.latestValue.toFixed(2) }} / {{ event.threshold.toFixed(2) }}</td>
              <td>{{ locale.formatDateTime(event.triggeredAt) }}</td>
              <td>
                {{ event.acknowledgedAt ? locale.formatDateTime(event.acknowledgedAt) : '未确认' }}
              </td>
              <td>
                <NFlex>
                  <NButton
                    v-if="!event.acknowledgedAt"
                    text
                    type="primary"
                    @click="alerts.acknowledge(event.id)"
                    >确认</NButton
                  >
                  <NButton text @click="jumpToMetric(event.metric, event.triggeredAt)"
                    >查看指标</NButton
                  >
                  <NButton text type="error" @click="removeEvent(event)">删除</NButton>
                </NFlex>
              </td>
            </tr>
          </tbody>
        </NTable>
        <NAlert v-if="eventsError && !activeEvents.length" type="error" title="告警事件加载失败">{{
          eventsError instanceof Error ? eventsError.message : String(eventsError)
        }}</NAlert>
        <NEmpty v-else-if="!loading" description="当前没有活动告警" />
      </NTabPane>

      <NTabPane name="history" :tab="`历史 (${historicalEvents.length})`">
        <NTable v-if="historicalEvents.length" size="small" striped>
          <thead>
            <tr>
              <th>指标</th>
              <th>触发值</th>
              <th>触发</th>
              <th>恢复</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="event in historicalEvents" :key="event.id">
              <td>{{ formatMetricLabel(event.metric) }}</td>
              <td>{{ event.triggerValue.toFixed(2) }}</td>
              <td>{{ locale.formatDateTime(event.triggeredAt) }}</td>
              <td>{{ event.recoveredAt ? locale.formatDateTime(event.recoveredAt) : '—' }}</td>
              <td>
                <NButton text @click="jumpToMetric(event.metric, event.triggeredAt)"
                  >查看指标</NButton
                >
                <NButton text type="error" @click="removeEvent(event)">删除</NButton>
              </td>
            </tr>
          </tbody>
        </NTable>
        <NAlert
          v-if="eventsError && !historicalEvents.length"
          type="error"
          title="告警历史加载失败"
          >{{ eventsError instanceof Error ? eventsError.message : String(eventsError) }}</NAlert
        >
        <NEmpty v-else-if="!loading" description="尚无已恢复告警" />
      </NTabPane>

      <NTabPane name="rules" :tab="`规则 (${rules.length})`">
        <NTable v-if="rules.length" size="small" striped>
          <thead>
            <tr>
              <th>名称</th>
              <th>条件</th>
              <th>持续</th>
              <th>冷却</th>
              <th>启用</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="rule in rules" :key="rule.id">
              <td>{{ rule.name }}</td>
              <td>
                {{ formatMetricLabel(rule.metric) }} ·
                {{ comparisonLabels[rule.comparison] ?? rule.comparison }} · {{ rule.threshold }}
              </td>
              <td>{{ rule.durationSeconds }} 秒</td>
              <td>{{ rule.cooldownSeconds }} 秒</td>
              <td>
                <NSwitch
                  :value="rule.enabled"
                  :loading="loading"
                  @update:value="toggleRule(rule, $event)"
                />
              </td>
              <td>
                <NFlex
                  ><NButton text @click="openEdit(rule)">编辑</NButton
                  ><NButton text type="error" @click="removeRule(rule)">删除</NButton></NFlex
                >
              </td>
            </tr>
          </tbody>
        </NTable>
        <NAlert v-if="rulesError && !rules.length" type="error" title="告警规则加载失败">{{
          rulesError instanceof Error ? rulesError.message : String(rulesError)
        }}</NAlert>
        <NEmpty v-else-if="!loading" description="尚未配置阈值规则" />
      </NTabPane>
    </NTabs>

    <NModal
      v-model:show="showRuleModal"
      preset="card"
      :title="modalTitle"
      style="width: min(560px, 92vw)"
    >
      <NForm label-placement="top">
        <NFormItem label="名称"
          ><NInput v-model:value="draft.name" placeholder="例如：TPS 持续低于 18"
        /></NFormItem>
        <NFlex :wrap="false">
          <NFormItem label="指标" style="flex: 1"
            ><NSelect v-model:value="draft.metric" :options="metricOptions"
          /></NFormItem>
          <NFormItem label="比较符" style="flex: 1"
            ><NSelect v-model:value="draft.comparison" :options="comparisonOptions"
          /></NFormItem>
        </NFlex>
        <NFlex :wrap="false">
          <NFormItem label="阈值" style="flex: 1"
            ><NInputNumber v-model:value="draft.threshold"
          /></NFormItem>
          <NFormItem label="持续秒数" style="flex: 1"
            ><NInputNumber v-model:value="draft.durationSeconds" :min="0" :max="86400"
          /></NFormItem>
          <NFormItem label="冷却秒数" style="flex: 1"
            ><NInputNumber v-model:value="draft.cooldownSeconds" :min="0" :max="604800"
          /></NFormItem>
        </NFlex>
        <NFlex align="center" justify="space-between">
          <NFlex align="center"
            ><NText>启用规则</NText><NSwitch v-model:value="draft.enabled"
          /></NFlex>
          <NFlex
            ><NButton @click="showRuleModal = false">取消</NButton
            ><NButton type="primary" :loading="loading" @click="saveRule">保存</NButton></NFlex
          >
        </NFlex>
      </NForm>
    </NModal>
  </NCard>
</template>
