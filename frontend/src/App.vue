<script setup lang="ts">
import {
  dateEnUS,
  dateZhCN,
  enUS,
  NConfigProvider,
  NDialogProvider,
  NGlobalStyle,
  NLoadingBarProvider,
  NMessageProvider,
  NNotificationProvider,
  zhCN,
} from 'naive-ui'
import { storeToRefs } from 'pinia'
import { computed } from 'vue'

import { useLocaleStore } from './stores/locale'
import { useThemeStore } from './stores/theme'
import GlobalInteractionHost from './shared/components/GlobalInteractionHost.vue'
import NotificationHost from './shared/components/NotificationHost.vue'

const themeStore = useThemeStore()
const localeStore = useLocaleStore()
const { naiveTheme, themeOverrides } = storeToRefs(themeStore)
// naive-ui 内置组件（分页、日期选择、上传等）的文案跟随应用语言切换。
const naiveLocale = computed(() => (localeStore.locale === 'en-US' ? enUS : zhCN))
const naiveDateLocale = computed(() => (localeStore.locale === 'en-US' ? dateEnUS : dateZhCN))
</script>

<template>
  <NConfigProvider
    :theme="naiveTheme"
    :theme-overrides="themeOverrides"
    :locale="naiveLocale"
    :date-locale="naiveDateLocale"
  >
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
