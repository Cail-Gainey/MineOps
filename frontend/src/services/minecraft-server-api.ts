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
import { translate } from '../locales/runtime'

const installationStatusCache = new Map<string, ServerInstallationStatus>()
const installationStatusRequests = new Map<string, Promise<ServerInstallationStatus>>()

/**
 * 读取缓存中的安装状态，未缓存时返回 null。
 * @param id - Server ID
 * @returns 缓存的安装状态或 null
 */
export function getCachedMinecraftServerInstallationStatus(
  id: string,
): ServerInstallationStatus | null {
  return installationStatusCache.get(id) ?? null
}

/**
 * 按多个维度筛选并列出 Minecraft Server。
 * @param search - 搜索关键字
 * @param sshSessionID - SSH Session 过滤
 * @param group - 分组过滤
 * @param tag - 标签过滤
 * @param state - 运行状态过滤
 * @param includeDeleted - 是否包含已软删除项
 * @returns Server 数组
 */
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

/**
 * 按 ID 读取一台 Minecraft Server。
 * @param id - Server ID
 * @param includeDeleted - 是否允许读取已软删除项
 * @returns Server 详情
 */
export async function getMinecraftServer(
  id: string,
  includeDeleted = false,
): Promise<MinecraftServer> {
  const result = await Get(id, includeDeleted)
  throwIfError(result.error)
  if (!result.server)
    throw new Error(translate('service.emptyResponse', { subject: 'Minecraft Server' }))
  return result.server
}

/**
 * 创建一台新的 Minecraft Server 登记。
 * @param input - Server 输入内容
 * @returns 创建后的 Server
 */
export async function createMinecraftServer(input: MinecraftServerInput): Promise<MinecraftServer> {
  const result = await Create(input)
  throwIfError(result.error)
  if (!result.server)
    throw new Error(translate('service.emptyResponse', { subject: 'Minecraft Server' }))
  return result.server
}

/**
 * 探测远端目录，识别可导入的 Server 信息。
 * @param sshSessionID - SSH Session ID
 * @param remotePath - 远端 Server 目录
 * @returns 远端 Server 探测结果
 */
export async function inspectRemoteMinecraftServer(
  sshSessionID: string,
  remotePath: string,
): Promise<RemoteServerInspection> {
  const result = await InspectRemote(sshSessionID, remotePath)
  throwIfError(result.error)
  if (!result.inspection)
    throw new Error(
      translate('service.missingField', {
        service: 'MinecraftServerService',
        field: translate('serviceField.inspection'),
      }),
    )
  return result.inspection
}

/**
 * 按探测结果把远端已有 Server 导入登记。
 * @param input - Server 输入内容
 * @returns 导入后的 Server
 */
export async function importRemoteMinecraftServer(
  input: MinecraftServerInput,
): Promise<MinecraftServer> {
  const result = await ImportRemote(input)
  throwIfError(result.error)
  if (!result.server)
    throw new Error(
      translate('service.missingField', {
        service: 'MinecraftServerService',
        field: translate('serviceField.importResult'),
      }),
    )
  return result.server
}

/**
 * 更新一台 Server 的登记信息。
 * @param id - Server ID
 * @param input - Server 输入内容
 * @returns 更新后的 Server
 */
export async function updateMinecraftServer(
  id: string,
  input: MinecraftServerInput,
): Promise<MinecraftServer> {
  const result = await Update(id, input)
  throwIfError(result.error)
  if (!result.server)
    throw new Error(translate('service.emptyResponse', { subject: 'Minecraft Server' }))
  return result.server
}

/**
 * 软删除一台 Server，保留远端文件。
 * @param id - Server ID
 * @returns 删除完成后的 Promise
 */
export async function softDeleteMinecraftServer(id: string): Promise<void> {
  const result = await SoftDelete(id)
  throwIfError(result.error)
}

/**
 * 恢复一台已软删除的 Server。
 * @param id - Server ID
 * @returns 恢复完成后的 Promise
 */
export async function restoreMinecraftServer(id: string): Promise<void> {
  const result = await Restore(id)
  throwIfError(result.error)
}

/**
 * 仅删除本地登记，保留远端文件；需要名称与路径二次确认。
 * @param id - Server ID
 * @param confirmedName - 用户输入的 Server 名称
 * @param confirmedPath - 用户输入的远端路径
 * @returns 删除完成后的 Promise
 */
export async function hardDeleteMinecraftServerRegistration(
  id: string,
  confirmedName: string,
  confirmedPath: string,
): Promise<void> {
  const result = await HardDeleteRegistration(id, confirmedName, confirmedPath)
  throwIfError(result.error)
}

