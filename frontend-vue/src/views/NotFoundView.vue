<script setup lang="ts">
import { useRouter } from 'vue-router'

const router = useRouter()
</script>

<template>
  <div class="nf-page">
    <div class="nf-code">404</div>
    <h1 class="nf-title">页面不存在</h1>
    <p class="nf-desc">你访问的地址不存在或已被移动。</p>
    <div class="nf-actions">
      <a-button type="primary" @click="router.push('/')">返回首页</a-button>
      <a-button @click="router.back()">返回上一页</a-button>
    </div>
  </div>
</template>

<style scoped>
.nf-page {
  min-height: 100dvh;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 24px;
  text-align: center;
  background: var(--bg-page);
}
.nf-code {
  font-size: 84px;
  font-weight: 700;
  line-height: 1;
  background: linear-gradient(135deg, var(--primary), var(--primary-dark));
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
}
.nf-title {
  margin: 0;
  font-size: 20px;
  color: var(--text-primary);
}
.nf-desc {
  margin: 0 0 16px;
  color: var(--text-tertiary);
  font-size: 14px;
}
.nf-actions {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  justify-content: center;
}
@media (max-width: 576px) {
  .nf-actions :deep(.ant-btn) { min-height: 40px; }
}
</style>