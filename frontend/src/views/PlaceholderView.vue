<script setup lang="ts">
import { NCard, NEmpty, NText } from 'naive-ui'
import { computed } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()
const title = computed(() => String(route.meta.title ?? '功能模块'))
</script>

<template>
  <NCard :title="title">
    <NEmpty description="模块入口已建立，后续阶段将在此接入完整业务流程。" />
    <NText depth="3">当前路由：{{ route.fullPath }}</NText>
  </NCard>
</template>
