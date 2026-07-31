<script setup lang="ts">
import { NButton, NDrawer, NDrawerContent, NFlex, NModal, NText } from 'naive-ui'

import { useInteractionStore } from '../../stores/interactions'
import { useLocaleStore } from '../../stores/locale'

const interactions = useInteractionStore()
const locale = useLocaleStore()
</script>

<template>
  <NModal
    :show="Boolean(interactions.confirmRequest)"
    preset="card"
    :title="interactions.confirmRequest?.title ?? ''"
    :mask-closable="false"
    :close-on-esc="true"
    role="alertdialog"
    aria-modal="true"
    style="width: min(480px, calc(100vw - 32px))"
    @esc="interactions.resolveConfirm(false)"
    @close="interactions.resolveConfirm(false)"
  >
    <NFlex vertical :size="12">
      <NText>{{ interactions.confirmRequest?.content }}</NText>
      <NText v-if="interactions.confirmRequest?.objectLabel" depth="2">
        {{ locale.t('dialog.objectLabel', { label: interactions.confirmRequest.objectLabel }) }}
      </NText>
      <NText v-if="interactions.confirmRequest?.impact" type="warning">
        {{ locale.t('dialog.impact', { impact: interactions.confirmRequest.impact }) }}
      </NText>
    </NFlex>
    <template #footer>
      <NFlex justify="end">
        <NButton @click="interactions.resolveConfirm(false)">
          {{ interactions.confirmRequest?.negativeText ?? locale.t('common.cancel') }}
        </NButton>
        <NButton
          :type="interactions.confirmRequest?.danger ? 'error' : 'primary'"
          @click="interactions.resolveConfirm(true)"
        >
          {{ interactions.confirmRequest?.positiveText ?? locale.t('common.confirm') }}
        </NButton>
      </NFlex>
    </template>
  </NModal>

  <NDrawer
    :show="Boolean(interactions.drawerRequest)"
    :width="interactions.drawerRequest?.width ?? 520"
    :mask-closable="true"
    @esc="interactions.closeDrawer()"
    @update:show="(show) => !show && interactions.closeDrawer()"
  >
    <NDrawerContent :title="interactions.drawerRequest?.title ?? ''" closable>
      <NText style="white-space: pre-wrap">{{ interactions.drawerRequest?.content }}</NText>
    </NDrawerContent>
  </NDrawer>
</template>
