<script setup lang="ts">
import { ref, onMounted, onUnmounted, markRaw } from 'vue'
import { useRouter } from 'vue-router'
import {
  Button, Tabs, TabPane, Modal, Input, InputNumber, Switch, Tag,
  Alert, Dropdown, Menu, MenuItem, message,
} from 'ant-design-vue'
import {
  PlusOutlined, PlayCircleOutlined, EditOutlined, DeleteOutlined,
  StopOutlined, ClockCircleOutlined, CheckCircleOutlined,
  CloseCircleOutlined, SyncOutlined, RobotOutlined, HistoryOutlined,
  MessageOutlined, ShopOutlined, TeamOutlined, LockOutlined,
} from '@ant-design/icons-vue'
import {
  listAgents, createAgent, updateAgent, deleteAgent,
  runAgent, listAgentSessions, getAgentSession,
  listMarket, installMarket, setAgentVisibility,
} from '../api'
import type { Agent, AgentSession, MarketItem } from '../api'
import PageSkeleton from '../components/common/PageSkeleton.vue'
import EmptyState from '../components/common/EmptyState.vue'
import SkillMarketCard from '../components/SkillMarketCard.vue'

// ── 数据 ──
// 后端列表项可能带 visibility（'private'|'tenant'）；缺失视为 private
type AgentRow = Agent & { visibility?: 'private' | 'tenant' }
const agents = ref<AgentRow[]>([])
const loadingAgents = ref(true)
const errorAgents = ref(false)
const sessions = ref<AgentSession[]>([])
const loadingSessions = ref(false)
const errorSessions = ref(false)
const activeTab = ref('agents')
const router = useRouter()

async function loadAgents() {
  loadingAgents.value = true
  errorAgents.value = false
  try {
    agents.value = await listAgents()
  } catch {
    errorAgents.value = true
    message.error('获取 Agent 列表失败')
  } finally {
    loadingAgents.value = false
  }
}

async function loadSessions() {
  loadingSessions.value = true
  errorSessions.value = false
  try {
    sessions.value = await listAgentSessions()
  } catch {
    errorSessions.value = true
    message.error('获取运行记录失败')
  } finally {
    loadingSessions.value = false
  }
}

onMounted(() => {
  loadAgents()
  loadSessions()
  loadMarket()
})
onUnmounted(() => stopPolling())

// ── 市场（Agent 市场：安装后出现在「我的 Agent」列表）──
const marketItems = ref<MarketItem[]>([])
const marketLoading = ref(false)
const marketError = ref(false)
const marketInstallingId = ref<string | null>(null)

async function loadMarket() {
  marketLoading.value = true
  marketError.value = false
  try {
    marketItems.value = await listMarket('agent')
  } catch {
    marketError.value = true
    message.error('获取 Agent 市场失败')
  } finally {
    marketLoading.value = false
  }
}

async function handleMarketInstall(item: MarketItem) {
  marketInstallingId.value = item.id
  try {
    await installMarket('agent', item.id)
    message.success(`「${item.name}」已安装`)
    await Promise.all([loadMarket(), loadAgents()])
  } catch (e: any) {
    const raw = e?.response?.data
    message.error('安装失败: ' + (raw?.message || raw?.detail || raw?.error || e?.message || ''))
  } finally {
    marketInstallingId.value = null
  }
}

// ── 新建 / 编辑 ──
interface EditorState {
  id: string
  name: string
  description: string
  system_prompt: string
  tools_text: string
  model: string
  max_tokens: number
  temperature: number
  max_turns: number
  timeout_seconds: number
  enabled: boolean
}
const editorOpen = ref(false)
const editorSaving = ref(false)
const editingId = ref('')
const form = ref<EditorState>({
  id: '', name: '', description: '', system_prompt: '',
  tools_text: '[]', model: 'deepseek-chat', max_tokens: 4096,
  temperature: 0.6, max_turns: 5, timeout_seconds: 120, enabled: true,
})

