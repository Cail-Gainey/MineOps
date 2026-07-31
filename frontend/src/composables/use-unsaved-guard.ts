import { onUnmounted, toValue, watch, type MaybeRefOrGetter } from 'vue'

import { setUnsavedState } from '../services/exit-guard-api'

/**
 * 在组件生命周期内登记未保存状态，退出应用时用于拦截。
 * @param owner - 模块标识
 * @param label - 展示给用户的名称
 * @param dirty - 是否存在未保存修改
 * @returns 无返回值
 */
export function useUnsavedGuard(
  owner: MaybeRefOrGetter<string>,
  label: MaybeRefOrGetter<string>,
  dirty: MaybeRefOrGetter<boolean>,
): void {
  let previousOwner = ''
  watch(
    [() => toValue(owner), () => toValue(label), () => toValue(dirty)],
    ([nextOwner, nextLabel, nextDirty]) => {
      if (previousOwner && previousOwner !== nextOwner) {
        void setUnsavedState(previousOwner, nextLabel || previousOwner, false)
      }
      previousOwner = nextOwner
      if (nextOwner) void setUnsavedState(nextOwner, nextLabel || nextOwner, nextDirty)
    },
    { immediate: true },
  )
  onUnmounted(() => {
    if (previousOwner) void setUnsavedState(previousOwner, toValue(label) || previousOwner, false)
  })
}
