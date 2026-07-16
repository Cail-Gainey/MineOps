<script setup lang="ts">
import {
  NConfigProvider,
  NDialogProvider,
  NGlobalStyle,
  NLoadingBarProvider,
  NMessageProvider,
  NNotificationProvider,
} from 'naive-ui'
import { storeToRefs } from 'pinia'

import { useThemeStore } from './stores/theme'
import GlobalInteractionHost from './shared/components/GlobalInteractionHost.vue'
import NotificationHost from './shared/components/NotificationHost.vue'

const themeStore = useThemeStore()
const { naiveTheme, themeOverrides } = storeToRefs(themeStore)
</script>

<template>
  <NConfigProvider :theme="naiveTheme" :theme-overrides="themeOverrides">
    <NGlobalStyle />
    <NLoadingBarProvider>
      <NDialogProvider>
        <NNotificationProvider>
          <NMessageProvider>
            <NotificationHost />
            <GlobalInteractionHost />
            <RouterView />
          </NMessageProvider>
        </NNotificationProvider>
      </NDialogProvider>
    </NLoadingBarProvider>
  </NConfigProvider>
</template>
