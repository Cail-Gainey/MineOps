<script setup lang="ts">
import * as monaco from 'monaco-editor/esm/vs/editor/editor.api.js'
import EditorWorker from 'monaco-editor/esm/vs/editor/editor.worker?worker'
import 'monaco-editor/esm/vs/basic-languages/ini/ini.contribution.js'
import 'monaco-editor/esm/vs/basic-languages/javascript/javascript.contribution.js'
import 'monaco-editor/esm/vs/basic-languages/shell/shell.contribution.js'
import { NAlert, NButton, NFlex, NTag, NText } from 'naive-ui'
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'

import type { RemoteTextDocument } from '../../models/remote-file'
import { useUnsavedGuard } from '../../composables/use-unsaved-guard'
import { useInteractionStore } from '../../stores/interactions'
import { useThemeStore } from '../../stores/theme'

const workerScope = self as typeof self & {
  MonacoEnvironment?: { getWorker: () => Worker }
}
workerScope.MonacoEnvironment = { getWorker: () => new EditorWorker() }

const props = defineProps<{
  document: RemoteTextDocument
  saving?: boolean
  conflictMessage?: string
  saveBlocked?: boolean
  closeBlocked?: boolean
}>()

const emit = defineEmits<{
  save: [content: string, versionToken: string]
  saveAs: [content: string]
  reload: []
  close: []
  dirtyChange: [dirty: boolean]
}>()

const interactions = useInteractionStore()
const theme = useThemeStore()
const editorElement = ref<HTMLElement | null>(null)
const dirty = ref(false)
const status = computed(() => (dirty.value ? '有未保存修改' : '与远端版本一致'))
const modelInstanceID = crypto.randomUUID()

useUnsavedGuard(
  computed(() => `remote-editor:${modelInstanceID}:${props.document.path}`),
  computed(() => `远程文件 ${props.document.path} 有未保存修改`),
  dirty,
)

let editor: monaco.editor.IStandaloneCodeEditor | null = null
let model: monaco.editor.ITextModel | null = null
let changeSubscription: monaco.IDisposable | null = null
let savedValue = ''

function languageForPath(filePath: string): string {
  const extension = filePath.split('.').pop()?.toLowerCase()
  if (extension === 'json') return 'json'
  if (extension === 'js' || extension === 'mjs' || extension === 'cjs') return 'javascript'
  if (extension === 'sh' || extension === 'bash' || extension === 'zsh') return 'shell'
  if (extension === 'properties' || extension === 'ini' || extension === 'conf') return 'ini'
  return 'plaintext'
}

function applyTheme(): void {
  monaco.editor.defineTheme('mineops-remote-editor', {
    base: theme.isDark ? 'vs-dark' : 'vs',
    inherit: true,
    rules: [],
    colors: {
      'editor.background': theme.tokens.editor.background,
      'editor.foreground': theme.tokens.editor.foreground,
      'editor.lineHighlightBackground': theme.tokens.editor.lineHighlight,
      'editor.selectionBackground': theme.tokens.editor.selection,
      'editorCursor.foreground': theme.tokens.accent.primary,
    },
  })
  monaco.editor.setTheme('mineops-remote-editor')
}

function replaceModel(document: RemoteTextDocument): void {
  const documentPath = document.path.startsWith('/') ? document.path : `/${document.path}`
  const uri = monaco.Uri.from({
    scheme: 'inmemory',
    authority: 'mineops',
    path: `/remote/${modelInstanceID}${documentPath}`,
  })
  if (model && !model.isDisposed() && model.uri.toString() === uri.toString()) {
    savedValue = document.content
    if (model.getValue() !== document.content) model.setValue(document.content)
    editor?.setModel(model)
    dirty.value = false
    return
  }

  changeSubscription?.dispose()
  changeSubscription = null
  editor?.setModel(null)
  model?.dispose()
  model = monaco.editor.createModel(document.content, languageForPath(document.path), uri)
  editor?.setModel(model)
  savedValue = document.content
  dirty.value = false
  changeSubscription = model.onDidChangeContent(() => {
    dirty.value = model?.getValue() !== savedValue
  })
}

function save(): void {
  emit('save', model?.getValue() ?? '', props.document.versionToken)
}

function saveAs(): void {
  emit('saveAs', model?.getValue() ?? '')
}

/** Requests editor closure and protects unsaved content with a global confirmation. */
async function requestClose(): Promise<boolean> {
  if (dirty.value) {
    const confirmed = await interactions.confirm({
      title: '关闭未保存的远程文件？',
      content: '当前编辑内容尚未写回远端，关闭后将丢失。',
      objectLabel: props.document.path,
      positiveText: '放弃并关闭',
      danger: true,
    })
    if (!confirmed) return false
  }
  emit('close')
  return true
}

defineExpose({ requestClose })

onMounted(() => {
  applyTheme()
  editor = monaco.editor.create(editorElement.value!, {
    automaticLayout: true,
    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
    fontSize: 13,
    minimap: { enabled: true },
    scrollBeyondLastLine: false,
    theme: 'mineops-remote-editor',
  })
  replaceModel(props.document)
})

watch(() => props.document, replaceModel)
watch(() => theme.tokens, applyTheme, { deep: true })
watch(dirty, (value) => emit('dirtyChange', value), { immediate: true })

onUnmounted(() => {
  changeSubscription?.dispose()
  editor?.dispose()
  model?.dispose()
})
</script>

<template>
  <section class="remote-editor">
    <NFlex align="center" justify="space-between" class="editor-toolbar">
      <div>
        <NText strong>{{ document.path }}</NText>
        <NText depth="3" class="editor-metadata">
          {{ document.encoding }} · {{ document.size }} bytes · {{ status }}
        </NText>
      </div>
      <NFlex>
        <NTag :type="dirty ? 'warning' : 'success'">{{ dirty ? 'Dirty' : 'Saved' }}</NTag>
        <NButton @click="$emit('reload')">重新加载</NButton>
        <NButton @click="saveAs">另存为</NButton>
        <NButton
          type="primary"
          :disabled="!dirty || Boolean(conflictMessage) || saveBlocked"
          :loading="saving"
          @click="save"
        >
          保存
        </NButton>
        <NButton :disabled="closeBlocked" @click="requestClose">关闭</NButton>
      </NFlex>
    </NFlex>
    <NAlert v-if="conflictMessage" type="warning" title="远端文件已变化" class="conflict-alert">
      {{ conflictMessage }} 请重新加载或另存为，MineOps 不会静默覆盖远端修改。
    </NAlert>
    <div ref="editorElement" class="editor-host" />
  </section>
</template>

<style scoped>
.remote-editor {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.editor-toolbar {
  margin-bottom: 8px;
}

.editor-metadata {
  display: block;
}

.conflict-alert {
  margin-bottom: 8px;
}

.editor-host {
  flex: 1;
  min-height: 360px;
  overflow: hidden;
  border: 1px solid var(--border-default);
  border-radius: 6px;
}
</style>
