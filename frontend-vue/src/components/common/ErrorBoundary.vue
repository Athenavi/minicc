<script setup lang="ts">
import { ref, onErrorCaptured } from 'vue'
import { Button } from 'ant-design-vue'
import { ExclamationCircleOutlined, ReloadOutlined } from '@ant-design/icons-vue'

const error = ref<Error | null>(null)
const isProduction = typeof window !== 'undefined' && window.location.hostname !== 'localhost' && !window.location.hostname.startsWith('127.')

onErrorCaptured((err) => {
  error.value = err instanceof Error ? err : new Error(String(err))
  // 阻止错误继续向上传播
  return false
})

function retry() {
  error.value = null
}

function reload() {
  window.location.reload()
}
</script>

<template>
  <template v-if="error">
    <div class="error-boundary" role="alert">
      <div class="error-icon"><ExclamationCircleOutlined /></div>
      <div class="error-title">页面渲染出错</div>
      <div class="error-message">{{ isProduction ? 'An unexpected error occurred. Please try again.' : error.message }}</div>
      <div class="error-actions">
        <Button type="primary" size="small" @click="retry">
          <template #icon><ReloadOutlined /></template>
          重试
        </Button>
        <Button size="small" @click="reload">刷新页面</Button>
      </div>
    </div>
  </template>
  <slot v-else />
</template>

<style scoped>
.error-boundary {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  min-height: 320px;
  padding: 40px 24px;
  text-align: center;
}
.error-icon {
  font-size: 40px;
  color: var(--colorError, #ef4444);
}
.error-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--text-primary, #0f1115);
}
.error-message {
  font-size: 13px;
  color: var(--text-tertiary, #81858c);
  max-width: 420px;
  word-break: break-word;
  font-family: var(--font-mono, monospace);
}
.error-actions {
  display: flex;
  gap: 8px;
  margin-top: 8px;
}
</style>
