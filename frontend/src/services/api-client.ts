import type { DTO } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/global/apperror/models'
import { localizeError } from '../locales/errors'

/** Client-side representation of one stable backend error with localized and technical messages. */
export class ApplicationError extends Error {
  readonly code: string
  readonly details: { [_ in string]?: unknown }
  readonly retryable: boolean
  readonly technicalMessage: string

  constructor(error: DTO) {
    super(localizeError(error.code, error.message))
    this.name = 'ApplicationError'
    this.code = error.code
    this.details = error.details ?? {}
    this.retryable = error.retryable
    this.technicalMessage = error.message
  }
}

/**
 * 把后端返回的错误 DTO 转换成异常抛出，无错误时直接返回。
 * @param error - 后端错误 DTO，可为空
 * @returns 无返回值
 */
export function throwIfError(error: DTO | null | undefined): void {
  if (error) {
    throw new ApplicationError(error)
  }
}
