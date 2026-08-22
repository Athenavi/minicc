<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, markRaw } from 'vue'
import {
  Card,
  Button,
  Input,
  InputNumber,
  Slider,
  Select,
  Tabs,
  TabPane,
  List,
  Tag,
  Popconfirm,
  Spin,
  Modal,
  message,
  Badge,
  Alert,
  Tooltip,
  Space,
} from 'ant-design-vue'
import {
  PlusOutlined,
  DeleteOutlined,
  EditOutlined,
  SearchOutlined,
  ThunderboltOutlined,
  DatabaseOutlined,
} from '@ant-design/icons-vue'
import EmptyState from '../components/common/EmptyState.vue'
import {
  listMemory,
  upsertMemory,
  updateMemory,
  deleteMemory,
  clearMemory,
  searchMemory,
  startOrganize,
  getOrganizeStatus,
  MEMORY_SLOTS,
  type MemoryEntry,
  type MemorySearchHit,
  type MemorySlot,
  type MemorySource,
  type OrganizeStatus,
} from '../api/memory'

const loading = ref(false)
const error = ref(false)
const entries = ref<MemoryEntry[]>([])
const counts = ref<Record<string, number>>({})
const total = ref(0)

const activeTab = ref<'all' | MemorySlot>('all')
const filteredEntries = computed(() =>
  activeTab.value === 'all' ? entries.value : entries.value.filter((e) => e.slot === activeTab.value),
)

// ── 语义检索 ──
const searchInput = ref('')
const searching = ref(false)
const searchingMode = ref<'semantic' | 'keyword'>('semantic')
const searchResults = ref<MemorySearchHit[] | null>(null)

async function handleSearch() {
  const q = searchInput.value.trim()
  if (!q) {
    clearSearch()
    return
  }
  searching.value = true
  try {
    const data = await searchMemory(q, { top_k: 10 })
    searchingMode.value = data.mode
    searchResults.value = data.results
    if (data.results.length === 0) {
      message.info('未找到相似记忆')
    }
  } catch (e: any) {
    message.error(e.response?.data?.error || '检索失败')
  } finally {
    searching.value = false
  }
}

function clearSearch() {
  searchInput.value = ''
  searchResults.value = null
}

// ── 增删改 ──
const formVisible = ref(false)
const saving = ref(false)
const editing = ref<MemoryEntry | null>(null)
const form = ref<{
  slot: MemorySlot
  key: string
  value: string
  confidence: number
  source: MemorySource
}>({
  slot: 'fact',
  key: '',
  value: '',
  confidence: 50,
  source: 'user_confirmed',
})

function openCreate(slot?: MemorySlot) {
  editing.value = null
  form.value = { slot: slot ?? 'fact', key: '', value: '', confidence: 50, source: 'user_confirmed' }
  formVisible.value = true
}

function openEdit(entry: MemoryEntry) {
  editing.value = entry
  form.value = {
    slot: entry.slot as MemorySlot,
    key: entry.key,
    value: entry.value,
    confidence: entry.confidence,
    source: entry.source as MemorySource,
  }
  formVisible.value = true
}

async function handleSave() {
  const { slot, key, value, confidence, source } = form.value
  if (!key.trim() || !value.trim()) {
    message.warning('键与内容均不能为空')
    return
  }
  saving.value = true
  try {
    if (editing.value) {
      await updateMemory({ id: editing.value.id, key, value, confidence, source })
      message.success('已更新记忆')
    } else {
      const res = await upsertMemory({ slot, key, value, confidence, source })
      if (res.duplicate_of) {
        message.warning(`检测到相似记忆「${res.duplicate_of.key}」，可在整理时自动合并`)
      } else {
        message.success('已保存记忆')
      }
    }
    formVisible.value = false
    await loadProfile()
  } catch (e: any) {
    message.error(e.response?.data?.error || '保存失败')
  } finally {
    saving.value = false
  }
}

async function handleDelete(id: string) {
  try {
    await deleteMemory(id)
    message.success('已删除')
    await loadProfile()
  } catch (e: any) {
    message.error(e.response?.data?.error || '删除失败')
  }
}

async function handleClearAll() {
  try {
    const res = await clearMemory()
    message.success(`已清空 ${res.deleted} 条记忆`)
    await loadProfile()
  } catch (e: any) {
    message.error(e.response?.data?.error || '清空失败')
  }
}

// ── 智能整理（异步 + 轮询） ──
const organize = ref<OrganizeStatus>({
  running: false,
  started_at: null,
  finished_at: null,
  result: null,
  error: null,
})
let organizeTimer: ReturnType<typeof setInterval> | null = null

function stopPolling() {
  if (organizeTimer) {
    clearInterval(organizeTimer)
    organizeTimer = null
  }
}

