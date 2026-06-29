# 任务 02：前端 Next.js 项目搭建

> **所属阶段**：Phase 0 - 基础骨架与协议定义
> **对应模块**：模块 1 (The Shell - 前端)
> **预估工时**：2-3 天
> **依赖**：无（与任务 01 并行）

---

## 1. 任务目标

搭建 Next.js 14+ (App Router) 前端项目，配置 TailwindCSS 和 Shadcn/ui，实现静态布局——左侧历史会话列表、右侧对话流区域。

## 2. 详细子任务

### 2.1 项目初始化

- [ ] 使用 `create-next-app` 创建项目：

```bash
npx create-next-app@latest frontend --typescript --tailwind --eslint --app --src-dir --import-alias "@/*"
```

- [ ] 安装 Shadcn/ui 核心组件：

```bash
cd frontend
npx shadcn@latest init
npx shadcn@latest add button input card scroll-area separator sheet dialog badge avatar
```

- [ ] 安装额外依赖：

```bash
npm i clsx tailwind-merge class-variance-authority lucide-react
npm i react-markdown remark-gfm rehype-highlight  # Markdown 渲染
npm i react-diff-viewer-continued                   # Diff 预览 (Phase 2)
```

### 2.2 目录结构实现

```
frontend/src/
├── app/
│   ├── layout.tsx              # 根布局（全局字体、主题 Provider）
│   ├── page.tsx                # 主页面（左侧历史 + 右侧对话）
│   └── globals.css             # Tailwind 全局样式
├── components/
│   ├── ui/                     # Shadcn/ui 组件（自动生成）
│   ├── chat/
│   │   ├── ChatLayout.tsx      # 左右分栏布局容器
│   │   ├── MessageList.tsx     # 消息流容器（骨架）
│   │   ├── MessageBubble.tsx   # 单条消息气泡（骨架）
│   │   └── MarkdownRenderer.tsx# Markdown 渲染组件（骨架）
│   ├── tools/                  # Phase 2 填充
│   │   └── ToolCallCard.tsx    # 骨架占位
│   ├── approvals/              # Phase 2 填充
│   │   └── ApprovalDialog.tsx  # 骨架占位
│   └── sidebar/
│       ├── SessionList.tsx     # 历史会话列表
│       └── SessionItem.tsx     # 单条历史记录
├── hooks/
│   └── useWebSocket.ts         # WebSocket 钩子（骨架）
└── lib/
    ├── utils.ts                # cn() 工具函数
    └── api.ts                  # API 客户端（骨架）
```

### 2.3 布局实现

#### 主布局 (`app/layout.tsx`)

- [ ] `Inter` 字体 + `Geist Mono` 等宽字体
- [ ] 暗黑模式 Provider（Shadcn `next-themes`）
- [ ] 全局样式：全屏高度、无滚动条 overflow 控制

#### 主页面 (`app/page.tsx`)

- [ ] 左右分栏：左侧 `Sidebar` (w-80, 可折叠) + 右侧 `ChatArea` (flex-1)
- [ ] 左侧 Sidebar：
  - [ ] "New Chat" 按钮（蓝色 CTA）
  - [ ] 历史会话列表（用 ScrollArea 包裹）
  - [ ] 每个 SessionItem 显示标题、时间戳
- [ ] 右侧 ChatArea：
  - [ ] 顶部 Header（显示当前会话名称 + 模型选择器占位）
  - [ ] 中间 MessageList（ScrollArea，flex-1）
  - [ ] 底部 InputBar（多行输入框 + Send 按钮）

#### 布局状态管理

- [ ] 移动端：对话页隐藏 Sidebar，通过汉堡菜单切换
- [ ] 桌面端：固定左侧栏，可拖拽调整宽度（可选）

### 2.4 核心 UI 组件骨架

#### `MessageBubble.tsx`

- [ ] 根据 `role` (user/assistant/system/tool) 渲染不同样式
- [ ] User 消息：右对齐，蓝色背景
- [ ] Assistant 消息：左对齐，灰色背景 + 头像图标
- [ ] 预留 `ContentBlock` 渲染插槽（文本块、工具调用卡片等）
- [ ] 显示时间戳（可选）

#### `MessageList.tsx`

- [ ] 接收 `Message[]` 数组渲染
- [ ] 自动滚动到底部（`useEffect` + `scrollIntoView`）
- [ ] 加载状态指示器（三点跳动动画）

#### `MarkdownRenderer.tsx`

- [ ] 基于 `react-markdown` + `remark-gfm` + `rehype-highlight`
- [ ] 代码块复制按钮
- [ ] 行号显示（代码块）
- [ ] 安全过滤（不允许渲染 HTML 标签）

### 2.5 WebSocket 钩子骨架 (`hooks/useWebSocket.ts`)

- [ ] 建立连接：`new WebSocket("ws://localhost:8000/ws/{session_id}")`
- [ ] 自动重连策略（指数退避，最多 5 次）
- [ ] 消息分发：`onMessage` 回调注册机制
- [ ] 发送方法：`send(message: string)` 和 `sendJSON(data: object)`
- [ ] 连接状态暴露：`connectionStatus: "connecting" | "connected" | "disconnected"`
- [ ] 心跳检测：每 30s ping，60s 无响应断开重连

### 2.6 API Client 骨架 (`lib/api.ts`)

- [ ] `GET /health` 健康检查
- [ ] `GET /api/tools` 获取工具列表
- [ ] `GET /api/sessions` 获取历史会话列表
- [ ] `GET /api/sessions/{id}` 获取会话详情

## 3. 验收标准

| # | 检查项 | 验证方式 |
|:-|:-|:-|
| 1 | `npm run dev` 启动无报错，访问 localhost:3000 | 浏览器查看 |
| 2 | 左侧侧边栏 + 右侧对话区布局正确 | 视觉检查 |
| 3 | 暗黑模式切换正常 | 按钮切换测试 |
| 4 | MessageBubble 区分 user/assistant 样式 | 静态数据测试 |
| 5 | Markdown 代码块含复制按钮和语法高亮 | 静态 Markdown 测试 |
| 6 | 移动端侧边栏自动隐藏 | 调整视口宽度测试 |
| 7 | Shadcn 组件 (Button/Input/ScrollArea) 渲染正常 | 视觉检查 |

## 4. 参考资源

- [Next.js 14 App Router](https://nextjs.org/docs/app)
- [Shadcn/ui 文档](https://ui.shadcn.com/)
- [TailwindCSS 暗黑模式](https://tailwindcss.com/docs/dark-mode)
- [react-markdown](https://github.com/remarkjs/react-markdown)
- 规划文档 §2 目录结构、§3 Phase 0 任务 0.3

## 5. 注意事项

- 所有组件优先使用 Server Components 除非需要交互 (use client)
- 使用 `cn()` 工具函数合并 Tailwind 类名
- 字体优先使用 next/font 加载，避免布局偏移 (CLS)
- 参考 DeepSeek-Reasonix 的桌面端布局理念：极简、高效
- 移动端适配是硬性要求，各组件需响应式
