import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import type { SSHSessionDTO } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/models'
import type {
  RemoteFile,
  RemoteTextDocument,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import { ApplicationError } from '../services/api-client'
import {
  chmodRemoteEntry,
  createRemoteDirectory,
  createRemoteFile,
  deleteRemoteEntry,
  extractRemoteZIP,
  listRemoteDirectory,
  pickAndDownloadRemoteFile,
  pickAndUploadRemoteFiles,
  readRemoteText,
  renameRemoteEntry,
  saveRemoteText,
  saveRemoteTextAs,
} from '../services/file-api'
import { listSSHSessions } from '../services/ssh-session-api'
import { useOperationsStore } from './operations'

export const useFilesStore = defineStore('files', () => {
  const operations = useOperationsStore()
  const sessions = ref<SSHSessionDTO[]>([])
  const selectedSessionID = ref('')
  const currentPath = ref('')
  const home = ref('')
  const entries = ref<RemoteFile[]>([])
  const document = ref<RemoteTextDocument | null>(null)
  const loading = ref(false)
  const saving = ref(false)
  const error = ref<unknown>(null)
  const sessionError = ref<unknown>(null)
  const conflictMessage = ref('')
  const history = ref<string[]>([])
  const historyIndex = ref(-1)

  const canBack = computed(() => historyIndex.value > 0)
  const canForward = computed(
    () => historyIndex.value >= 0 && historyIndex.value < history.value.length - 1,
  )
  const partialMessage = computed(() =>
    sessionError.value && sessions.value.length
      ? 'SSH Session 列表刷新失败，继续使用已加载连接。'
      : '',
  )
  const activeTransfers = computed(() =>
    operations.active.filter((operation) => operation.targetType === 'file'),
  )

  /**
   * 加载 SSH Session 列表并进入初始目录。
   * @param preferredSessionID - 优先选中的 SSH Session ID
   * @param initialPath - 初始目录路径
   * @returns 初始化完成后的 Promise
   */
  async function initialize(preferredSessionID = '', initialPath = '~'): Promise<void> {
    loading.value = true
    sessionError.value = null
    try {
      sessions.value = await listSSHSessions()
      if (preferredSessionID) {
        if (!sessions.value.some((item) => item.id === preferredSessionID)) {
          throw new Error('指定的 SSH Session 不存在或当前不可访问')
        }
        selectedSessionID.value = preferredSessionID
      } else if (
        !selectedSessionID.value ||
        !sessions.value.some((item) => item.id === selectedSessionID.value)
      ) {
        selectedSessionID.value = sessions.value[0]?.id ?? ''
      }
      currentPath.value = ''
      home.value = ''
      history.value = []
      historyIndex.value = -1
      document.value = null
      if (selectedSessionID.value) await navigate(initialPath, false)
    } catch (reason) {
      sessionError.value = reason
      error.value = reason
    } finally {
      loading.value = false
    }
  }

  /**
   * 切换目标 SSH Session 并重置浏览历史。
   * @param id - SSH Session ID
   * @returns 切换完成后的 Promise
   */
  async function selectSession(id: string): Promise<void> {
    selectedSessionID.value = id
    currentPath.value = ''
    home.value = ''
    history.value = []
    historyIndex.value = -1
    document.value = null
    await navigate('~', false)
  }

  /**
   * 进入指定远端目录，可选择是否记入前进后退历史。
   * @param path - 目标目录路径
   * @param recordHistory - 是否记入浏览历史
   * @returns 跳转完成后的 Promise
   */
  async function navigate(path: string, recordHistory = true): Promise<void> {
    if (!selectedSessionID.value) return
    loading.value = true
    error.value = null
    try {
      const directory = await listRemoteDirectory(selectedSessionID.value, path, currentPath.value)
      currentPath.value = directory.path
      home.value = directory.home
      entries.value = directory.entries
      if (recordHistory && history.value[historyIndex.value] !== directory.path) {
        history.value = history.value.slice(0, historyIndex.value + 1)
        history.value.push(directory.path)
        historyIndex.value = history.value.length - 1
      } else if (!recordHistory && history.value.length === 0) {
        history.value = [directory.path]
        historyIndex.value = 0
      }
    } catch (reason) {
      error.value = reason
      throw reason
    } finally {
      loading.value = false
    }
  }

  /**
   * 回到浏览历史中的上一个目录。
   * @returns 跳转完成后的 Promise
   */
  async function back(): Promise<void> {
    if (!canBack.value) return
    historyIndex.value--
    const path = history.value[historyIndex.value]
    if (path !== undefined) await navigate(path, false)
  }

  /**
   * 前进到浏览历史中的下一个目录。
   * @returns 跳转完成后的 Promise
   */
  async function forward(): Promise<void> {
    if (!canForward.value) return
    historyIndex.value++
    const path = history.value[historyIndex.value]
    if (path !== undefined) await navigate(path, false)
  }

  /**
   * 进入上级目录。
   * @returns 跳转完成后的 Promise
   */
  async function up(): Promise<void> {
    if (!currentPath.value || currentPath.value === '/') return
    const parent = currentPath.value.slice(0, currentPath.value.lastIndexOf('/')) || '/'
    await navigate(parent)
  }

  /**
   * 打开远端条目：目录则进入，文件则载入文本编辑器。
   * @param entry - 目标远端条目
   * @returns 打开完成后的 Promise
   */
  async function open(entry: RemoteFile): Promise<void> {
    if (entry.kind === 'directory') {
      await navigate(entry.path)
      return
    }
    if (entry.kind !== 'file') return
    loading.value = true
    error.value = null
    conflictMessage.value = ''
    try {
      document.value = await readRemoteText(selectedSessionID.value, entry.path, currentPath.value)
    } catch (reason) {
      error.value = reason
      throw reason
    } finally {
      loading.value = false
    }
  }

  /**
   * 重新读取当前打开的远端文本文件。
   * @returns 重载完成后的 Promise
   */
  async function reloadDocument(): Promise<void> {
    if (!document.value) return
    document.value = await readRemoteText(
      selectedSessionID.value,
      document.value.path,
      currentPath.value,
    )
    conflictMessage.value = ''
  }

  /**
   * 按预期版本保存当前远端文本文件。
   * @param content - 完整文件内容
   * @param expectedVersion - 读取时拿到的版本标识
   * @returns 保存完成后的 Promise
   */
  async function saveDocument(content: string, expectedVersion: string): Promise<void> {
    if (!document.value) return
    saving.value = true
    conflictMessage.value = ''
    try {
      document.value = await saveRemoteText(
        selectedSessionID.value,
        document.value.path,
        currentPath.value,
        content,
        expectedVersion,
      )
      await navigate(currentPath.value, false)
    } catch (reason) {
      if (reason instanceof ApplicationError && reason.code === 'validation.conflict') {
        conflictMessage.value = reason.message
      }
      throw reason
    } finally {
      saving.value = false
    }
  }

  /**
   * 把当前内容另存为远端新文件。
   * @param path - 目标文件路径
   * @param content - 文件内容
   * @returns 保存完成后的 Promise
   */
  async function saveDocumentAs(path: string, content: string): Promise<void> {
    saving.value = true
    try {
      document.value = await saveRemoteTextAs(
        selectedSessionID.value,
        path,
        currentPath.value,
        content,
      )
      conflictMessage.value = ''
      await navigate(currentPath.value, false)
    } finally {
      saving.value = false
    }
  }

  /**
   * 执行一次会改变目录内容的操作，完成后自动刷新列表。
   * @param action - 待执行的异步动作
   * @returns 操作完成后的 Promise
   */
  async function mutate(action: () => Promise<void>): Promise<void> {
    loading.value = true
    try {
      await action()
      await navigate(currentPath.value, false)
    } finally {
      loading.value = false
    }
  }

  /**
   * 选择本地文件并上传到当前远端目录。
   * @returns 上传发起后的 Promise
   */
  async function upload(): Promise<void> {
    if (!selectedSessionID.value) return
    const result = await pickAndUploadRemoteFiles(
      selectedSessionID.value,
      currentPath.value || '~',
      currentPath.value,
    )
    if (!result.cancelled && result.operationID) {
      await operations.refresh()
    }
  }

  /**
   * 选择本地保存位置并下载远端文件。
   * @param entry - 目标远端条目
   * @returns 下载发起后的 Promise
   */
  async function download(entry: RemoteFile): Promise<void> {
    if (!selectedSessionID.value || entry.kind !== 'file') return
    const result = await pickAndDownloadRemoteFile(
      selectedSessionID.value,
      entry.path,
      currentPath.value,
    )
    if (!result.cancelled && result.operationID) {
      await operations.refresh()
    }
  }

  /**
   * 在远端解压 ZIP 压缩包到指定目录。
   * @param entry - 压缩包条目
   * @param destinationPath - 解压目标目录
   * @returns 解压发起后的 Promise
   */
  async function extractZIP(entry: RemoteFile, destinationPath: string): Promise<void> {
    if (!selectedSessionID.value || entry.kind !== 'file') return
    const result = await extractRemoteZIP(
      selectedSessionID.value,
      entry.path,
      destinationPath,
      currentPath.value,
    )
    if (!result.cancelled && result.operationID) await operations.refresh()
  }

  return {
    back,
    activeTransfers,
    canBack,
    canForward,
    chmod: (path: string, mode: string) =>
      mutate(() => chmodRemoteEntry(selectedSessionID.value, path, currentPath.value, mode)),
    closeDocument: () => (document.value = null),
    conflictMessage,
    createDirectory: (path: string) =>
      mutate(() => createRemoteDirectory(selectedSessionID.value, path, currentPath.value)),
    createFile: (path: string) =>
      mutate(() => createRemoteFile(selectedSessionID.value, path, currentPath.value)),
    currentPath,
    deleteEntry: (path: string, recursive: boolean) =>
      mutate(() => deleteRemoteEntry(selectedSessionID.value, path, currentPath.value, recursive)),
    document,
    download,
    entries,
    error,
    extractZIP,
    forward,
    home,
    initialize,
    loading,
    navigate,
    open,
    partialMessage,
    reloadDocument,
    rename: (source: string, target: string) =>
      mutate(() => renameRemoteEntry(selectedSessionID.value, source, target, currentPath.value)),
    saveDocument,
    saveDocumentAs,
    saving,
    selectedSessionID,
    selectSession,
    sessionError,
    sessions,
    up,
    upload,
  }
})
