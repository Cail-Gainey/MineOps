<script setup lang="ts">
import * as monaco from 'monaco-editor/esm/vs/editor/editor.api.js'
import EditorWorker from 'monaco-editor/esm/vs/editor/editor.worker?worker'
import 'monaco-editor/esm/vs/basic-languages/ini/ini.contribution.js'
import { NButton, NFlex, NTag, NText } from 'naive-ui'
import { storeToRefs } from 'pinia'
import { onMounted, onUnmounted, ref, watch } from 'vue'

import { useThemeStore } from '../../stores/theme'
import { accentColours } from '../../themes/tokens'

const workerScope = self as typeof self & {
  MonacoEnvironment?: { getWorker: () => Worker }
}
workerScope.MonacoEnvironment = {
  getWorker: () => new EditorWorker(),
}

const editorElement = ref<HTMLElement | null>(null)
const dirty = ref(false)
const modelGeneration = ref(0)
const status = ref('等待初始化')

const themeStore = useThemeStore()
const { accent, isDark } = storeToRefs(themeStore)

let editor: monaco.editor.IStandaloneCodeEditor | null = null
let model: monaco.editor.ITextModel | null = null
let savedValue = ''
let changeSubscription: monaco.IDisposable | null = null

const initialValue = `# MineOps Monaco Spike\nserver-port=25565\nmotd=你好，MineOps\n`

function applyTheme(): void {
  monaco.editor.defineTheme('mineops-active', {
    base: isDark.value ? 'vs-dark' : 'vs',
    inherit: true,
    rules: [],
    colors: {
      'editor.background': isDark.value ? '#0f172a' : '#f8fafc',
      'editor.foreground': isDark.value ? '#e2e8f0' : '#0f172a',
      'editorCursor.foreground': accentColours[accent.value],
    },
  })
  monaco.editor.setTheme('mineops-active')
}

function replaceModel(value: string): void {
  changeSubscription?.dispose()
  const previousModel = model
  modelGeneration.value++
  model = monaco.editor.createModel(
    value,
    'ini',
    monaco.Uri.parse(`inmemory://mineops/server-${modelGeneration.value}.properties`),
  )
  editor?.setModel(model)
  previousModel?.dispose()
  savedValue = value
  dirty.value = false
  changeSubscription = model.onDidChangeContent(() => {
    dirty.value = model?.getValue() !== savedValue
  })
  status.value = `Model ${modelGeneration.value} / ${formatBytes(value.length)}`
}

function loadOneMegabyte(): void {
  const line = 'view-distance=10 # MineOps 中文配置与冲突验证\n'
  const value = line.repeat(Math.ceil(1_048_576 / line.length)).slice(0, 1_048_576)
  replaceModel(value)
}

function markSaved(): void {
  savedValue = model?.getValue() ?? ''
  dirty.value = false
  status.value = '已保存当前内存快照'
}

function resetModel(): void {
  replaceModel(initialValue)
}

function formatBytes(bytes: number): string {
  return `${(bytes / 1024).toFixed(1)} KiB`
}

onMounted(() => {
  applyTheme()
  editor = monaco.editor.create(editorElement.value!, {
    automaticLayout: true,
    fontSize: 13,
    minimap: { enabled: true },
    scrollBeyondLastLine: false,
    theme: 'mineops-active',
  })
  replaceModel(initialValue)
  status.value = 'Worker 与编辑器已初始化'
})

watch([isDark, accent], applyTheme)

onUnmounted(() => {
  changeSubscription?.dispose()
  editor?.dispose()
  model?.dispose()
})
</script>

<template>
  <NFlex vertical>
    <NFlex align="center">
      <NButton type="primary" @click="loadOneMegabyte">加载 1MB 文件</NButton>
      <NButton @click="resetModel">重建 Model</NButton>
      <NButton type="success" @click="markSaved">标记已保存</NButton>
      <NTag :type="dirty ? 'warning' : 'success'">{{ dirty ? '有未保存修改' : '已保存' }}</NTag>
    </NFlex>
    <div ref="editorElement" class="editor-host" />
    <NText depth="3">{{ status }}</NText>
  </NFlex>
</template>

<style scoped>
.editor-host {
  width: 100%;
  height: 420px;
  overflow: hidden;
  border: 1px solid var(--n-border-color);
  border-radius: 6px;
}
</style>
