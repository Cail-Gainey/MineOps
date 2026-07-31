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
import { hasMessage } from '../../locales/runtime'
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

const columns = computed<DataTableColumns<RemoteFile>>(() => [
  {
    title: locale.t('files.column.name'),
    key: 'name',
    minWidth: 260,
    render: (row) => h(NText, { strong: row.kind === 'directory' }, { default: () => row.name }),
  },
  {
    title: locale.t('files.column.kind'),
    key: 'kind',
    width: 110,
    render: (row) => h(NTag, { bordered: false }, { default: () => row.kind }),
  },
  {
    title: locale.t('files.column.size'),
    key: 'size',
    width: 120,
    render: (row) => (row.kind === 'directory' ? '—' : formatBytes(row.size)),
  },
  {
    title: locale.t('files.column.mode'),
    key: 'mode',
    width: 100,
    render: (row) => row.mode.toString(8).padStart(4, '0'),
  },
  {
    title: locale.t('files.column.modifiedAt'),
    key: 'modifiedAt',
    width: 190,
    render: (row) => locale.formatDateTime(row.modifiedAt),
  },
])

const selectedSessionLabel = computed(() => {
  const session = files.sessions.find((item) => item.id === files.selectedSessionID)
  return session ? `${session.name} · ${session.username}@${session.host}:${session.port}` : ''
})

/**
 * 把字节数换算成合适的存储单位。
 * @param value - 字节数
 * @returns 带单位的容量文本
 */