async function pollOrganize() {
  stopPolling()
  organizeTimer = setInterval(async () => {
    try {
      const st = await getOrganizeStatus()
      organize.value = st
      if (!st.running) {
        stopPolling()
        if (st.result) {
          const r = st.result
          const parts: string[] = []
          if (r.merged) parts.push(`合并 ${r.merged} 条重复`)
          if (r.backfilled) parts.push(`补齐 ${r.backfilled} 条向量`)
          if (r.archived) parts.push(`归档 ${r.archived} 条`)
          if (r.evicted) parts.push(`淘汰 ${r.evicted} 条`)
          if (parts.length) message.success('整理完成：' + parts.join('，'))
          else message.success('整理完成：无需调整')
          await loadProfile()
        } else if (st.error) {
          message.error('整理失败：' + st.error)
        }
      }
    } catch {
      stopPolling()
    }
  }, 1200)
}

async function runOrganize() {
  try {
    const res = await startOrganize()
    organize.value = res.status
    if (res.started) {
      message.info('智能整理已启动（后台运行）')
      pollOrganize()
    } else {
      message.info('已有整理任务在运行中')
      pollOrganize()
    }
  } catch (e: any) {
    message.error(e.response?.data?.error || '启动整理失败')
  }
}

// ── 加载 ──
async function loadProfile() {
  loading.value = true
  error.value = false
  try {
    const data = await listMemory()
    entries.value = data.entries
    counts.value = data.counts
    total.value = data.total
    organize.value = data.organize
  } catch (e: any) {
    error.value = true
    if (e.response?.status === 503) {
      message.error('记忆服务不可用（需要 PostgreSQL）')
    } else {
      message.error(e.response?.data?.error || '加载记忆失败')
    }
  } finally {
    loading.value = false
  }
}

onMounted(loadProfile)
onUnmounted(stopPolling)

function slotLabel(slot: string): string {
  return MEMORY_SLOTS.find((s) => s.slot === slot)?.label ?? slot
}
function pct(n: number): string {
  return Math.round(n * 100) + '%'
}
</script>

