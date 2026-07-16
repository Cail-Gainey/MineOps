const metricLabels: Record<string, string> = {
  'host.cpu': '主机 CPU 使用率',
  'host.memory': '主机内存使用率',
  'host.load.1m': '主机 1 分钟负载',
  'host.disk.used': '磁盘已用空间',
  'host.disk.read_bytes_per_second': '磁盘读取速率',
  'host.disk.write_bytes_per_second': '磁盘写入速率',
  'host.network.receive_bytes_per_second': '网络接收速率',
  'host.network.transmit_bytes_per_second': '网络发送速率',
  'process.cpu': 'Java CPU 使用率',
  'process.rss': 'Java 内存',
  'process.threads': 'Java 线程数',
  'minecraft.tps': 'Minecraft TPS',
  'minecraft.mspt': 'Minecraft MSPT',
}

/**
 * 将指标标识转换为中文显示名称，未知指标保留原值。
 * @param metric - 指标标识
 * @returns 中文指标名称或原始标识
 */
export function formatMetricLabel(metric: string): string {
  return metricLabels[metric] ?? metric
}
