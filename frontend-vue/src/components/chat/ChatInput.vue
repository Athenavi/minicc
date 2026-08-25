<script setup lang="ts">
import { ref, onMounted, watch, computed, nextTick } from 'vue'
import { Input, Button, Select, message } from 'ant-design-vue'
import { SendOutlined, StopOutlined, PaperClipOutlined, CloseOutlined, FileOutlined, BranchesOutlined } from '@ant-design/icons-vue'
import { uploadFile, listModels } from '../../api'
import type { LlmModel } from '../../api'
import type { ChatAttachment } from './chat-types'

const props = defineProps<{
  loading: boolean
  mode: string
  modeOptions: { label: string; value: string }[]
  disabled?: boolean
  /** P2-C: 褰撳墠浼氳瘽 ID锛岀敤浜庢寜浼氳瘽鎸佷箙鍖栬崏绋?*/
  sessionId?: string
  /** 妯″瀷璺敱锛氬綋鍓嶄細璇?llm_config.model锛堢┖ = 鍚庣榛樿璺敱锛?*/
  model?: string
}>()

const emit = defineEmits<{
  (e: 'send', text: string, attachments?: ChatAttachment[]): void
  (e: 'stop'): void
  (e: 'update:mode', mode: string): void
  /** 妯″瀷璺敱锛氱敤鎴烽€夋嫨浜嗘ā鍨嬶紙绌哄瓧绗︿覆 = 鎭㈠鍚庣榛樿锛?*/
  (e: 'model-change', model: string): void
  /** P3-C: 鏂滄潬鍛戒护 */
  (e: 'command', cmd: string): void
  /** 涓婁笅鏂囧揩鎹锋寜閽細灞曞紑渚ф爮锛堣嫢涓烘娊灞夋ā寮忥級 */
  (e: 'open-panel'): void
}>()

const input = ref('')
const textareaRef = ref()
const fileInputRef = ref<HTMLInputElement | null>(null)
const dragOver = ref(false)

// P1-2 闄勪欢绠＄悊
const pendingAttachments = ref<ChatAttachment[]>([])
const uploading = ref(false)

// 鍏佽鐨勬枃浠剁被鍨嬶紙鍥剧墖 + 甯歌鏂囨。锛?const IMAGE_TYPES = ['image/png', 'image/jpeg', 'image/gif', 'image/webp', 'image/svg+xml']
const MAX_FILE_SIZE = 50 * 1024 * 1024 // 50MB

// 鈹€鈹€ 妯″瀷璺敱閫夋嫨鍣紙GET /v1/models锛屼粎 enabled锛夆攢鈹€
const models = ref<LlmModel[]>([])
const modelsLoading = ref(false)
const modelValue = ref('')
const modelOptions = computed(() =>
  models.value.map(m => ({
    // label锛歞isplay_name + provider 鍓嶇紑锛泇alue锛氭ā鍨?name锛堝悗绔矾鐢辩敤锛?    label: m.display_name ? `${m.provider} 路 ${m.display_name}` : `${m.provider} 路 ${m.name}`,
    value: m.name,
  })),
)

function onModelChange(v: any) {
  modelValue.value = String(v || '')
  emit('model-change', modelValue.value)
}

// 浼氳瘽鍒囨崲/鎭㈠鏃跺悓姝ワ紙鐖剁粍浠跺洖浼?llm_config.model锛涚┖ = 鍚庣榛樿锛?watch(() => props.model, (v) => { modelValue.value = v || '' }, { immediate: true })

// 鎸傝浇鑷姩鑱氱劍锛坉eepseek 杈撳叆鍖哄父椹昏仛鐒︼級+ 鍔犺浇鍙敤妯″瀷
onMounted(async () => {
  textareaRef.value?.focus?.()
  try {
    modelsLoading.value = true
    models.value = await listModels()
  } catch {
    // 鍙栦笉鍒版ā鍨嬪垪琛ㄦ椂涓嬫媺涓虹┖锛岃蛋鍚庣榛樿璺敱
    models.value = []
  } finally {
    modelsLoading.value = false
  }
})

// P2-C: 鑽夌鎸佷箙鍖栵紙鎸変細璇?ID 瀛?localStorage锛?const DRAFT_PREFIX = 'chiron:draft:'
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
// 浼氳瘽鍒囨崲鏃跺姞杞借崏绋?watch(() => props.sessionId, () => loadDraft(), { immediate: true })
// 杈撳叆鍙樺寲鏃朵繚瀛樿崏绋匡紙闃叉姈閬垮厤棰戠箒鍐欏叆锛?let saveTimer: ReturnType<typeof setTimeout> | null = null
watch(input, () => {
  if (saveTimer) clearTimeout(saveTimer)
  saveTimer = setTimeout(saveDraft, 300)
})

