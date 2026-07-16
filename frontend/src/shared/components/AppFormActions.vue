<script setup lang="ts">
import { NButton, NFlex, NText } from 'naive-ui'

withDefaults(
  defineProps<{
    dirty: boolean
    submitting?: boolean
    disabled?: boolean
    submitText?: string
    discardText?: string
    statusText?: string
    showDiscard?: boolean
  }>(),
  {
    submitting: false,
    disabled: false,
    submitText: '保存',
    discardText: '放弃修改',
    statusText: '',
    showDiscard: true,
  },
)

defineEmits<{
  submit: []
  discard: []
}>()
</script>

<template>
  <NFlex align="center" justify="space-between" class="form-actions">
    <NText depth="3">{{ statusText || (dirty ? '有未保存修改' : '所有修改已保存') }}</NText>
    <NFlex>
      <slot name="before" />
      <NButton
        v-if="showDiscard"
        :disabled="!dirty || submitting || disabled"
        @click="$emit('discard')"
      >
        {{ discardText }}
      </NButton>
      <NButton
        type="primary"
        :disabled="!dirty || disabled"
        :loading="submitting"
        @click="$emit('submit')"
      >
        {{ submitText }}
      </NButton>
    </NFlex>
  </NFlex>
</template>

<style scoped>
.form-actions {
  min-height: 34px;
}
</style>
