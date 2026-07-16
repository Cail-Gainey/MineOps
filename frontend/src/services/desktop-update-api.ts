import type { DesktopUpdateStatus } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/service/models'
import {
  Cancel,
  Check,
  Download,
  Restart,
  Status,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/desktopupdateservice'
import { throwIfError } from './api-client'

/** Returns the latest Desktop release-check state without starting network activity. */
export async function getDesktopUpdateStatus(): Promise<DesktopUpdateStatus> {
  const result = await Status()
  throwIfError(result.error)
  return result.status
}

/** Checks the configured Desktop release channel without downloading or installing an update. */
export async function checkDesktopUpdates(): Promise<DesktopUpdateStatus> {
  const result = await Check()
  throwIfError(result.error)
  return result.status
}

/** Downloads, verifies, and prepares the available update without restarting. */
export async function downloadDesktopUpdate(): Promise<DesktopUpdateStatus> {
  const result = await Download()
  throwIfError(result.error)
  return result.status
}

/** Cancels the active Desktop update download. */
export async function cancelDesktopUpdate(): Promise<DesktopUpdateStatus> {
  const result = await Cancel()
  throwIfError(result.error)
  return result.status
}

/** Restarts MineOps into the prepared update after optional unsaved-content confirmation. */
export async function restartDesktopUpdate(discardUnsaved: boolean): Promise<DesktopUpdateStatus> {
  const result = await Restart(discardUnsaved)
  throwIfError(result.error)
  return result.status
}
