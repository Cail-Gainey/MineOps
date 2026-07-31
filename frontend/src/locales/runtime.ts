import { ref } from 'vue'

import type { LocaleName } from './define'
import { resources, type ResourceKey } from './resources'

/** 文案占位符取值，形如 `{count}` 会被替换为对应值。 */
export type MessageParams = Record<string, string | number>

/** 12/24 小时制显示偏好。 */
export type TimeFormatName = '12h' | '24h'

/** 界面支持的语言列表，顺序与设置页展示顺序一致。 */
export const supportedLocales: LocaleName[] = ['zh-CN', 'en-US']

/** 缺失文案时回退的基准语言。 */
export const fallbackLocale: LocaleName = 'zh-CN'

/**
 * 当前生效语言。放在 Store 之外是为了让 Store、Service 与纯函数模块
 * 都能直接读取，同时仍保持 Vue 响应式。
 */
export const activeLocale = ref<LocaleName>(fallbackLocale)

/** 当前生效的 12/24 小时制偏好。 */
export const activeTimeFormat = ref<TimeFormatName>('24h')

/**
 * 把任意语言取值归一化为受支持的语言，异常值回退为基准语言。
 * @param value - 待归一化的语言标识
 * @returns 受支持的语言标识
 */
export function normalizeLocale(value: string): LocaleName {
  return supportedLocales.includes(value as LocaleName) ? (value as LocaleName) : fallbackLocale
}

/**
 * 应用界面语言，并同步 `<html lang>` 供样式与无障碍工具识别。
 * @param value - Settings 中保存的语言值
 * @returns 无返回值
 */
export function setActiveLocale(value: string): void {
  activeLocale.value = normalizeLocale(value)
  if (typeof document !== 'undefined') document.documentElement.lang = activeLocale.value
}

/**
 * 应用 12/24 小时制显示偏好，异常值按 24 小时制处理。
 * @param value - Settings 中保存的时间格式
 * @returns 无返回值
 */
export function setActiveTimeFormat(value: string): void {
  activeTimeFormat.value = value === '12h' ? '12h' : '24h'
}

/**
 * 判断给定字符串是否为已登记的文案键。
 * @param key - 待判断的键
 * @returns 已登记时返回 true
 */
export function hasMessage(key: string): key is ResourceKey {
  return key in resources[fallbackLocale]
}

/**
 * 读取当前语言文案并替换 `{name}` 占位符，缺失时回退到基准语言。
 * @param key - 稳定文案键
 * @param params - 文案占位符取值
 * @returns 本地化后的 UI 文案
 */
export function translate(key: ResourceKey, params: MessageParams = {}): string {
  let message: string = resources[activeLocale.value][key] ?? resources[fallbackLocale][key] ?? key
  for (const [name, value] of Object.entries(params)) {
    message = message.replaceAll(`{${name}}`, String(value))
  }
  return message
}

/**
 * 按当前语言与 12/24 小时制偏好格式化日期时间。
 * @param value - Date、时间戳或可解析的日期字符串
 * @returns 本地化日期时间文本
 */
export function formatDateTime(value: string | number | Date): string {
  return new Date(value).toLocaleString(activeLocale.value, {
    hour12: activeTimeFormat.value === '12h',
  })
}

/**
 * 按当前语言格式化日期。
 * @param value - Date、时间戳或可解析的日期字符串
 * @returns 本地化日期文本
 */
export function formatDate(value: string | number | Date): string {
  return new Date(value).toLocaleDateString(activeLocale.value)
}

/**
 * 按当前语言与 12/24 小时制偏好格式化时间。
 * @param value - Date、时间戳或可解析的日期字符串
 * @returns 本地化时间文本
 */
export function formatTime(value: string | number | Date): string {
  return new Date(value).toLocaleTimeString(activeLocale.value, {
    hour12: activeTimeFormat.value === '12h',
  })
}

/**
 * 按当前语言格式化数值。
 * @param value - 原始数值
 * @param options - Intl.NumberFormat 选项
 * @returns 本地化数值文本
 */
export function formatNumber(value: number, options: Intl.NumberFormatOptions = {}): string {
  return new Intl.NumberFormat(activeLocale.value, options).format(value)
}