function openCreate() {
  editingId.value = ''
  form.value = {
    id: '', name: '', description: '', system_prompt: '',
    tools_text: '[]', model: 'deepseek-chat', max_tokens: 4096,
    temperature: 0.6, max_turns: 5, timeout_seconds: 120, enabled: true,
  }
  editorOpen.value = true
}

function openEdit(a: Agent) {
  editingId.value = a.id
  const llm = a.llm_config || {}
  form.value = {
    id: a.id,
    name: a.name,
    description: a.description || '',
    system_prompt: a.system_prompt || '',
    tools_text: JSON.stringify(a.tools || [], null, 2),
    model: String(llm.model || 'deepseek-chat'),
    max_tokens: Number(llm.max_tokens || 4096),
    temperature: Number(llm.temperature || 0.6),
    max_turns: a.max_turns || 5,
    timeout_seconds: a.timeout_seconds || 120,
    enabled: a.enabled,
  }
  editorOpen.value = true
}

function parseToolsText(): any[] | null {
  try {
    const v = JSON.parse(form.value.tools_text)
    return Array.isArray(v) ? v : null
  } catch {
    message.error('tools 不是合法的 JSON 数组')
    return null
  }
}

async function saveEditor() {
  const f = form.value
  if (!f.name.trim()) { message.warning('请填写名称'); return }
  const tools = parseToolsText()
  if (tools === null) return
  const body: Partial<Agent> = {
    name: f.name.trim(),
    description: f.description,
    system_prompt: f.system_prompt,
    tools,
    llm_config: { model: f.model || 'deepseek-chat', max_tokens: f.max_tokens, temperature: f.temperature },
    max_turns: f.max_turns,
    timeout_seconds: f.timeout_seconds,
    enabled: f.enabled,
  }
  editorSaving.value = true
  try {
    if (editingId.value) {
      await updateAgent(editingId.value, body)
      message.success('已保存')
    } else {
      await createAgent(body)
      message.success('已创建')
    }
    editorOpen.value = false
    await loadAgents()
  } catch (e: any) {
    message.error('保存失败: ' + (e?.response?.data?.error || e?.message || ''))
  } finally {
    editorSaving.value = false
  }
}

// ── 启停 / 删除 ──
async function toggleEnabled(a: Agent) {
  try {
    await updateAgent(a.id, { enabled: !a.enabled })
    a.enabled = !a.enabled
  } catch {
    message.error('操作失败')
  }
}

// ── 共享可见性（团队共享 / 私有；owner-only，非属主 403 提示）──
async function toggleVisibility(a: AgentRow) {
  const next = a.visibility === 'tenant' ? 'private' : 'tenant'
  try {
    await setAgentVisibility(a.id, next)
    message.success(next === 'tenant' ? '已共享给团队' : '已设为私有')
    await loadAgents()
  } catch (e: any) {
    const raw = e?.response?.data
    const msg = raw?.message || raw?.detail || raw?.error || ''
    if (e?.response?.status === 403) {
      message.error('只能操作自己创建的 Agent' + (msg ? `：${msg}` : ''))
    } else {
      message.error('操作失败' + (msg ? `：${msg}` : ''))
    }
  }
}

function requestDelete(a: Agent) {
  Modal.confirm({
    title: '删除 Agent',
    content: `确定删除「${a.name}」？其运行记录也会一并删除。`,
    okText: '删除',
    okButtonProps: { danger: true },
    cancelText: '取消',
    onOk: async () => {
      try {
        await deleteAgent(a.id)
        message.success('已删除')
        await loadAgents()
        await loadSessions()
      } catch {
        message.error('删除失败')
      }
    },
  })
}

// ── 互联互通：在对话中发起会话（携带 Agent 配置上下文）──
function chatWithAgent(a: Agent) {
  router.push({ path: '/chat', query: { agent: a.id } })
}

// ── 运行 + 轮询 ──
const runOpen = ref(false)
const runTarget = ref<Agent | null>(null)
const runTask = ref('')
const runSession = ref<AgentSession | null>(null)
const runSubmitting = ref(false)
let pollTimer: number | undefined

