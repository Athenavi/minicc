<script setup lang="ts">
import { ref, onMounted, watch, computed, nextTick } from 'vue'
import { Input, Button, Select, message } from 'ant-design-vue'
import { SendOutlined, StopOutlined, PaperClipOutlined, CloseOutlined, FileOutlined, BranchesOutlined } from '@ant-design/icons-vue'
import { uploadFile } from '../../api'
import type { ChatAttachment } from './chat-types'

const props = defineProps<{
  loading: boolean
  mode: string
  modeOptions: { label: string; value: string }[]
  disabled?: boolean
  /** P2-C: 当前会话 ID，用于按会话持久化草稿 */
  sessionId?: string
}>()

const emit = defineEmits<{
  (e: 'send', text: string, attachments?: ChatAttachment[]): void
  (e: 'stop'): void
  (e: 'update:mode', mode: string): void
  /** P3-C: 斜杠命令 */
  (e: 'command', cmd: string): void
  /** 上下文快捷按钮：展开侧栏（若为抽屉模式） */
  (e: 'open-panel'): void
}>()

const input = ref('')
const textareaRef = ref()
const fileInputRef = ref<HTMLInputElement | null>(null)
const dragOver = ref(false)

// P1-2 附件管理
const pendingAttachments = ref<ChatAttachment[]>([])
const uploading = ref(false)

// 允许的文件类型（图片 + 常见文档）
const IMAGE_TYPES = ['image/png', 'image/jpeg', 'image/gif', 'image/webp', 'image/svg+xml']
const MAX_FILE_SIZE = 50 * 1024 * 1024 // 50MB

// 挂载自动聚焦（deepseek 输入区常驻聚焦）
onMounted(() => { textareaRef.value?.focus?.() })

// P2-C: 草稿持久化（按会话 ID 存 localStorage）
const DRAFT_PREFIX = 'minicc:draft:'
function draftKey(sid?: string) { return sid ? DRAFT_PREFIX + sid : '' }
function loadDraft() {
  const key = draftKey(props.sessionId)
  if (key) {
    input.value = localStorage.getItem(key) || ''
  } else {
    input.value = ''
  }
}
function saveDraft() {
  const key = draftKey(props.sessionId)
  if (!key) return
  if (input.value) localStorage.setItem(key, input.value)
  else localStorage.removeItem(key)
}
// 会话切换时加载草稿
watch(() => props.sessionId, () => loadDraft(), { immediate: true })
// 输入变化时保存草稿（防抖避免频繁写入）
let saveTimer: ReturnType<typeof setTimeout> | null = null
watch(input, () => {
  if (saveTimer) clearTimeout(saveTimer)
  saveTimer = setTimeout(saveDraft, 300)
})

function onKeydown(e: KeyboardEvent) {
  // P3-C: 斜杠命令面板导航
  if (showSlashMenu.value) {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      slashIndex.value = Math.min(slashIndex.value + 1, filteredCommands.value.length - 1)
      return
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault()
      slashIndex.value = Math.max(slashIndex.value - 1, 0)
      return
    }
    if (e.key === 'Escape') {
      showSlashMenu.value = false
      return
    }
  }
  // Enter 发送（无修饰键）；Shift+Enter 换行；Cmd/Ctrl+Enter 也发送（兼容）
  if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
    e.preventDefault()
    // P3-C: 如果在斜杠菜单上且选中了命令，执行命令而非发送
    if (showSlashMenu.value && filteredCommands.value[slashIndex.value]) {
      const cmd = filteredCommands.value[slashIndex.value].cmd
      input.value = ''
      showSlashMenu.value = false
      emit('command', cmd)
      return
    }
    submit()
  }
}

function submit() {
  const text = input.value.trim()
  if ((!text && !pendingAttachments.value.length) || props.loading) return
  const atts = pendingAttachments.value.length ? [...pendingAttachments.value] : undefined
  input.value = ''
  pendingAttachments.value = []
  // P2-C: 发送后清空草稿
  saveDraft()
  emit('send', text, atts)
}

// P1-2 文件选择
function triggerFilePick() {
  fileInputRef.value?.click()
}

