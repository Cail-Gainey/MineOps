import {
  Activity,
  Blocks,
  Box,
  Coffee,
  FileText,
  Gauge,
  HardDrive,
  Server,
  Settings,
  TerminalSquare,
} from '@lucide/vue'
import type { Component } from 'vue'

import type { ResourceKey } from '../locales/resources'

export interface FeatureModule {
  id: string
  path: string
  title: string
  titleKey: ResourceKey
  icon: Component
  menu: boolean
  dialogs: string[]
  permissions: string[]
  lifecycle: 'application' | 'route'
  component: () => Promise<{ default: Component }>
}

export const featureModules: FeatureModule[] = [
  {
    id: 'dashboard',
    path: '/',
    title: '概览',
    titleKey: 'nav.dashboard',
    icon: Gauge,
    menu: true,
    dialogs: [],
    permissions: [],
    lifecycle: 'application',
    component: () => import('../views/DashboardView.vue'),
  },
  {
    id: 'servers',
    path: '/servers',
    title: 'MC Servers',
    titleKey: 'nav.servers',
    icon: Server,
    menu: true,
    dialogs: ['server-wizard', 'server-delete'],
    permissions: ['server.read'],
    lifecycle: 'route',
    component: () => import('../features/servers/MinecraftServersView.vue'),
  },
  {
    id: 'server-detail',
    path: '/servers/:serverID',
    title: 'Server Detail',
    titleKey: 'nav.servers',
    icon: Server,
    menu: false,
    dialogs: ['server-operation'],
    permissions: ['server.read'],
    lifecycle: 'route',
    component: () => import('../features/servers/ServerDetailView.vue'),
  },
  {
    id: 'ssh-sessions',
    path: '/ssh-sessions',
    title: 'SSH Sessions',
    titleKey: 'nav.sshSessions',
    icon: TerminalSquare,
    menu: true,
    dialogs: ['ssh-session-edit'],
    permissions: ['ssh.read'],
    lifecycle: 'route',
    component: () => import('../features/ssh-sessions/SSHSessionsView.vue'),
  },
  {
    id: 'files',
    path: '/files/:sshSessionID?',
    title: 'Files',
    titleKey: 'nav.files',
    icon: FileText,
    menu: false,
    dialogs: ['file-conflict'],
    permissions: ['file.read'],
    lifecycle: 'route',
    component: () => import('../features/files/FilesView.vue'),
  },
  {
    id: 'terminal',
    path: '/terminal/:sshSessionID?',
    title: 'SSH Terminal',
    titleKey: 'nav.terminal',
    icon: TerminalSquare,
    menu: false,
    dialogs: ['terminal-close'],
    permissions: ['ssh.connect'],
    lifecycle: 'route',
    component: () => import('../features/terminal/TerminalWorkspace.vue'),
  },
  {
    id: 'java-runtimes',
    path: '/java-runtimes',
    title: 'Java Runtimes',
    titleKey: 'nav.javaRuntimes',
    icon: Coffee,
    menu: true,
    dialogs: ['java-install'],
    permissions: ['java.read'],
    lifecycle: 'route',
    component: () => import('../features/java-runtimes/JavaRuntimesView.vue'),
  },
  {
    id: 'operations',
    path: '/operations',
    title: 'Operations',
    titleKey: 'nav.operations',
    icon: Blocks,
    menu: true,
    dialogs: ['operation-error'],
    permissions: ['operation.read'],
    lifecycle: 'application',
    component: () => import('../features/operations/OperationsView.vue'),
  },
  {
    id: 'monitoring',
    path: '/monitoring',
    title: 'Monitoring',
    titleKey: 'nav.monitoring',
    icon: Activity,
    menu: true,
    dialogs: [],
    permissions: ['metric.read'],
    lifecycle: 'route',
    component: () => import('../features/monitoring/MonitoringView.vue'),
  },
  {
    id: 'performance',
    path: '/performance',
    title: 'Performance Center',
    titleKey: 'nav.performance',
    icon: HardDrive,
    menu: true,
    dialogs: ['spark-privacy'],
    permissions: ['spark.read'],
    lifecycle: 'route',
    component: () => import('../features/performance/PerformanceView.vue'),
  },
  {
    id: 'settings',
    path: '/settings',
    title: 'Settings',
    titleKey: 'nav.settings',
    icon: Settings,
    menu: true,
    dialogs: ['settings-reset'],
    permissions: ['settings.read', 'settings.write'],
    lifecycle: 'application',
    component: () => import('../features/settings/SettingsView.vue'),
  },
]

if (import.meta.env.DEV) {
  featureModules.push({
    id: 'spikes',
    path: '/spikes',
    title: '阶段 0 技术基线',
    titleKey: 'nav.spikes',
    icon: Box,
    menu: false,
    dialogs: [],
    permissions: [],
    lifecycle: 'route',
    component: () => import('../views/HomeView.vue'),
  })
}
