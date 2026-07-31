import { defineStore } from 'pinia'
import { ref } from 'vue'
import { translate } from '../locales/runtime'

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

  /**
   * 重新加载当前 SSH Session 上的 Java 运行时列表。
   * @returns 刷新完成后的 Promise
   */
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

  /**
   * 在远端主机上探测可用的 Java 候选。
   * @returns 探测完成后的 Promise
   */
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

  /**
   * 把远端某个 Java 可执行文件登记为运行时并刷新列表。
   * @param executablePath - 远端可执行文件路径
   * @returns 登记后的 Java 运行时
   */
  async function importPath(executablePath: string): Promise<JavaRuntime> {
    const javaRuntime = await importJavaRuntime(sshSessionID.value, executablePath)
    await refresh()
    return javaRuntime
  }

  /**
   * 发起一次受管的远端 JDK 安装任务。
   * @param majorVersion - Approved Java major version
   * @param architecture - Linux JDK architecture
   * @returns Started Operation ID
   */
  async function install(majorVersion: number, architecture: string): Promise<string> {
    if (!sshSessionID.value) throw new Error(translate('store.selectSSHSessionFirst'))
    if (installing.value) throw new Error(translate('store.javaInstallRunning'))
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

  /**
   * 把某个 Java 运行时设为该主机的默认项。
   * @param id - Java 运行时 ID
   * @returns 设置完成后的 Promise
   */
  async function setDefault(id: string): Promise<void> {
    await setDefaultJavaRuntime(sshSessionID.value, id)
    await refresh()
  }

  /**
   * 删除一条 Java 运行时登记并刷新列表。
   * @param id - Java 运行时 ID
   * @returns 删除完成后的 Promise
   */
  async function remove(id: string): Promise<void> {
    await deleteJavaRuntime(id)
    await refresh()
  }

  /**
   * 跟踪 JDK 安装任务进度，结束后刷新运行时列表。
   * @param operation - 安装任务
   * @returns 无返回值
   */
  function applyInstallOperation(operation: Operation): void {
    installOperation.value = operation
    if (operation.state === 'pending' || operation.state === 'running') return
    installing.value = false
    if (operation.state === 'failed') {
      installError.value = new Error(
        operation.message || operation.errorCode || translate('store.javaInstallFailed'),
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
