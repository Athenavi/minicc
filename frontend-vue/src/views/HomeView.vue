<script setup lang="ts">
import { markRaw, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Button } from 'ant-design-vue'
import {
  MessageOutlined, UserOutlined, ApartmentOutlined, BlockOutlined,
  BookOutlined, ThunderboltOutlined, ArrowRightOutlined, ArrowUpOutlined,
} from '@ant-design/icons-vue'
import HomeScene3D from '../components/home/HomeScene3D.vue'
import WorkstationNav from '../components/WorkstationNav.vue'

const router = useRouter()

interface Feature {
  title: string
  en: string
  desc: string
  path: string
  icon: any
}

// 图标组件须 markRaw：避免被 Vue 响应式代理（图标是静态组件，代理会破坏渲染）
const features: Feature[] = [
  { title: '对话', en: 'CHAT', desc: '常规 / 极简 / PTC / 创造四种模式，工具调用全程可视化', path: '/chat', icon: markRaw(MessageOutlined) },
  { title: 'Agent', en: 'AGENTS', desc: '多智能体协同，任务分发与结果追踪', path: '/agents', icon: markRaw(UserOutlined) },
  { title: '工作流', en: 'WORKFLOW', desc: '可视化编排多步任务，节点自由连线', path: '/workflow', icon: markRaw(ApartmentOutlined) },
  { title: '技能', en: 'SKILLS', desc: '插件化技能市场，按需装载与卸载', path: '/skills', icon: markRaw(BlockOutlined) },
  { title: '知识库', en: 'KNOWLEDGE', desc: '文档入库、向量检索，让 Agent 有据可依', path: '/knowledge', icon: markRaw(BookOutlined) },
  { title: '插件', en: 'PLUGINS', desc: 'MCP 服务配置，扩展 Agent 的能力边界', path: '/plugins', icon: markRaw(ThunderboltOutlined) },
]

const QUICKSTART_CMD = 'docker compose up -d postgres redis\ncp .env.example .env\npython run.py start'

const copied = ref(false)

async function copyQuickstart() {
  try {
    await navigator.clipboard.writeText(QUICKSTART_CMD)
    copied.value = true
    setTimeout(() => { copied.value = false }, 2000)
  } catch { /* clipboard unavailable */ }
}

function scrollToFeatures() {
  document.getElementById('features')?.scrollIntoView({ behavior: 'smooth' })
}

// ── 特性卡片：IntersectionObserver 交错入场 + hover 3D tilt ──
// 触屏设备禁用 3D tilt 和 CTA 磁吸（无鼠标 hover 语义，且会触发误触抖动）
const isTouch = typeof window !== 'undefined'
  && ('ontouchstart' in window || navigator.maxTouchPoints > 0)

const cardEls = ref<(HTMLElement | null)[]>([])
let cardObserver: IntersectionObserver | null = null
let cardFallbackTimer: number | undefined

function setCardRef(el: HTMLElement | null, index: number) {
  cardEls.value[index] = el
}

function revealCards() {
  cardEls.value.forEach(el => el?.classList.add('visible'))
  cardObserver?.disconnect()
}

onMounted(() => {
  cardObserver = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (entry.isIntersecting) {
          entry.target.classList.add('visible')
          cardObserver?.unobserve(entry.target)
        }
      }
    },
    { threshold: 0.12 },
  )
  cardEls.value.forEach(el => el && cardObserver?.observe(el))
  // 兜底：即使 IO 未触发（滚动容器异常等），2s 后强制显示所有卡片
  cardFallbackTimer = window.setTimeout(revealCards, 2000)
  window.addEventListener('scroll', onScroll, { passive: true })
})
onUnmounted(() => {
  cardObserver?.disconnect()
  if (cardFallbackTimer !== undefined) window.clearTimeout(cardFallbackTimer)
  window.removeEventListener('scroll', onScroll)
})

function onCardMove(e: MouseEvent) {
  if (isTouch) return
  const card = e.currentTarget as HTMLElement
  const r = card.getBoundingClientRect()
  const px = (e.clientX - r.left) / r.width - 0.5
  const py = (e.clientY - r.top) / r.height - 0.5
  card.style.transform =
    `perspective(600px) translateY(-3px) rotateX(${(-py * 6).toFixed(2)}deg) rotateY(${(px * 6).toFixed(2)}deg)`
}

