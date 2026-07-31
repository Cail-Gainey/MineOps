import type { ObservedHostKeyInput } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/models'
import { ApplicationError } from './api-client'
import { replaceKnownHost, trustFirstKnownHost } from './known-host-api'
import { useInteractionStore } from '../stores/interactions'
import { useNotificationStore } from '../stores/notifications'
import { translate } from '../locales/runtime'

/**
 * 判断错误是否为 SSH 主机指纹被拒绝(首次连接未信任或指纹变更)。
 * @param error - 任意抛出的错误对象。
 * @returns {error is ApplicationError} 是否可进入指纹信任确认流程。
 */
export function isHostKeyRejected(error: unknown): error is ApplicationError {
  return error instanceof ApplicationError && error.code === 'ssh.host_key_rejected'
}

/**
 * 弹出主机指纹确认框(首连信任/指纹变更两种形态),用户确认后把指纹写入 Known Hosts。
 * @param error - 携带 observed host key details 的 host_key_rejected 错误。
 * @returns {Promise<boolean>} 用户确认且指纹保存成功时为 true;取消或保存失败为 false。
 */
export async function confirmAndTrustHostKey(error: ApplicationError): Promise<boolean> {
  const interactions = useInteractionStore()
  const notifications = useNotificationStore()
  const details = error.details
  const decision = String(details.decision ?? '')
  const changed = decision === 'changed'
  const input: ObservedHostKeyInput = {
    hostIdentifier: String(details.hostIdentifier ?? ''),
    host: String(details.host ?? ''),
    port: Number(details.port ?? 0),
    algorithm: String(details.algorithm ?? ''),
    publicKeyBase64: String(details.publicKeyBase64 ?? ''),
    fingerprint: String(details.fingerprint ?? ''),
  }
  const confirmed = await interactions.confirm({
    title: changed ? translate('hostKey.changedTitle') : translate('hostKey.firstTitle'),
    content: changed ? translate('hostKey.changedContent') : translate('hostKey.firstContent'),
    objectLabel: `${input.host}:${input.port} · ${input.algorithm} · ${input.fingerprint}`,
    impact: changed
      ? translate('hostKey.changedImpact', {
          fingerprint: String(details.previousFingerprint ?? translate('common.unknown')),
        })
      : translate('hostKey.firstImpact'),
    positiveText: changed ? translate('hostKey.changedConfirm') : translate('hostKey.firstConfirm'),
    danger: changed,
  })
  if (!confirmed) return false
  try {
    if (changed) await replaceKnownHost(input)
    else await trustFirstKnownHost(input)
    notifications.push({
      kind: changed ? 'warning' : 'success',
      title: changed ? translate('hostKey.replaced') : translate('hostKey.trusted'),
      content: input.fingerprint,
      dedupeKey: `known-host:accepted:${input.hostIdentifier}:${input.fingerprint}`,
    })
    return true
  } catch (reason) {
    notifications.push({
      kind: 'error',
      title: translate('hostKey.saveFailed'),
      content: reason instanceof Error ? reason.message : String(reason),
      dedupeKey: `known-host:accept-error:${input.hostIdentifier}`,
    })
    return false
  }
}

/**
 * 执行 SSH 操作，并在首次指纹或指纹变化被拒绝时弹出确认，保存信任后重试一次。
 * @param {() => Promise<T>} operation - 可能返回 ssh.host_key_rejected 的 SSH 操作。
 * @returns {Promise<T>} 首次执行或确认信任后重试得到的结果。
 */
export async function runWithHostKeyTrustConfirmation<T>(operation: () => Promise<T>): Promise<T> {
  try {
    return await operation()
  } catch (error) {
    if (!isHostKeyRejected(error)) throw error
    const trusted = await confirmAndTrustHostKey(error)
    if (!trusted) {
      throw new Error(translate('hostKey.rejected'), { cause: error })
    }
    return operation()
  }
}
