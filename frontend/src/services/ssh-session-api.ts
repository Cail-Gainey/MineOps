import type {
  SSHSessionDTO,
  SSHSessionInput,
  SSHPreflightDTO,
  SSHConnectionTestDTO,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/models'
import {
  Create,
  Delete,
  Get,
  List,
  Preflight,
  TestConnection,
  TestInput,
  Update,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/sshsessionservice'
import { throwIfError } from './api-client'
import { runWithHostKeyTrustConfirmation } from './host-key-trust'

/** Fetches filtered SSH Sessions through generated Wails bindings. */
export async function listSSHSessions(
  search = '',
  group = '',
  favouriteOnly = false,
): Promise<SSHSessionDTO[]> {
  const result = await List(search, group, favouriteOnly, 200, 0)
  throwIfError(result.error)
  return result.sessions
}

/** Fetches one safe SSH Session DTO through generated Wails bindings. */
export async function getSSHSession(id: string): Promise<SSHSessionDTO> {
  const result = await Get(id)
  throwIfError(result.error)
  if (!result.session) throw new Error('SSH Session 响应为空')
  return result.session
}

/** Creates an SSH Session using write-only credential input. */
export async function createSSHSession(input: SSHSessionInput): Promise<SSHSessionDTO> {
  const result = await Create(input)
  throwIfError(result.error)
  if (!result.session) throw new Error('SSH Session 创建响应为空')
  return result.session
}

/** Updates an SSH Session using optional write-only replacement credential input. */
export async function updateSSHSession(id: string, input: SSHSessionInput): Promise<SSHSessionDTO> {
  const result = await Update(id, input)
  throwIfError(result.error)
  if (!result.session) throw new Error('SSH Session 更新响应为空')
  return result.session
}

/** Deletes an unreferenced SSH Session and its credential. */
export async function deleteSSHSession(id: string): Promise<void> {
  const result = await Delete(id)
  throwIfError(result.error)
}

/** Authenticates through the configured SSH route and measures encrypted request round-trip latency. */
export async function preflightSSHSession(id: string): Promise<SSHPreflightDTO> {
  const result = await Preflight(id)
  throwIfError(result.error)
  return result
}

/** Performs SSH handshake, strict host-key verification, authentication, and a no-op command. */
export async function testSSHSessionConnection(id: string): Promise<SSHConnectionTestDTO> {
  return runWithHostKeyTrustConfirmation(async () => {
    const result = await TestConnection(id)
    throwIfError(result.error)
    return result
  })
}

/**
 * @param id - Existing SSH Session ID, or an empty string for a new draft.
 * @param input - Unsaved metadata and write-only credential input.
 * @returns Authenticated SSH handshake and command-channel evidence.
 */
export async function testSSHSessionInput(
  id: string,
  input: SSHSessionInput,
): Promise<SSHConnectionTestDTO> {
  return runWithHostKeyTrustConfirmation(async () => {
    const result = await TestInput(id, input)
    throwIfError(result.error)
    return result
  })
}