function onCardLeave(e: MouseEvent) {
  const card = e.currentTarget as HTMLElement
  card.style.transform = ''
}

// ── CTA 磁吸：按钮轻微跟随鼠标 ──
function onCtaMove(e: MouseEvent) {
  if (isTouch) return
  const btn = e.currentTarget as HTMLElement
  const r = btn.getBoundingClientRect()
  const dx = (e.clientX - r.left - r.width / 2) * 0.12
  const dy = (e.clientY - r.top - r.height / 2) * 0.12
  btn.style.transform = `translate(${dx.toFixed(1)}px, ${dy.toFixed(1)}px)`
}

function onCtaLeave(e: MouseEvent) {
  const btn = e.currentTarget as HTMLElement
  btn.style.transform = ''
}

// ── 滚动到顶部按钮（长页面导航辅助） ──
const showTop = ref(false)
function onScroll() {
  showTop.value = window.scrollY > 600
}
function scrollToTop() {
  window.scrollTo({ top: 0, behavior: 'smooth' })
}
</script>

<template>
  <!-- 品牌页强制深色（与 deepseek harness landing 一致），.dark 只作用于本页子树 -->
  <div class="home dark">
    <!-- Hero：three.js 粒子场 + 渐变光晕 + 网格纹理 + 入场动画 -->
    <section class="hero">
      <div class="hero-glow" aria-hidden />
      <div class="hero-grid-bg" aria-hidden />
      <HomeScene3D />
      <div class="hero-content">
        <span class="hero-badge">
          <span class="hero-badge-dot" />MiniCC · 自托管 AI Agent 控制台
        </span>
        <h1 class="hero-title">
          让 Agent
          <span class="hero-title-accent">持续工作</span>
          <br />在真实场景中
        </h1>
        <p class="hero-sub">
          对话、Agent、工作流、技能、知识库与插件一体化，全栈能力自由组合；
          <br class="hero-br" />轨迹可循、过程可见，你的本地智能工作台。
        </p>
        <div class="hero-actions">
          <Button type="primary" size="large" class="hero-cta glow" @mousemove="onCtaMove" @mouseleave="onCtaLeave" @click="router.push('/chat')">
            开始对话
            <ArrowRightOutlined />
          </Button>
          <Button size="large" class="hero-cta ghost" @mousemove="onCtaMove" @mouseleave="onCtaLeave" @click="scrollToFeatures">浏览功能</Button>
        </div>
      </div>
    </section>

    <!-- 特性网格 -->
        <!-- 六大工作台统一入口：快速命令 + 工作台网格 + 最近活动（互联互通） -->
    <WorkstationNav />

