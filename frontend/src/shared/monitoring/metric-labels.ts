import { hasMessage, translate } from '../../locales/runtime'

/**
 * 将指标标识转换为当前界面语言的显示名称，未登记的指标保留原值。
 * @param metric - 指标标识
 * @returns 指标显示名称或原始标识
 */
export function formatMetricLabel(metric: string): string {
  const key = `metric.${metric}`
  return hasMessage(key) ? translate(key) : metric
}
