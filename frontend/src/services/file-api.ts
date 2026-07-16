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

export const fileDropEventName = 'mineops:file-drop'

export async function listRemoteDirectory(
  sshSessionID: string,
  path: string,
  currentDirectory: string,
): Promise<RemoteDirectory> {
  const result = await List(sshSessionID, path, currentDirectory)
  throwIfError(result.error)
  if (!result.directory) throw new Error('FileService 未返回远程目录')
  return result.directory
}

export async function readRemoteText(
  sshSessionID: string,
  path: string,
  currentDirectory: string,
): Promise<RemoteTextDocument> {
  const result = await ReadText(sshSessionID, path, currentDirectory)
  throwIfError(result.error)
  if (!result.document) throw new Error('FileService 未返回远程文本')
  return result.document
}

export async function saveRemoteText(
  sshSessionID: string,
  path: string,
  currentDirectory: string,
  content: string,
  expectedVersion: string,
): Promise<RemoteTextDocument> {
  const result = await SaveText(sshSessionID, path, currentDirectory, content, expectedVersion)
  throwIfError(result.error)
  if (!result.document) throw new Error('FileService 未返回保存后的远程文本')
  return result.document
}

export async function saveRemoteTextAs(
  sshSessionID: string,
  path: string,
  currentDirectory: string,
  content: string,
): Promise<RemoteTextDocument> {
  const result = await SaveTextAs(sshSessionID, path, currentDirectory, content)
  throwIfError(result.error)
  if (!result.document) throw new Error('FileService 未返回另存后的远程文本')
  return result.document
}

export async function createRemoteDirectory(
  sshSessionID: string,
  path: string,
  currentDirectory: string,
): Promise<void> {
  throwIfError((await CreateDirectory(sshSessionID, path, currentDirectory)).error)
}

export async function createRemoteFile(
  sshSessionID: string,
  path: string,
  currentDirectory: string,
): Promise<void> {
  throwIfError((await CreateFile(sshSessionID, path, currentDirectory)).error)
}

export async function renameRemoteEntry(
  sshSessionID: string,
  sourcePath: string,
  targetPath: string,
  currentDirectory: string,
): Promise<void> {
  throwIfError((await Rename(sshSessionID, sourcePath, targetPath, currentDirectory)).error)
}

export async function deleteRemoteEntry(
  sshSessionID: string,
  path: string,
  currentDirectory: string,
  recursive: boolean,
): Promise<void> {
  throwIfError((await Delete(sshSessionID, path, currentDirectory, recursive)).error)
}

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

export function subscribeFileDrop(listener: (event: FileDropEvent) => void): () => void {
  return Events.On(fileDropEventName, (event) => listener(event.data))
}

export type { RemoteFile }