<template>
  <div class="memory-view">
    <div class="page-header">
      <div class="title">
        <DatabaseOutlined />
        <span>长期记忆</span>
        <Badge :count="total" :overflow-count="999" color="#6366f1" />
        <span class="subtitle">跨会话留存 · 语义检索 · 自动整理</span>
      </div>
      <Space>
        <Button :loading="organize.running" @click="runOrganize">
          <template #icon><ThunderboltOutlined /></template>
          智能整理
        </Button>
        <Popconfirm title="确定清空全部长期记忆？此操作不可恢复" ok-text="清空" cancel-text="取消" @confirm="handleClearAll">
          <Button danger>清空记忆</Button>
        </Popconfirm>
        <Button type="primary" @click="openCreate()">
          <template #icon><PlusOutlined /></template>
          新建记忆
        </Button>
      </Space>
    </div>

    <!-- 整理状态 -->
    <Alert
      v-if="organize.running"
      type="info"
      show-icon
      message="正在后台整理记忆（去重 / 归档 / 补齐向量）…"
      style="margin-bottom: 16px"
    />
    <Alert
      v-else-if="organize.result"
      type="success"
      show-icon
      style="margin-bottom: 16px"
      :message="`上次整理：合并 ${organize.result.merged} · 补齐 ${organize.result.backfilled} · 归档 ${organize.result.archived} · 淘汰 ${organize.result.evicted}`"
    />

    <!-- 语义检索 -->
    <Card class="search-card" :bordered="true">
      <Space style="width: 100%" wrap>
        <Input
          v-model:value="searchInput"
          placeholder="语义检索：如「用户偏好用什么编辑器」"
          style="width: 360px"
          allow-clear
          @press-enter="handleSearch"
          @search="handleSearch"
        >
          <template #prefix><SearchOutlined /></template>
        </Input>
        <Button type="primary" :loading="searching" @click="handleSearch">智能检索</Button>
        <Button v-if="searchResults !== null" @click="clearSearch">返回列表</Button>
        <Tag v-if="searchResults !== null" :color="searchingMode === 'semantic' ? 'blue' : 'orange'">
          {{ searchingMode === 'semantic' ? '语义模式' : '关键词模式' }}
        </Tag>
      </Space>
    </Card>

    <!-- 检索结果 -->
    <div v-if="searchResults !== null" class="results">
      <List
        :data-source="searchResults"
        :locale="{ emptyText: '未找到相似记忆' }"
      >
        <template #renderItem="{ item }">
          <List.Item>
            <Card class="entry-card" size="small" style="width: 100%">
              <div class="entry-head">
                <Tag color="purple">{{ slotLabel(item.slot) }}</Tag>
                <span class="entry-key">{{ item.key }}</span>
                <Tooltip :title="`相关度 ${pct(item.similarity)} · 重排序分 ${item.score.toFixed(2)}`">
                  <Tag color="green">{{ pct(item.score) }}</Tag>
                </Tooltip>
              </div>
              <div class="entry-value">{{ item.value }}</div>
            </Card>
          </List.Item>
        </template>
      </List>
    </div>

    <!-- 浏览（按槽位 Tab） -->
    <Spin v-else :spinning="loading">
      <Tabs v-model:activeKey="activeTab">
        <TabPane key="all">
          <template #tab>全部 ({{ total }})</template>
        </TabPane>
        <TabPane v-for="s in MEMORY_SLOTS" :key="s.slot">
          <template #tab>{{ s.label }} ({{ counts[s.slot] || 0 }})</template>
        </TabPane>
      </Tabs>

      <EmptyState
        v-if="error && !loading"
        size="page"
        :icon="markRaw(DatabaseOutlined)"
        description="加载失败"
        hint="无法获取记忆数据，请稍后重试"
      >
        <Button type="primary" @click="loadProfile">重试</Button>
      </EmptyState>

      <EmptyState
        v-else-if="filteredEntries.length === 0 && !loading"
        size="page"
        :icon="markRaw(DatabaseOutlined)"
        description="暂无该分类记忆"
        hint="点击右上角「新建记忆」，记录用户偏好与长期上下文"
      />

      <List v-else :data-source="filteredEntries">
        <template #renderItem="{ item }">
          <List.Item>
            <Card class="entry-card" size="small" style="width: 100%">
              <div class="entry-head">
                <Tag color="purple">{{ item.slot_label }}</Tag>
                <span class="entry-key">{{ item.key }}</span>
                <Tag :color="item.source === 'user_confirmed' ? 'green' : 'default'">{{ item.source_label }}</Tag>
                <Tooltip title="置信度（整理时高置信条目优先保留）">
                  <Tag color="blue">置信 {{ item.confidence }}</Tag>
                </Tooltip>
                <Space class="entry-actions">
                  <Button size="small" type="text" title="编辑" @click="openEdit(item)">
                    <template #icon><EditOutlined /></template>
                  </Button>
                  <Popconfirm title="删除这条记忆？" ok-text="删除" cancel-text="取消" @confirm="handleDelete(item.id)">
                    <Button size="small" type="text" danger title="删除">
                      <template #icon><DeleteOutlined /></template>
                    </Button>
                  </Popconfirm>
                </Space>
              </div>
              <div class="entry-value">{{ item.value }}</div>
              <div class="entry-meta">访问 {{ item.access_count }} 次 · 更新于 {{ item.updated_at?.slice(0, 10) }}</div>
            </Card>
          </List.Item>
        </template>
      </List>
    </Spin>

    <!-- 新建 / 编辑弹窗 -->
    <Modal
      v-model:open="formVisible"
      :title="editing ? '编辑记忆' : '新建记忆'"
      @ok="handleSave"
      :confirm-loading="saving"
      ok-text="保存"
      cancel-text="取消"
    >
      <div class="form-row">
        <label>分类</label>
        <Select v-model:value="form.slot" style="width: 100%">
          <Select.Option v-for="s in MEMORY_SLOTS" :key="s.slot" :value="s.slot">{{ s.label }}</Select.Option>
        </Select>
      </div>
      <div class="form-row">
        <label>键（key）</label>
        <Input v-model:value="form.key" placeholder="如 timezone / stack / 拒接需求" />
      </div>
      <div class="form-row">
        <label>内容（value）</label>
        <Input.TextArea v-model:value="form.value" :rows="3" placeholder="记忆的具体内容" />
      </div>
      <div class="form-row">
        <label>置信度 {{ form.confidence }}</label>
        <Slider v-model:value="form.confidence" :min="0" :max="100" />
      </div>
      <div class="form-row">
        <label>来源</label>
        <Select v-model:value="form.source" style="width: 100%">
          <Select.Option value="user_confirmed">用户确认</Select.Option>
          <Select.Option value="derived">对话提炼</Select.Option>
          <Select.Option value="tool_written">工具写入</Select.Option>
        </Select>
      </div>
    </Modal>
  </div>
</template>

<style scoped>
.memory-view {
  max-width: 920px;
  margin: 0 auto;
  padding: 28px 24px 60px;
}
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 20px;
}
.title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 24px;
  font-weight: 700;
  letter-spacing: -0.01em;
}
.title .subtitle {
  font-size: 12px;
  font-weight: 400;
  color: var(--text-secondary, #888);
}
.search-card {
  margin-bottom: 16px;
}
.entry-card {
  margin-bottom: 8px;
}
.entry-head {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.entry-key {
  font-weight: 600;
}
.entry-actions {
  margin-left: auto;
}
.entry-value {
  margin: 6px 0;
  white-space: pre-wrap;
  line-height: 1.5;
}
.entry-meta {
  font-size: 12px;
  color: var(--text-secondary, #888);
}

/* 移动端：检索框占满、标题区换行 */
@media (max-width: 768px) {
  .memory-view {
    padding: 20px 16px 48px;
  }
  .search-card :deep(.ant-input-affix-wrapper) {
    width: 100% !important;
  }
  .title {
    font-size: 20px;
  }
  .title .subtitle {
    display: none;
  }
  .entry-head {
    row-gap: 4px;
  }
}
</style>
