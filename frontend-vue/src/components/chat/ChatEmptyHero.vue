<script setup lang="ts">
const emit = defineEmits<{ (e: 'suggest', text: string): void }>()

const suggestions = [
  { icon: '📝', title: '代码生成', desc: '写一段 Python 代码实现排序算法', prompt: '写一段 Python 代码实现排序算法' },
  { icon: '✏️', title: '创意写作', desc: '帮我写一篇关于 AI 的短文', prompt: '帮我写一篇关于 AI 的短文' },
  { icon: '📊', title: '数据分析', desc: '分析这份数据的趋势', prompt: '分析这份数据的趋势' },
  { icon: '🎯', title: '方案策划', desc: '帮我做一个项目计划', prompt: '帮我做一个项目计划' },
]
</script>

<template>
  <div class="hero">
    <div class="hero-glow" aria-hidden />
    <div class="hero-content">
      <div class="hero-logo">MC</div>
      <h1 class="hero-title">你好，有什么可以帮助你的？</h1>
      <div class="suggestion-grid">
        <div
          v-for="s in suggestions"
          :key="s.title"
          class="suggestion-card"
          @click="emit('suggest', s.prompt)"
        >
          <div class="card-icon">{{ s.icon }}</div>
          <div class="card-title">{{ s.title }}</div>
          <div class="card-desc">{{ s.desc }}</div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.hero { position: relative; flex: 1; display: flex; align-items: center; justify-content: center; overflow: hidden; }
.hero-glow { position: absolute; inset: -40% -20% auto -20%; height: 70%; background: radial-gradient(ellipse 50% 50% at 50% 0%, var(--primary-bg), transparent 70%); pointer-events: none; }
.hero-content { width: 560px; max-width: calc(100vw - 48px); text-align: center; position: relative; z-index: 1; }
.hero-logo { width: 56px; height: 56px; margin: 0 auto 18px; border-radius: 16px; background: linear-gradient(135deg, var(--primary), var(--primary-dark)); color: #fff; font-weight: 700; font-size: 20px; display: flex; align-items: center; justify-content: center; box-shadow: var(--shadow-lg); }
.hero-title { font-size: 24px; font-weight: 650; color: var(--text-primary); letter-spacing: -0.01em; margin-bottom: 28px; }
.suggestion-grid { display: flex; flex-wrap: wrap; gap: 12px; justify-content: center; }
.suggestion-card { width: 250px; padding: 14px; border-radius: var(--radius-lg); border: 1px solid var(--border-card); background: var(--bg-card); cursor: pointer; transition: all 0.2s; text-align: left; box-shadow: var(--shadow-md); }
.suggestion-card:hover { border-color: var(--primary); transform: translateY(-2px); box-shadow: var(--shadow-lg); }
.card-icon { font-size: 20px; margin-bottom: 8px; }
.card-title { font-size: 14px; font-weight: 600; color: var(--text-primary); margin-bottom: 4px; }
.card-desc { font-size: 12px; color: var(--text-tertiary); line-height: 1.4; }
@media (max-width: 768px) { .suggestion-card { width: 100%; } .hero-content { width: 100%; } }
</style>
