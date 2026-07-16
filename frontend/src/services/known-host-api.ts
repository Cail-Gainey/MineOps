import type {
  KnownHostDTO,
  ObservedHostKeyInput,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/models'
import {
  Delete,
  List,
  Replace,
  TrustFirst,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/knownhostservice'
import { throwIfError } from './api-client'

/** Lists active and historical Known Hosts through generated Wails bindings. */
export async function listKnownHosts(search = ''): Promise<KnownHostDTO[]> {
  const result = await List(search, 500, 0)
  throwIfError(result.error)
  return result.knownHosts
}

/** Deletes one Known Host record from encrypted SQLite. */
export async function deleteKnownHost(id: string): Promise<void> {
  const result = await Delete(id)
  throwIfError(result.error)
}

/** Persists a separately confirmed first-seen host key. */
export async function trustFirstKnownHost(input: ObservedHostKeyInput): Promise<KnownHostDTO> {
  const result = await TrustFirst(input)
  throwIfError(result.error)
  if (!result.knownHost) throw new Error('Known Host 首次信任响应为空')
  return result.knownHost
}

/** Persists a separately confirmed changed fingerprint while retaining history. */
export async function replaceKnownHost(input: ObservedHostKeyInput): Promise<KnownHostDTO> {
  const result = await Replace(input)
  throwIfError(result.error)
  if (!result.knownHost) throw new Error('Known Host 指纹替换响应为空')
  return result.knownHost
}
