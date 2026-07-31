import { defineStore } from 'pinia'
import { ref } from 'vue'

import { resources, type LocaleName, type ResourceKey } from '../locales/resources'

export const useLocaleStore = defineStore('locale', () => {
  const locale = ref<LocaleName>('zh-CN')
  const timeFormat = ref<'12h' | '24h'>('24h')

  /**
   * 读取当前语言资源并替换简单占位符。
   * @param key - 稳定资源键
   * @param params - 文案占位符值
   * @returns 本地化后的 UI 文案
   */
  function t(key: ResourceKey, params: Record<string, string | number> = {}): string {
    let message: string = resources[locale.value][key] ?? resources['zh-CN'][key]
    for (const [name, value] of Object.entries(params)) {
      message = message.replaceAll(`{${name}}`, String(value))
    }
    return message
  }

  /**
   * 应用受支持的界面语言，异常值回退为简体中文。
   * @param value - Settings 中保存的语言值
   * @returns void
   */
  function setLocale(value: string): void {
    locale.value = value === 'en-US' ? 'en-US' : 'zh-CN'
    document.documentElement.lang = locale.value
  }

  /**
   * 应用已保存的 12/24 小时制显示偏好。
   * @param value - Settings 中保存的时间格式
   * @returns void
   */
  function setTimeFormat(value: string): void {
    timeFormat.value = value === '12h' ? '12h' : '24h'
  }

  /**
   * 按已提交的语言与 12/24 小时制偏好格式化时间。
   * @param value - Date、时间戳或可解析的日期字符串
   * @returns 本地化日期时间文本
   */
  function formatDateTime(value: string | number | Date): string {
    return new Date(value).toLocaleString(locale.value, { hour12: timeFormat.value === '12h' })
  }

  return { formatDateTime, locale, setLocale, setTimeFormat, t, timeFormat }
})
