import type { EChartsInitOpts } from 'echarts/core'

/**
 * 返回受硬件加速开关约束的 ECharts 初始化参数。
 * @param hardwareAcceleration - Settings.General.HardwareAcceleration 当前值
 * @returns 传给 vue-echarts init-options 的参数
 */
export function chartInitOptions(hardwareAcceleration: boolean): EChartsInitOpts {
  return {
    renderer: 'canvas',
    // 脏矩形渲染只重绘发生变化的区域，与有无 GPU 无关，始终开启。
    useDirtyRect: true,
    // 关闭硬件加速时 Canvas 由 CPU 光栅化，Retina 下 2x 像素比意味着四倍填充量，降到 1x。
    devicePixelRatio: hardwareAcceleration ? Math.min(window.devicePixelRatio || 1, 2) : 1,
  }
}

/**
 * 把硬件加速状态写到根元素，供 CSS 合成提示与后续渲染消费方读取。
 * @param hardwareAcceleration - Settings.General.HardwareAcceleration 当前值
 * @returns 无返回值
 */
export function applyHardwareAccelerationAttribute(hardwareAcceleration: boolean): void {
  document.documentElement.dataset.hardwareAcceleration = hardwareAcceleration ? 'on' : 'off'
}
