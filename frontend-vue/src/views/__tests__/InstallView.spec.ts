import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'

// jsdom 未实现 matchMedia，ant-design-vue 的组件依赖它
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

// 安装状态机：覆盖「无数据库 → 安装模式」的分支
//  1) 无令牌 → 显示"缺少令牌"引导（死锁修复的核心断言：不再只显示说明文字）
//  2) 有令牌 + APP_SECRET 已配置 → 直接进入 Step 2 数据库配置表单
//  3) 有令牌 + APP_SECRET 未配置 → 进入 Step 1 环境检测
//  4) 数据库就绪但无 owner → 正常模式创建管理员表单（legacy 分支）
//  5) 已初始化 → 显示系统已初始化视图
const statusNoDb = { data: { data: { needed: true, db: false, redis: false } } }
const statusNoOwner = { data: { data: { needed: true, db: true, redis: true, deps: [] } } }
const statusInstalled = { data: { data: { needed: false, db: true, redis: true } } }

// stub 需渲染默认 slot，否则 a-spin 内的向导内容不可见
const stubs = {
  'a-spin': { template: '<div><slot /></div>' },
  'a-steps': { template: '<div><slot /></div>' },
  'a-step': { template: '<div><slot /></div>' },
  'a-form': { template: '<form @submit.prevent="$emit(\'finish\')"><slot /></form>' },
  'a-form-item': { template: '<div><slot /></div>' },
  'a-input': { template: '<input :value="modelValue" />' },
  'a-input-password': { template: '<input type="password" />' },
  'a-input-number': { template: '<input type="number" />' },
  'a-button': { template: '<button><slot /></button>' },
}

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
}))

vi.mock('../../api/install', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../api/install')>()
  return {
    ...actual,
    getInstallStatus: vi.fn(),
    getInstallStep1: vi.fn(),
    postInstallStep2: vi.fn(),
    postInstallStep3: vi.fn(),
    setupSystem: vi.fn(),
    hasInstallToken: vi.fn(),
    getInstallToken: vi.fn(() => ''),
  }
})

import InstallView from '../InstallView.vue'
import * as installApi from '../../api/install'

const mockedApi = installApi as unknown as {
  getInstallStatus: ReturnType<typeof vi.fn>
  getInstallStep1: ReturnType<typeof vi.fn>
  hasInstallToken: ReturnType<typeof vi.fn>
}

describe('InstallView 安装模式状态机（死锁修复验证）', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('无数据库 + 无令牌 → 显示令牌引导，而非空白', async () => {
    mockedApi.getInstallStatus.mockResolvedValue(statusNoDb.data.data as any)
    mockedApi.hasInstallToken.mockReturnValue(false)

    const wrapper = mount(InstallView, { global: { stubs } })
    await flushPromises()

    // 死锁修复前：hint-info 的 v-if 抢占 v-else-if 链，向导分支永不渲染，页面只剩说明文字。
    // 修复后：应渲染"安装模式受令牌保护"引导
    expect(wrapper.text()).toContain('安装页面受令牌保护')
    expect(wrapper.text()).toContain('install_url')
  })

  it('无数据库 + 有令牌 + APP_SECRET 已配置 → 直接进入 Step 2 数据库配置', async () => {
    mockedApi.getInstallStatus.mockResolvedValue(statusNoDb.data.data as any)
    mockedApi.hasInstallToken.mockReturnValue(true)
    mockedApi.getInstallStep1.mockResolvedValue({
      completed: false,
      app_secret_set: true,
      data_writable: true,
      step2_done: false,
      step3_done: false,
    } as any)

    const wrapper = mount(InstallView, { global: { stubs } })
    await flushPromises()

    expect(wrapper.text()).toContain('填写 PostgreSQL 连接信息')
  })

  it('无数据库 + 有令牌 + APP_SECRET 未配置 → Step 1 环境检测提示配置主密钥', async () => {
    mockedApi.getInstallStatus.mockResolvedValue(statusNoDb.data.data as any)
    mockedApi.hasInstallToken.mockReturnValue(true)
    mockedApi.getInstallStep1.mockResolvedValue({
      completed: false,
      app_secret_set: false,
      data_writable: true,
      step2_done: false,
      step3_done: false,
    } as any)

    const wrapper = mount(InstallView, { global: { stubs } })
    await flushPromises()

    expect(wrapper.text()).toContain('系统未检测到有效的')
    expect(wrapper.text()).toContain('APP_SECRET')
  })

  it('数据库就绪但无 owner → 正常模式创建管理员表单（legacy 分支）', async () => {
    mockedApi.getInstallStatus.mockResolvedValue(statusNoOwner.data.data as any)

    const wrapper = mount(InstallView, { global: { stubs } })
    await flushPromises()

    expect(wrapper.text()).toContain('检测到系统尚未初始化')
    expect(wrapper.text()).toContain('初始化系统')
  })

  it('已初始化 → 显示系统已初始化视图', async () => {
    mockedApi.getInstallStatus.mockResolvedValue(statusInstalled.data.data as any)

    const wrapper = mount(InstallView, { global: { stubs } })
    await flushPromises()

    expect(wrapper.text()).toContain('系统已初始化')
    expect(wrapper.text()).toContain('前往登录')
  })
})
