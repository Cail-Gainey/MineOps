import type { RemoteProcessIdentity } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import {
  GetState,
  Recover,
  Restart,
  Start,
  Stop,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/lifecycleservice'
import { throwIfError } from './api-client'

export interface LifecycleStateSnapshot {
  state: string
  identity?: RemoteProcessIdentity
}

/** Probes the remote process before returning lifecycle state. */
export async function getLifecycleState(serverID: string): Promise<LifecycleStateSnapshot> {
  const result = await GetState(serverID)
  throwIfError(result.error)
  const snapshot: LifecycleStateSnapshot = { state: result.state }
  if (result.identity) snapshot.identity = result.identity
  return snapshot
}

/** Starts one Server and returns its durable Operation ID. */
export async function startServer(
  serverID: string,
  firewallConfirmed = false,
  tmuxInstallConfirmed = false,
): Promise<string> {
  const result = await Start(serverID, firewallConfirmed, tmuxInstallConfirmed)
  throwIfError(result.error)
  return result.operationID ?? ''
}

/** Stops one Server gracefully or with explicit force escalation. */
export async function stopServer(serverID: string, force = false): Promise<string> {
  const result = await Stop(serverID, force)
  throwIfError(result.error)
  return result.operationID ?? ''
}

/** Restarts one Server under a single resource-locked Operation. */
export async function restartServer(
  serverID: string,
  tmuxInstallConfirmed = false,
): Promise<string> {
  const result = await Restart(serverID, tmuxInstallConfirmed)
  throwIfError(result.error)
  return result.operationID ?? ''
}

/** Reconciles all persisted active process identities. */
export async function recoverServerLifecycles(): Promise<void> {
  const result = await Recover()
  throwIfError(result.error)
}
