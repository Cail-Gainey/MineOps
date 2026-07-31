<script setup lang="ts">
import { NCard, NEmpty, NText } from 'naive-ui'
import { computed } from 'vue'
import { useRoute } from 'vue-router'

import { useLocaleStore } from '../stores/locale'

const route = useRoute()
const locale = useLocaleStore()
const title = computed(() => {
  const titleKey = route.meta.titleKey
  return typeof titleKey === 'string'
    ? locale.t(titleKey as Parameters<typeof locale.t>[0])
    : locale.t('placeholder.title')
})
</script>

<template>
  <NCard :title="title">
    <NEmpty :description="locale.t('placeholder.description')" />
    <NText depth="3">{{ locale.t('placeholder.currentRoute', { path: route.fullPath }) }}</NText>
  </NCard>
</template>
