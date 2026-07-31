import { hasMessage, translate } from './runtime'

/**
 * 把稳定的后端错误码解析成当前界面语言的文案。
 * 未登记的错误码回退到后端诊断信息，保证信息不丢失。
 * @param code - Stable Application Error code
 * @param technicalMessage - 后端原始诊断信息
 * @returns 本地化后的用户可读错误文案
 */
export function localizeError(code: string, technicalMessage: string): string {
  const key = `error.${code}`
  if (!hasMessage(key)) return technicalMessage || code
  const localized = translate(key)
  if (technicalMessage && technicalMessage !== localized) {
    return translate('error.withDetail', { message: localized, detail: technicalMessage })
  }
  return localized
}
