import { createRouter, createWebHashHistory } from 'vue-router'

import AppShell from '../app/AppShell.vue'
import { featureModules } from '../app/feature-registry'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {
      path: '/',
      component: AppShell,
      children: featureModules.map((feature) => ({
        path: feature.path === '/' ? '' : feature.path.slice(1),
        name: feature.id,
        component: feature.component,
        meta: {
          titleKey: feature.titleKey,
          permissions: feature.permissions,
          dialogs: feature.dialogs,
          lifecycle: feature.lifecycle,
        },
      })),
    },
  ],
})

export default router
