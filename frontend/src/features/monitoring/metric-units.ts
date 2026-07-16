export interface MetricDisplayScale {
  factor: number
  unit: string
}

/** @param value - 原始数值 @returns 保留两位小数的数值 */
export function roundToTwo(value: number): number {
  return Number(value.toFixed(2))
}

/** @param value - 字节或每秒字节数 @param perSecond - 是否为速率 @returns 自动缩放单位 */
export function byteDisplayScale(value: number, perSecond = false): MetricDisplayScale {
  const units = perSecond
    ? ['B/s', 'KiB/s', 'MiB/s', 'GiB/s', 'TiB/s']
    : ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  let factor = 1
  let index = 0
  const magnitude = Math.abs(value)
  while (magnitude / factor >= 1024 && index < units.length - 1) {
    factor *= 1024
    index++
  }
  return { factor, unit: units[index] ?? (perSecond ? 'B/s' : 'B') }
}

/** @param unit - Metric 原始单位 @param magnitude - 当前数值范围 @returns 页面展示缩放规则 */
export function metricDisplayScale(
  unit: string | undefined,
  magnitude: number,
): MetricDisplayScale {
  switch (unit) {
    case 'bytes':
      return byteDisplayScale(magnitude)
    case 'bytes_per_second':
      return byteDisplayScale(magnitude, true)
    case 'percent':
      return { factor: 1, unit: '%' }
    case 'milliseconds':
      return { factor: 1, unit: 'ms' }
    case 'ticks_per_second':
      return { factor: 1, unit: 'TPS' }
    case 'seconds':
      if (magnitude >= 3600) return { factor: 3600, unit: 'h' }
      if (magnitude >= 60) return { factor: 60, unit: 'min' }
      return { factor: 1, unit: 's' }
    case 'count':
      return { factor: 1, unit: '个' }
    default:
      return { factor: 1, unit: '' }
  }
}
