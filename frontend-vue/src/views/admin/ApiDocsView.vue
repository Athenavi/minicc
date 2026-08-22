<script setup lang="ts">
import { computed } from 'vue'

// spec URL：开发环境指向后端 8080，生产同域
const specUrl = computed(() => {
  const origin = window.location.origin
  // 开发环境（5173）→ 后端 8080；生产同域
  const apiOrigin = origin.replace(':5173', ':8080')
  return `${apiOrigin}/docs/openapi.yaml`
})

// redoc CDN 展示
const redocUrl = computed(() =>
  `https://redocly.github.io/redoc/?spec-url=${encodeURIComponent(specUrl.value)}`
)
</script>

<template>
  <div class="api-docs-page">
    <div class="api-docs-header">
      <h2>API 文档</h2>
      <p class="api-docs-desc">MiniCC API 完整文档（OpenAPI 3.0）— 含认证、聊天、企业功能、SSO、系统监控</p>
    </div>
    <div class="redoc-wrapper">
      <iframe
        :src="redocUrl"
        frameborder="0"
        class="redoc-frame"
        title="MiniCC API 文档"
      />
    </div>
  </div>
</template>

<style scoped>
.api-docs-page {
  padding: 24px;
  height: calc(100vh - 64px);
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.api-docs-header {
  margin-bottom: 16px;
  flex-shrink: 0;
}

.api-docs-header h2 {
  margin: 0 0 4px;
}

.api-docs-desc {
  color: var(--text-secondary, #666);
  margin: 0;
  font-size: 14px;
}

.redoc-wrapper {
  flex: 1;
  overflow: hidden;
  border: 1px solid var(--border-card, #e8e8e8);
  border-radius: 8px;
  background: var(--bg-card, #fff);
}

.redoc-frame {
  width: 100%;
  height: 100%;
  border: none;
}

/* 移动端 */
@media (max-width: 640px) {
  .api-docs-page { padding: 16px 12px; height: calc(100vh - 56px); }
  .api-docs-header h2 { font-size: 18px; }
  .api-docs-desc { font-size: 13px; }
}
</style>
