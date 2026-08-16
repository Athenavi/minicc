<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Input, Button, Select } from 'ant-design-vue'
import { SendOutlined, StopOutlined } from '@ant-design/icons-vue'

const props = defineProps<{
  loading: boolean
  mode: string
  modeOptions: { label: string; value: string }[]
  disabled?: boolean
}>()

const emit = defineEmits<{
  (e: 'send', text: string): void
  (e: 'stop'): void
  (e: 'update:mode', mode: string): void
}>()

const input = ref('')
const textareaRef = ref()

// 挂载自动聚焦（deepseek 输入区常驻聚焦）
onMounted(() => { textareaRef.value?.focus?.() })

function onKeydown(e: KeyboardEvent) {
  // Enter 发送（无修饰键）；Shift+Enter 换行；Cmd/Ctrl+Enter 也发送（兼容）
  if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
    e.preventDefault()
    submit()
  }
}

function submit() {
  const text = input.value.trim()
  if (!text || props.loading) return
  input.value = ''
  emit('send', text)
}
</script>

<template>
  <div class="input-area">
    <div class="input-card">
      <Input.TextArea
        ref="textareaRef"
        v-model:value="input"
        :rows="1"
        :auto-size="{ minRows: 1, maxRows: 5 }"
        placeholder="发送消息..."
        class="input-field"
        :disabled="disabled"
        @keydown="onKeydown"
      />
      <div class="input-actions">
        <div class="input-left">
          <Select
            :model-value="mode"
            :options="modeOptions"
            size="small"
            style="width: 110px"
            @update:value="(v: any) => emit('update:mode', String(v))"
          />
        </div>
        <div class="input-left">
          <span class="input-hint">Enter 发送 · Shift+Enter 换行</span>
          <Button
            :type="loading ? 'default' : 'primary'"
            shape="circle"
            :disabled="(!input.trim() && !loading) || disabled"
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
  display: flex; flex-direction: column; gap: 12px;
  width: 100%; max-width: 780px; margin: 0 auto;
  padding: 10px 12px 12px;
  border: 1px solid var(--border); border-radius: 22px;
  background: var(--bg-input); box-shadow: var(--shadow-md);
  font-size: 16px; line-height: 24px;
}
.input-field { background: transparent !important; }
.input-field :deep(textarea) { color: var(--text-primary) !important; font-size: 16px !important; line-height: 24px !important; }
.input-actions { display: flex; align-items: center; justify-content: space-between; }
.input-left { display: flex; align-items: center; gap: 8px; }
.input-hint { font-size: 12px; color: var(--text-tertiary); }
@media (max-width: 768px) { .input-area { padding: 0 12px 8px; } }
</style>
