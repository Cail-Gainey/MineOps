<script setup lang="ts">
import { NTooltip } from 'naive-ui'
import type { Component } from 'vue'

withDefaults(
  defineProps<{
    icon: Component
    label: string
    size?: number
  }>(),
  { size: 18 },
)
</script>

<template>
  <NTooltip placement="bottom" trigger="hover">
    <template #trigger>
      <span class="app-icon" role="img" :aria-label="label">
        <component :is="icon" :size="size" aria-hidden="true" />
      </span>
    </template>
    {{ label }}
  </NTooltip>
</template>

<style scoped>
.app-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  line-height: 1;
}
</style>
