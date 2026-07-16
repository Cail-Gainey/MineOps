import {
  ClientErrorReport,
  ErrorService,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services'

export type ErrorContext = 'query' | 'mutation' | 'application' | 'window' | 'promise'

/**
 * @param error - 未处理的客户端错误
 * @param context - 错误来源
 * @returns void
 */
export function reportClientError(error: unknown, context: ErrorContext, route = ''): void {
  console.error(`[MineOps:${context}]`, error)
  const normalized = error instanceof Error ? error : new Error(String(error))
  void ErrorService.ReportClientError(
    new ClientErrorReport({
      name: normalized.name,
      message: normalized.message,
      stack: normalized.stack ?? '',
      source: context,
      route,
    }),
  ).catch((reportError) => console.error('[MineOps:error-reporting]', reportError))
}
