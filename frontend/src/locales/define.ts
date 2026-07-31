/** 受支持的界面语言标识。 */
export type LocaleName = 'zh-CN' | 'en-US'

/** 单个语言模块：以简体中文为基准语言，其余语言必须提供完全一致的键集合。 */
export interface MessageModule<T extends Record<string, string>> {
  'zh-CN': T
  'en-US': Record<keyof T, string>
}

/**
 * 声明一个语言模块，并在编译期约束各语言的键集合完全一致。
 * @param messages - 以 zh-CN 为基准的多语言文案表
 * @returns 原样返回的语言模块，便于聚合时保留字面量键类型
 */
export function defineMessages<T extends Record<string, string>>(
  messages: MessageModule<T>,
): MessageModule<T> {
  return messages
}
