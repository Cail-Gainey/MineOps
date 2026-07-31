import { backendErrorMessages } from './messages/backend-errors'
import { commonMessages } from './messages/common'
import { dashboardMessages } from './messages/dashboard'
import { filesMessages } from './messages/files'
import { javaMessages } from './messages/java'
import { metricMessages } from './messages/metrics'
import { monitoringMessages } from './messages/monitoring'
import { navMessages } from './messages/nav'
import { operationsMessages } from './messages/operations'
import { performanceMessages } from './messages/performance'
import { playersMessages } from './messages/players'
import { serverDetailMessages } from './messages/server-detail'
import { serverPanelMessages } from './messages/server-panels'
import { serversMessages } from './messages/servers'
import { settingsMessages } from './messages/settings'
import { shellMessages } from './messages/shell'
import { sshMessages } from './messages/ssh'
import { systemMessages } from './messages/system'

export type { LocaleName } from './define'

/**
 * 全量界面文案表。各语言模块通过 defineMessages 在编译期保证键集合一致，
 * 因此这里只需按模块聚合，无需再做运行时校验。
 *
 * 本文件由 scripts/generate-resources.py 依据 src/locales/messages 目录生成。
 */
export const resources = {
  'zh-CN': {
    ...backendErrorMessages['zh-CN'],
    ...commonMessages['zh-CN'],
    ...dashboardMessages['zh-CN'],
    ...filesMessages['zh-CN'],
    ...javaMessages['zh-CN'],
    ...metricMessages['zh-CN'],
    ...monitoringMessages['zh-CN'],
    ...navMessages['zh-CN'],
    ...operationsMessages['zh-CN'],
    ...performanceMessages['zh-CN'],
    ...playersMessages['zh-CN'],
    ...serverDetailMessages['zh-CN'],
    ...serverPanelMessages['zh-CN'],
    ...serversMessages['zh-CN'],
    ...settingsMessages['zh-CN'],
    ...shellMessages['zh-CN'],
    ...sshMessages['zh-CN'],
    ...systemMessages['zh-CN'],
  },
  'en-US': {
    ...backendErrorMessages['en-US'],
    ...commonMessages['en-US'],
    ...dashboardMessages['en-US'],
    ...filesMessages['en-US'],
    ...javaMessages['en-US'],
    ...metricMessages['en-US'],
    ...monitoringMessages['en-US'],
    ...navMessages['en-US'],
    ...operationsMessages['en-US'],
    ...performanceMessages['en-US'],
    ...playersMessages['en-US'],
    ...serverDetailMessages['en-US'],
    ...serverPanelMessages['en-US'],
    ...serversMessages['en-US'],
    ...settingsMessages['en-US'],
    ...shellMessages['en-US'],
    ...sshMessages['en-US'],
    ...systemMessages['en-US'],
  },
}

/** 全量文案键，供 t() 与需要持有键名的模块使用。 */
export type ResourceKey = keyof (typeof resources)['zh-CN']