async function handleFiles(files: FileList | File[]) {
  const arr = Array.from(files)
  if (!arr.length) return
  uploading.value = true
  try {
    for (const file of arr) {
      if (file.size > MAX_FILE_SIZE) {
        message.error(`${file.name} 超过 50MB 限制`)
        continue
      }
      const result = await uploadFile(file)
      pendingAttachments.value.push({
        id: result.id,
        name: result.name,
        size: result.size,
        mimeType: result.mimeType,
        url: result.url,
        isImage: IMAGE_TYPES.includes(result.mimeType),
      })
    }
  } catch (e: any) {
    message.error('文件上传失败: ' + (e.message || '网络错误'))
  } finally {
    uploading.value = false
    if (fileInputRef.value) fileInputRef.value.value = ''
  }
}

function onFileChange(e: Event) {
  const target = e.target as HTMLInputElement
  if (target.files?.length) handleFiles(target.files)
}

// P1-2 拖拽上传
function onDragOver(e: DragEvent) {
  e.preventDefault()
  dragOver.value = true
}
function onDragLeave(e: DragEvent) {
  e.preventDefault()
  dragOver.value = false
}
function onDrop(e: DragEvent) {
  e.preventDefault()
  dragOver.value = false
  if (e.dataTransfer?.files?.length) handleFiles(e.dataTransfer.files)
}

// P1-2 粘贴图片
function onPaste(e: ClipboardEvent) {
  const items = e.clipboardData?.items
  if (!items) return
  const files: File[] = []
  for (const it of items) {
    if (it.kind === 'file') {
      const f = it.getAsFile()
      if (f) files.push(f)
    }
  }
  if (files.length) handleFiles(files)
  // P3-E: 大文本粘贴折叠预览
  const text = e.clipboardData?.getData('text')
  if (text && text.length > 1000) {
    e.preventDefault()
    pastedLargeText.value = text
  }
}

// P3-E: 大文本粘贴折叠预览
const pastedLargeText = ref('')
const PASTE_PREVIEW = 200
function acceptPastedText() {
  input.value += pastedLargeText.value
  pastedLargeText.value = ''
  nextTick(() => textareaRef.value?.focus?.())
}
function discardPastedText() {
  pastedLargeText.value = ''
}

function removeAttachment(id: string) {
  const idx = pendingAttachments.value.findIndex(a => a.id === id)
  if (idx >= 0) pendingAttachments.value.splice(idx, 1)
}

// ── P3-C: 斜杠命令 ──
const SLASH_COMMANDS = [
  { cmd: '/clear', desc: '清空当前对话' },
  { cmd: '/export', desc: '导出当前会话为 Markdown' },
  { cmd: '/new', desc: '新建会话' },
  { cmd: '/theme', desc: '切换暗色/亮色模式' },
  { cmd: '/stop', desc: '停止生成' },
]
const showSlashMenu = ref(false)
const slashIndex = ref(0)
const filteredCommands = computed(() => {
  const q = input.value.trim().toLowerCase()
  if (!q.startsWith('/')) return []
  return SLASH_COMMANDS.filter(c => c.cmd.startsWith(q))
})
function onSlashInput() {
  showSlashMenu.value = filteredCommands.value.length > 0
  slashIndex.value = 0
}
</script>

