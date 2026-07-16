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

/** Lists persisted Java Runtimes for one SSH Session. */
export async function listJavaRuntimes(
  sshSessionID: string,
  majorVersion = 0,
): Promise<JavaRuntime[]> {
  const result = await List(sshSessionID, majorVersion, 500, 0)
  throwIfError(result.error)
  return result.javaRuntimes
}

/** Discovers and validates remote Java candidates. */
export async function discoverJavaRuntimes(sshSessionID: string): Promise<JavaCandidate[]> {
  const result = await Discover(sshSessionID)
  throwIfError(result.error)
  return result.candidates
}

/** Validates one manually supplied remote Java executable. */
export async function validateJavaRuntime(
  sshSessionID: string,
  executablePath: string,
): Promise<JavaCandidate> {
  const result = await Validate(sshSessionID, executablePath)
  throwIfError(result.error)
  if (!result.candidates[0]) throw new Error('Java 验证响应为空')
  return result.candidates[0]
}

/** Validates and registers one remote Java executable. */
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
 * Starts a managed remote JDK installation.
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

/** Marks one Java Runtime as the SSH Session default. */
export async function setDefaultJavaRuntime(sshSessionID: string, id: string): Promise<void> {
  const result = await SetDefault(sshSessionID, id)
  throwIfError(result.error)
}

/** Deletes one unreferenced Java registration without deleting remote files. */
export async function deleteJavaRuntime(id: string): Promise<void> {
  const result = await Delete(id)
  throwIfError(result.error)
}

/** Recommends the closest compatible Java Runtime for a Minecraft server version. */
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

/** Lists approved provider-neutral Linux JDK artifacts from the configured catalog. */
export async function listJDKArtifacts(
  majorVersion: number,
  architecture: string,
): Promise<JDKArtifact[]> {
  const result = await ListArtifacts(majorVersion, architecture)
  throwIfError(result.error)
  return result.artifacts
}
