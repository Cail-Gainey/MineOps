import { onUnmounted, toValue, watch, type MaybeRefOrGetter } from 'vue'

import { setUnsavedState } from '../services/exit-guard-api'

/** Synchronizes one local dirty flag with the native application exit guard. */
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
