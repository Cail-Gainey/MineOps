import type { MinecraftServerInput } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/models'
import type { MinecraftServer } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import {
  Create,
  CreateBackup,
  Get,
  HardDeleteRemote,
  HardDeleteRegistration,
  ImportRemote,
  InspectInstallationStatus,
  InspectRemote,
  List,
  ListBackups,
  ListPropertyBackups,
  ReadProperties,
  Restore,
  RestoreBackup,
  RestorePropertyBackup,
  SaveProperties,
  SoftDelete,
  Update,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/minecraftserverservice'
import type {
  RemoteServerInspection,
  ServerBackup,
  ServerInstallationStatus,
  ServerPropertiesBackup,
  ServerPropertiesSnapshot,
  ServerPropertyUpdate,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/service/models'
import { throwIfError } from './api-client'

const installationStatusCache = new Map<string, ServerInstallationStatus>()
const installationStatusRequests = new Map<string, Promise<ServerInstallationStatus>>()

/** Returns the latest in-memory installation status without starting remote inspection. */
export function getCachedMinecraftServerInstallationStatus(
  id: string,
): ServerInstallationStatus | null {
  return installationStatusCache.get(id) ?? null
}

/** Lists Minecraft Servers through generated Wails bindings. */
export async function listMinecraftServers(
  search = '',
  sshSessionID = '',
  group = '',
  tag = '',
  state = '',
  includeDeleted = false,
): Promise<MinecraftServer[]> {
  const result = await List(search, sshSessionID, group, tag, state, includeDeleted, 500, 0)
  throwIfError(result.error)
  return result.servers
}

/** Fetches one Minecraft Server. */
export async function getMinecraftServer(
  id: string,
  includeDeleted = false,
): Promise<MinecraftServer> {
  const result = await Get(id, includeDeleted)
  throwIfError(result.error)
  if (!result.server) throw new Error('Minecraft Server 响应为空')
  return result.server
}

/** Creates one SSH-bound Minecraft Server record. */
export async function createMinecraftServer(input: MinecraftServerInput): Promise<MinecraftServer> {
  const result = await Create(input)
  throwIfError(result.error)
  if (!result.server) throw new Error('Minecraft Server 创建响应为空')
  return result.server
}

/** Performs read-only inspection before importing an existing remote Server. */
export async function inspectRemoteMinecraftServer(
  sshSessionID: string,
  remotePath: string,
): Promise<RemoteServerInspection> {
  const result = await InspectRemote(sshSessionID, remotePath)
  throwIfError(result.error)
  if (!result.inspection) throw new Error('MinecraftServerService 未返回远程检查结果')
  return result.inspection
}

/** Registers an inspected remote Server without changing its directory. */
export async function importRemoteMinecraftServer(
  input: MinecraftServerInput,
): Promise<MinecraftServer> {
  const result = await ImportRemote(input)
  throwIfError(result.error)
  if (!result.server) throw new Error('MinecraftServerService 未返回导入结果')
  return result.server
}

/** Updates Server metadata without changing SSH binding or remote path. */
export async function updateMinecraftServer(
  id: string,
  input: MinecraftServerInput,
): Promise<MinecraftServer> {
  const result = await Update(id, input)
  throwIfError(result.error)
  if (!result.server) throw new Error('Minecraft Server 更新响应为空')
  return result.server
}

/** Soft-deletes Server metadata while preserving remote files. */
export async function softDeleteMinecraftServer(id: string): Promise<void> {
  const result = await SoftDelete(id)
  throwIfError(result.error)
}

/** Restores a soft-deleted Server record. */
export async function restoreMinecraftServer(id: string): Promise<void> {
  const result = await Restore(id)
  throwIfError(result.error)
}

/** Permanently deletes only the registration after exact confirmation. */
export async function hardDeleteMinecraftServerRegistration(
  id: string,
  confirmedName: string,
  confirmedPath: string,
): Promise<void> {
  const result = await HardDeleteRegistration(id, confirmedName, confirmedPath)
  throwIfError(result.error)
}

/** Starts exact-confirmed remote directory and registration deletion. */
export async function hardDeleteRemoteMinecraftServer(
  id: string,
  confirmedName: string,
  confirmedPath: string,
): Promise<string> {
  const result = await HardDeleteRemote(id, confirmedName, confirmedPath)
  throwIfError(result.error)
  if (!result.operationID) throw new Error('MinecraftServerService 未返回硬删除 Operation ID')
  return result.operationID
}

/** Lists managed remote backups for one Server. */
export async function listMinecraftServerBackups(id: string): Promise<ServerBackup[]> {
  const result = await ListBackups(id)
  throwIfError(result.error)
  return result.backups
}

/** Starts a stopped Server directory backup. */
export async function createMinecraftServerBackup(id: string): Promise<string> {
  const result = await CreateBackup(id)
  throwIfError(result.error)
  if (!result.operationID) throw new Error('MinecraftServerService 未返回备份 Operation ID')
  return result.operationID
}

/** Starts safe validation and atomic Server directory restoration. */
export async function restoreMinecraftServerBackup(
  id: string,
  backupPath: string,
): Promise<string> {
  const result = await RestoreBackup(id, backupPath)
  throwIfError(result.error)
  if (!result.operationID) throw new Error('MinecraftServerService 未返回恢复 Operation ID')
  return result.operationID
}

/** Reads raw and structured server.properties state. */
export async function readMinecraftServerProperties(id: string): Promise<ServerPropertiesSnapshot> {
  const result = await ReadProperties(id)
  throwIfError(result.error)
  if (!result.properties?.document)
    throw new Error('MinecraftServerService 未返回 server.properties')
  return result.properties
}

/** Saves raw content or structured key updates with a conflict token. */
export async function saveMinecraftServerProperties(
  id: string,
  mode: 'raw' | 'structured',
  rawContent: string,
  expectedVersion: string,
  updates: ServerPropertyUpdate[] = [],
  firewallConfirmed = false,
): Promise<ServerPropertiesSnapshot> {
  const result = await SaveProperties(
    id,
    mode,
    rawContent,
    expectedVersion,
    updates,
    firewallConfirmed,
  )
  throwIfError(result.error)
  if (!result.properties?.document)
    throw new Error('MinecraftServerService 未返回保存后的 server.properties')
  return result.properties
}

/** Lists retained pre-save server.properties revisions. */
export async function listMinecraftServerPropertyBackups(
  id: string,
): Promise<ServerPropertiesBackup[]> {
  const result = await ListPropertyBackups(id)
  throwIfError(result.error)
  return result.backups
}

/** Restores one retained server.properties revision with conflict detection. */
export async function restoreMinecraftServerPropertyBackup(
  id: string,
  backupPath: string,
  expectedVersion: string,
  firewallConfirmed = false,
): Promise<ServerPropertiesSnapshot> {
  const result = await RestorePropertyBackup(id, backupPath, expectedVersion, firewallConfirmed)
  throwIfError(result.error)
  if (!result.properties?.document)
    throw new Error('MinecraftServerService 未返回恢复后的 server.properties')
  return result.properties
}

/** Compares remote files with Server metadata and the latest Installation checkpoints. */
export async function inspectMinecraftServerInstallationStatus(
  id: string,
  force = false,
): Promise<ServerInstallationStatus> {
  if (!force) {
    const cached = installationStatusCache.get(id)
    if (cached) return cached
    const pending = installationStatusRequests.get(id)
    if (pending) return pending
  }
  const request = (async () => {
    const result = await InspectInstallationStatus(id)
    throwIfError(result.error)
    if (!result.status) throw new Error('MinecraftServerService 未返回安装一致性状态')
    installationStatusCache.set(id, result.status)
    return result.status
  })()
  installationStatusRequests.set(id, request)
  try {
    return await request
  } finally {
    if (installationStatusRequests.get(id) === request) installationStatusRequests.delete(id)
  }
}
