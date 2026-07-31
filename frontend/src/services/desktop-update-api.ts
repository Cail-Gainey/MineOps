import type { DesktopUpdateStatus } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/service/models'
import {
  Cancel,
  Check,
  Download,
  Restart,
  Status,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/desktopupdateservice'
import { throwIfError } from './api-client'

/**
 * 读取当前桌面更新状态。
 * @returns 桌面更新状态
 */
export async function getDesktopUpdateStatus(): Promise<DesktopUpdateStatus> {
  const result = await Status()
  throwIfError(result.error)
  return result.status
}

/**
 * 立即检查当前通道是否有新版本。
 * @returns 检查后的桌面更新状态
 */
export async function checkDesktopUpdates(): Promise<DesktopUpdateStatus> {
  const result = await Check()
  throwIfError(result.error)
  return result.status
}

/**
 * 下载并校验更新包。
 * @returns 下载后的桌面更新状态
 */
export async function downloadDesktopUpdate(): Promise<DesktopUpdateStatus> {
  const result = await Download()
  throwIfError(result.error)
  return result.status
}

/**
 * 取消正在进行的更新下载。
 * @returns 取消后的桌面更新状态
 */
export async function cancelDesktopUpdate(): Promise<DesktopUpdateStatus> {
  const result = await Cancel()
  throwIfError(result.error)
  return result.status
}

/**
 * 重启应用并安装已准备好的更新。
 * @param discardUnsaved - 是否放弃未保存内容直接重启
 * @returns 重启前的桌面更新状态
 */
export async function restartDesktopUpdate(discardUnsaved: boolean): Promise<DesktopUpdateStatus> {
  const result = await Restart(discardUnsaved)
  throwIfError(result.error)
  return result.status
}
