import type { JavaRuntime } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import type { JavaCandidate } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/service/models'
import type { JDKArtifact } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/port/models'
import {
  Delete,
  Discover,
  Import,
  Install,
  List,
  ListArtifacts,
  Recommend,
  SetDefault,
  Validate,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/javaruntimeservice'
import { throwIfError } from './api-client'

/**
 * 列出某个 SSH Session 上已登记的 Java 运行时。
 * @param sshSessionID - SSH Session ID
 * @param majorVersion - 主版本号过滤，0 表示不过滤
 * @returns Java 运行时数组
 */
export async function listJavaRuntimes(
  sshSessionID: string,
  majorVersion = 0,
): Promise<JavaRuntime[]> {
  const result = await List(sshSessionID, majorVersion, 500, 0)
  throwIfError(result.error)
  return result.javaRuntimes
}

/**
 * 在远端主机上探测可用的 Java 候选。
 * @param sshSessionID - SSH Session ID
 * @returns Java 候选数组
 */
export async function discoverJavaRuntimes(sshSessionID: string): Promise<JavaCandidate[]> {
  const result = await Discover(sshSessionID)
  throwIfError(result.error)
  return result.candidates
}

/**
 * 校验远端某个可执行文件是否为可用的 Java。
 * @param sshSessionID - SSH Session ID
 * @param executablePath - 远端可执行文件路径
 * @returns 校验后的 Java 候选
 */
export async function validateJavaRuntime(
  sshSessionID: string,
  executablePath: string,
): Promise<JavaCandidate> {
  const result = await Validate(sshSessionID, executablePath)
  throwIfError(result.error)
  if (!result.candidates[0]) throw new Error('Java 验证响应为空')
  return result.candidates[0]
}

/**
 * 把远端某个 Java 可执行文件登记为运行时。
 * @param sshSessionID - SSH Session ID
 * @param executablePath - 远端可执行文件路径
 * @returns 登记后的 Java 运行时
 */
export async function importJavaRuntime(
  sshSessionID: string,
  executablePath: string,
): Promise<JavaRuntime> {
  const result = await Import(sshSessionID, executablePath)
  throwIfError(result.error)
  if (!result.javaRuntime) throw new Error('Java 导入响应为空')
  return result.javaRuntime
}

/**
 * 发起一次受管的远端 JDK 安装。
 * @param sshSessionID - Target SSH Session ID
 * @param majorVersion - Approved Java major version
 * @param architecture - Linux JDK architecture
 * @returns Started Operation ID
 */
export async function installManagedJavaRuntime(
  sshSessionID: string,
  majorVersion: number,
  architecture: string,
): Promise<string> {
  const result = await Install(sshSessionID, majorVersion, architecture)
  throwIfError(result.error)
  if (!result.operationID) throw new Error('JavaRuntimeService 未返回安装 Operation ID')
  return result.operationID
}

/**
 * 把某个 Java 运行时设为该 SSH Session 的默认项。
 * @param sshSessionID - SSH Session ID
 * @param id - Java 运行时 ID
 * @returns 设置完成后的 Promise
 */
export async function setDefaultJavaRuntime(sshSessionID: string, id: string): Promise<void> {
  const result = await SetDefault(sshSessionID, id)
  throwIfError(result.error)
}

/**
 * 删除一条 Java 运行时登记。
 * @param id - Java 运行时 ID
 * @returns 删除完成后的 Promise
 */
export async function deleteJavaRuntime(id: string): Promise<void> {
  const result = await Delete(id)
  throwIfError(result.error)
}

/**
 * 按服务端类型与 Minecraft 版本推荐合适的 Java 运行时。
 * @param sshSessionID - SSH Session ID
 * @param serverType - 服务端类型
 * @param minecraftVersion - Minecraft 版本
 * @returns 推荐的 Java 运行时
 */
export async function recommendJavaRuntime(
  sshSessionID: string,
  serverType: string,
  minecraftVersion: string,
): Promise<JavaRuntime> {
  const result = await Recommend(sshSessionID, serverType, minecraftVersion)
  throwIfError(result.error)
  if (!result.javaRuntime) throw new Error('Java 推荐响应为空')
  return result.javaRuntime
}

/**
 * 列出可下载安装的 JDK 构件。
 * @param majorVersion - JDK 主版本号
 * @param architecture - 目标架构
 * @returns JDK 构件数组
 */
export async function listJDKArtifacts(
  majorVersion: number,
  architecture: string,
): Promise<JDKArtifact[]> {
  const result = await ListArtifacts(majorVersion, architecture)
  throwIfError(result.error)
  return result.artifacts
}
