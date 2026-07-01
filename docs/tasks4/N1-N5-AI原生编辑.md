# Phase N: AI 原生编辑

> **总预估**：3 周 | **前置**：Phase M

## 概述

Phase N 是 V0.4 的核心差异化功能。AI 不再通过 `write_to_file` / `str_replace_editor` 等工具间接编辑文件，而是**直接在编辑器中操作**，就像人类开发者一样。

---

## N1: AI 编辑协议（1 周）

### 后端 — EditorAction 协议

`backend/app/tools/editor_actions.py`：

```python
class EditorAction(BaseModel):
    type: str  # "select" | "replace" | "insert" | "delete" | "stream"
    path: str
    range: Optional[Range]  # 选中范围（行/列）
    text: str               # 替换/插入的内容
    cursor: Optional[Position]  # 操作后光标位置
```

### 后端 — StreamEdit 协议（流式写入）

```python
class StreamEditAction(BaseModel):
    type: "stream_start" | "stream_chunk" | "stream_end"
    path: str
    position: Position
    characters: str  # 逐字符流
```

### 新工具

| 工具名 | 描述 | 权限 |
|:-------|:-----|:-----|
| `editor_select` | 选中指定范围的代码 | READ |
| `editor_replace` | 替换选中内容 | WRITE |
| `editor_insert` | 在指定位置插入代码 | WRITE |
| `editor_stream` | 流式写入内容 | WRITE |

### AI System Prompt 更新

在 Session Guidance 中增加编辑器工具的说明，告知 AI：
- 优先使用 `editor_select` + `editor_replace` 进行精准修改
- 使用 `editor_insert` 在光标后添加新代码
- 使用 `editor_stream` 输出长代码块（逐字符展示）

---

## N2: Tab-to-Accept（1 周）

`frontend/src/components/editor/InlineCompletion.tsx`：

### 功能

- AI 在光标位置预测下一步代码
- 灰色幽灵文本显示预测内容
- 按 Tab 接受，按 Esc 拒绝
- 接受后继续预测下一个位置

### 实现

```typescript
interface InlineCompletion {
  text: string;           // 补全内容
  position: Position;     // 补全位置
  source: "llm" | "lsp" | "snippet";
}
```

- 使用 Monaco `registerInlineCompletionProvider` API
- 请求 LLM 获取补全（优化：预取 + 缓存）
- 去抖：300ms 无输入后触发
- 上下文窗口：当前文件 ±50 行 + 附近文件摘要

### 性能

- 补全请求最长时间：2s（超时降级到 LSP）
- 单文件补全缓存：最后 5 个位置
- 不阻塞编辑器输入

---

## N3: Natural Edit（1 周）

`frontend/src/components/editor/NaturalEditPanel.tsx`：

### 功能

用户选中一段代码后，在浮窗中描述修改意图：

```
[用户选择] 第 15-25 行
[用户输入] "把这个函数改为 async"
[AI 输出] 选中范围的新版本
[用户确认] Accept / Revise / Discard
```

### 实现

- Monaco `onDidChangeCursorPosition` + 选区事件
- 浮窗 UI（类似 Copilot 聊天面板但更轻量）
- 按 Ctrl+I / Cmd+I 触发
- 历史记录（可回退到之前的 AI 建议）

### 后端 API

```
POST /api/editor/natural-edit
{
  "path": "/src/main.py",
  "selected_range": {"start": {"line": 15, "col": 0}, "end": {"line": 25, "col": 0}},
  "instruction": "把这个函数改为 async",
  "code_context": "...选中的代码..."
}
→ {
  "suggestions": ["修改后代码..."],
  "explanation": "因为...所以改为 async"
}
```

---

## N4: Multi-cursor AI + N5: Undo/Redo（1 周）

### N4: Multi-cursor AI

- 用户创建多个光标（Alt+Click）
- AI 同时在所有光标位置执行相同操作
- 适用场景：批量重命名、添加类型注解、添加日志

### N5: Undo/Redo

- 每个 AI 操作作为一个独立的 Undo 单元
- 使用 Monaco `pushUndoStop()` API
- 操作前自动保存快照
- 支持逐级回退 AI 操作

## 验收标准

- [ ] N1: AI 编辑协议工具可用，StreamEdit 流式展示
- [ ] N2: Tab-to-Accept 幽灵文本补全
- [ ] N3: Natural Edit 浮窗，选中→描述→修改
- [ ] N4: Multi-cursor AI 批量操作
- [ ] N5: AI 操作可撤销/重做
- [ ] 所有现有测试通过