function stopPolling() {
  if (pollTimer !== undefined) { window.clearInterval(pollTimer); pollTimer = undefined }
}

function startPolling(sessionId: string) {
  stopPolling()
  pollTimer = window.setInterval(async () => {
    try {
      const s = await getAgentSession(sessionId)
      runSession.value = s
      // 同步到历史列表（若有）
      const idx = sessions.value.findIndex(x => x.id === s.id)
      if (idx >= 0) sessions.value[idx] = s
      if (s.status === 'completed' || s.status === 'failed') stopPolling()
    } catch {
      stopPolling()
    }
  }, 2000)
}

function openRun(a: Agent) {
  runTarget.value = a
  runTask.value = ''
  runSession.value = null
  runSubmitting.value = false
  stopPolling()
  runOpen.value = true
}

async function submitRun() {
  if (!runTarget.value) return
  const task = runTask.value.trim()
  if (!task) { message.warning('请输入任务'); return }
  runSubmitting.value = true
  try {
    const s = await runAgent(runTarget.value.id, task)
    runSession.value = s
    sessions.value.unshift(s)
    message.success('任务已派发，正在执行…')
    startPolling(s.id)
  } catch (e: any) {
    message.error('派发失败: ' + (e?.response?.data?.error || e?.message || ''))
  } finally {
    runSubmitting.value = false
  }
}

function closeRun() {
  stopPolling()
  runOpen.value = false
}

// ── 历史结果查看 ──
const detailOpen = ref(false)
const detailSession = ref<AgentSession | null>(null)

async function openDetail(s: AgentSession) {
  try {
    const full = await getAgentSession(s.id)
    detailSession.value = full
  } catch {
    detailSession.value = s
  }
  detailOpen.value = true
}

// ── 结果展示 ──
function parseResult(s: AgentSession): any {
  try { return s.result ? JSON.parse(s.result) : {} } catch { return { output: s.result } }
}

const statusMeta: Record<string, { label: string; color: string; icon: any }> = {
  pending: { label: '排队中', color: 'default', icon: ClockCircleOutlined },
  running: { label: '执行中', color: 'processing', icon: SyncOutlined },
  completed: { label: '已完成', color: 'success', icon: CheckCircleOutlined },
  failed: { label: '失败', color: 'error', icon: CloseCircleOutlined },
}