/**
 * 删除登记并清除远端文件；需要名称与路径二次确认。
 * @param id - Server ID
 * @param confirmedName - 用户输入的 Server 名称
 * @param confirmedPath - 用户输入的远端路径
 * @returns 关联的 Operation ID
 */
export async function hardDeleteRemoteMinecraftServer(
  id: string,
  confirmedName: string,
  confirmedPath: string,
): Promise<string> {
  const result = await HardDeleteRemote(id, confirmedName, confirmedPath)
  throwIfError(result.error)
  if (!result.operationID)
    throw new Error(
      translate('service.missingField', {
        service: 'MinecraftServerService',
        field: translate('serviceField.hardDeleteOperationID'),
      }),
    )
  return result.operationID
}

/**
 * 列出某台 Server 的存档备份。
 * @param id - Server ID
 * @returns 备份数组
 */
export async function listMinecraftServerBackups(id: string): Promise<ServerBackup[]> {
  const result = await ListBackups(id)
  throwIfError(result.error)
  return result.backups
}

/**
 * 为某台 Server 创建一份存档备份。
 * @param id - Server ID
 * @returns 关联的 Operation ID
 */
export async function createMinecraftServerBackup(id: string): Promise<string> {
  const result = await CreateBackup(id)
  throwIfError(result.error)
  if (!result.operationID)
    throw new Error(
      translate('service.missingField', {
        service: 'MinecraftServerService',
        field: translate('serviceField.backupOperationID'),
      }),
    )
  return result.operationID
}

/**
 * 用指定备份恢复某台 Server 的存档。
 * @param id - Server ID
 * @param backupPath - 备份文件路径
 * @returns 关联的 Operation ID
 */
export async function restoreMinecraftServerBackup(
  id: string,
  backupPath: string,
): Promise<string> {
  const result = await RestoreBackup(id, backupPath)
  throwIfError(result.error)
  if (!result.operationID)
    throw new Error(
      translate('service.missingField', {
        service: 'MinecraftServerService',
        field: translate('serviceField.restoreOperationID'),
      }),
    )
  return result.operationID
}

/**
 * 读取 server.properties 及其版本标识。
 * @param id - Server ID
 * @returns 配置快照
 */
export async function readMinecraftServerProperties(id: string): Promise<ServerPropertiesSnapshot> {
  const result = await ReadProperties(id)
  throwIfError(result.error)
  if (!result.properties?.document)
    throw new Error(
      translate('service.missingField', {
        service: 'MinecraftServerService',
        field: 'server.properties',
      }),
    )
  return result.properties
}

/**
 * 按原文或结构化方式保存 server.properties，版本不匹配时拒绝写入。
 * @param id - Server ID
 * @param mode - 保存方式：原文或结构化
 * @param rawContent - 原文模式下的完整内容
 * @param expectedVersion - 读取时拿到的版本标识
 * @param updates - 结构化模式下的字段更新
 * @returns 保存后的配置快照
 */
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
    throw new Error(
      translate('service.missingField', {
        service: 'MinecraftServerService',
        field: translate('serviceField.savedProperties'),
      }),
    )
  return result.properties
}

/**
 * 列出 server.properties 的历史备份。
 * @param id - Server ID
 * @returns 配置备份数组
 */
export async function listMinecraftServerPropertyBackups(
  id: string,
): Promise<ServerPropertiesBackup[]> {
  const result = await ListPropertyBackups(id)
  throwIfError(result.error)
  return result.backups
}

/**
 * 用指定备份恢复 server.properties。
 * @param id - Server ID
 * @param backupPath - 备份文件路径
 * @param expectedVersion - 当前配置的版本标识
 * @param firewallConfirmed - 端口变化时是否已确认同步防火墙
 * @returns 恢复后的配置快照
 */
export async function restoreMinecraftServerPropertyBackup(
  id: string,
  backupPath: string,
  expectedVersion: string,
  firewallConfirmed = false,
): Promise<ServerPropertiesSnapshot> {
  const result = await RestorePropertyBackup(id, backupPath, expectedVersion, firewallConfirmed)
  throwIfError(result.error)
  if (!result.properties?.document)
    throw new Error(
      translate('service.missingField', {
        service: 'MinecraftServerService',
        field: translate('serviceField.restoredProperties'),
      }),
    )
  return result.properties
}

/**
 * 探测某台 Server 的安装完整性，可强制跳过缓存。
 * @param id - Server ID
 * @param force - 是否忽略缓存强制重新探测
 * @returns 安装状态
 */
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
    if (!result.status)
      throw new Error(
        translate('service.missingField', {
          service: 'MinecraftServerService',
          field: translate('serviceField.installationStatus'),
        }),
      )
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