function onKeydown(e: KeyboardEvent) {
  // P3-C: 鏂滄潬鍛戒护闈㈡澘瀵艰埅
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
  // Enter 鍙戦€侊紙鏃犱慨楗伴敭锛夛紱Shift+Enter 鎹㈣锛汣md/Ctrl+Enter 涔熷彂閫侊紙鍏煎锛?  if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
    e.preventDefault()
    // P3-C: 濡傛灉鍦ㄦ枩鏉犺彍鍗曚笂涓旈€変腑浜嗗懡浠わ紝鎵ц鍛戒护鑰岄潪鍙戦€?    if (showSlashMenu.value && filteredCommands.value[slashIndex.value]) {
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
  // P2-C: 鍙戦€佸悗娓呯┖鑽夌
  saveDraft()
  emit('send', text, atts)
}

// P1-2 鏂囦欢閫夋嫨
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
        message.error(`${file.name} 瓒呰繃 50MB 闄愬埗`)
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
    message.error('鏂囦欢涓婁紶澶辫触: ' + (e.message || '缃戠粶閿欒'))
  } finally {
    uploading.value = false
    if (fileInputRef.value) fileInputRef.value.value = ''
  }
}

function onFileChange(e: Event) {
  const target = e.target as HTMLInputElement
  if (target.files?.length) handleFiles(target.files)
}

// P1-2 鎷栨嫿涓婁紶
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

// P1-2 绮樿创鍥剧墖
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
  // P3-E: 澶ф枃鏈矘璐存姌鍙犻瑙?  const text = e.clipboardData?.getData('text')
  if (text && text.length > 1000) {
    e.preventDefault()
    pastedLargeText.value = text
  }
}

// P3-E: 澶ф枃鏈矘璐存姌鍙犻瑙?const pastedLargeText = ref('')
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

