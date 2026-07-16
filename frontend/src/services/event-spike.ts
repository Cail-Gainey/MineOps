import { Events } from '@wailsio/runtime'

import {
  EventSpikeService,
  type EventSpikeBatch,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services'

export const eventSpikeName = 'mineops:spike:event-batch'

/**
 * @param listener - 批量事件监听器
 * @returns 取消订阅函数
 */
export function subscribeEventSpike(listener: (batch: EventSpikeBatch) => void): () => void {
  return Events.On(eventSpikeName, (event) => listener(event.data))
}

/**
 * @returns 启动批量事件流的 Promise
 */
export function startEventSpike() {
  return EventSpikeService.Start(16, 256)
}

/**
 * @returns 停止批量事件流并返回计数器的 Promise
 */
export function stopEventSpike() {
  return EventSpikeService.Stop()
}
