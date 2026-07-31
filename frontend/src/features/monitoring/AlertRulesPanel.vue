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
import { hasMessage } from '../../locales/runtime'
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

const metricNames = [
  'host.cpu',
  'host.memory',
  'host.disk.used',
  'process.cpu',
  'process.rss',
  'minecraft.tps',
  'minecraft.mspt',
]
const comparisons = ['greater_than', 'greater_than_or_equal', 'less_than', 'less_than_or_equal']
const metricOptions = computed(() =>
  metricNames.map((value) => ({ label: formatMetricLabel(value), value })),
)
const comparisonOptions = computed(() =>
  comparisons.map((value) => ({ label: comparisonLabel(value), value })),
)
const modalTitle = computed(() =>
  editingRuleID.value ? locale.t('alerts.modalEdit') : locale.t('alerts.modalCreate'),
)

/**
 * 把比较符标识映射成本地化标签。
 * @param comparison - 比较符标识
 * @returns 本地化标签，未知比较符原样返回
 */
function comparisonLabel(comparison: string): string {
  const key = `alerts.comparison.${comparison}`
  return hasMessage(key) ? locale.t(key) : comparison
}

/**
 * 把规则表单恢复为默认值并退出编辑态。
 * @returns 无返回值
 */
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

/**
 * 打开新建阈值规则的表单。
 * @returns 无返回值
 */
function openCreate(): void {
  resetDraft()
  showRuleModal.value = true
}

/**
 * 打开指定阈值规则的编辑表单。
 * @param rule - 待编辑的规则
 * @returns 无返回值
 */
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

/**
 * 按当前是新建还是编辑提交阈值规则。
 * @returns 保存完成后的 Promise
 */
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
      title: editingRuleID.value ? locale.t('alerts.ruleUpdated') : locale.t('alerts.ruleCreated'),
      dedupeKey: 'alert:rule:save',
    })
  } catch (reason) {
    notifications.push({
      kind: 'error',
      title: locale.t('alerts.ruleSaveFailed'),
      content: reason instanceof Error ? reason.message : String(reason),
      dedupeKey: 'alert:rule:save:error',
    })
  }
}

/**
 * 启用或停用一条阈值规则。
 * @param rule - 目标规则
 * @param enabled - 是否启用
 * @returns 切换完成后的 Promise
 */
async function toggleRule(rule: AlertRule, enabled: boolean): Promise<void> {
  await alerts.update(new AlertRule({ ...rule, enabled }))
}

/**
 * 二次确认后删除一条阈值规则。
 * @param rule - 目标规则
 * @returns 删除完成后的 Promise
 */
async function removeRule(rule: AlertRule): Promise<void> {
  const confirmed = await interactions.confirm({
    title: locale.t('alerts.ruleDeleteTitle'),
    content: locale.t('alerts.ruleDeleteContent'),
    objectLabel: rule.name,
    impact: `${formatMetricLabel(rule.metric)} ${comparisonLabel(rule.comparison)} ${rule.threshold}`,
    positiveText: locale.t('alerts.ruleDeleteConfirm'),
    danger: true,
  })
  if (confirmed) await alerts.remove(rule.id)
}

/**
 * 二次确认后删除一条告警记录。
 * @param event - 目标告警事件
 * @returns 删除完成后的 Promise
 */
async function removeEvent(event: AlertEvent): Promise<void> {
  const confirmed = await interactions.confirm({
    title: locale.t('alerts.eventDeleteTitle'),
    content:
      event.state === 'active'
        ? locale.t('alerts.eventDeleteActiveContent')
        : locale.t('alerts.eventDeleteHistoryContent'),
    objectLabel: formatMetricLabel(event.metric),
    impact: locale.t('alerts.eventValueSummary', {
      value: event.latestValue.toFixed(2),
      threshold: event.threshold.toFixed(2),
    }),
    positiveText: locale.t('alerts.eventDeleteConfirm'),
    danger: true,
  })
  if (!confirmed) return
  try {
    await alerts.removeEvent(event.id)
    notifications.push({
      kind: 'success',
      title: locale.t('alerts.eventDeleted'),
      dedupeKey: `alert:event:delete:${event.id}`,
    })
  } catch (reason) {
    notifications.push({
      kind: 'error',
      title: locale.t('alerts.eventDeleteFailed'),
      content: reason instanceof Error ? reason.message : String(reason),
      dedupeKey: `alert:event:delete:error:${event.id}`,
    })
  }
}

