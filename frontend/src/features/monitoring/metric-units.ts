export interface MetricDisplayScale {
  factor: number
  unit: string
}

/**
 * 把数值四舍五入到两位小数。
 * @param value - 原始数值
 * @returns 保留两位小数的数值
 */
export function roundToTwo(value: number): number {
  return Number(value.toFixed(2))
}

/**
 * 按数量级给出字节或字节每秒的展示换算系数与单位。
 * @param value - 字节数量级
 * @param perSecond - 是否为每秒速率单位
 * @returns 展示换算系数与单位
 */
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

/**
 * 按指标单位与数量级给出展示换算系数与单位。
 * @param unit - 指标单位标识
 * @param magnitude - 该组数据的最大绝对值
 * @returns 展示换算系数与单位
 */
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
