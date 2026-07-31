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
import { useLocaleStore } from '../../stores/locale'
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
const locale = useLocaleStore()
const theme = useThemeStore()
const editorElement = ref<HTMLElement | null>(null)
const dirty = ref(false)
const status = computed(() => (dirty.value ? locale.t('editor.dirty') : locale.t('editor.clean')))
const modelInstanceID = crypto.randomUUID()

useUnsavedGuard(
  computed(() => `remote-editor:${modelInstanceID}:${props.document.path}`),
  computed(() => locale.t('editor.unsavedGuard', { path: props.document.path })),
  dirty,
)

let editor: monaco.editor.IStandaloneCodeEditor | null = null
let model: monaco.editor.ITextModel | null = null
let changeSubscription: monaco.IDisposable | null = null
let savedValue = ''

/**
 * 按文件扩展名推断编辑器语法高亮语言。
 * @param filePath - 远端文件路径
 * @returns Monaco 语言标识
 */
function languageForPath(filePath: string): string {
  const extension = filePath.split('.').pop()?.toLowerCase()
  if (extension === 'json') return 'json'
  if (extension === 'js' || extension === 'mjs' || extension === 'cjs') return 'javascript'
  if (extension === 'sh' || extension === 'bash' || extension === 'zsh') return 'shell'
  if (extension === 'properties' || extension === 'ini' || extension === 'conf') return 'ini'
  return 'plaintext'
}

/**
 * 按当前明暗模式注册并应用编辑器主题。
 * @returns 无返回值
 */
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

/**
 * 用新文档替换编辑器模型并重置脏标记。
 * @param document - 远端文本文档
 * @returns 无返回值
 */
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

/**
 * 向外抛出保存事件，携带内容与版本标识。
 * @returns 无返回值
 */
function save(): void {
  emit('save', model?.getValue() ?? '', props.document.versionToken)
}

/**
 * 向外抛出另存为事件，携带当前内容。
 * @returns 无返回值
 */
function saveAs(): void {
  emit('saveAs', model?.getValue() ?? '')
}

/**
 * 请求关闭编辑器；存在未保存修改时先二次确认。
 * @returns 允许关闭时返回 true
 */
async function requestClose(): Promise<boolean> {
  if (dirty.value) {
    const confirmed = await interactions.confirm({
      title: locale.t('editor.closeTitle'),
      content: locale.t('editor.closeContent'),
      objectLabel: props.document.path,
      positiveText: locale.t('editor.closeConfirm'),
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
          {{
            locale.t('editor.metadata', {
              encoding: document.encoding,
              size: document.size,
              status,
            })
          }}
        </NText>
      </div>
      <NFlex>
        <NTag :type="dirty ? 'warning' : 'success'">{{ dirty ? 'Dirty' : 'Saved' }}</NTag>
        <NButton @click="$emit('reload')">{{ locale.t('editor.reload') }}</NButton>
        <NButton @click="saveAs">{{ locale.t('editor.saveAs') }}</NButton>
        <NButton
          type="primary"
          :disabled="!dirty || Boolean(conflictMessage) || saveBlocked"
          :loading="saving"
          @click="save"
        >
          {{ locale.t('common.save') }}
        </NButton>
        <NButton :disabled="closeBlocked" @click="requestClose">
          {{ locale.t('common.close') }}
        </NButton>
      </NFlex>
    </NFlex>
    <NAlert
      v-if="conflictMessage"
      type="warning"
      :title="locale.t('editor.conflictTitle')"
      class="conflict-alert"
    >
      {{ locale.t('editor.conflictContent', { message: conflictMessage }) }}
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
