# Phase P: 可视化 Debugger

> **总预估**：3 周 | **前置**：无

## P1: Python Debugger 集成（1 周）

### 后端 — Debug Adapter

`backend/app/tools/debugger.py`：

```python
class DebugSession:
    """Python 调试会话 —— 基于 pydebug / debugpy。"""

    async def launch(self, file_path: str, args: list[str] = None):
        """启动调试会话。"""

    async def set_breakpoint(self, file_path: str, line: int):
        """设置断点。"""

    async def continue_(self):
        """继续执行。"""

    async def step_over(self):
        """单步跳过。"""

    async def step_into(self):
        """单步进入。"""

    async def get_variables(self) -> list[Variable]:
        """获取当前作用域变量。"""

    async def get_stack_frames(self) -> list[StackFrame]:
        """获取调用栈。"""

    async def stop(self):
        """停止调试。"""
```

### DAP 协议集成

使用 Debug Adapter Protocol（DAP）：
- `debugpy` 作为后端（Python 官方调试器）
- DAP over WebSocket（编辑器面板连接）
- 支持：断点、单步、变量查看、调用栈

## P2: 变量监视面板（0.5 周）

`frontend/src/components/debugger/VariablePanel.tsx`：

- 树形变量查看器（展开/折叠）
- 按类型着色（str=绿色，int=蓝色，list=紫色）
- 搜索过滤变量名
- AI 可读格式（变量变化高亮）

## P3: 调用栈可视化（0.5 周）

`frontend/src/components/debugger/CallStack.tsx`：

- 栈帧列表（当前帧在顶部）
- 点击跳转到对应文件/行
- 每帧显示：函数名、文件、行号、参数预览
- AI 自动分析调用栈（"问题在第 3 帧"）

## P4: AI Debug 循环（1 周）

### 自动 Debug 流程

```
1. [用户/测试失败] → 触发 AI Debug
2. [AI] 分析错误栈 → 猜测根因
3. [AI] 设置断点 → 启动调试
4. [AI] 单步执行 → 检查变量
5. [AI] 确认根因 → 生成修复方案
6. [AI] 展示修复 Diff → 等待用户确认
7. [用户确认] → 应用修复
8. [AI] 重新运行测试 → 验证修复
```

### 新工具

| 工具名 | 描述 | 权限 |
|:-------|:-----|:-----|
| `debug_launch` | 启动调试会话 | EXECUTE |
| `debug_set_breakpoint` | 设置断点 | WRITE |
| `debug_step` | 单步调试 | EXECUTE |
| `debug_get_vars` | 获取变量 | READ |
| `debug_auto_debug` | 自动 Debug 循环 | EXECUTE |

## 验收标准

- [ ] Python 调试会话可启动
- [ ] 断点设置和命中
- [ ] 变量监视面板显示正确
- [ ] 调用栈可视化可点击跳转
- [ ] AI Debug 循环完成自动修复
- [ ] 所有测试通过
