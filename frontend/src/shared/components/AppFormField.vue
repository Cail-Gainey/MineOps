<script setup lang="ts">
import { NFormItem, NText } from 'naive-ui'

defineProps<{
  label: string
  path?: string
  help?: string
  validationMessage?: string
  required?: boolean
}>()
</script>

<template>
  <NFormItem
    :label="label"
    :required="required"
    v-bind="{
      ...(path === undefined ? {} : { path }),
      ...(validationMessage
        ? { validationStatus: 'error' as const, feedback: validationMessage }
        : {}),
    }"
  >
    <div class="field-content">
      <slot />
      <NText v-if="help && !validationMessage" depth="3" class="field-help">{{ help }}</NText>
    </div>
  </NFormItem>
</template>

<style scoped>
.field-content {
  width: 100%;
}

.field-help {
  display: block;
  margin-top: 6px;
  font-size: 12px;
}
</style>