<section id="features" class="features">
      <h2 class="section-title">六大能力，一个控制台</h2>
      <p class="section-sub">每一块能力都可以独立使用，也可以自由组合</p>
      <div class="feature-grid">
        <div
          v-for="(f, i) in features"
          :key="f.title"
          class="feature-card"
          :style="{ '--card-delay': `${i * 60}ms` }"
          :ref="(el: any) => setCardRef(el as HTMLElement | null, i)"
          role="link"
          tabindex="0"
          @mousemove="onCardMove"
          @mouseleave="onCardLeave"
          @click="router.push(f.path)"
          @keydown.enter="router.push(f.path)"
        >
          <span class="feature-icon-wrap">
            <component :is="f.icon" class="feature-icon" />
          </span>
          <span class="feature-en">{{ f.en }}</span>
          <div class="feature-title">{{ f.title }}</div>
          <div class="feature-desc">{{ f.desc }}</div>
          <span class="feature-go">进入 <ArrowRightOutlined /></span>
        </div>
      </div>
    </section>

    <!-- 产品展示：真实工作台窗口预览（玻璃拟态） -->
    <section class="showcase">
      <h2 class="section-title">真实工作台，一次看够</h2>
      <p class="section-sub">对话、轨迹、工具调用，过程全程可见</p>
      <div class="showcase-grid">
        <!-- 窗口 1：对话界面 -->
        <div class="window-card">
          <div class="window-chrome">
            <span class="win-dot red" /><span class="win-dot yellow" /><span class="win-dot green" />
            <span class="win-title">MiniCC · 对话</span>
          </div>
          <div class="window-body chat-preview">
            <div class="pv-msg assistant">
              <div class="pv-bubble">我来帮你分析这份数据，先把需求拆解成几步…</div>
            </div>
            <div class="pv-msg user">
              <div class="pv-bubble user">请用 Python 生成季度趋势图</div>
            </div>
            <div class="pv-msg assistant">
              <div class="pv-tool"><span class="pv-tool-dot" />python_exec · 运行中</div>
              <div class="pv-bubble">已生成趋势图：Q2 环比 +23%。下面是代码与图表…</div>
            </div>
            <div class="pv-input"><span>发送消息…</span></div>
          </div>
        </div>
        <!-- 窗口 2：历史导航（轨迹 + 会话） -->
        <div class="window-card">
          <div class="window-chrome">
            <span class="win-dot red" /><span class="win-dot yellow" /><span class="win-dot green" />
            <span class="win-title">MiniCC · 历史导航</span>
          </div>
          <div class="window-body panel-preview">
            <div class="pv-panel-head"><span class="pv-panel-title">会话：数据分析</span><span class="pv-panel-caret">▾</span></div>
            <div class="pv-timeline">
              <div class="pv-timeline-track">
                <span class="pv-span" style="left: 0%; width: 26%" />
                <span class="pv-span" style="left: 34%; width: 18%" />
                <span class="pv-span" style="left: 60%; width: 32%" />
              </div>
            </div>
            <div class="pv-row"><span class="pv-dot" />分析这份数据的趋势</div>
            <div class="pv-row active"><span class="pv-dot" />生成季度趋势图</div>
            <div class="pv-row"><span class="pv-dot" />对比去年同期表现</div>
            <div class="pv-row"><span class="pv-dot" />汇总为周报</div>
          </div>
        </div>
      </div>
    </section>

    <!-- 快速开始：终端代码块 -->
    <section class="quickstart">
      <h2 class="section-title">快速开始</h2>
      <p class="section-sub">一条命令启动依赖，三行进入工作台</p>
      <div class="terminal-card">
        <div class="window-chrome">
          <span class="win-dot red" /><span class="win-dot yellow" /><span class="win-dot green" />
          <span class="win-title">zsh · minicc</span>
          <button type="button" class="term-copy" @click="copyQuickstart">{{ copied ? '已复制' : '复制' }}</button>
        </div>
        <div class="terminal-body">
          <div class="term-line"><span class="term-prompt">$</span> docker compose up -d postgres redis</div>
          <div class="term-line"><span class="term-prompt">$</span> cp .env.example .env</div>
          <div class="term-line"><span class="term-prompt">$</span> python run.py start</div>
          <div class="term-line term-out">→ MiniCC 已启动：http://localhost:5173</div>
        </div>
      </div>
    </section>

    <!-- CTA -->
    <section class="cta">
      <div class="cta-card">
        <h2 class="cta-title">准备好开始了吗？</h2>
        <p class="cta-sub">创建你的第一个会话，体验完整的 Agent 工作流</p>
        <Button type="primary" size="large" class="hero-cta glow" @mousemove="onCtaMove" @mouseleave="onCtaLeave" @click="router.push('/chat')">
          立即使用 MiniCC
          <ArrowRightOutlined />
        </Button>
      </div>
    </section>

    <footer class="home-footer">
      <span class="home-footer-brand">
        <span class="home-footer-logo">MC</span>MiniCC
      </span>
      <span class="home-footer-note">自托管 · 开源 · 你的数据留在你的机器上</span>
    </footer>

    <!-- 滚动到顶部按钮（长页面辅助导航） -->
    <Transition name="top-fade">
      <button
        v-if="showTop"
        type="button"
        class="scroll-top"
        title="回到顶部"
        aria-label="回到顶部"
        @click="scrollToTop"
      >
        <ArrowUpOutlined />
      </button>
    </Transition>
  </div>
