<script setup lang="ts">
import { NButton, NFlex, NText } from 'naive-ui'

import { useLocaleStore } from '../../stores/locale'

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
    submitText: '',
    discardText: '',
    statusText: '',
    showDiscard: true,
  },
)

defineEmits<{
  submit: []
  discard: []
}>()

const locale = useLocaleStore()
</script>

<template>
  <NFlex align="center" justify="space-between" class="form-actions">
    <NText depth="3">{{
      statusText || (dirty ? locale.t('form.dirty') : locale.t('form.clean'))
    }}</NText>
    <NFlex>
      <slot name="before" />
      <NButton
        v-if="showDiscard"
        :disabled="!dirty || submitting || disabled"
        @click="$emit('discard')"
      >
        {{ discardText || locale.t('form.discard') }}
      </NButton>
      <NButton
        type="primary"
        :disabled="!dirty || disabled"
        :loading="submitting"
        @click="$emit('submit')"
      >
        {{ submitText || locale.t('common.save') }}
      </NButton>
    </NFlex>
  </NFlex>
</template>

<style scoped>
.form-actions {
  min-height: 34px;
}
</style>
