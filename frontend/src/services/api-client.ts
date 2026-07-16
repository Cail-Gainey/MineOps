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

/** Throws one localized ApplicationError when a Wails result contains an error DTO. */
export function throwIfError(error: DTO | null | undefined): void {
  if (error) {
    throw new ApplicationError(error)
  }
}
