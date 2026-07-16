import { defineStore } from 'pinia'
import { ref } from 'vue'

import type {
  JavaRuntime,
  Operation,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import type { JavaCandidate } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/service/models'
import {
  deleteJavaRuntime,
  discoverJavaRuntimes,
  importJavaRuntime,
  installManagedJavaRuntime,
  listJavaRuntimes,
  setDefaultJavaRuntime,
} from '../services/java-runtime-api'
import { getOperation, subscribeOperationProgress } from '../services/operation-api'

export const useJavaRuntimesStore = defineStore('java-runtimes', () => {
  const sshSessionID = ref('')
  const majorVersion = ref(0)
  const runtimes = ref<JavaRuntime[]>([])
  const candidates = ref<JavaCandidate[]>([])
  const loading = ref(false)
  const discovering = ref(false)
  const installing = ref(false)
  const installOperationID = ref('')
  const installOperation = ref<Operation | null>(null)
  const installSSHSessionID = ref('')
  const runtimeError = ref<unknown>(null)
  const discoveryError = ref<unknown>(null)
  const installError = ref<unknown>(null)

  /** Loads persisted Java Runtimes using current filters. */
  async function refresh(): Promise<void> {
    if (!sshSessionID.value) {
      runtimes.value = []
      return
    }
    loading.value = true
    runtimeError.value = null
    try {
      runtimes.value = await listJavaRuntimes(sshSessionID.value, majorVersion.value)
    } catch (reason) {
      runtimeError.value = reason
      throw reason
    } finally {
      loading.value = false
    }
  }

  /** Discovers verified remote Java candidates in policy order. */
  async function discover(): Promise<void> {
    if (!sshSessionID.value) return
    discovering.value = true
    discoveryError.value = null
    try {
      candidates.value = await discoverJavaRuntimes(sshSessionID.value)
    } catch (reason) {
      discoveryError.value = reason
      throw reason
    } finally {
      discovering.value = false
    }
  }

  /** Imports one candidate and refreshes persisted rows. */
  async function importPath(executablePath: string): Promise<JavaRuntime> {
    const javaRuntime = await importJavaRuntime(sshSessionID.value, executablePath)
    await refresh()
    return javaRuntime
  }

  /**
   * Starts one managed remote JDK installation Operation.
   * @param majorVersion - Approved Java major version
   * @param architecture - Linux JDK architecture
   * @returns Started Operation ID
   */
  async function install(majorVersion: number, architecture: string): Promise<string> {
    if (!sshSessionID.value) throw new Error('请先选择 SSH Session')
    if (installing.value) throw new Error('已有 Java 安装 Operation 正在执行')
    installing.value = true
    installError.value = null
    installOperation.value = null
    installOperationID.value = ''
    installSSHSessionID.value = sshSessionID.value
    try {
      const operationID = await installManagedJavaRuntime(
        installSSHSessionID.value,
        majorVersion,
        architecture,
      )
      installOperationID.value = operationID
      void getOperation(operationID)
        .then(applyInstallOperation)
        .catch(() => undefined)
      return operationID
    } catch (reason) {
      installing.value = false
      installError.value = reason
      throw reason
    }
  }

  /** Sets the default Runtime for the selected SSH Session. */
  async function setDefault(id: string): Promise<void> {
    await setDefaultJavaRuntime(sshSessionID.value, id)
    await refresh()
  }

  /** Deletes a registration and refreshes persisted rows. */
  async function remove(id: string): Promise<void> {
    await deleteJavaRuntime(id)
    await refresh()
  }

  function applyInstallOperation(operation: Operation): void {
    installOperation.value = operation
    if (operation.state === 'pending' || operation.state === 'running') return
    installing.value = false
    if (operation.state === 'failed') {
      installError.value = new Error(
        operation.message || operation.errorCode || 'Java 安装 Operation 执行失败',
      )
    } else {
      installError.value = null
    }
    if (sshSessionID.value === installSSHSessionID.value) void refresh().catch(() => undefined)
  }

  subscribeOperationProgress((operation) => {
    if (operation.id !== installOperationID.value) return
    applyInstallOperation(operation)
  })

  return {
    candidates,
    discover,
    discovering,
    discoveryError,
    importPath,
    install,
    installing,
    installError,
    installOperation,
    installOperationID,
    loading,
    majorVersion,
    refresh,
    remove,
    runtimes,
    runtimeError,
    setDefault,
    sshSessionID,
  }
})
