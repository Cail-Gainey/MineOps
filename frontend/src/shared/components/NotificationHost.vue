<script setup lang="ts">
import { useNotification } from 'naive-ui'
import { watch } from 'vue'

import { useNotificationStore } from '../../stores/notifications'

const store = useNotificationStore()
const notification = useNotification()

watch(
  () => [...store.pending],
  (items) => {
    for (const item of items) {
      notification.create({
        type: item.kind,
        title: item.title,
        ...(item.content === undefined ? {} : { content: item.content }),
        duration: item.duration,
        keepAliveOnHover: true,
      })
      store.consume(item.id)
    }
  },
  { immediate: true },
)
</script>

<template><span aria-hidden="true" /></template>
