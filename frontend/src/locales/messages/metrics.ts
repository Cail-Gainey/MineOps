import { defineMessages } from '../define'

/** 指标标识与指标单位的展示文案。 */
export const metricMessages = defineMessages({
  'zh-CN': {
    'metric.host.cpu': '主机 CPU 使用率',
    'metric.host.memory': '主机内存使用率',
    'metric.host.load.1m': '主机 1 分钟负载',
    'metric.host.disk.used': '磁盘已用空间',
    'metric.host.disk.read_bytes_per_second': '磁盘读取速率',
    'metric.host.disk.write_bytes_per_second': '磁盘写入速率',
    'metric.host.network.receive_bytes_per_second': '网络接收速率',
    'metric.host.network.transmit_bytes_per_second': '网络发送速率',
    'metric.process.cpu': 'Java CPU 使用率',
    'metric.process.rss': 'Java 内存',
    'metric.process.threads': 'Java 线程数',
    'metric.minecraft.tps': 'Minecraft TPS',
    'metric.minecraft.mspt': 'Minecraft MSPT',
  },
  'en-US': {
    'metric.host.cpu': 'Host CPU usage',
    'metric.host.memory': 'Host memory usage',
    'metric.host.load.1m': 'Host load (1 min)',
    'metric.host.disk.used': 'Disk space used',
    'metric.host.disk.read_bytes_per_second': 'Disk read rate',
    'metric.host.disk.write_bytes_per_second': 'Disk write rate',
    'metric.host.network.receive_bytes_per_second': 'Network receive rate',
    'metric.host.network.transmit_bytes_per_second': 'Network transmit rate',
    'metric.process.cpu': 'Java CPU usage',
    'metric.process.rss': 'Java memory',
    'metric.process.threads': 'Java threads',
    'metric.minecraft.tps': 'Minecraft TPS',
    'metric.minecraft.mspt': 'Minecraft MSPT',
  },
})
