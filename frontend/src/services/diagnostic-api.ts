import {
  DiagnosticService,
  type DiagnosticExportResult,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services'
import type { DiagnosticPackage } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/service/models'
import { throwIfError } from './api-client'

/**
 * 导出一份有界、脱敏的 MineOps 诊断包到指定位置。
 * @param destination - 诊断包保存路径
 * @returns 诊断包元信息
 */
export async function exportDiagnosticPackage(destination: string): Promise<DiagnosticPackage> {
  const result: DiagnosticExportResult = await DiagnosticService.Export(destination)
  throwIfError(result.error)
  if (!result.package) throw new Error('DiagnosticService 未返回诊断包信息')
  return result.package
}