<template>
  <div
    class="input-area"
    @dragover="onDragOver"
    @dragleave="onDragLeave"
    @drop="onDrop"
  >
    <div class="input-card" :class="{ 'drag-active': dragOver }">
      <!-- P3-E: 大文本粘贴折叠预览 -->
      <div v-if="pastedLargeText" class="paste-preview">
        <div class="paste-preview-text">
          {{ pastedLargeText.slice(0, PASTE_PREVIEW) }}<span v-if="pastedLargeText.length > PASTE_PREVIEW">…</span>
        </div>
        <div class="paste-preview-meta">已粘贴 {{ pastedLargeText.length }} 字符</div>
        <div class="paste-preview-actions">
          <button class="paste-btn discard" type="button" @click="discardPastedText">丢弃</button>
          <button class="paste-btn accept" type="button" @click="acceptPastedText">插入</button>
        </div>
      </div>
      <!-- P3-C: 斜杠命令面板 -->
      <div v-if="showSlashMenu" class="slash-menu">
        <div
          v-for="(c, i) in filteredCommands"
          :key="c.cmd"
          class="slash-item"
          :class="{ active: i === slashIndex }"
          @mouseenter="slashIndex = i"
          @click="emit('command', c.cmd); input = ''; showSlashMenu = false"
        >
          <span class="slash-cmd">{{ c.cmd }}</span>
          <span class="slash-desc">{{ c.desc }}</span>
        </div>
      </div>
      <!-- P1-2 附件预览区 -->
      <div v-if="pendingAttachments.length" class="attachment-preview">
        <div v-for="att in pendingAttachments" :key="att.id" class="att-thumb">
          <img v-if="att.isImage" :src="att.url" :alt="att.name" class="att-thumb-img" />
          <div v-else class="att-thumb-file">
            <FileOutlined />
            <span class="att-thumb-name">{{ att.name }}</span>
          </div>
          <button class="att-remove" type="button" title="移除" @click="removeAttachment(att.id)">
            <CloseOutlined />
          </button>
        </div>
      </div>
      <Input.TextArea
        ref="textareaRef"
        v-model:value="input"
        :rows="1"
        :auto-size="{ minRows: 1, maxRows: 5 }"
        :placeholder="dragOver ? '松开以上传文件' : '发送消息...（输入 / 查看命令）'"
        class="input-field"
        :disabled="disabled"
        aria-label="消息输入框"
        @keydown="onKeydown"
        @input="onSlashInput"
        @paste="onPaste"
      />
      <input
        ref="fileInputRef"
        type="file"
        multiple
        style="display: none"
        @change="onFileChange"
      />
      <div class="input-actions">
        <div class="input-left">
          <Button
            type="text"
            size="small"
            class="attach-btn"
            :loading="uploading"
            title="上传文件"
            @click="triggerFilePick"
          >
            <template #icon><PaperClipOutlined /></template>
          </Button>
          <span class="mode-label">模式</span>
          <Select
            :model-value="mode"
            :options="modeOptions"
            size="small"
            style="width: 110px"
            :title="`当前模式：${modeOptions.find(o => o.value === mode)?.label || mode}（仅影响后续消息）`"
            @update:value="(v: any) => emit('update:mode', String(v))"
          />
          <Button
            type="text"
            size="small"
            class="context-btn"
            title="打开上下文面板（会话/轨迹/上下文）"
            @click="emit('open-panel')"
          >
            <template #icon><BranchesOutlined /></template>
            <span class="context-label">上下文</span>
          </Button>
        </div>
        <div class="input-left">
          <span class="input-hint">Enter 发送 · Shift+Enter 换行</span>
          <Button
            class="send-btn"
            :type="loading ? 'default' : 'primary'"
            shape="circle"
            :disabled="(!input.trim() && !pendingAttachments.length && !loading) || disabled"
            :title="loading ? '停止' : '发送'"
            @click="loading ? emit('stop') : submit()"
          >
            <template #icon>
              <StopOutlined v-if="loading" />
              <SendOutlined v-else />
            </template>
          </Button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* 浮动胶囊输入卡（deepseek InputBar floating capsule：22px 圆角 + 阴影 + 16/24 字号） */
.input-area { padding: 0 16px 8px; }
.input-card {
  position: relative;
  display: flex; flex-direction: column; gap: 12px;
  width: 100%; max-width: 780px; margin: 0 auto;
  padding: 10px 12px 12px;
  border: 1px solid var(--border); border-radius: 22px;
  background: var(--bg-input); box-shadow: var(--shadow-md);
  font-size: 16px; line-height: 24px;
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}
/* 聚焦态：主色描边 + 主色光晕（deepseek InputBar focus ring） */
.input-card:focus-within {
  border-color: var(--primary);
  box-shadow: var(--shadow-md), 0 0 0 3px var(--primary-bg);
}
.input-field { background: transparent !important; }
.input-field :deep(textarea) { color: var(--text-primary) !important; font-size: 16px !important; line-height: 24px !important; }
.input-actions { display: flex; align-items: center; justify-content: space-between; }
.input-left { display: flex; align-items: center; gap: 8px; }
.input-hint { font-size: 12px; color: var(--text-tertiary); }
/* 模式选择器标签 */
.mode-label { flex: none; font-size: 12px; color: var(--text-tertiary); }
/* 上下文快捷按钮：展开侧栏（抽屉模式） */
.context-btn { color: var(--text-tertiary); display: inline-flex; align-items: center; gap: 4px; }
.context-btn:hover { color: var(--primary) !important; }
.context-label { font-size: 12px; }
@media (max-width: 576px) {
  .context-label { display: none; }
  .context-btn.ant-btn { min-width: 40px; height: 40px; }
}
/* 发送按钮：可发送时主色、hover 微放大 + 加深 */
.send-btn { transition: transform 0.15s ease, box-shadow 0.15s ease, opacity 0.15s ease; }
.send-btn:not(:disabled):hover { transform: scale(1.06); box-shadow: 0 4px 12px var(--primary-bg); }
.send-btn:disabled { opacity: 0.45; }
@media (max-width: 768px) { .input-area { padding: 0 12px 8px; } }
/* ── 移动端：输入区贴底 + 安全区 + 触控目标放大 + 工具栏换行 ── */
@media (max-width: 768px) {
  .input-area { padding: 0 12px calc(8px + env(safe-area-inset-bottom)); }
  .input-card { border-radius: 18px; }
  .input-actions { gap: 8px; }
  .input-hint { display: none; } /* 窄屏隐藏提示文字，占位符承担语义 */
}
@media (max-width: 576px) {
  .input-area { padding: 0 8px calc(8px + env(safe-area-inset-bottom)); }
  .input-card { padding: 8px 10px 10px; gap: 10px; }
  .input-actions { flex-wrap: wrap; row-gap: 6px; }
  .input-left { gap: 4px; }
  .attach-btn.ant-btn { min-width: 40px; height: 40px; }
  .send-btn.ant-btn { width: 40px; height: 40px; }
  .paste-btn { min-height: 36px; }
}

