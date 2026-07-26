import type { ObservedHostKeyInput } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/models'
import { ApplicationError } from './api-client'
import { replaceKnownHost, trustFirstKnownHost } from './known-host-api'
import { useInteractionStore } from '../stores/interactions'
import { useNotificationStore } from '../stores/notifications'

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
    title: changed ? '警告：SSH 主机指纹已变化' : '首次连接：信任 SSH 主机？',
    content: changed
      ? '这可能表示服务器已重装、密钥已轮换，或连接正被中间人攻击。请通过可信渠道核对新指纹。'
      : 'MineOps 尚未保存该主机的密钥。请通过可信渠道核对算法和 SHA256 指纹。',
    objectLabel: `${input.host}:${input.port} · ${input.algorithm} · ${input.fingerprint}`,
    impact: changed
      ? `旧指纹：${String(details.previousFingerprint ?? '未知')}。确认后旧记录会保留为变更历史。`
      : '确认后该指纹会写入 SQLCipher，后续变化将默认拒绝连接。',
    positiveText: changed ? '我已核对，替换指纹' : '我已核对，信任主机',
    danger: changed,
  })
  if (!confirmed) return false
  try {
    if (changed) await replaceKnownHost(input)
    else await trustFirstKnownHost(input)
    notifications.push({
      kind: changed ? 'warning' : 'success',
      title: changed ? 'SSH 主机指纹已替换' : 'SSH 主机已加入信任',
      content: input.fingerprint,
      dedupeKey: `known-host:accepted:${input.hostIdentifier}:${input.fingerprint}`,
    })
    return true
  } catch (reason) {
    notifications.push({
      kind: 'error',
      title: '保存 SSH 主机指纹失败',
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
    if (!trusted) throw new Error('SSH 主机指纹未被信任，连接已停止')
    return operation()
  }
}
