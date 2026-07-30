import { VueQueryPlugin } from '@tanstack/vue-query'
import { createPinia } from 'pinia'
import { createApp } from 'vue'

import App from './App.vue'
import router from './router'
import { reportClientError } from './services/error-reporter'
import { queryClient } from './services/query-client'
import { useErrorCenterStore } from './stores/error-center'
import { useSettingsStore } from './stores/settings'
import './styles/base.css'

const app = createApp(App)
const pinia = createPinia()

app.use(pinia).use(router).use(VueQueryPlugin, { queryClient })

const errorCenter = useErrorCenterStore(pinia)
app.config.errorHandler = (error, _instance, info) => {
  errorCenter.capture(error, `vue:${info}`)
  reportClientError(error, 'application', router.currentRoute.value.fullPath)
}
window.addEventListener('error', (event) => {
  // ResizeObserver 断环提示是浏览器可自愈的良性信号（echarts/monaco/naive-ui 均可能触发），不作为错误采集。
  if (typeof event.message === 'string' && event.message.includes('ResizeObserver loop')) return
  const error = event.error ?? new Error(event.message)
  errorCenter.capture(error, 'window')
  reportClientError(error, 'window', router.currentRoute.value.fullPath)
})
window.addEventListener('unhandledrejection', (event) => {
  errorCenter.capture(event.reason, 'promise')
  reportClientError(event.reason, 'promise', router.currentRoute.value.fullPath)
})

const settings = useSettingsStore(pinia)
settings.connect()
void settings.load().catch((error) => errorCenter.capture(error, 'settings:bootstrap'))

app.mount('#app')
