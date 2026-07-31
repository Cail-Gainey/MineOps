import type { LocaleName } from './resources'

const errorMessages: Record<LocaleName, Record<string, string>> = {
  'zh-CN': {
    'internal.unexpected': '应用发生未预期错误',
    'validation.invalid_argument': '输入参数无效',
    'validation.required': '缺少必填信息',
    'validation.conflict': '当前状态与操作冲突',
    'ssh.connection_failed': 'SSH 连接失败',
    'ssh.authentication_failed': 'SSH 身份认证失败',
    'ssh.host_key_rejected': 'SSH 主机密钥未通过校验',
    'sftp.transfer_failed': '远程文件传输失败',
    'sftp.path_rejected': '远程路径被安全策略拒绝',
    'io.read_failed': '读取数据失败',
    'io.write_failed': '写入数据失败',
    'io.not_found': '目标不存在',
    'io.permission_denied': '当前身份权限不足',
    'http.connection_failed': '外部服务连接失败',
    'http.status_failed': '外部服务返回异常状态',
    'http.response_too_large': '外部服务响应超过大小限制',
    'artifact.checksum_mismatch': 'Artifact 完整性校验失败',
    'artifact.size_exceeded': 'Artifact 超过大小限制',
    'installation.preflight_failed': '安装前置检查失败',
    'installation.directory_conflict': '安装目录发生冲突',
    'installation.java_failed': 'Java 准备失败',
    'installation.artifact_failed': '服务端 Artifact 安装失败',
    'installation.eula_failed': 'EULA 处理失败',
    'installation.first_start_failed': '服务端首次启动失败',
    'installation.registration_failed': '服务端注册失败',
    'process.start_failed': '远程进程启动失败',
    'process.exit_failed': '远程进程停止失败',
    'process.cancelled': '操作已取消或超时',
    'crypto.key_unavailable': '加密密钥不可用',
    'crypto.decrypt_failed': '加密数据解密失败',
    'crypto.integrity_failed': '加密数据完整性校验失败',
    'collector.error': 'SSH 指标采集失败',
    'collector.no_data': 'SSH 指标采集尚未产生数据',
    'metric.collection_failed': '指标采集失败',
    'metric.query_failed': '指标查询失败',
    'metric.capacity_exceeded': '指标存储容量不足',
    'spark.unavailable': 'Minecraft spark 当前不可用',
    'spark.unsupported': 'Minecraft spark 版本或能力不受支持',
    'spark.parse_failed': 'Minecraft spark 响应解析失败',
    'firewall.unsupported': '当前防火墙后端不受支持',
    'firewall.permission_denied': '防火墙操作权限不足',
    'firewall.apply_failed': '防火墙规则应用失败',
  },
  'en-US': {
    'internal.unexpected': 'The application encountered an unexpected error',
    'validation.invalid_argument': 'One or more input values are invalid',
    'validation.required': 'Required information is missing',
    'validation.conflict': 'The operation conflicts with the current state',
    'ssh.connection_failed': 'The SSH connection failed',
    'ssh.authentication_failed': 'SSH authentication failed',
    'ssh.host_key_rejected': 'The SSH host key was rejected',
    'sftp.transfer_failed': 'The remote file transfer failed',
    'sftp.path_rejected': 'The remote path was rejected by the safety policy',
    'io.read_failed': 'Failed to read data',
    'io.write_failed': 'Failed to write data',
    'io.not_found': 'The requested item was not found',
    'io.permission_denied': 'The current identity does not have permission',
    'http.connection_failed': 'Failed to connect to the external service',
    'http.status_failed': 'The external service returned an error status',
    'http.response_too_large': 'The external service response exceeded the size limit',
    'artifact.checksum_mismatch': 'Artifact integrity verification failed',
    'artifact.size_exceeded': 'The artifact exceeded the size limit',
    'installation.preflight_failed': 'The installation preflight failed',
    'installation.directory_conflict': 'The installation directory conflicts with existing data',
    'installation.java_failed': 'Java preparation failed',
    'installation.artifact_failed': 'The server artifact installation failed',
    'installation.eula_failed': 'The EULA step failed',
    'installation.first_start_failed': 'The server failed its first start',
    'installation.registration_failed': 'The server registration failed',
    'process.start_failed': 'The remote process failed to start',
    'process.exit_failed': 'The remote process failed to stop',
    'process.cancelled': 'The operation was cancelled or timed out',
    'crypto.key_unavailable': 'The encryption key is unavailable',
    'crypto.decrypt_failed': 'Failed to decrypt protected data',
    'crypto.integrity_failed': 'Protected data failed its integrity check',
    'collector.error': 'SSH metric collection failed',
    'collector.no_data': 'SSH metric collection has not produced data yet',
    'metric.collection_failed': 'Metric collection failed',
    'metric.query_failed': 'Metric query failed',
    'metric.capacity_exceeded': 'Metric storage capacity was exceeded',
    'spark.unavailable': 'Minecraft spark is unavailable',
    'spark.unsupported': 'The Minecraft spark version or capability is unsupported',
    'spark.parse_failed': 'Failed to parse the Minecraft spark response',
    'firewall.unsupported': 'The firewall backend is unsupported',
    'firewall.permission_denied': 'The firewall operation requires additional permission',
    'firewall.apply_failed': 'Failed to apply the firewall rule',
  },
}

/**
 * 读取当前文档语言对应的语言包名称。
 * @returns 语言包名称
 */
function activeLocale(): LocaleName {
  return typeof document !== 'undefined' && document.documentElement.lang === 'en-US'
    ? 'en-US'
    : 'zh-CN'
}

/**
 * 把稳定的后端错误码解析成当前界面语言的文案。
 * @param code - Stable Application Error code
 * @param technicalMessage - Original backend diagnostic message
 * @returns Localized user-facing message with a safe fallback
 */
export function localizeError(code: string, technicalMessage: string): string {
  const locale = activeLocale()
  const localized = errorMessages[locale][code]
  if (!localized) return technicalMessage || code
  if (locale === 'zh-CN' && technicalMessage && technicalMessage !== localized) {
    return `${localized}：${technicalMessage}`
  }
  return localized
}
