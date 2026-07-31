import { defineStore } from 'pinia'

import type { ResourceKey } from '../locales/resources'
import {
  activeLocale,
  activeTimeFormat,
  formatDate,
  formatDateTime,
  formatNumber,
  formatTime,
  setActiveLocale,
  setActiveTimeFormat,
  translate,
  type MessageParams,
} from '../locales/runtime'

/**
 * 界面语言 Store。实际状态保存在 locales/runtime，
 * 便于 Service、纯函数模块与组件共用同一份语言状态。
 */
export const useLocaleStore = defineStore('locale', () => {
  /**
   * 读取当前语言资源并替换简单占位符。
   * @param key - 稳定资源键
   * @param params - 文案占位符值
   * @returns 本地化后的 UI 文案
   */
  function t(key: ResourceKey, params: MessageParams = {}): string {
    return translate(key, params)
  }

  return {
    formatDate,
    formatDateTime,
    formatNumber,
    formatTime,
    locale: activeLocale,
    setLocale: setActiveLocale,
    setTimeFormat: setActiveTimeFormat,
    t,
    timeFormat: activeTimeFormat,
  }
})