/* P1-2 附件预览区 */
.attachment-preview { display: flex; flex-wrap: wrap; gap: 8px; padding: 4px 0 8px; }
.att-thumb { position: relative; width: 64px; height: 64px; border-radius: 8px; border: 1px solid var(--border); background: var(--bg-card); overflow: hidden; }
.att-thumb-img { width: 100%; height: 100%; object-fit: cover; }
.att-thumb-file { width: 100%; height: 100%; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 4px; padding: 4px; color: var(--text-tertiary); font-size: 10px; }
.att-thumb-name { max-width: 56px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.att-remove { position: absolute; top: 2px; right: 2px; width: 18px; height: 18px; border-radius: 50%; border: none; background: rgba(0,0,0,0.6); color: #fff; display: flex; align-items: center; justify-content: center; cursor: pointer; font-size: 10px; }
.att-remove:hover { background: rgba(220,38,38,0.85); }
.attach-btn { color: var(--text-tertiary); display: inline-flex; align-items: center; justify-content: center; }
.attach-btn:hover { color: var(--primary); }
/* 拖拽态：边框主色 + 背景淡色 */
.input-card.drag-active { border-color: var(--primary); background: var(--primary-bg); box-shadow: var(--shadow-md), 0 0 0 3px var(--primary-bg); }

/* P3-C: 斜杠命令面板 */
.slash-menu { position: absolute; bottom: 100%; left: 0; right: 0; margin-bottom: 4px; background: var(--bg-card); border: 1px solid var(--border); border-radius: 8px; box-shadow: var(--shadow-lg); overflow: hidden; z-index: 10; }
.slash-item { display: flex; justify-content: space-between; align-items: center; padding: 8px 12px; cursor: pointer; transition: background 0.1s ease; }
.slash-item.active { background: var(--bg-hover); }
.slash-cmd { font-weight: 600; color: var(--primary); font-size: 13px; }
.slash-desc { color: var(--text-tertiary); font-size: 12px; }

/* P3-E: 大文本粘贴折叠预览 */
.paste-preview { border: 1px solid var(--border); border-radius: 8px; padding: 8px 12px; background: var(--bg-secondary); margin-bottom: 4px; }
.paste-preview-text { font-size: 12px; color: var(--text-secondary); line-height: 1.5; max-height: 80px; overflow: hidden; white-space: pre-wrap; word-break: break-all; }
.paste-preview-meta { font-size: 11px; color: var(--text-tertiary); margin: 4px 0; }
.paste-preview-actions { display: flex; justify-content: flex-end; gap: 8px; }
.paste-btn { padding: 2px 10px; border-radius: 6px; border: 1px solid var(--border); background: var(--bg-card); color: var(--text-secondary); font-size: 12px; cursor: pointer; }
.paste-btn.accept { background: var(--primary); color: #fff; border-color: var(--primary); }
.paste-btn.accept:hover { opacity: 0.9; }
.paste-btn.discard:hover { color: var(--danger, #dc2626); border-color: var(--danger, #dc2626); }
</style>
