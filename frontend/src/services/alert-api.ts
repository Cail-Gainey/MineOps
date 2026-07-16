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

/** Lists all threshold rules for one Server. */
export async function listAlertRules(serverID: string): Promise<AlertRule[]> {
  const result = await ListRules(serverID, '', false, 500, 0)
  throwIfError(result.error)
  return result.rules
}

/** Creates one bounded threshold rule. */
export async function createAlertRule(rule: AlertRule): Promise<AlertRule> {
  const result = await CreateRule(rule)
  throwIfError(result.error)
  if (!result.rule) throw new Error('AlertService 未返回新建规则')
  return result.rule
}

/** Updates one bounded threshold rule. */
export async function updateAlertRule(rule: AlertRule): Promise<AlertRule> {
  const result = await UpdateRule(rule)
  throwIfError(result.error)
  if (!result.rule) throw new Error('AlertService 未返回更新规则')
  return result.rule
}

/** Deletes one rule and recovers any active event. */
export async function deleteAlertRule(ruleID: string): Promise<void> {
  const result = await DeleteRule(ruleID)
  throwIfError(result.error)
}

/** Permanently deletes one durable alert incident. */
export async function deleteAlertEvent(eventID: string): Promise<void> {
  const result = await DeleteEvent(eventID)
  throwIfError(result.error)
}

/** Lists active or historical alert events for one Server. */
export async function listAlertEvents(serverID: string, state = ''): Promise<AlertEvent[]> {
  const result = await ListEvents(serverID, state, 500, 0)
  throwIfError(result.error)
  return result.events
}

/** Marks one alert incident as reviewed. */
export async function acknowledgeAlert(eventID: string): Promise<AlertEvent> {
  const result = await Acknowledge(eventID)
  throwIfError(result.error)
  if (!result.event) throw new Error('AlertService 未返回确认后的事件')
  return result.event
}

/** Subscribes to durable alert incident transitions. */
export function subscribeAlertEvents(listener: (event: AlertEvent) => void): () => void {
  return Events.On(alertEventName, (event) => listener(event.data))
}