</template>

<style scoped>
/* 品牌页强制深色：.home.dark 下 CSS 变量为暗色系（deepseek harness landing 同款） */
.home { min-height: 100%; background: var(--bg-page); color: var(--text-primary); overflow-x: hidden; }

/* ── Hero：全宽、渐变光晕 + 细网格纹理 + three.js 粒子场 ── */
.hero {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: min(560px, 72vh);
  padding: 72px 24px 64px;
  text-align: center;
  overflow: hidden;
}
.hero-glow {
  position: absolute;
  inset: -30% -20% auto -20%;
  height: 85%;
  background:
    radial-gradient(ellipse 45% 55% at 50% 0%, var(--primary-bg), transparent 70%),
    radial-gradient(ellipse 30% 40% at 78% 12%, rgba(103, 158, 254, 0.14), transparent 70%),
    radial-gradient(ellipse 30% 40% at 22% 10%, rgba(65, 118, 230, 0.16), transparent 70%);
  pointer-events: none;
}
.hero-grid-bg {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(var(--border-card) 1px, transparent 1px),
    linear-gradient(90deg, var(--border-card) 1px, transparent 1px);
  background-size: 44px 44px;
  mask-image: radial-gradient(ellipse 60% 55% at 50% 0%, black 20%, transparent 75%);
  -webkit-mask-image: radial-gradient(ellipse 60% 55% at 50% 0%, black 20%, transparent 75%);
  pointer-events: none;
}
.hero-content { position: relative; z-index: 2; max-width: 760px; }
/* 玻璃徽标：半透明 + 毛玻璃 + 高光边缘 */
.hero-badge {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 5px 14px;
  border: 1px solid hsla(0, 0%, 100%, 0.14);
  border-radius: var(--radius-full);
  background: hsla(0, 0%, 100%, 0.06);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  font-size: 12px;
  font-weight: 500;
  color: var(--text-secondary);
  box-shadow: 0 0 0 1px hsla(0, 0%, 100%, 0.04), inset 0 1px 1px hsla(0, 0%, 100%, 0.12);
  animation: heroReveal 0.55s ease-out both;
}
.hero-badge-dot {
  width: 7px; height: 7px; border-radius: 50%;
  background: var(--primary);
  animation: dotPulse 2.4s ease-in-out infinite;
}
@keyframes dotPulse {
  0%, 100% { box-shadow: 0 0 6px var(--primary); }
  50% { box-shadow: 0 0 16px var(--primary); }
}
.hero-title {
  margin: 22px 0 16px;
  font-size: clamp(34px, 5.5vw, 56px);
  line-height: 1.15;
  font-weight: 700;
  letter-spacing: -0.02em;
  color: var(--text-primary);
  animation: heroReveal 0.8s 0.08s cubic-bezier(0.22, 0.8, 0.36, 1) both;
}
.hero-title-accent {
  background: linear-gradient(100deg, var(--primary), var(--accent));
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
  -webkit-text-fill-color: transparent;
}
.hero-sub {
  margin: 0 auto;
  font-size: 15px;
  line-height: 26px;
  color: var(--text-secondary);
  animation: heroReveal 0.8s 0.16s cubic-bezier(0.22, 0.8, 0.36, 1) both;
}
.hero-actions { display: flex; gap: 12px; justify-content: center; margin-top: 30px; animation: heroReveal 0.8s 0.24s cubic-bezier(0.22, 0.8, 0.36, 1) both; }
/* 模糊浮现入场（deepseek ds-hero-enter 语义）。
   注意：from 不含 opacity——动画异常/卡住时内容仍可见（仅模糊偏移），绝不隐藏 */
