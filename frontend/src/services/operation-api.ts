import { Events } from '@wailsio/runtime'

import {
  OperationService,
  type OperationListResult,
  type OperationResult,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services'
import type { Operation } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import { throwIfError } from './api-client'

export const operationProgressEventName = 'mineops:operation:progress'

function unwrapOperationList(result: OperationListResult): Operation[] {
  throwIfError(result.error)
  return result.operations
}

export async function getOperation(id: string): Promise<Operation> {
  const result: OperationResult = await OperationService.Get(id)
  throwIfError(result.error)
  if (!result.operation) {
    throw new Error('OperationService 未返回 Operation')
  }
  return result.operation
}

export async function listActiveOperations(targetID = ''): Promise<Operation[]> {
  return unwrapOperationList(await OperationService.ListActive(targetID))
}

export async function listOperationHistory(
  targetID = '',
  limit = 100,
  offset = 0,
): Promise<Operation[]> {
  return unwrapOperationList(await OperationService.ListHistory(targetID, limit, offset))
}

export async function cancelOperation(id: string): Promise<void> {
  const result = await OperationService.Cancel(id)
  throwIfError(result.error)
}

/** Deletes one terminal Operation history row. */
export async function deleteOperationHistory(id: string): Promise<number> {
  const result = await OperationService.DeleteHistory(id)
  throwIfError(result.error)
  return result.deleted
}

/** Clears every terminal Operation while preserving active work. */
export async function clearOperationHistory(): Promise<number> {
  const result = await OperationService.ClearHistory()
  throwIfError(result.error)
  return result.deleted
}

export function subscribeOperationProgress(listener: (operation: Operation) => void): () => void {
  return Events.On(operationProgressEventName, (event) => listener(event.data))
}
