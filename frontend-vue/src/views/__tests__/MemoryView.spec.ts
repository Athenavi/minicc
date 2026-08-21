import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'

// jsdom 未实现 matchMedia，ant-design-vue 的 Tabs/Grid 等组件（useBreakpoint）依赖它
if (!window.matchMedia) {
  window.matchMedia = ((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  })) as any
}

vi.mock('../../api/memory', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../api/memory')>()
  return {
    ...actual,
    listMemory: vi.fn(),
    upsertMemory: vi.fn(),
    updateMemory: vi.fn(),
    deleteMemory: vi.fn(),
    clearMemory: vi.fn(),
    searchMemory: vi.fn(),
    startOrganize: vi.fn(),
    getOrganizeStatus: vi.fn(),
  }
})

import MemoryView from '../MemoryView.vue'
import * as memoryApi from '../../api/memory'
import type { MemorySearchHit } from '../../api/memory'

function makeEntry(overrides: Partial<MemorySearchHit> = {}): MemorySearchHit {
  return {
    id: 'mem-1',
    slot: 'preference',
    slot_label: '偏好',
    key: 'editor',
    value: 'VSCode',
    confidence: 80,
    source: 'user_confirmed',
    source_label: '用户确认',
    has_embedding: true,
    access_count: 3,
    last_accessed_at: null,
    status: 'active',
    created_at: '2026-08-21T00:00:00Z',
    updated_at: '2026-08-21T00:00:00Z',
    similarity: 0.8,
    score: 0.7,
    ...overrides,
  }
}

describe('MemoryView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(memoryApi.listMemory).mockResolvedValue({
      entries: [],
      counts: { identity: 0, preference: 0, decision: 0, fact: 0 },
      total: 0,
      slots: [
        { slot: 'identity', label: '身份' },
        { slot: 'preference', label: '偏好' },
        { slot: 'decision', label: '关键决策' },
        { slot: 'fact', label: '长期事实' },
      ],
      organize: { running: false, started_at: null, finished_at: null, result: null, error: null },
    })
  })

  it('渲染标题、检索提示与记忆操作骨架', async () => {
    const wrapper = mount(MemoryView)
    await flushPromises()
    const text = wrapper.text()
    expect(text).toContain('长期记忆')
    // L2 档案卡设计意图（跨会话留存 · 语义检索 · 自动整理）
    expect(text).toContain('语义检索')
    expect(text).toContain('自动整理')
    // 三个核心操作按钮
    for (const label of ['智能整理', '清空记忆', '新建记忆']) {
      expect(text).toContain(label)
    }
    // 语义检索输入框
    const input = wrapper.find('input')
    expect(input.exists()).toBe(true)
    expect((input.element as HTMLInputElement).placeholder).toContain('语义检索')
  })

  it('挂载即加载记忆列表并展示条目', async () => {
    vi.mocked(memoryApi.listMemory).mockResolvedValue({
      entries: [makeEntry(), makeEntry({ id: 'mem-2', slot: 'fact', slot_label: '长期事实', key: 'stack', value: 'Go + Python' })],
      counts: { identity: 0, preference: 1, decision: 0, fact: 1 },
      total: 2,
      slots: [],
      organize: { running: false, started_at: null, finished_at: null, result: null, error: null },
    })
    const wrapper = mount(MemoryView)
    await flushPromises()
    expect(memoryApi.listMemory).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('editor')
    expect(wrapper.text()).toContain('VSCode')
    expect(wrapper.text()).toContain('Go + Python')
  })

  it('点击「智能检索」调用 searchMemory 并展示结果', async () => {
    vi.mocked(memoryApi.searchMemory).mockResolvedValue({
      query: '编辑器',
      mode: 'semantic',
      count: 1,
      results: [makeEntry({ value: 'VSCode', similarity: 0.92, score: 0.88 })],
    })
    const wrapper = mount(MemoryView)
    await flushPromises()

    await wrapper.find('input').setValue('编辑器')
    // 找到「智能检索」按钮并点击
    const buttons = wrapper.findAll('button')
    const searchBtn = buttons.find((b) => b.text().includes('智能检索'))
    expect(searchBtn).toBeTruthy()
    await searchBtn!.trigger('click')
    await flushPromises()

    expect(memoryApi.searchMemory).toHaveBeenCalledWith('编辑器', { top_k: 10 })
    expect(wrapper.text()).toContain('语义模式')
    expect(wrapper.text()).toContain('返回列表')
  })

  it('点击「智能整理」触发 startOrganize', async () => {
    vi.mocked(memoryApi.startOrganize).mockResolvedValue({
      started: true,
      status: { running: true, started_at: 1, finished_at: null, result: null, error: null },
    })
    const wrapper = mount(MemoryView)
    await flushPromises()
    const buttons = wrapper.findAll('button')
    const orgBtn = buttons.find((b) => b.text().includes('智能整理'))
    expect(orgBtn).toBeTruthy()
    await orgBtn!.trigger('click')
    await flushPromises()
    expect(memoryApi.startOrganize).toHaveBeenCalledOnce()
  })
})