@keyframes heroReveal {
  from { transform: translateY(22px); filter: blur(10px); }
  to { transform: none; filter: blur(0); }
}
.hero-cta {
  position: relative;
  overflow: hidden;
  border-radius: var(--radius-xl);
  padding: 0 26px;
  height: 42px;
  font-size: 14px;
  transition: transform 0.15s ease-out, box-shadow 0.2s ease, border-color 0.2s ease, background 0.2s ease;
}
/* 主 CTA：蓝色光晕 + 扫光动画（deepseek arrow-sweep 语义） */
.hero-cta.glow {
  box-shadow: 0 0 0 1px var(--primary-bg), 0 6px 24px var(--primary-bg), inset 0 1px 1px hsla(0, 0%, 100%, 0.35);
}
.hero-cta.glow:hover { box-shadow: 0 0 0 1px var(--primary-bg), 0 8px 32px var(--primary-bg), inset 0 1px 1px hsla(0, 0%, 100%, 0.35); }
.hero-cta.glow::after {
  content: '';
  position: absolute;
  top: 0; left: -70%;
  width: 45%; height: 100%;
  background: linear-gradient(100deg, transparent, hsla(0, 0%, 100%, 0.45), transparent);
  transform: skewX(-20deg);
  animation: sweep 3s ease-in-out infinite;
}
@keyframes sweep {
  0% { left: -70%; }
  55%, 100% { left: 130%; }
}
.hero-cta.ghost {
  border-color: hsla(0, 0%, 100%, 0.16);
  color: var(--text-primary);
  background: hsla(0, 0%, 100%, 0.04);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
}
.hero-cta.ghost:hover { border-color: hsla(0, 0%, 100%, 0.3); background: hsla(0, 0%, 100%, 0.08); }

/* ── 特性网格（玻璃卡片 + hover 旋转渐变边框） ── */
.features { max-width: 1080px; margin: 0 auto; padding: 40px 24px 56px; position: relative; }
/* 浮动背景光斑：让玻璃卡片透出动态光 */
.features::before {
  content: '';
  position: absolute;
  inset: -10% -10% auto -10%;
  height: 70%;
  background:
    radial-gradient(ellipse 30% 40% at 18% 30%, rgba(65, 118, 230, 0.14), transparent 70%),
    radial-gradient(ellipse 30% 40% at 82% 20%, rgba(103, 158, 254, 0.12), transparent 70%);
  pointer-events: none;
  animation: floatGlow 9s ease-in-out infinite alternate;
}
@keyframes floatGlow {
  from { transform: translateY(-8px); opacity: 0.8; }
  to { transform: translateY(10px); opacity: 1; }
}
.section-title { font-size: 28px; font-weight: 700; letter-spacing: -0.01em; text-align: center; }
.section-sub { margin-top: 8px; text-align: center; font-size: 14px; color: var(--text-tertiary); }
.feature-grid {
  position: relative;
  z-index: 1;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 14px;
  margin-top: 34px;
}
/* 玻璃卡片：半透明 + 毛玻璃 + 高光边缘。
   默认可见（opacity 不依赖 JS）；.visible 只触发位置入场，即使 JS/IO 失效也永不隐藏 */
