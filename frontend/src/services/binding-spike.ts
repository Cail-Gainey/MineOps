import {
  BindingSpikeRequest,
  BindingSpikeService,
} from '../../bindings/github.com/Cail-Gainey/MineOps/internal/desktop/services'

/**
 * @returns Wails DTO 回显调用
 */
export function runBindingEcho() {
  return BindingSpikeService.Echo(
    new BindingSpikeRequest({
      name: 'MineOps 绑定验证',
      tags: ['typescript', 'slice', '中文'],
      metadata: { source: 'stage-0', protocol: 'wails-v3' },
    }),
  )
}

/**
 * @param milliseconds - 等待毫秒数
 * @returns 可取消的 Wails Promise
 */
export function runCancellationWait(milliseconds: number) {
  return BindingSpikeService.Wait(milliseconds)
}
