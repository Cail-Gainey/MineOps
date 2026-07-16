import {
  DiagnosticService,
  type DiagnosticExportResult,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services'
import type { DiagnosticPackage } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/service/models'
import { throwIfError } from './api-client'

/** 导出有界、脱敏的 MineOps 诊断包。 */
export async function exportDiagnosticPackage(destination: string): Promise<DiagnosticPackage> {
  const result: DiagnosticExportResult = await DiagnosticService.Export(destination)
  throwIfError(result.error)
  if (!result.package) throw new Error('DiagnosticService 未返回诊断包信息')
  return result.package
}
