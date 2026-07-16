import { Events } from '@wailsio/runtime'

export interface SecondInstanceLaunch {
  args: string[]
  workingDir: string
  additionalData?: { [key: string]: string | undefined } | undefined
}

export const secondInstanceEventName = 'mineops:lifecycle:second-instance'

/**
 * @param listener - 第二实例启动监听器
 * @returns 取消订阅函数
 */
export function subscribeSecondInstance(
  listener: (data: SecondInstanceLaunch) => void,
): () => void {
  return Events.On(secondInstanceEventName, (event) => listener(event.data))
}