// 鈹€鈹€ P3-C: 鏂滄潬鍛戒护 鈹€鈹€
const SLASH_COMMANDS = [
  { cmd: '/clear', desc: '娓呯┖褰撳墠瀵硅瘽' },
  { cmd: '/export', desc: '瀵煎嚭褰撳墠浼氳瘽涓?Markdown' },
  { cmd: '/new', desc: '鏂板缓浼氳瘽' },
  { cmd: '/theme', desc: '鍒囨崲鏆楄壊/浜壊妯″紡' },
  { cmd: '/stop', desc: '鍋滄鐢熸垚' },
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
      <!-- P3-E: 澶ф枃鏈矘璐存姌鍙犻瑙?-->
      <div v-if="pastedLargeText" class="paste-preview">
        <div class="paste-preview-text">
          {{ pastedLargeText.slice(0, PASTE_PREVIEW) }}<span v-if="pastedLargeText.length > PASTE_PREVIEW">鈥?/span>
        </div>
        <div class="paste-preview-meta">宸茬矘璐?{{ pastedLargeText.length }} 瀛楃</div>
        <div class="paste-preview-actions">
          <button class="paste-btn discard" type="button" @click="discardPastedText">涓㈠純</button>
          <button class="paste-btn accept" type="button" @click="acceptPastedText">鎻掑叆</button>
        </div>
      </div>
      <!-- P3-C: 鏂滄潬鍛戒护闈㈡澘 -->
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
      <!-- P1-2 闄勪欢棰勮鍖?-->
      <div v-if="pendingAttachments.length" class="attachment-preview">
        <div v-for="att in pendingAttachments" :key="att.id" class="att-thumb">
          <img v-if="att.isImage" :src="att.url" :alt="att.name" class="att-thumb-img" />
          <div v-else class="att-thumb-file">
            <FileOutlined />
            <span class="att-thumb-name">{{ att.name }}</span>
          </div>
          <button class="att-remove" type="button" title="绉婚櫎" @click="removeAttachment(att.id)">
            <CloseOutlined />
          </button>
        </div>
      </div>
      <Input.TextArea
        ref="textareaRef"
        v-model:value="input"
        :rows="1"
        :auto-size="{ minRows: 1, maxRows: 5 }"
        :placeholder="dragOver ? '鏉惧紑浠ヤ笂浼犳枃浠? : '鍙戦€佹秷鎭?..锛堣緭鍏?/ 鏌ョ湅鍛戒护锛?"
        class="input-field"
        :disabled="disabled"
        aria-label="娑堟伅杈撳叆妗?
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
            title="涓婁紶鏂囦欢"
            @click="triggerFilePick"
          >
            <template #icon><PaperClipOutlined /></template>
          </Button>
          <span class="mode-label">妯″紡</span>
          <Select
            :model-value="mode"
            :options="modeOptions"
            size="small"
            style="width: 110px"
            :title="`褰撳墠妯″紡锛?{modeOptions.find(o => o.value === mode)?.label || mode}锛堜粎褰卞搷鍚庣画娑堟伅锛塦"
            @update:value="(v: any) => emit('update:mode', String(v))"
          />
          <span class="mode-label">妯″瀷</span>
          <Select
            class="model-select"
            :model-value="modelValue"
            :options="modelOptions"
            :loading="modelsLoading"
            size="small"
            allow-clear
            placeholder="榛樿妯″瀷"
            :title="`褰撳墠妯″瀷锛?{modelValue || '榛樿锛堝悗绔矾鐢憋級'}锛堜粎褰卞搷鍚庣画娑堟伅锛塦"
            @update:value="onModelChange"
          />
          <Button
            type="text"
            size="small"
            class="context-btn"
            title="鎵撳紑涓婁笅鏂囬潰鏉匡紙浼氳瘽/杞ㄨ抗/涓婁笅鏂囷級"
            @click="emit('open-panel')"
          >
            <template #icon><BranchesOutlined /></template>
            <span class="context-label">涓婁笅鏂?/span>
          </Button>
        </div>
        <div class="input-left">
          <span class="input-hint">Enter 鍙戦€?路 Shift+Enter 鎹㈣</span>
          <Button
            class="send-btn"
            :type="loading ? 'default' : 'primary'"
            shape="circle"
            :disabled="(!input.trim() && !pendingAttachments.length && !loading) || disabled"
            :title="loading ? '鍋滄' : '鍙戦€?"
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
/* 娴姩鑳跺泭杈撳叆鍗★紙deepseek InputBar floating capsule锛?2px 鍦嗚 + 闃村奖 + 16/24 瀛楀彿锛?*/
.input-area { padding: 0 16px 8px; }
.input-card {
  position: relative;
  display: flex; flex-direction: column; gap: 12px;
  width: 100%; max-width: 780px; margin: 0 auto;
  padding: 10px 12px 12px;
  border: var(--sig-border-width) solid var(--border); border-radius: var(--sig-radius-input);
  background: var(--bg-input); box-shadow: var(--sig-input-shadow);
  font-size: 16px; line-height: 24px;
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}
/* 鑱氱劍鎬侊細涓昏壊鎻忚竟 + 涓昏壊鍏夋檿 */
.input-card:focus-within {
  border-color: var(--primary);
  box-shadow: var(--sig-input-shadow), 0 0 0 var(--sig-input-ring) var(--primary-bg);
}
.input-field { background: transparent !important; }
.input-field :deep(textarea) { color: var(--text-primary) !important; font-size: 16px !important; line-height: 24px !important; }
.input-actions { display: flex; align-items: center; justify-content: space-between; }
.input-left { display: flex; align-items: center; gap: 8px; }
.input-hint { font-size: 12px; color: var(--text-tertiary); }
/* 妯″紡閫夋嫨鍣ㄦ爣绛?*/
.mode-label { flex: none; font-size: 12px; color: var(--text-tertiary); }
/* 妯″瀷璺敱涓嬫媺锛堜笌妯″紡鍒囨崲鍣ㄥ悓鎺掞紱绐勫睆鏀剁獎闃叉孩鍑猴級 */
.model-select { width: 170px; }
.model-select :deep(.ant-select-selector) { font-size: 12px; }
@media (max-width: 768px) { .model-select { width: 150px; } }
@media (max-width: 576px) { .model-select { width: 126px; } }
/* 涓婁笅鏂囧揩鎹锋寜閽細灞曞紑渚ф爮锛堟娊灞夋ā寮忥級 */
.context-btn { color: var(--text-tertiary); display: inline-flex; align-items: center; gap: 4px; }
.context-btn:hover { color: var(--primary) !important; }
.context-label { font-size: 12px; }
@media (max-width: 576px) {
  .context-label { display: none; }
  .context-btn.ant-btn { min-width: 40px; height: 40px; }
}
/* 鍙戦€佹寜閽細鍙彂閫佹椂涓昏壊銆乭over 寰斁澶?+ 鍔犳繁 */
.send-btn { transition: transform 0.15s ease, box-shadow 0.15s ease, opacity 0.15s ease; }
.send-btn:not(:disabled):hover { transform: scale(1.06); box-shadow: 0 4px 12px var(--primary-bg); filter: brightness(1.05); }
.send-btn:disabled { opacity: 0.45; }
@media (max-width: 768px) { .input-area { padding: 0 12px 8px; } }
/* 鈹€鈹€ 绉诲姩绔細杈撳叆鍖鸿创搴?+ 瀹夊叏鍖?+ 瑙︽帶鐩爣鏀惧ぇ + 宸ュ叿鏍忔崲琛?鈹€鈹€ */
@media (max-width: 768px) {
  .input-area { padding: 0 12px calc(8px + env(safe-area-inset-bottom)); }
  .input-card { border-radius: var(--sig-radius-input); }
  .input-actions { gap: 8px; flex-wrap: wrap; row-gap: 6px; }
  .input-hint { display: none; } /* 绐勫睆闅愯棌鎻愮ず鏂囧瓧锛屽崰浣嶇鎵挎媴璇箟 */
}
@media (max-width: 576px) {
  .input-area { padding: 0 8px calc(8px + env(safe-area-inset-bottom)); }
  .input-card { padding: 8px 10px 10px; gap: 10px; }
  .input-actions { flex-wrap: wrap; row-gap: 6px; }
  .input-left { gap: 4px; }
  .input-left:last-child { margin-left: auto; } /* 鍙戦€佺粍闈犲彸锛岄伩鍏嶄笌宸︿晶宸ュ叿缁勬姠琛?*/
  .attach-btn.ant-btn { min-width: 40px; height: 40px; }
  .send-btn.ant-btn { width: 40px; height: 40px; }
  .paste-btn { min-height: 36px; }
}

/* P1-2 闄勪欢棰勮鍖?*/
.attachment-preview { display: flex; flex-wrap: wrap; gap: 8px; padding: 4px 0 8px; }
.att-thumb { position: relative; width: 64px; height: 64px; border-radius: var(--sig-radius-card); border: 1px solid var(--border); background: var(--bg-card); overflow: hidden; }
.att-thumb-img { width: 100%; height: 100%; object-fit: cover; }
.att-thumb-file { width: 100%; height: 100%; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 4px; padding: 4px; color: var(--text-tertiary); font-size: 10px; }
.att-thumb-name { max-width: 56px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.att-remove { position: absolute; top: 2px; right: 2px; width: 18px; height: 18px; border-radius: 50%; border: none; background: var(--bg-overlay); color: #fff; display: flex; align-items: center; justify-content: center; cursor: pointer; font-size: 10px; }
.att-remove:hover { background: var(--error); }
.attach-btn { color: var(--text-tertiary); display: inline-flex; align-items: center; justify-content: center; }
.attach-btn:hover { color: var(--primary); }
/* 鎷栨嫿鎬侊細杈规涓昏壊 + 鑳屾櫙娣¤壊 */
.input-card.drag-active { border-color: var(--primary); background: var(--primary-bg); box-shadow: var(--shadow-md), 0 0 0 3px var(--primary-bg); }

/* P3-C: 鏂滄潬鍛戒护闈㈡澘 */
.slash-menu { position: absolute; bottom: 100%; left: 0; right: 0; margin-bottom: 4px; background: var(--bg-card); border: 1px solid var(--border); border-radius: var(--sig-radius-card); box-shadow: var(--shadow-lg); overflow: hidden; z-index: 10; }
.slash-item { display: flex; justify-content: space-between; align-items: center; padding: 8px 12px; cursor: pointer; transition: background 0.1s ease; }
.slash-item.active { background: var(--bg-hover); }
.slash-cmd { font-weight: 600; color: var(--primary); font-size: 13px; }
.slash-desc { color: var(--text-tertiary); font-size: 12px; }

/* P3-E: 澶ф枃鏈矘璐存姌鍙犻瑙?*/
.paste-preview { border: 1px solid var(--border); border-radius: var(--sig-radius-card); padding: 8px 12px; background: var(--bg-secondary); margin-bottom: 4px; }
.paste-preview-text { font-size: 12px; color: var(--text-secondary); line-height: 1.5; max-height: 80px; overflow: hidden; white-space: pre-wrap; word-break: break-all; }
.paste-preview-meta { font-size: 11px; color: var(--text-tertiary); margin: 4px 0; }
.paste-preview-actions { display: flex; justify-content: flex-end; gap: 8px; }
.paste-btn { padding: 2px 10px; border-radius: var(--sig-radius-button); border: 1px solid var(--border); background: var(--bg-card); color: var(--text-secondary); font-size: 12px; cursor: pointer; }
.paste-btn.accept { background: var(--primary); color: #fff; border-color: var(--primary); }
.paste-btn.accept:hover { opacity: 0.9; }
.paste-btn.discard:hover { color: var(--error); border-color: var(--error); }
</style>