/**
 * 跳转到监控页并定位到该告警对应的指标与时刻。
 * @param metric - 指标名称
 * @param timestamp - 告警发生时刻
 * @returns 跳转完成后的 Promise
 */
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
  <NCard :title="locale.t('alerts.cardTitle')">
    <template #header-extra>
      <NButton type="primary" size="small" @click="openCreate">
        {{ locale.t('alerts.newRule') }}
      </NButton>
    </template>
    <NAlert v-if="partialMessage" type="warning" :title="locale.t('alerts.partialTitle')">{{
      partialMessage
    }}</NAlert>
    <NTabs type="line" animated>
      <NTabPane name="active" :tab="locale.t('alerts.tabActive', { count: activeEvents.length })">
        <NTable v-if="activeEvents.length" size="small" striped>
          <thead>
            <tr>
              <th>{{ locale.t('alerts.col.metric') }}</th>
              <th>{{ locale.t('alerts.col.triggerValue') }}</th>
              <th>{{ locale.t('alerts.col.start') }}</th>
              <th>{{ locale.t('alerts.col.acknowledge') }}</th>
              <th>{{ locale.t('common.actions') }}</th>
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
                {{
                  event.acknowledgedAt
                    ? locale.formatDateTime(event.acknowledgedAt)
                    : locale.t('alerts.unacknowledged')
                }}
              </td>
              <td>
                <NFlex>
                  <NButton
                    v-if="!event.acknowledgedAt"
                    text
                    type="primary"
                    @click="alerts.acknowledge(event.id)"
                  >
                    {{ locale.t('common.confirm') }}
                  </NButton>
                  <NButton text @click="jumpToMetric(event.metric, event.triggeredAt)">
                    {{ locale.t('alerts.viewMetric') }}
                  </NButton>
                  <NButton text type="error" @click="removeEvent(event)">
                    {{ locale.t('common.delete') }}
                  </NButton>
                </NFlex>
              </td>
            </tr>
          </tbody>
        </NTable>
        <NAlert
          v-if="eventsError && !activeEvents.length"
          type="error"
          :title="locale.t('alerts.eventsLoadFailed')"
          >{{ eventsError instanceof Error ? eventsError.message : String(eventsError) }}</NAlert
        >
        <NEmpty v-else-if="!loading" :description="locale.t('alerts.noActive')" />
      </NTabPane>

      <NTabPane
        name="history"
        :tab="locale.t('alerts.tabHistory', { count: historicalEvents.length })"
      >
        <NTable v-if="historicalEvents.length" size="small" striped>
          <thead>
            <tr>
              <th>{{ locale.t('alerts.col.metric') }}</th>
              <th>{{ locale.t('alerts.col.triggerValue') }}</th>
              <th>{{ locale.t('alerts.col.triggered') }}</th>
              <th>{{ locale.t('alerts.col.recovered') }}</th>
              <th>{{ locale.t('common.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="event in historicalEvents" :key="event.id">
              <td>{{ formatMetricLabel(event.metric) }}</td>
              <td>{{ event.triggerValue.toFixed(2) }}</td>
              <td>{{ locale.formatDateTime(event.triggeredAt) }}</td>
              <td>{{ event.recoveredAt ? locale.formatDateTime(event.recoveredAt) : '—' }}</td>
              <td>
                <NButton text @click="jumpToMetric(event.metric, event.triggeredAt)">
                  {{ locale.t('alerts.viewMetric') }}
                </NButton>
                <NButton text type="error" @click="removeEvent(event)">
                  {{ locale.t('common.delete') }}
                </NButton>
              </td>
            </tr>
          </tbody>
        </NTable>
        <NAlert
          v-if="eventsError && !historicalEvents.length"
          type="error"
          :title="locale.t('alerts.historyLoadFailed')"
          >{{ eventsError instanceof Error ? eventsError.message : String(eventsError) }}</NAlert
        >
        <NEmpty v-else-if="!loading" :description="locale.t('alerts.noHistory')" />
      </NTabPane>

      <NTabPane name="rules" :tab="locale.t('alerts.tabRules', { count: rules.length })">
        <NTable v-if="rules.length" size="small" striped>
          <thead>
            <tr>
              <th>{{ locale.t('alerts.col.name') }}</th>
              <th>{{ locale.t('alerts.col.condition') }}</th>
              <th>{{ locale.t('alerts.col.duration') }}</th>
              <th>{{ locale.t('alerts.col.cooldown') }}</th>
              <th>{{ locale.t('alerts.col.enabled') }}</th>
              <th>{{ locale.t('common.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="rule in rules" :key="rule.id">
              <td>{{ rule.name }}</td>
              <td>
                {{ formatMetricLabel(rule.metric) }} · {{ comparisonLabel(rule.comparison) }} ·
                {{ rule.threshold }}
              </td>
              <td>{{ locale.t('alerts.seconds', { value: rule.durationSeconds }) }}</td>
              <td>{{ locale.t('alerts.seconds', { value: rule.cooldownSeconds }) }}</td>
              <td>
                <NSwitch
                  :value="rule.enabled"
                  :loading="loading"
                  @update:value="toggleRule(rule, $event)"
                />
              </td>
              <td>
                <NFlex>
                  <NButton text @click="openEdit(rule)">{{ locale.t('common.edit') }}</NButton>
                  <NButton text type="error" @click="removeRule(rule)">
                    {{ locale.t('common.delete') }}
                  </NButton>
                </NFlex>
              </td>
            </tr>
          </tbody>
        </NTable>
        <NAlert
          v-if="rulesError && !rules.length"
          type="error"
          :title="locale.t('alerts.rulesLoadFailed')"
          >{{ rulesError instanceof Error ? rulesError.message : String(rulesError) }}</NAlert
        >
        <NEmpty v-else-if="!loading" :description="locale.t('alerts.noRules')" />
      </NTabPane>
    </NTabs>

    <NModal
      v-model:show="showRuleModal"
      preset="card"
      :title="modalTitle"
      style="width: min(560px, 92vw)"
    >
      <NForm label-placement="top">
        <NFormItem :label="locale.t('alerts.form.name')">
          <NInput
            v-model:value="draft.name"
            :placeholder="locale.t('alerts.form.namePlaceholder')"
          />
        </NFormItem>
        <NFlex :wrap="false">
          <NFormItem :label="locale.t('alerts.form.metric')" style="flex: 1">
            <NSelect v-model:value="draft.metric" :options="metricOptions" />
          </NFormItem>
          <NFormItem :label="locale.t('alerts.form.comparison')" style="flex: 1">
            <NSelect v-model:value="draft.comparison" :options="comparisonOptions" />
          </NFormItem>
        </NFlex>
        <NFlex :wrap="false">
          <NFormItem :label="locale.t('alerts.form.threshold')" style="flex: 1">
            <NInputNumber v-model:value="draft.threshold" />
          </NFormItem>
          <NFormItem :label="locale.t('alerts.form.duration')" style="flex: 1">
            <NInputNumber v-model:value="draft.durationSeconds" :min="0" :max="86400" />
          </NFormItem>
          <NFormItem :label="locale.t('alerts.form.cooldown')" style="flex: 1">
            <NInputNumber v-model:value="draft.cooldownSeconds" :min="0" :max="604800" />
          </NFormItem>
        </NFlex>
        <NFlex align="center" justify="space-between">
          <NFlex align="center">
            <NText>{{ locale.t('alerts.form.enabled') }}</NText>
            <NSwitch v-model:value="draft.enabled" />
          </NFlex>
          <NFlex>
            <NButton @click="showRuleModal = false">{{ locale.t('common.cancel') }}</NButton>
            <NButton type="primary" :loading="loading" @click="saveRule">
              {{ locale.t('common.save') }}
            </NButton>
          </NFlex>
        </NFlex>
      </NForm>
    </NModal>
  </NCard>
</template>