function formatTime(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return `${d.getMonth() + 1}-${d.getDate()} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

function toolCount(a: Agent): number {
  return Array.isArray(a.tools) ? a.tools.length : 0
}
</script>

<template>
  <div class="agents-page">
    <div class="page-head">
      <div class="page-head-text">
        <h1 class="page-title">Agents</h1>
        <p class="page-sub">创建专属 Agent，定义提示词与工具，一键派发真实执行</p>
      </div>
      <Button type="primary" @click="openCreate">
        <template #icon><PlusOutlined /></template>
        新建 Agent
      </Button>
    </div>

    <Tabs v-model:activeKey="activeTab" class="agents-tabs">
      <!-- ── Tab 1：我的 Agent ── -->
      <TabPane key="agents" tab="我的 Agent">
        <PageSkeleton v-if="loadingAgents" variant="cards" :columns="3" :rows="6" :header="false" />
        <EmptyState
          v-else-if="errorAgents"
          size="page"
          :icon="markRaw(RobotOutlined)"
          description="加载失败"
          hint="无法获取 Agent 列表，请稍后重试"
        >
          <Button type="primary" @click="loadAgents">重试</Button>
        </EmptyState>
        <EmptyState
          v-else-if="agents.length === 0"
          size="page"
          :icon="markRaw(RobotOutlined)"
          description="还没有 Agent"
          hint="点击右上角「新建 Agent」，定义提示词与工具，开始派发任务"
        >
          <Button type="primary" @click="openCreate">
            <template #icon><PlusOutlined /></template>
            新建 Agent
          </Button>
        </EmptyState>
        <div v-else class="agent-grid">
          <div v-for="a in agents" :key="a.id" class="agent-card" :class="{ disabled: !a.enabled }">
            <div class="card-top">
              <span class="card-avatar"><RobotOutlined /></span>
              <div class="card-titles">
                <span class="card-name">{{ a.name }}</span>
                <span class="card-desc">{{ a.description || '暂无描述' }}</span>
              </div>
              <Dropdown trigger="click" placement="bottomRight">
                <Button type="text" size="small" class="card-more" title="更多操作" @click.stop>
                  <template #icon><EditOutlined /></template>
                </Button>
                <template #overlay>
                  <Menu class="card-menu">
                    <MenuItem key="edit" @click="openEdit(a)"><EditOutlined class="menu-icon" />编辑</MenuItem>
                    <MenuItem key="toggle" @click="toggleEnabled(a)">
                      <template v-if="a.enabled"><StopOutlined class="menu-icon" />停用</template>
                      <template v-else><PlayCircleOutlined class="menu-icon" />启用</template>
                    </MenuItem>
                    <MenuItem key="visibility" @click="toggleVisibility(a)">
                      <template v-if="a.visibility === 'tenant'"><LockOutlined class="menu-icon" />设为私有</template>
                      <template v-else><TeamOutlined class="menu-icon" />共享给团队</template>
                    </MenuItem>
                    <MenuItem key="delete" danger @click="requestDelete(a)"><DeleteOutlined class="menu-icon" />删除</MenuItem>
                  </Menu>
                </template>
              </Dropdown>
            </div>
            <div class="card-meta">
              <Tag v-if="a.visibility === 'tenant'" color="green">团队共享</Tag>
              <Tag :color="a.enabled ? 'green' : 'default'">{{ a.enabled ? '启用' : '已停用' }}</Tag>
              <Tag>{{ toolCount(a) }} 工具</Tag>
              <Tag>最多 {{ a.max_turns }} 轮</Tag>
            </div>
            <div class="card-actions">
              <Button type="primary" size="small" :disabled="!a.enabled" @click="openRun(a)">
                <template #icon><PlayCircleOutlined /></template>
                运行
              </Button>
              <Button size="small" title="在对话中发起会话" @click="chatWithAgent(a)">
                <template #icon><MessageOutlined /></template>
                发起对话
              </Button>
            </div>
          </div>
        </div>
      </TabPane>

      <!-- ── Agent 市场 ── -->
      <TabPane key="market" tab="市场">
        <PageSkeleton v-if="marketLoading" variant="cards" :columns="3" :rows="6" :header="false" />
        <EmptyState
          v-else-if="marketError"
          size="page"
          :icon="markRaw(ShopOutlined)"
          description="市场加载失败"
          hint="无法获取市场内容，请稍后重试"
        >
          <Button type="primary" @click="loadMarket">重试</Button>
        </EmptyState>
        <SkillMarketCard
          v-else
          :items="marketItems"
          type="agent"
          :installing-id="marketInstallingId"
          @install="handleMarketInstall"
        />
      </TabPane>

      <!-- ── Tab 2：运行记录 ── -->
      <TabPane key="sessions" tab="运行记录">
        <PageSkeleton v-if="loadingSessions" variant="list" :rows="6" :header="false" />
        <EmptyState
          v-else-if="errorSessions"
          size="page"
          :icon="markRaw(HistoryOutlined)"
          description="加载失败"
          hint="无法获取运行记录，请稍后重试"
        >
          <Button type="primary" @click="loadSessions">重试</Button>
        </EmptyState>
        <EmptyState
          v-else-if="sessions.length === 0"
          size="page"
          :icon="markRaw(HistoryOutlined)"
          description="暂无运行记录"
          hint="从「我的 Agent」派发任务后，运行结果将在此显示"
        />
        <div v-else class="session-list">
          <div v-for="s in sessions" :key="s.id" class="session-row">
            <span class="session-status-icon" :class="s.status">
              <component :is="statusMeta[s.status]?.icon" v-if="statusMeta[s.status]" />
            </span>
            <div class="session-main">
              <div class="session-task">{{ s.task }}</div>
              <div class="session-meta">
                <Tag :color="statusMeta[s.status]?.color">{{ statusMeta[s.status]?.label || s.status }}</Tag>
                <span v-if="s.agent_name" class="session-agent">{{ s.agent_name }}</span>
                <span class="session-time">{{ formatTime(s.created_at) }}</span>
              </div>
            </div>
            <Button size="small" type="text" @click="openDetail(s)">查看结果</Button>
          </div>
        </div>
      </TabPane>
    </Tabs>

    <!-- ── 新建/编辑 Modal ── -->
    <Modal
      :open="editorOpen"
      :title="editingId ? '编辑 Agent' : '新建 Agent'"
      :confirm-loading="editorSaving"
      width="640px"
      ok-text="保存"
      cancel-text="取消"
      @ok="saveEditor"
      @cancel="editorOpen = false"
    >
      <div class="editor-form">
        <div class="form-row">
          <label class="form-label">名称 *</label>
          <Input v-model:value="form.name" placeholder="如：数据分析师" :maxlength="60" />
        </div>
        <div class="form-row">
          <label class="form-label">描述</label>
          <Input v-model:value="form.description" placeholder="一句话说明 Agent 的职责" :maxlength="200" />
        </div>
        <div class="form-row">
          <label class="form-label">系统提示词</label>
          <Input.TextArea
            v-model:value="form.system_prompt"
            :rows="5"
            placeholder="定义 Agent 的角色、行为准则与目标…"
          />
        </div>
        <div class="form-row form-grid">
          <div class="form-field">
            <label class="form-label">模型</label>
            <Input v-model:value="form.model" placeholder="deepseek-chat" />
          </div>
          <div class="form-field">
            <label class="form-label">最大轮次</label>
            <InputNumber v-model:value="form.max_turns" :min="1" :max="30" class="w-full" />
          </div>
          <div class="form-field">
            <label class="form-label">超时（秒）</label>
            <InputNumber v-model:value="form.timeout_seconds" :min="10" :max="3600" class="w-full" />
          </div>
          <div class="form-field">
            <label class="form-label">Temperature</label>
            <InputNumber v-model:value="form.temperature" :min="0" :max="2" :step="0.1" class="w-full" />
          </div>
          <div class="form-field">
            <label class="form-label">Max Tokens</label>
            <InputNumber v-model:value="form.max_tokens" :min="256" :max="32768" :step="512" class="w-full" />
          </div>
          <div class="form-field">
            <label class="form-label">启用</label>
            <Switch v-model:checked="form.enabled" />
          </div>
        </div>
        <div class="form-row">
          <label class="form-label">工具（JSON）</label>
          <Input.TextArea
            v-model:value="form.tools_text"
            :rows="4"
            placeholder='[{"name":"shell_exec","description":"执行命令","parameters":{"type":"object","properties":{}}}]'
            class="tools-input"
          />
        </div>
      </div>
    </Modal>

    <!-- ── 运行 Modal ── -->
    <Modal
      :open="runOpen"
      :title="`运行「${runTarget?.name || ''}」`"
      :footer="null"
      :closable="true"
      width="600px"
      @cancel="closeRun"
    >
      <Input.TextArea
        v-model:value="runTask"
        :rows="3"
        placeholder="描述要完成的任务…"
        :disabled="!!runSession && runSession.status === 'running'"
      />
      <div class="run-actions">
        <Button
          type="primary"
          :loading="runSubmitting"
          :disabled="!!runSession && (runSession.status === 'running' || runSession.status === 'pending')"
          @click="submitRun"
        >
          <template #icon><PlayCircleOutlined /></template>
          派发任务
        </Button>
      </div>

      <div v-if="runSession" class="run-result">
        <Alert
          v-if="runSession.status === 'running' || runSession.status === 'pending'"
          type="info"
          show-icon
          :message="runSession.status === 'pending' ? '任务排队中…' : 'Agent 正在执行…（LLM 推理 + 工具调用）'"
        />
        <Alert
          v-else-if="runSession.status === 'failed'"
          type="error"
          show-icon
          message="执行失败"
          :description="parseResult(runSession).error || parseResult(runSession).output || '未知错误'"
        />
        <template v-else-if="runSession.status === 'completed'">
          <Alert type="success" show-icon message="执行完成" />
          <div class="result-block">
            <div class="result-label">输出</div>
            <pre class="result-output">{{ parseResult(runSession).output || '（无输出）' }}</pre>
          </div>
          <div v-if="parseResult(runSession).token_usage || parseResult(runSession).duration || parseResult(runSession).tool_calls?.length" class="result-meta">
            <Tag v-if="parseResult(runSession).token_usage">tokens: {{ JSON.stringify(parseResult(runSession).token_usage) }}</Tag>
            <Tag v-if="parseResult(runSession).duration">耗时 {{ parseResult(runSession).duration.toFixed(1) }}s</Tag>
            <Tag v-if="parseResult(runSession).tool_calls?.length">工具调用 {{ parseResult(runSession).tool_calls.length }} 次</Tag>
          </div>
        </template>
      </div>
    </Modal>

    <!-- ── 历史结果详情 Modal ── -->
    <Modal
      :open="detailOpen"
      :title="`运行结果 · ${detailSession?.agent_name || ''}`"
      :footer="null"
      width="640px"
      @cancel="detailOpen = false"
    >
      <div v-if="detailSession" class="run-result">
        <div class="session-meta">
          <Tag :color="statusMeta[detailSession.status]?.color">{{ statusMeta[detailSession.status]?.label || detailSession.status }}</Tag>
          <span class="session-time">{{ formatTime(detailSession.created_at) }}</span>
        </div>
        <Alert
          v-if="detailSession.status === 'failed'"
          type="error" show-icon message="执行失败"
          :description="parseResult(detailSession).error || parseResult(detailSession).output || '未知错误'"
        />
        <div class="result-block">
          <div class="result-label">任务</div>
          <pre class="result-output">{{ detailSession.task }}</pre>
        </div>
        <div v-if="parseResult(detailSession).output" class="result-block">
          <div class="result-label">输出</div>
          <pre class="result-output">{{ parseResult(detailSession).output }}</pre>
        </div>
        <div v-if="parseResult(detailSession).token_usage || parseResult(detailSession).duration || parseResult(detailSession).tool_calls?.length" class="result-meta">
          <Tag v-if="parseResult(detailSession).token_usage">tokens: {{ JSON.stringify(parseResult(detailSession).token_usage) }}</Tag>
          <Tag v-if="parseResult(detailSession).duration">耗时 {{ parseResult(detailSession).duration.toFixed(1) }}s</Tag>
          <Tag v-if="parseResult(detailSession).tool_calls?.length">工具调用 {{ parseResult(detailSession).tool_calls.length }} 次</Tag>
        </div>
      </div>
    </Modal>
  </div>
</template>

<style scoped>
.agents-page { max-width: 1080px; margin: 0 auto; padding: 28px 24px 60px; }
.page-head { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-bottom: 20px; }
.page-title { font-size: 24px; font-weight: 700; margin: 0; letter-spacing: -0.01em; }
.page-sub { margin: 4px 0 0; color: var(--text-tertiary); font-size: 13px; }
.agents-tabs :deep(.ant-tabs-nav) { margin-bottom: 20px; }

/* ── Agent 卡片 ── */
.agent-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 14px; }
.agent-card {
  display: flex; flex-direction: column; gap: 12px;
  padding: 18px 18px 14px;
  border: 1px solid var(--border-card);
  border-radius: var(--radius-lg);
  background: var(--bg-card);
  box-shadow: var(--shadow-md);
  transition: transform 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease;
}
.agent-card:hover { transform: translateY(-2px); border-color: var(--primary); box-shadow: var(--shadow-lg); }
.agent-card.disabled { opacity: 0.6; }
.card-top { display: flex; align-items: flex-start; gap: 10px; }
.card-avatar {
  flex: none; width: 38px; height: 38px; border-radius: 10px;
  background: var(--primary-bg); color: var(--primary);
  display: inline-flex; align-items: center; justify-content: center; font-size: 18px;
}
.card-titles { flex: 1; min-width: 0; }
.card-name { display: block; font-size: 15px; font-weight: 600; color: var(--text-primary); }
.card-desc {
  display: block; margin-top: 3px; font-size: 12px; color: var(--text-tertiary);
  line-height: 1.5; overflow: hidden; text-overflow: ellipsis; display: -webkit-box;
  -webkit-line-clamp: 2; -webkit-box-orient: vertical;
}
.card-more { color: var(--text-tertiary); }
.card-meta { display: flex; flex-wrap: wrap; gap: 4px; }
.card-actions { display: flex; justify-content: flex-end; }
.card-menu { min-width: 140px; border-radius: 10px; padding: 4px; box-shadow: var(--shadow-lg); }
.card-menu :deep(.ant-dropdown-menu-item) { display: flex; align-items: center; gap: 8px; font-size: 13px; border-radius: 6px; }
.menu-icon { font-size: 14px; }

/* ── 运行记录 ── */
.session-list { display: flex; flex-direction: column; gap: 8px; }
.session-row {
  display: flex; align-items: center; gap: 12px;
  padding: 12px 16px;
  border: 1px solid var(--border-card);
  border-radius: var(--radius-lg);
  background: var(--bg-card);
  box-shadow: var(--shadow-md);
}
.session-status-icon { flex: none; width: 30px; height: 30px; border-radius: 50%; display: inline-flex; align-items: center; justify-content: center; font-size: 14px; }
.session-status-icon.pending { background: var(--bg-secondary); color: var(--text-tertiary); }
.session-status-icon.running { background: var(--primary-bg); color: var(--primary); }
.session-status-icon.completed { background: rgba(34, 197, 94, 0.12); color: #22c55e; }
.session-status-icon.failed { background: rgba(239, 68, 68, 0.12); color: #ef4444; }
.session-main { flex: 1; min-width: 0; }
.session-task { font-size: 13px; color: var(--text-primary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.session-meta { display: flex; align-items: center; gap: 8px; margin-top: 4px; }
.session-agent { font-size: 12px; color: var(--text-secondary); }
.session-time { font-size: 12px; color: var(--text-tertiary); }

/* ── 表单 ── */
.editor-form { display: flex; flex-direction: column; gap: 14px; }
.form-row { display: flex; flex-direction: column; gap: 6px; }
.form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
.form-field { display: flex; flex-direction: column; gap: 6px; }
.form-label { font-size: 12px; color: var(--text-secondary); font-weight: 500; }
.w-full { width: 100%; }
.tools-input :deep(textarea) { font-family: var(--font-mono); font-size: 12px; }

/* ── 运行结果 ── */
.run-actions { display: flex; justify-content: flex-end; margin-top: 12px; }
.run-result { margin-top: 16px; display: flex; flex-direction: column; gap: 10px; }
.result-block { border: 1px solid var(--border-card); border-radius: 10px; background: var(--bg-secondary); overflow: hidden; }
.result-label { padding: 8px 12px; font-size: 12px; color: var(--text-tertiary); border-bottom: 1px solid var(--border-card); }
.result-output { margin: 0; padding: 12px; font-family: var(--font-mono); font-size: 12.5px; line-height: 1.7; color: var(--text-primary); white-space: pre-wrap; word-break: break-word; max-height: 300px; overflow-y: auto; }
.result-meta { display: flex; flex-wrap: wrap; gap: 4px; }

@media (max-width: 640px) {
  .agents-page { padding: 20px 16px 48px; }
  .form-grid { grid-template-columns: 1fr; }
}

@media (max-width: 768px) {
  .page-head { flex-direction: column; align-items: flex-start; }
  .page-head > .ant-btn { width: 100%; }
  .session-row { flex-wrap: wrap; row-gap: 8px; }
  .session-main { flex-basis: calc(100% - 42px); }
  .session-task { white-space: normal; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; }
}
</style>
