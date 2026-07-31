import { Events } from '@wailsio/runtime'

import { AlertRule } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import type { AlertEvent } from '../../bindings/github.com/Cail-Gainey/MineOps/internal/model/models'
import {
  Acknowledge,
  CreateRule,
  DeleteEvent,
  DeleteRule,
  ListEvents,
  ListRules,
  UpdateRule,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services/alertservice'
import { throwIfError } from './api-client'

export const alertEventName = 'mineops:alert:event'

/**
 * 列出某台 Server 的全部阈值告警规则。
 * @param serverID - 目标 Server ID
 * @returns 告警规则数组
 */
export async function listAlertRules(serverID: string): Promise<AlertRule[]> {
  const result = await ListRules(serverID, '', false, 500, 0)
  throwIfError(result.error)
  return result.rules
}

/**
 * 创建一条阈值告警规则。
 * @param rule - 待创建的规则
 * @returns 创建后的规则
 */
export async function createAlertRule(rule: AlertRule): Promise<AlertRule> {
  const result = await CreateRule(rule)
  throwIfError(result.error)
  if (!result.rule) throw new Error('AlertService 未返回新建规则')
  return result.rule
}

/**
 * 更新一条已有的阈值告警规则。
 * @param rule - 含 ID 的规则内容
 * @returns 更新后的规则
 */
export async function updateAlertRule(rule: AlertRule): Promise<AlertRule> {
  const result = await UpdateRule(rule)
  throwIfError(result.error)
  if (!result.rule) throw new Error('AlertService 未返回更新规则')
  return result.rule
}

/**
 * 删除一条阈值告警规则。
 * @param ruleID - 规则 ID
 * @returns 删除完成后的 Promise
 */
export async function deleteAlertRule(ruleID: string): Promise<void> {
  const result = await DeleteRule(ruleID)
  throwIfError(result.error)
}

/**
 * 删除一条告警事件记录。
 * @param eventID - 事件 ID
 * @returns 删除完成后的 Promise
 */
export async function deleteAlertEvent(eventID: string): Promise<void> {
  const result = await DeleteEvent(eventID)
  throwIfError(result.error)
}

/**
 * 按状态列出某台 Server 的告警事件。
 * @param serverID - 目标 Server ID
 * @param state - 事件状态，空串表示不过滤
 * @returns 告警事件数组
 */
export async function listAlertEvents(serverID: string, state = ''): Promise<AlertEvent[]> {
  const result = await ListEvents(serverID, state, 500, 0)
  throwIfError(result.error)
  return result.events
}

/**
 * 确认一条告警事件。
 * @param eventID - 事件 ID
 * @returns 确认后的事件
 */
export async function acknowledgeAlert(eventID: string): Promise<AlertEvent> {
  const result = await Acknowledge(eventID)
  throwIfError(result.error)
  if (!result.event) throw new Error('AlertService 未返回确认后的事件')
  return result.event
}

/**
 * 订阅告警事件推送。
 * @param listener - 收到事件时的回调
 * @returns 取消订阅函数
 */
export function subscribeAlertEvents(listener: (event: AlertEvent) => void): () => void {
  return Events.On(alertEventName, (event) => listener(event.data))
}
