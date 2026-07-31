import type { Terminal } from '@xterm/xterm'

export interface TerminalOutputSnapshot {
  queuedBytes: number
  droppedBytes: number
  writtenBytes: number
}

interface QueuedChunk {
  data: string
  bytes: number
}

// TerminalOutputBuffer 在生产者输出快于界面渲染时限制终端的渲染量。
export class TerminalOutputBuffer {
  private readonly encoder = new TextEncoder()
  private readonly queue: QueuedChunk[] = []
  private queuedBytes = 0
  private droppedBytes = 0
  private writtenBytes = 0
  private scheduledFrame: number | null = null
  private writing = false
  private disposed = false

  /**
   * @param terminal - xterm.js 实例
   * @param maximumQueuedBytes - 最大排队字节数
   * @param drainBytesPerFrame - 每帧最大写入字节数
   */
  public constructor(
    private readonly terminal: Terminal,
    private readonly maximumQueuedBytes = 2 * 1024 * 1024,
    private readonly drainBytesPerFrame = 128 * 1024,
  ) {}

  /**
   * @param data - 待渲染输出
   * @returns void
   */
  public enqueue(data: string): void {
    if (this.disposed || data.length === 0) {
      return
    }

    const bytes = this.encoder.encode(data).byteLength
    if (bytes > this.maximumQueuedBytes) {
      this.droppedBytes += bytes
      return
    }

    while (this.queuedBytes + bytes > this.maximumQueuedBytes && this.queue.length > 0) {
      const dropped = this.queue.shift()!
      this.queuedBytes -= dropped.bytes
      this.droppedBytes += dropped.bytes
    }

    this.queue.push({ data, bytes })
    this.queuedBytes += bytes
    this.scheduleDrain()
  }

  /**
   * @returns 当前队列与吞吐计数器
   */
  public snapshot(): TerminalOutputSnapshot {
    return {
      queuedBytes: this.queuedBytes,
      droppedBytes: this.droppedBytes,
      writtenBytes: this.writtenBytes,
    }
  }

  /**
   * @returns void
   */
  public dispose(): void {
    this.disposed = true
    if (this.scheduledFrame !== null) {
      window.cancelAnimationFrame(this.scheduledFrame)
    }
    this.scheduledFrame = null
    this.queue.length = 0
    this.queuedBytes = 0
  }

  private scheduleDrain(): void {
    if (this.disposed || this.writing || this.scheduledFrame !== null) {
      return
    }
    this.scheduledFrame = window.requestAnimationFrame(() => this.drain())
  }

  private drain(): void {
    this.scheduledFrame = null
    if (this.disposed || this.writing || this.queue.length === 0) {
      return
    }

    let drainedBytes = 0
    let payload = ''
    while (this.queue.length > 0 && drainedBytes < this.drainBytesPerFrame) {
      const chunk = this.queue.shift()!
      this.queuedBytes -= chunk.bytes
      drainedBytes += chunk.bytes
      payload += chunk.data
    }

    this.writing = true
    this.terminal.write(payload, () => {
      this.writing = false
      this.writtenBytes += drainedBytes
      this.scheduleDrain()
    })
  }
}
