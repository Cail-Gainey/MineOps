<script setup lang="ts">
import {
  NAlert,
  NButton,
  NFlex,
  NForm,
  NFormItem,
  NInput,
  NModal,
  NProgress,
  NTag,
  NText,
  type DataTableColumns,
  type DropdownOption,
} from 'naive-ui'
import { computed, h, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import type { RemoteFile } from '../../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import AppDataTable from '../../shared/components/AppDataTable.vue'
import { subscribeFileDrop } from '../../services/file-api'
import { useFilesStore } from '../../stores/files'
import { useInteractionStore } from '../../stores/interactions'
import { useLocaleStore } from '../../stores/locale'
import { useNotificationStore } from '../../stores/notifications'
import { useOperationsStore } from '../../stores/operations'
import RemoteTextEditor from './RemoteTextEditor.vue'

const props = withDefaults(
  defineProps<{
    lockedSessionID?: string
    initialPath?: string
    embedded?: boolean
  }>(),
  { lockedSessionID: '', initialPath: '~', embedded: false },
)

const files = useFilesStore()
const interactions = useInteractionStore()
const locale = useLocaleStore()
const notifications = useNotificationStore()
const operations = useOperationsStore()
const route = useRoute()
const router = useRouter()
const dialogMode = ref<'file' | 'directory' | 'rename' | 'chmod' | 'saveAs' | 'extract' | null>(
  null,
)
const dialogValue = ref('')
const dialogContent = ref('')
const selectedEntry = ref<RemoteFile | null>(null)
let unsubscribeFileDrop: (() => void) | null = null

const columns: DataTableColumns<RemoteFile> = [
  {
    title: '名称',
    key: 'name',
    minWidth: 260,
    render: (row) => h(NText, { strong: row.kind === 'directory' }, { default: () => row.name }),
  },
  {
    title: '类型',
    key: 'kind',
    width: 110,
    render: (row) => h(NTag, { bordered: false }, { default: () => row.kind }),
  },
  {
    title: '大小',
    key: 'size',
    width: 120,
    render: (row) => (row.kind === 'directory' ? '—' : formatBytes(row.size)),
  },
  {
    title: '权限',
    key: 'mode',
    width: 100,
    render: (row) => row.mode.toString(8).padStart(4, '0'),
  },
  {
    title: '修改时间',
    key: 'modifiedAt',
    width: 190,
    render: (row) => locale.formatDateTime(row.modifiedAt),
  },
]

const selectedSessionLabel = computed(() => {
  const session = files.sessions.find((item) => item.id === files.selectedSessionID)
  return session ? `${session.name} · ${session.username}@${session.host}:${session.port}` : ''
})

function formatBytes(value: number): string {
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KiB`
  return `${(value / 1024 / 1024).toFixed(1)} MiB`
}

function transferLabel(type: string): string {
  if (type === 'upload') return '上传'
  if (type === 'download') return '下载'
  if (type === 'extract') return '解压'
  return type
}

function contextOptions(row: RemoteFile): DropdownOption[] {
  const options: DropdownOption[] = [
    { label: row.kind === 'directory' ? '打开目录' : '打开文本', key: 'open' },
  ]
  if (row.kind === 'file') options.push({ label: '下载到本地', key: 'download' })
  if (row.kind === 'file' && row.name.toLowerCase().endsWith('.zip'))
    options.push({ label: '安全解压到新目录', key: 'extract' })
  options.push({ label: '重命名', key: 'rename' }, { label: '修改权限', key: 'chmod' })
  options.push({ type: 'divider', key: 'divider' }, { label: '删除', key: 'delete' })
  return options
}

async function openEntry(entry: RemoteFile): Promise<void> {
  try {
    await files.open(entry)
  } catch (error) {
    notifyError('打开远程条目失败', error)
  }
}

function openDialog(
  mode: typeof dialogMode.value,
  entry: RemoteFile | null = null,
  content = '',
): void {
  selectedEntry.value = entry
  dialogMode.value = mode
  dialogContent.value = content
  dialogValue.value =
    mode === 'rename'
      ? (entry?.name ?? '')
      : mode === 'chmod'
        ? (entry?.mode.toString(8).padStart(4, '0') ?? '0644')
        : mode === 'extract'
          ? (entry?.name.replace(/\.zip$/i, '') ?? '')
          : ''
}

async function submitDialog(): Promise<void> {
  const value = dialogValue.value.trim()
  if (!dialogMode.value || !value) return
  try {
    if (dialogMode.value === 'file') await files.createFile(value)
    if (dialogMode.value === 'directory') await files.createDirectory(value)
    if (dialogMode.value === 'rename' && selectedEntry.value)
      await files.rename(selectedEntry.value.path, value)
    if (dialogMode.value === 'chmod' && selectedEntry.value)
      await files.chmod(selectedEntry.value.path, value)
    if (dialogMode.value === 'saveAs') await files.saveDocumentAs(value, dialogContent.value)
    if (dialogMode.value === 'extract' && selectedEntry.value)
      await files.extractZIP(selectedEntry.value, value)
    dialogMode.value = null
  } catch (error) {
    notifyError('远程文件操作失败', error)
  }
}

async function deleteEntry(entry: RemoteFile): Promise<void> {
  const recursive = entry.kind === 'directory'
  const confirmed = await interactions.confirm({
    title: recursive ? '递归删除远程目录？' : '删除远程文件？',
    content: recursive
      ? '目录及全部子项将被永久删除。根目录、Home 和危险路径会由 Go 端拒绝。'
      : '远程文件将被永久删除。',
    objectLabel: entry.path,
    positiveText: '确认删除',
    danger: true,
  })
  if (!confirmed) return
  try {
    await files.deleteEntry(entry.path, recursive)
  } catch (error) {
    notifyError('删除远程条目失败', error)
  }
}

async function uploadFiles(): Promise<void> {
  try {
    await files.upload()
  } catch (error) {
    notifyError('启动文件上传失败', error)
  }
}

async function downloadFile(entry: RemoteFile): Promise<void> {
  try {
    await files.download(entry)
  } catch (error) {
    notifyError('启动文件下载失败', error)
  }
}

function handleContext(key: string | number, entry: RemoteFile): void {
  if (key === 'open') void openEntry(entry)
  if (key === 'download') void downloadFile(entry)
  if (key === 'extract') openDialog('extract', entry)
  if (key === 'rename') openDialog('rename', entry)
  if (key === 'chmod') openDialog('chmod', entry)
  if (key === 'delete') void deleteEntry(entry)
}

async function saveEditor(content: string, versionToken: string): Promise<void> {
  try {
    await files.saveDocument(content, versionToken)
    notifications.push({ kind: 'success', title: '远程文件已保存', dedupeKey: 'files:saved' })
  } catch (error) {
    if (!files.conflictMessage) notifyError('保存远程文件失败', error)
  }
}

function notifyError(title: string, error: unknown): void {
  notifications.push({
    kind: 'error',
    title,
    content: error instanceof Error ? error.message : String(error),
    dedupeKey: `files:${title}`,
  })
}

function runFileAction(title: string, action: () => Promise<void>): void {
  void action().catch((error: unknown) => notifyError(title, error))
}

onMounted(() => {
  unsubscribeFileDrop = subscribeFileDrop((event) => {
    if (event.error) {
      notifications.push({
        kind: 'error',
        title: '拖放上传启动失败',
        content: event.error.message,
        dedupeKey: 'files:drop:error',
      })
      return
    }
    notifications.push({
      kind: 'success',
      title: `已创建 ${event.fileCount} 个文件的上传任务`,
      content: '目录和符号链接会由 Go 端拒绝，传输进度可在当前页或 Operation Center 查看。',
      dedupeKey: `files:drop:${event.operationID}`,
    })
    void operations.refresh()
  })
  const preferredSessionID = props.lockedSessionID || String(route.params.sshSessionID ?? '').trim()
  void files.initialize(preferredSessionID, props.initialPath)
})
onUnmounted(() => unsubscribeFileDrop?.())
</script>

<template>
  <section
    class="files-page"
    :data-file-drop-target="files.selectedSessionID ? 'files-upload' : undefined"
    :data-ssh-session-id="files.selectedSessionID"
    :data-remote-directory="files.currentPath"
  >
    <NFlex v-if="!embedded" align="center" justify="space-between" wrap class="page-header">
      <NFlex align="center">
        <NButton @click="router.push({ name: 'ssh-sessions' })">返回</NButton>
        <NText v-if="selectedSessionLabel" depth="3">{{ selectedSessionLabel }}</NText>
      </NFlex>
      <NText v-if="files.selectedSessionID" depth="3">{{ files.currentPath || initialPath }}</NText>
    </NFlex>
    <NFlex v-else align="center" justify="space-between" wrap>
      <NText depth="3"
        >SSH {{ files.selectedSessionID }} · {{ files.currentPath || initialPath }}</NText
      >
      <NText depth="3">复用当前 Server 连接，支持上传、下载、编辑、解压和拖放。</NText>
    </NFlex>

    <NAlert v-if="files.partialMessage" type="warning" title="部分连接状态不可用">{{
      files.partialMessage
    }}</NAlert>
    <NAlert v-if="!files.sessions.length && !files.loading" type="info" title="尚无 SSH Session"
      >请先创建并验证 SSH Session。</NAlert
    >
    <NAlert v-else-if="files.selectedSessionID" type="info" :show-icon="false">
      可将本地普通文件直接拖放到此页面上传到当前目录；目录和符号链接不会递归上传。
    </NAlert>

    <template v-if="files.selectedSessionID">
      <NFlex wrap>
        <NButton :disabled="!files.canBack" @click="runFileAction('读取远程目录失败', files.back)"
          >后退</NButton
        >
        <NButton
          :disabled="!files.canForward"
          @click="runFileAction('读取远程目录失败', files.forward)"
          >前进</NButton
        >
        <NButton @click="runFileAction('读取远程目录失败', files.up)">上级</NButton>
        <NButton @click="runFileAction('读取远程目录失败', () => files.navigate(files.home || '~'))"
          >Home</NButton
        >
        <NInput
          :value="files.currentPath"
          style="min-width: 280px; flex: 1"
          @update:value="files.currentPath = $event"
          @keyup.enter="runFileAction('读取远程目录失败', () => files.navigate(files.currentPath))"
        />
        <NButton
          :loading="files.loading"
          @click="runFileAction('读取远程目录失败', () => files.navigate(files.currentPath))"
          >刷新</NButton
        >
        <NButton type="primary" @click="uploadFiles">上传文件</NButton>
        <NButton @click="openDialog('file')">新建文件</NButton>
        <NButton @click="openDialog('directory')">新建目录</NButton>
      </NFlex>

      <section v-if="files.activeTransfers.length" class="transfer-list">
        <NText strong>文件传输</NText>
        <div v-for="operation in files.activeTransfers" :key="operation.id" class="transfer-row">
          <div class="transfer-summary">
            <NText>{{ transferLabel(operation.type) }} · {{ operation.message }}</NText>
            <NButton size="tiny" tertiary type="warning" @click="operations.cancel(operation.id)"
              >取消</NButton
            >
          </div>
          <NProgress
            type="line"
            :percentage="Math.round(operation.progress * 100)"
            :status="operation.state === 'failed' ? 'error' : 'default'"
          />
        </div>
      </section>

      <RemoteTextEditor
        v-if="files.document"
        :document="files.document"
        :saving="files.saving"
        :conflict-message="files.conflictMessage"
        @save="saveEditor"
        @save-as="(content) => openDialog('saveAs', null, content)"
        @reload="runFileAction('重新读取远程文件失败', files.reloadDocument)"
        @close="files.closeDocument"
      />
      <AppDataTable
        v-else
        :columns="columns"
        :data="files.entries"
        :loading="files.loading"
        :error="files.error"
        :partial-message="files.partialMessage"
        :context-options="contextOptions"
        :scroll-x="860"
        empty-description="当前目录为空"
        @retry="runFileAction('读取远程目录失败', () => files.navigate(files.currentPath || '~'))"
        @open="openEntry"
        @context-action="handleContext"
      />
    </template>

    <NModal
      :show="Boolean(dialogMode)"
      preset="card"
      :title="
        dialogMode === 'chmod'
          ? '修改权限'
          : dialogMode === 'rename'
            ? '重命名'
            : dialogMode === 'extract'
              ? '安全解压 ZIP'
              : dialogMode === 'directory'
                ? '新建目录'
                : dialogMode === 'saveAs'
                  ? '远程另存为'
                  : '新建文件'
      "
      style="width: min(520px, calc(100vw - 32px))"
      @update:show="(show) => !show && (dialogMode = null)"
    >
      <NForm @submit.prevent="submitDialog">
        <NFormItem :label="dialogMode === 'chmod' ? '八进制权限' : '名称或路径'">
          <NInput v-model:value="dialogValue" autofocus @keyup.enter="submitDialog" />
        </NFormItem>
        <NFlex justify="end"
          ><NButton @click="dialogMode = null">取消</NButton
          ><NButton type="primary" :loading="files.loading || files.saving" @click="submitDialog"
            >确认</NButton
          ></NFlex
        >
      </NForm>
    </NModal>
  </section>
</template>

<style scoped>
.files-page {
  display: flex;
  min-width: 0;
  min-height: 100%;
  flex-direction: column;
  gap: 16px;
}

.files-page.file-drop-target-active {
  outline: 2px dashed var(--n-color-target, #18a058);
  outline-offset: -8px;
}

.transfer-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px;
  border: 1px solid var(--n-border-color);
  border-radius: 6px;
}

.transfer-row {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.transfer-summary {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
</style>
