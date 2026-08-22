// chat 共享类型与工具函数

export interface ChatSession {
  id: string
  title: string
  /** 置顶会话固定在列表顶部（登录用户存 DB，guest 随 localStorage 持久化） */
  pinned?: boolean
  /** P3-D: 会话标签（前端 localStorage 存储，用于分类筛选） */
  tag?: string
  created_at: string
  updated_at: string
}

export type ToolStatus = 'running' | 'done' | 'error'

export interface DateDividerItem {
  kind: 'date_divider'
  content: string // 日期文本（如 8月16日）
  id?: string
  time?: string
}

export interface ChatItemBase {
  kind: string
  /** 消息时间（HH:mm 字符串）；历史消息来自 created_at，实时消息缺省（渲染时取当前时间） */
  time?: string
  /** 稳定 id（虚拟列表 key + 流式定位；历史=消息 id，实时=运行时生成） */
  id?: string
}

export interface TextItem extends ChatItemBase {
  kind: 'text'
  role: 'user' | 'assistant'
  content: string
  streaming?: boolean
  /** 消息发送失败（网络/服务端错误），UI 展示重试按钮 */
  error?: boolean
  /** 失败消息的错误提示文案 */
  errorMsg?: string
  /** 附件列表（用户消息可携带图片/文件，发送时一并上传） */
  attachments?: ChatAttachment[]
  /** P2-F: 生成被用户手动停止，显示"继续生成"提示 */
  stopped?: boolean
}

export interface ChatAttachment {
  id: string
  name: string
  size: number
  mimeType: string
  /** 上传后的资源 URL（如 /v1/media/{id}/download 或 data: URL 预览） */
  url: string
  /** 是否为图片（图片在消息气泡内内联展示） */
  isImage: boolean
}

export interface ReasoningItem extends ChatItemBase {
  kind: 'reasoning'
  content: string
  streaming?: boolean
}

export interface ToolCallItem extends ChatItemBase {
  kind: 'tool_call'
  id: string
  name: string
  arguments: string
  status: ToolStatus
}

export interface ToolResultItem extends ChatItemBase {
  kind: 'tool_result'
  toolCallId: string
  content: string
  isError: boolean
}

export interface TurnStatsItem extends ChatItemBase {
  kind: 'turn_stats'
  inputTokens: number
  outputTokens: number
  durationSec?: number
}

export type ChatItem = TextItem | ReasoningItem | ToolCallItem | ToolResultItem | TurnStatsItem | DateDividerItem

export const THINK_START = '[thinking]'
export const THINK_END = '[/thinking]'

/** 格式化文件大小 */
export function formatSize(bytes: number): string {
  if (!bytes) return ''
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

/** 相对时间（刚刚/N 分钟前/N 小时前/日期） */
export function formatRelativeTime(iso: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  const now = new Date()
  const diff = now.getTime() - d.getTime()
  if (diff < 60000) return '刚刚'
  if (diff < 3600000) return `${Math.floor(diff / 60000)} 分钟前`
  if (diff < 86400000) return `${Math.floor(diff / 3600000)} 小时前`
  return d.toLocaleDateString('zh-CN', { month: 'short', day: 'numeric' })
}

/** 时钟时间（HH:mm）：历史消息日期还原（S 修复：不再永远显示当前时间） */
export function formatClock(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return `${d.getHours().toString().padStart(2, '0')}:${d.getMinutes().toString().padStart(2, '0')}`
}

/** 解析流式内容中的思考块与正文（deepseek ReasoningRow 语义）
 *
 * 语义依据：main.py 的思考模式 prompt 要求模型"先输出 [thinking]...[/thinking]
 * 再输出回答"，故思考块只解析**消息开头**位置：
 * 1. 开头完整闭合 `[thinking]...[/thinking]` → 提取为 reasoning，其余为 body
 * 2. 开头未闭合 `[thinking]...`（流式进行中）→ 全部为 reasoning
 * 3. 其它位置出现 [thinking] 字样（讲解/代码示例）→ 完整保留在正文，绝不吞内容
 *
 * loose=true（历史消息回放）：流式保存的原始文本可能含多次/碎片化
 * [thinking] 标签（chunk 边界切割残留），此时全局提取配对块并剥离孤立标签。
 */
export function splitThinking(src: string, opts?: { loose?: boolean }): { reasoning: string; body: string } {
  if (opts?.loose) {
    const reasoningParts: string[] = []
    const body = src
      .replace(/\[thinking\]([\s\S]*?)\[\/thinking\]/g, (_m, b: string) => {
        const t = b.trim()
        if (t) reasoningParts.push(t)
        return ''
      })
      .replace(/\[thinking\]|\[\/thinking\]/g, '') // 剥离流式切割残留的孤立标签
    return { reasoning: reasoningParts.join('\n').trim(), body: body.trim() }
  }
  const closed = src.match(/^\s*\[thinking\]([\s\S]*?)\[\/thinking\]([\s\S]*)$/)
  if (closed) {
    return { reasoning: closed[1].trim(), body: closed[2].trim() }
  }
  const tail = src.match(/^\s*\[thinking\]([\s\S]*)$/)
  if (tail) {
    return { reasoning: tail[1].trim(), body: '' }
  }
  return { reasoning: '', body: src.trim() }
}

/** 剥离后端安全净化添加的 <user_input> 包装（Go InputSanitizer.Sanitize） */
export function stripUserInputTag(content: string): string {
  return content.replace(/^\s*<user_input>\s*([\s\S]*?)\s*<\/user_input>\s*$/, '$1').trim()
}

/** rAF 节流（deepseek use-throttled-visual-update） */
export function throttleRaf<T extends (...args: any[]) => void>(fn: T): T {
  let raf = 0
  let lastArgs: any[]
  const wrapped = ((...args: any[]) => {
    lastArgs = args
    if (raf) return
    raf = requestAnimationFrame(() => {
      raf = 0
      fn(...lastArgs)
    })
  }) as T
  return wrapped
}
