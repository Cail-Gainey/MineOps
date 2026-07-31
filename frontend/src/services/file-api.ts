import { Events } from '@wailsio/runtime'

import type {
  RemoteFile,
  RemoteTextDocument,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import type { RemoteDirectory } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/service/models'
import type { FileDropEvent } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/models'
import {
  Chmod,
  CreateDirectory,
  CreateFile,
  Delete,
  ExtractZIP,
  List,
  PickAndDownload,
  PickAndUpload,
  ReadText,
  Rename,
  SaveText,
  SaveTextAs,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/fileservice'
import { throwIfError } from './api-client'
import { translate } from '../locales/runtime'

export const fileDropEventName = 'mineops:file-drop'

/**
 * 列出远端目录内容。
 * @param sshSessionID - SSH Session ID
 * @param path - 目标路径
 * @param currentDirectory - 当前目录，用于解析相对路径
 * @returns 远端目录内容
 */
export async function listRemoteDirectory(
  sshSessionID: string,
  path: string,
  currentDirectory: string,
): Promise<RemoteDirectory> {
  const result = await List(sshSessionID, path, currentDirectory)
  throwIfError(result.error)
  if (!result.directory)
    throw new Error(
      translate('service.missingField', {
        service: 'FileService',
        field: translate('serviceField.remoteDirectory'),
      }),
    )
  return result.directory
}

/**
 * 读取远端文本文件及其版本标识。
 * @param sshSessionID - SSH Session ID
 * @param path - 文件路径
 * @param currentDirectory - 当前目录，用于解析相对路径
 * @returns 远端文本文档
 */
export async function readRemoteText(
  sshSessionID: string,
  path: string,
  currentDirectory: string,
): Promise<RemoteTextDocument> {
  const result = await ReadText(sshSessionID, path, currentDirectory)
  throwIfError(result.error)
  if (!result.document)
    throw new Error(
      translate('service.missingField', {
        service: 'FileService',
        field: translate('serviceField.remoteText'),
      }),
    )
  return result.document
}

/**
 * 按预期版本保存远端文本文件，版本不匹配时拒绝写入。
 * @param sshSessionID - SSH Session ID
 * @param path - 文件路径
 * @param currentDirectory - 当前目录，用于解析相对路径
 * @param content - 新内容
 * @param expectedVersion - 读取时拿到的版本标识
 * @returns 保存后的远端文本文档
 */
export async function saveRemoteText(
  sshSessionID: string,
  path: string,
  currentDirectory: string,
  content: string,
  expectedVersion: string,
): Promise<RemoteTextDocument> {
  const result = await SaveText(sshSessionID, path, currentDirectory, content, expectedVersion)
  throwIfError(result.error)
  if (!result.document)
    throw new Error(
      translate('service.missingField', {
        service: 'FileService',
        field: translate('serviceField.savedRemoteText'),
      }),
    )
  return result.document
}

/**
 * 把内容另存为远端新文件。
 * @param sshSessionID - SSH Session ID
 * @param path - 目标文件路径
 * @param currentDirectory - 当前目录，用于解析相对路径
 * @param content - 文件内容
 * @returns 保存后的远端文本文档
 */
export async function saveRemoteTextAs(
  sshSessionID: string,
  path: string,
  currentDirectory: string,
  content: string,
): Promise<RemoteTextDocument> {
  const result = await SaveTextAs(sshSessionID, path, currentDirectory, content)
  throwIfError(result.error)
  if (!result.document)
    throw new Error(
      translate('service.missingField', {
        service: 'FileService',
        field: translate('serviceField.savedAsRemoteText'),
      }),
    )
  return result.document
}

/**
 * 在远端创建目录。
 * @param sshSessionID - SSH Session ID
 * @param path - 目标目录路径
 * @param currentDirectory - 当前目录，用于解析相对路径
 * @returns 创建完成后的 Promise
 */
export async function createRemoteDirectory(
  sshSessionID: string,
  path: string,
  currentDirectory: string,
): Promise<void> {
  throwIfError((await CreateDirectory(sshSessionID, path, currentDirectory)).error)
}

/**
 * 在远端创建空文件。
 * @param sshSessionID - SSH Session ID
 * @param path - 目标文件路径
 * @param currentDirectory - 当前目录，用于解析相对路径
 * @returns 创建完成后的 Promise
 */
export async function createRemoteFile(
  sshSessionID: string,
  path: string,
  currentDirectory: string,
): Promise<void> {
  throwIfError((await CreateFile(sshSessionID, path, currentDirectory)).error)
}

/**
 * 重命名或移动远端文件与目录。
 * @param sshSessionID - SSH Session ID
 * @param sourcePath - 源路径
 * @param targetPath - 目标路径
 * @param currentDirectory - 当前目录，用于解析相对路径
 * @returns 重命名完成后的 Promise
 */
export async function renameRemoteEntry(
  sshSessionID: string,
  sourcePath: string,
  targetPath: string,
  currentDirectory: string,
): Promise<void> {
  throwIfError((await Rename(sshSessionID, sourcePath, targetPath, currentDirectory)).error)
}

/**
 * 删除远端文件或目录。
 * @param sshSessionID - SSH Session ID
 * @param path - 目标路径
 * @param currentDirectory - 当前目录，用于解析相对路径
 * @param recursive - 是否递归删除目录
 * @returns 删除完成后的 Promise
 */
export async function deleteRemoteEntry(
  sshSessionID: string,
  path: string,
  currentDirectory: string,
  recursive: boolean,
): Promise<void> {
  throwIfError((await Delete(sshSessionID, path, currentDirectory, recursive)).error)
}

/**
 * 修改远端文件或目录的权限位。
 * @param sshSessionID - SSH Session ID
 * @param path - 目标路径
 * @param currentDirectory - 当前目录，用于解析相对路径
 * @param mode - 八进制权限字符串
 * @returns 修改完成后的 Promise
 */
export async function chmodRemoteEntry(
  sshSessionID: string,
  path: string,
  currentDirectory: string,
  mode: string,
): Promise<void> {
  throwIfError((await Chmod(sshSessionID, path, currentDirectory, mode)).error)
}

export interface FileTransferStart {
  operationID: string
  selectionCount: number
  cancelled: boolean
}

/**
 * 选择本地文件并上传到远端目录。
 * @param sshSessionID - SSH Session ID
 * @param remoteDirectory - 远端目标目录
 * @param currentDirectory - 当前目录，用于解析相对路径
 * @returns 文件传输任务的启动信息
 */
export async function pickAndUploadRemoteFiles(
  sshSessionID: string,
  remoteDirectory: string,
  currentDirectory: string,
): Promise<FileTransferStart> {
  const result = await PickAndUpload(sshSessionID, remoteDirectory, currentDirectory)
  throwIfError(result.error)
  return {
    operationID: result.operationID ?? '',
    selectionCount: result.selectionCount ?? 0,
    cancelled: result.cancelled ?? false,
  }
}

/**
 * 选择本地保存位置并下载远端文件。
 * @param sshSessionID - SSH Session ID
 * @param remotePath - 远端文件路径
 * @param currentDirectory - 当前目录，用于解析相对路径
 * @returns 文件传输任务的启动信息
 */
export async function pickAndDownloadRemoteFile(
  sshSessionID: string,
  remotePath: string,
  currentDirectory: string,
): Promise<FileTransferStart> {
  const result = await PickAndDownload(sshSessionID, remotePath, currentDirectory)
  throwIfError(result.error)
  return {
    operationID: result.operationID ?? '',
    selectionCount: result.selectionCount ?? 0,
    cancelled: result.cancelled ?? false,
  }
}

/**
 * 在远端解压 ZIP 压缩包到指定目录。
 * @param sshSessionID - SSH Session ID
 * @param archivePath - 压缩包路径
 * @param destinationPath - 解压目标目录
 * @param currentDirectory - 当前目录，用于解析相对路径
 * @returns 文件传输任务的启动信息
 */
export async function extractRemoteZIP(
  sshSessionID: string,
  archivePath: string,
  destinationPath: string,
  currentDirectory: string,
): Promise<FileTransferStart> {
  const result = await ExtractZIP(sshSessionID, archivePath, destinationPath, currentDirectory)
  throwIfError(result.error)
  return {
    operationID: result.operationID ?? '',
    selectionCount: result.selectionCount ?? 0,
    cancelled: result.cancelled ?? false,
  }
}

/**
 * 订阅窗口文件拖放事件。
 * @param listener - 收到事件时的回调
 * @returns 取消订阅函数
 */
export function subscribeFileDrop(listener: (event: FileDropEvent) => void): () => void {
  return Events.On(fileDropEventName, (event) => listener(event.data))
}

export type { RemoteFile }
