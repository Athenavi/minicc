## 前端重新设计 — 完成总结

### 修改的文件

| 文件 | 操作 | 说明 |
|------|------|------|
| `frontend/src/app/globals.css` | 重写 | 新增 Inter + JetBrains Mono 字体（Google Fonts）、新色彩体系（`#F7F8FC`/#EEF1F8/#635BFF/#0A2540 等）、完整暗色模式对应值、自定义设计 tokens 映射到 Tailwind v4 `@theme` |
| `frontend/src/app/workspace/page.tsx` | 重写 | 完整三栏布局（左240px/中fill/右320px），包含所有设计规范中的组件 |
| `frontend/src/components/ThemeToggle.tsx` | 新建 | 日间/夜间/跟随系统 三态主题切换按钮 |

### 三栏布局结构

```
┌─ Left Panel (240px) ─┬─ Center Panel (fill) ────┬─ Right Panel (320px) ─┐
│  Conversation        │  [≡] workspace/agent [≡]  │  Media              │
│  [Search...]         │  ● Claude 3.5 Sonnet      │  [Files][Media][Ctx] │
│  Conv 1  ●           │                           │  ┌──── TS ────┐     │
│  Conv 2              │  ┌── User bubble ──┐      │  │ Button.tsx │     │
│  Conv 3              │  │                 │      │  └────────────┘     │
│  ─────────────────   │  └────────────────┘      │  Current Context     │
│  [D] Developer       │  ◀── AI (purple avatar)   │  ● src/components/  │
│      Pro Plan   ⚙   │  ┌── Code block ─────┐    │  ● src/styles/      │
└──────────────────────┘  │ tsx     [Copy]    │    └──────────────────────┘
                          └───────────────────┘
                          [📎] Type a message... [➤]
                          main  0⚠  2◥   Ln12,Col34
```

### 关键功能保留

- SSE 事件流处理（streaming、tool calls、turn_done）
- 发送消息（/submit API）
- 新建/切换/删除对话
- 聊天权限审批对话框
- 侧栏折叠/展开（左右两侧独立控制）
- 免费额度横幅提醒
- 代码输出面板
- 文件列表、媒体库标签页
- 暗色模式通过原有的 `.dark` class 和 CSS 变量继承生效