.feature-card {
  position: relative;
  display: flex;
  flex-direction: column;
  padding: 20px 20px 16px;
  border: 1px solid hsla(0, 0%, 100%, 0.08);
  border-radius: var(--radius-lg);
  background: hsla(0, 0%, 100%, 0.05);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  box-shadow: 0 0 0 1px hsla(0, 0%, 100%, 0.03), 0 4px 12px rgba(115, 163, 210, 0.12), inset 0 1px 1px hsla(0, 0%, 100%, 0.1);
  cursor: pointer;
  /* tilt 平滑：transform 快过渡，其余常规 */
  transition: transform 0.15s ease-out, border-color 0.2s ease, box-shadow 0.2s ease;
}
.feature-card.visible {
  animation: cardIn 0.5s cubic-bezier(0.22, 0.8, 0.36, 1) both;
  animation-delay: var(--card-delay, 0ms);
}
@keyframes cardIn {
  from { transform: translateY(12px); }
}
.feature-card:hover {
  border-color: hsla(0, 0%, 100%, 0.16);
  box-shadow: 0 0 0 1px hsla(0, 0%, 100%, 0.06), 0 8px 24px rgba(115, 163, 210, 0.22), inset 0 1px 1px hsla(0, 0%, 100%, 0.14);
}
/* hover 旋转渐变边框（deepseek rotating-border） */
.feature-card::before {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: inherit;
  padding: 1px;
  background: conic-gradient(from var(--home-border-angle), transparent 0%, var(--primary) 20%, transparent 45%);
  -webkit-mask: linear-gradient(#000 0 0) content-box, linear-gradient(#000 0 0);
  -webkit-mask-composite: xor;
  mask: linear-gradient(#000 0 0) content-box, linear-gradient(#000 0 0);
  mask-composite: exclude;
  opacity: 0;
  transition: opacity 0.25s ease;
  pointer-events: none;
}
.feature-card:hover::before {
  opacity: 1;
  animation: home-rotating-border 3.2s linear infinite;
}
.feature-card:focus-visible { outline: 2px solid var(--primary); outline-offset: 2px; }
.feature-icon-wrap {
  width: 40px; height: 40px;
  display: inline-flex; align-items: center; justify-content: center;
  border-radius: 10px;
  background: var(--primary-bg);
  color: var(--primary);
  margin-bottom: 12px;
  box-shadow: inset 0 1px 1px hsla(0, 0%, 100%, 0.12);
}
.feature-icon { font-size: 19px; }
/* 英文设计感小字（deepseek 特性卡英文副标） */
.feature-en {
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.16em;
  color: var(--primary);
  opacity: 0.75;
  margin-bottom: 3px;
}
.feature-title { font-size: 15px; font-weight: 600; color: var(--text-primary); }
.feature-desc { margin-top: 6px; font-size: 13px; line-height: 20px; color: var(--text-tertiary); flex: 1; }
.feature-go { margin-top: 12px; font-size: 12px; color: var(--primary); opacity: 0; transition: opacity 0.2s ease; }
.feature-card:hover .feature-go { opacity: 1; }

/* ── 产品展示：mac 窗口玻璃预览 ── */
.showcase { max-width: 1080px; margin: 0 auto; padding: 24px 24px 56px; position: relative; }
.showcase-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
  gap: 18px;
  margin-top: 34px;
}
.window-card {
  border-radius: 14px;
  overflow: hidden;
  border: 1px solid hsla(0, 0%, 100%, 0.1);
  background: hsla(0, 0%, 100%, 0.045);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  box-shadow: 0 0 0 1px hsla(0, 0%, 100%, 0.04), 0 12px 40px rgba(0, 0, 0, 0.4), inset 0 1px 1px hsla(0, 0%, 100%, 0.12);
  transition: transform 0.25s ease, box-shadow 0.25s ease;
}
.window-card:hover { transform: translateY(-4px); box-shadow: 0 0 0 1px hsla(0, 0%, 100%, 0.06), 0 18px 56px rgba(0, 0, 0, 0.5), inset 0 1px 1px hsla(0, 0%, 100%, 0.16); }
.window-chrome {
  display: flex;
  align-items: center;
  gap: 7px;
  height: 36px;
  padding: 0 12px;
  border-bottom: 1px solid hsla(0, 0%, 100%, 0.07);
  background: hsla(0, 0%, 100%, 0.03);
}
.win-dot { width: 10px; height: 10px; border-radius: 50%; flex: none; }
.win-dot.red { background: #ff5f57; }
.win-dot.yellow { background: #febc2e; }
.win-dot.green { background: #28c840; }
.win-title { flex: 1; text-align: center; font-size: 11px; color: var(--text-tertiary); margin-right: 30px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.window-body { padding: 16px; }
/* 对话预览 */
.chat-preview { display: flex; flex-direction: column; gap: 10px; min-height: 200px; }
.pv-msg { display: flex; }
.pv-msg.user { justify-content: flex-end; }
.pv-bubble {
  max-width: 82%;
  padding: 8px 12px;
  border-radius: 14px;
  border-bottom-left-radius: 4px;
  background: hsla(0, 0%, 100%, 0.07);
  border: 1px solid hsla(0, 0%, 100%, 0.06);
  font-size: 12px;
  line-height: 19px;
  color: var(--text-primary);
}
.pv-msg.user .pv-bubble {
  border-radius: 14px;
  border-bottom-right-radius: 4px;
  background: var(--primary-bg);
  border-color: transparent;
  color: var(--bubble-user-text);
}
.pv-tool {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 6px;
  padding: 3px 10px;
  border-radius: 999px;
  background: hsla(0, 0%, 100%, 0.05);
  border: 1px solid hsla(0, 0%, 100%, 0.08);
  font-size: 11px;
  color: var(--text-secondary);
  font-family: var(--font-mono);
}
.pv-tool-dot {
  width: 6px; height: 6px; border-radius: 50%;
  background: var(--primary);
  animation: dotPulse 1.6s ease-in-out infinite;
}
.pv-input {
  margin-top: 2px;
  padding: 8px 12px;
  border-radius: 12px;
  border: 1px solid hsla(0, 0%, 100%, 0.1);
  background: hsla(0, 0%, 100%, 0.04);
  font-size: 12px;
  color: var(--text-tertiary);
}
/* 历史导航预览 */
.panel-preview { display: flex; flex-direction: column; gap: 6px; min-height: 200px; }
.pv-panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 4px 8px 8px;
  border-bottom: 1px solid hsla(0, 0%, 100%, 0.07);
  margin-bottom: 4px;
}
.pv-panel-title { font-size: 12px; font-weight: 600; color: var(--text-primary); }
.pv-panel-caret { font-size: 10px; color: var(--text-tertiary); }
.pv-timeline {
  height: 34px;
  border-radius: 8px;
  background: hsla(0, 0%, 100%, 0.04);
  border: 1px solid hsla(0, 0%, 100%, 0.06);
  position: relative;
}
.pv-timeline-track { position: absolute; inset: 12px 10px; }
.pv-span {
  position: absolute;
  top: 0;
  height: 8px;
  border-radius: 2px;
  background: linear-gradient(90deg, var(--primary), var(--accent));
  opacity: 0.85;
}
.pv-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 8px;
  font-size: 12px;
  color: var(--text-secondary);
}
.pv-row.active { background: var(--primary-bg); color: var(--primary); font-weight: 600; }
.pv-dot { width: 6px; height: 6px; border-radius: 50%; background: var(--primary); opacity: 0.7; }

/* ── 快速开始：终端代码块 ── */
.quickstart { max-width: 760px; margin: 0 auto; padding: 24px 24px 64px; }
.terminal-card {
  margin-top: 30px;
  border-radius: 14px;
  overflow: hidden;
  border: 1px solid hsla(0, 0%, 100%, 0.1);
  background: rgba(8, 10, 14, 0.75);
  box-shadow: 0 0 0 1px hsla(0, 0%, 100%, 0.04), 0 16px 48px rgba(0, 0, 0, 0.45);
}
.terminal-card .window-chrome { background: rgba(255, 255, 255, 0.03); }
.term-copy {
  flex: none;
  margin-left: auto;
  min-height: 28px;
  border: 1px solid hsla(0, 0%, 100%, 0.12);
  border-radius: 6px;
  background: transparent;
  color: var(--text-tertiary);
  font-size: 11px;
  padding: 4px 12px;
  cursor: pointer;
  transition: color 0.15s ease, border-color 0.15s ease;
}
.term-copy:hover { color: var(--text-primary); border-color: hsla(0, 0%, 100%, 0.3); }
.terminal-body { padding: 16px 18px; font-family: var(--font-mono); font-size: 12.5px; line-height: 24px; }
.term-line { color: var(--text-primary); white-space: pre-wrap; word-break: break-all; }
.term-prompt { color: var(--primary); font-weight: 600; margin-right: 8px; }
.term-out { color: var(--text-tertiary); margin-top: 6px; }

/* ── CTA ── */
.cta { max-width: 1080px; margin: 0 auto; padding: 0 24px 56px; }
.cta-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 48px 24px;
  border-radius: 16px;
  background: linear-gradient(135deg, var(--primary-bg), transparent 65%), hsla(0, 0%, 100%, 0.045);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border: 1px solid hsla(0, 0%, 100%, 0.1);
  box-shadow: 0 0 0 1px hsla(0, 0%, 100%, 0.04), 0 12px 40px rgba(0, 0, 0, 0.35), inset 0 1px 1px hsla(0, 0%, 100%, 0.12);
  text-align: center;
}
.cta-title { font-size: 24px; font-weight: 700; }
.cta-sub { font-size: 14px; color: var(--text-tertiary); }
.cta .hero-cta { margin-top: 14px; }

/* ── 页脚 ── */
.home-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 18px 28px;
  border-top: 1px solid hsla(0, 0%, 100%, 0.08);
  font-size: 12px;
  color: var(--text-tertiary);
}
.home-footer-brand { display: flex; align-items: center; gap: 8px; font-weight: 600; color: var(--text-secondary); }
.home-footer-logo {
  width: 20px; height: 20px; border-radius: 6px;
  background: linear-gradient(135deg, var(--primary), var(--primary-dark));
  color: #fff; font-size: 9px; font-weight: 700;
  display: inline-flex; align-items: center; justify-content: center;
}

@media (max-width: 640px) {
  .hero { min-height: 520px; padding: 56px 20px 40px; }
  .hero-br { display: none; }
  .hero-actions { flex-direction: column; align-items: center; }
  .feature-grid { grid-template-columns: 1fr; }
  .showcase-grid { grid-template-columns: 1fr; }
  .home-footer { flex-direction: column; text-align: center; }
}
/* 平板：中屏优化（特性卡 2 列、窗口预览保持 2 列、终端缩小） */
@media (max-width: 768px) and (min-width: 641px) {
  .feature-grid { grid-template-columns: repeat(2, 1fr); }
  .showcase-grid { grid-template-columns: 1fr; }
  .hero-title { font-size: clamp(30px, 6vw, 44px); }
}
/* 窄屏：区块间距压缩，触控目标保持 */
@media (max-width: 640px) {
  .features, .showcase { padding-left: 16px; padding-right: 16px; }
  .quickstart { padding-left: 16px; padding-right: 16px; }
  .cta { padding-left: 16px; padding-right: 16px; }
  .feature-grid { gap: 12px; }
  .hero-cta { width: 100%; max-width: 320px; }
}
/* 触屏设备：禁用 3D tilt 和磁吸 transform，避免抖动 */
@media (hover: none) {
  .feature-card:hover { transform: none; }
  .hero-cta:hover { transform: none; }
}
/* 减少动效偏好：跳过入场动画，内容立即可见 */
@media (prefers-reduced-motion: reduce) {
  .hero-badge, .hero-title, .hero-sub, .hero-actions,
  .feature-card.visible, .hero-badge-dot, .hero-cta.glow::after,
  .feature-card::before, .features::before { animation: none; }
  .feature-card { opacity: 1; }
  .hero-cta { transition: none; }
  .scroll-top { transition: none; }
}

/* ── 滚动到顶部按钮 ── */
.scroll-top {
  position: fixed;
  right: 24px;
  bottom: 24px;
  width: 40px;
  height: 40px;
  border-radius: 50%;
  border: 1px solid hsla(0, 0%, 100%, 0.16);
  background: hsla(0, 0%, 100%, 0.08);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  color: var(--text-primary);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  z-index: 50;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.3), inset 0 1px 1px hsla(0, 0%, 100%, 0.12);
  transition: transform 0.2s ease, border-color 0.2s ease, background 0.2s ease;
}
.scroll-top:hover {
  transform: translateY(-2px);
  border-color: hsla(0, 0%, 100%, 0.3);
  background: hsla(0, 0%, 100%, 0.14);
}
.scroll-top:active { transform: translateY(0) scale(0.94); }
.top-fade-enter-active, .top-fade-leave-active { transition: opacity 0.2s ease, transform 0.2s ease; }
.top-fade-enter-from, .top-fade-leave-to { opacity: 0; transform: translateY(8px); }
</style>

<!-- rotating-border 需要的注册属性与关键帧（@property 不能放 scoped 块） -->
<style>
@property --home-border-angle {
  syntax: '<angle>';
  initial-value: 0deg;
  inherits: false;
}
@keyframes home-rotating-border {
  to { --home-border-angle: 360deg; }
}
</style>
