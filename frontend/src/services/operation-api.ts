import { Events } from '@wailsio/runtime'

import {
  OperationService,
  type OperationListResult,
  type OperationResult,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services'
import type { Operation } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import { throwIfError } from './api-client'

export const operationProgressEventName = 'mineops:operation:progress'

/**
 * 抛出 Operation 接口返回的错误并取出列表本体。
 * @param result - OperationService 的原始返回值
 * @returns Operation 数组
 */
function unwrapOperationList(result: OperationListResult): Operation[] {
  throwIfError(result.error)
  return result.operations
}

/**
 * 按 ID 读取一条 Operation。
 * @param id - Operation ID
 * @returns Operation 详情
 */
export async function getOperation(id: string): Promise<Operation> {
  const result: OperationResult = await OperationService.Get(id)
  throwIfError(result.error)
  if (!result.operation) {
    throw new Error('OperationService 未返回 Operation')
  }
  return result.operation
}

/**
 * 列出进行中的 Operation。
 * @param targetID - 目标对象 ID，空串表示全部
 * @returns Operation 数组
 */
export async function listActiveOperations(targetID = ''): Promise<Operation[]> {
  return unwrapOperationList(await OperationService.ListActive(targetID))
}

/**
 * 分页列出已结束的 Operation 历史。
 * @param targetID - 目标对象 ID，空串表示全部
 * @param limit - 单页条数
 * @param offset - 偏移量
 * @returns Operation 数组
 */
export async function listOperationHistory(
  targetID = '',
  limit = 100,
  offset = 0,
): Promise<Operation[]> {
  return unwrapOperationList(await OperationService.ListHistory(targetID, limit, offset))
}

/**
 * 取消一条进行中的 Operation。
 * @param id - Operation ID
 * @returns 取消完成后的 Promise
 */
export async function cancelOperation(id: string): Promise<void> {
  const result = await OperationService.Cancel(id)
  throwIfError(result.error)
}

/**
 * 删除一条 Operation 历史记录。
 * @param id - Operation ID
 * @returns 删除的记录数
 */
export async function deleteOperationHistory(id: string): Promise<number> {
  const result = await OperationService.DeleteHistory(id)
  throwIfError(result.error)
  return result.deleted
}

/**
 * 清空全部已结束的 Operation 历史。
 * @returns 删除的记录数
 */
export async function clearOperationHistory(): Promise<number> {
  const result = await OperationService.ClearHistory()
  throwIfError(result.error)
  return result.deleted
}

/**
 * 订阅 Operation 进度推送。
 * @param listener - 收到进度时的回调
 * @returns 取消订阅函数
 */
export function subscribeOperationProgress(listener: (operation: Operation) => void): () => void {
  return Events.On(operationProgressEventName, (event) => listener(event.data))
}