function formatBytes(value: number): string {
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KiB`
  return `${(value / 1024 / 1024).toFixed(1)} MiB`
}

/**
 * 把传输类型映射成本地化标签。
 * @param type - 传输类型标识
 * @returns 本地化标签，未知类型原样返回
 */
function transferLabel(type: string): string {
  const key = `files.transfer.${type}`
  return hasMessage(key) ? locale.t(key) : type
}

/**
 * 按条目类型构建文件列表的右键菜单项。
 * @param row - 当前行的远端条目
 * @returns 下拉菜单项数组
 */
function contextOptions(row: RemoteFile): DropdownOption[] {
  const options: DropdownOption[] = [
    {
      label:
        row.kind === 'directory' ? locale.t('files.openDirectory') : locale.t('files.openText'),
      key: 'open',
    },
  ]
  if (row.kind === 'file')
    options.push({ label: locale.t('files.downloadToLocal'), key: 'download' })
  if (row.kind === 'file' && row.name.toLowerCase().endsWith('.zip'))
    options.push({ label: locale.t('files.extractSafely'), key: 'extract' })
  options.push(
    { label: locale.t('files.rename'), key: 'rename' },
    { label: locale.t('files.chmod'), key: 'chmod' },
  )
  options.push(
    { type: 'divider', key: 'divider' },
    { label: locale.t('common.delete'), key: 'delete' },
  )
  return options
}

/**
 * 打开远端目录或文本文件。
 * @param entry - 目标远端条目
 * @returns 打开完成后的 Promise
 */
async function openEntry(entry: RemoteFile): Promise<void> {
  try {
    await files.open(entry)
  } catch (error) {
    notifyError(locale.t('files.openFailed'), error)
  }
}

/**
 * 打开新建、重命名或改权限对话框并预填内容。
 * @param mode - 对话框模式
 * @param entry - 关联的远端条目，可为空
 * @param content - 预填内容
 * @returns 无返回值
 */
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

/**
 * 按当前对话框模式提交新建、重命名或改权限操作。
 * @returns 提交完成后的 Promise
 */
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
    notifyError(locale.t('files.operationFailed'), error)
  }
}

/**
 * 二次确认后删除远端文件或目录。
 * @param entry - 目标远端条目
 * @returns 删除完成后的 Promise
 */
async function deleteEntry(entry: RemoteFile): Promise<void> {
  const recursive = entry.kind === 'directory'
  const confirmed = await interactions.confirm({
    title: recursive ? locale.t('files.deleteDirTitle') : locale.t('files.deleteFileTitle'),
    content: recursive ? locale.t('files.deleteDirContent') : locale.t('files.deleteFileContent'),
    objectLabel: entry.path,
    positiveText: locale.t('files.deleteConfirm'),
    danger: true,
  })
  if (!confirmed) return
  try {
    await files.deleteEntry(entry.path, recursive)
  } catch (error) {
    notifyError(locale.t('files.deleteFailed'), error)
  }
}

/**
 * 选择本地文件并上传到当前远端目录。
 * @returns 上传发起后的 Promise
 */
async function uploadFiles(): Promise<void> {
  try {
    await files.upload()
  } catch (error) {
    notifyError(locale.t('files.uploadFailed'), error)
  }
}

/**
 * 选择本地保存位置并下载远端文件。
 * @param entry - 目标远端条目
 * @returns 下载发起后的 Promise
 */
async function downloadFile(entry: RemoteFile): Promise<void> {
  try {
    await files.download(entry)
  } catch (error) {
    notifyError(locale.t('files.downloadFailed'), error)
  }
}

/**
 * 分发文件列表右键菜单选中的动作。
 * @param key - 菜单项 key
 * @param entry - 当前行的远端条目
 * @returns 无返回值
 */
function handleContext(key: string | number, entry: RemoteFile): void {
  if (key === 'open') void openEntry(entry)
  if (key === 'download') void downloadFile(entry)
  if (key === 'extract') openDialog('extract', entry)
  if (key === 'rename') openDialog('rename', entry)
  if (key === 'chmod') openDialog('chmod', entry)
  if (key === 'delete') void deleteEntry(entry)
}

/**
 * 保存远端文本编辑器中的内容。
 * @param content - 完整文件内容
 * @param versionToken - 读取时拿到的版本标识
 * @returns 保存完成后的 Promise
 */
async function saveEditor(content: string, versionToken: string): Promise<void> {
  try {
    await files.saveDocument(content, versionToken)
    notifications.push({
      kind: 'success',
      title: locale.t('files.saved'),
      dedupeKey: 'files:saved',
    })
  } catch (error) {
    if (!files.conflictMessage) notifyError(locale.t('files.saveFailed'), error)
  }
}

/**
 * 推送一条文件操作错误通知。
 * @param title - 通知标题
 * @param error - 捕获到的错误
 * @returns 无返回值
 */
function notifyError(title: string, error: unknown): void {
  notifications.push({
    kind: 'error',
    title,
    content: error instanceof Error ? error.message : String(error),
    dedupeKey: `files:${title}`,
  })
}

/**
 * 执行一个文件操作，失败时统一推送错误通知。
 * @param title - 失败通知的标题
 * @param action - 待执行的异步动作
 * @returns 无返回值
 */
function runFileAction(title: string, action: () => Promise<void>): void {
  void action().catch((error: unknown) => notifyError(title, error))
}

onMounted(() => {
  unsubscribeFileDrop = subscribeFileDrop((event) => {
    if (event.error) {
      notifications.push({
        kind: 'error',
        title: locale.t('files.dropFailed'),
        content: event.error.message,
        dedupeKey: 'files:drop:error',
      })
      return
    }
    notifications.push({
      kind: 'success',
      title: locale.t('files.dropStarted', { count: event.fileCount }),
      content: locale.t('files.dropStartedContent'),
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
        <NButton @click="router.push({ name: 'ssh-sessions' })">
          {{ locale.t('common.back') }}
        </NButton>
        <NText v-if="selectedSessionLabel" depth="3">{{ selectedSessionLabel }}</NText>
      </NFlex>
      <NText v-if="files.selectedSessionID" depth="3">{{ files.currentPath || initialPath }}</NText>
    </NFlex>
    <NFlex v-else align="center" justify="space-between" wrap>
      <NText depth="3"
        >SSH {{ files.selectedSessionID }} · {{ files.currentPath || initialPath }}</NText
      >
      <NText depth="3">{{ locale.t('files.embeddedHint') }}</NText>
    </NFlex>

    <NAlert v-if="files.partialMessage" type="warning" :title="locale.t('files.partialTitle')">{{
      files.partialMessage
    }}</NAlert>
    <NAlert
      v-if="!files.sessions.length && !files.loading"
      type="info"
      :title="locale.t('files.noSessionTitle')"
    >
      {{ locale.t('files.noSessionContent') }}
    </NAlert>
    <NAlert v-else-if="files.selectedSessionID" type="info" :show-icon="false">
      {{ locale.t('files.dropHint') }}
    </NAlert>

    <template v-if="files.selectedSessionID">
      <NFlex wrap>
        <NButton
          :disabled="!files.canBack"
          @click="runFileAction(locale.t('files.readDirFailed'), files.back)"
        >
          {{ locale.t('files.back') }}
        </NButton>
        <NButton
          :disabled="!files.canForward"
          @click="runFileAction(locale.t('files.readDirFailed'), files.forward)"
        >
          {{ locale.t('files.forward') }}
        </NButton>
        <NButton @click="runFileAction(locale.t('files.readDirFailed'), files.up)">
          {{ locale.t('files.up') }}
        </NButton>
        <NButton
          @click="
            runFileAction(locale.t('files.readDirFailed'), () => files.navigate(files.home || '~'))
          "
        >
          Home
        </NButton>
        <NInput
          :value="files.currentPath"
          style="min-width: 280px; flex: 1"
          @update:value="files.currentPath = $event"
          @keyup.enter="
            runFileAction(locale.t('files.readDirFailed'), () => files.navigate(files.currentPath))
          "
        />
        <NButton
          :loading="files.loading"
          @click="
            runFileAction(locale.t('files.readDirFailed'), () => files.navigate(files.currentPath))
          "
        >
          {{ locale.t('common.refresh') }}
        </NButton>
        <NButton type="primary" @click="uploadFiles">{{ locale.t('files.uploadFiles') }}</NButton>
        <NButton @click="openDialog('file')">{{ locale.t('files.newFile') }}</NButton>
        <NButton @click="openDialog('directory')">{{ locale.t('files.newDirectory') }}</NButton>
      </NFlex>

      <section v-if="files.activeTransfers.length" class="transfer-list">
        <NText strong>{{ locale.t('files.transfers') }}</NText>
        <div v-for="operation in files.activeTransfers" :key="operation.id" class="transfer-row">
          <div class="transfer-summary">
            <NText>{{ transferLabel(operation.type) }} · {{ operation.message }}</NText>
            <NButton size="tiny" tertiary type="warning" @click="operations.cancel(operation.id)">
              {{ locale.t('common.cancel') }}
            </NButton>
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
        @reload="runFileAction(locale.t('files.reloadFailed'), files.reloadDocument)"
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
        :empty-description="locale.t('files.empty')"
        @retry="
          runFileAction(locale.t('files.readDirFailed'), () =>
            files.navigate(files.currentPath || '~'),
          )
        "
        @open="openEntry"
        @context-action="handleContext"
      />
    </template>

    <NModal
      :show="Boolean(dialogMode)"
      preset="card"
      :title="
        dialogMode === 'chmod'
          ? locale.t('files.chmod')
          : dialogMode === 'rename'
            ? locale.t('files.rename')
            : dialogMode === 'extract'
              ? locale.t('files.dialog.extract')
              : dialogMode === 'directory'
                ? locale.t('files.newDirectory')
                : dialogMode === 'saveAs'
                  ? locale.t('files.dialog.saveAs')
                  : locale.t('files.newFile')
      "
      style="width: min(520px, calc(100vw - 32px))"
      @update:show="(show) => !show && (dialogMode = null)"
    >
      <NForm @submit.prevent="submitDialog">
        <NFormItem
          :label="
            dialogMode === 'chmod'
              ? locale.t('files.dialog.modeLabel')
              : locale.t('files.dialog.nameLabel')
          "
        >
          <NInput v-model:value="dialogValue" autofocus @keyup.enter="submitDialog" />
        </NFormItem>
        <NFlex justify="end">
          <NButton @click="dialogMode = null">{{ locale.t('common.cancel') }}</NButton>
          <NButton type="primary" :loading="files.loading || files.saving" @click="submitDialog">
            {{ locale.t('common.confirm') }}
          </NButton>
        </NFlex>
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
