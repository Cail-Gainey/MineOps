import type { Component } from 'vue'

import type { ResourceKey } from '../locales/resources'
import { featureModules } from './feature-registry'

export interface NavigationEntry {
  path: string
  icon: Component
  titleKey: ResourceKey
}

export const navigationEntries: NavigationEntry[] = featureModules
  .filter((feature) => feature.menu)
  .map((feature) => ({
    path: feature.path,
    icon: feature.icon,
    titleKey: feature.titleKey,
  }))
