<script setup lang="ts">
import { LineChart } from 'echarts/charts'
import {
  DataZoomComponent,
  GridComponent,
  TooltipComponent,
  type DataZoomComponentOption,
  type GridComponentOption,
  type TooltipComponentOption,
} from 'echarts/components'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import type { ComposeOption } from 'echarts/core'
import type { LineSeriesOption } from 'echarts/charts'
import { NButton, NFlex, NText } from 'naive-ui'
import { storeToRefs } from 'pinia'
import { computed, onUnmounted, ref } from 'vue'
import VChart from 'vue-echarts'

import { useThemeStore } from '../../stores/theme'
import { accentColours } from '../../themes/tokens'

use([CanvasRenderer, LineChart, GridComponent, TooltipComponent, DataZoomComponent])

type ChartOption = ComposeOption<
  LineSeriesOption | GridComponentOption | TooltipComponentOption | DataZoomComponentOption
>
type MetricPoint = [number, number | null]

const themeStore = useThemeStore()
const { accent, isDark } = storeToRefs(themeStore)
const points = ref<MetricPoint[]>(createPoints(1_000))
const realtimeRunning = ref(false)
const status = ref('1,000 个初始点')
let realtimeTimer: number | null = null

const option = computed<ChartOption>(() => {
  const axisColour = isDark.value ? '#94a3b8' : '#475569'
  const splitColour = isDark.value ? '#334155' : '#e2e8f0'
  return {
    animation: points.value.length < 10_000,
    grid: { left: 48, right: 24, top: 24, bottom: 72 },
    tooltip: { trigger: 'axis' },
    dataZoom: [
      { type: 'inside', filterMode: 'none' },
      { type: 'slider', filterMode: 'none', bottom: 16 },
    ],
    xAxis: {
      type: 'time',
      axisLabel: { color: axisColour },
      axisLine: { lineStyle: { color: axisColour } },
      splitLine: { lineStyle: { color: splitColour } },
    },
    yAxis: {
      type: 'value',
      name: 'TPS',
      min: 0,
      max: 20,
      axisLabel: { color: axisColour },
      axisLine: { lineStyle: { color: axisColour } },
      splitLine: { lineStyle: { color: splitColour } },
    },
    series: [
      {
        type: 'line',
        name: 'TPS',
        data: points.value,
        showSymbol: false,
        connectNulls: false,
        sampling: 'lttb',
        lineStyle: { color: accentColours[accent.value], width: 1.5 },
        itemStyle: { color: accentColours[accent.value] },
      },
    ],
  }
})

function createPoints(count: number): MetricPoint[] {
  const start = Date.now() - count * 1_000
  return Array.from({ length: count }, (_, index) => {
    const value = index % 137 === 0 ? null : 18 + Math.sin(index / 20) * 1.5
    return [start + index * 1_000, value]
  })
}

function loadOneHundredThousandPoints(): void {
  points.value = createPoints(100_000)
  status.value = '100,000 点 / LTTB 降采样 / 包含空洞'
}

function toggleRealtime(): void {
  if (realtimeTimer !== null) {
    window.clearInterval(realtimeTimer)
    realtimeTimer = null
    realtimeRunning.value = false
    status.value = `实时追加已停止 / ${points.value.length} 点`
    return
  }

  realtimeRunning.value = true
  realtimeTimer = window.setInterval(() => {
    const nextIndex = points.value.length + 1
    const nextValue = nextIndex % 40 === 0 ? null : 18 + Math.sin(nextIndex / 8) * 1.5
    points.value = [...points.value.slice(-1_999), [Date.now(), nextValue]]
    status.value = `实时追加中 / ${points.value.length} 点`
  }, 250)
}

onUnmounted(() => {
  if (realtimeTimer !== null) {
    window.clearInterval(realtimeTimer)
  }
})
</script>

<template>
  <NFlex vertical>
    <NFlex>
      <NButton type="primary" @click="loadOneHundredThousandPoints">加载十万点</NButton>
      <NButton :type="realtimeRunning ? 'warning' : 'success'" @click="toggleRealtime">
        {{ realtimeRunning ? '停止实时追加' : '启动实时追加' }}
      </NButton>
    </NFlex>
    <VChart class="chart" :option="option" autoresize />
    <NText depth="3">{{ status }}</NText>
  </NFlex>
</template>

<style scoped>
.chart {
  width: 100%;
  height: 420px;
}
</style>
