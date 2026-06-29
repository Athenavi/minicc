# 任务 04：上下文装配系统 ContextBuilder

> **所属阶段**：Phase 1 - 会话主循环与上下文装配 (The Core Loop)
> **对应模块**：模块 4 (Context Assembly)
> **预估工时**：2-3 天
> **依赖**：任务 01 (Pydantic 模型)

---

## 1. 任务目标

实现 ContextBuilder，在每次 LLM 请求前自动收集项目的 Git 状态、项目规则 (.minicc/rules.md)、长期记忆 (memory.md)、系统信息等，拼装成完整的 System Prompt 上下文。

## 2. 详细子任务

### 2.1 ContextBuilder 类设计

- [ ] 文件：`backend/app/core/context_builder.py`

```python
class ContextBuilder:
    """
    上下文装配系统。
    负责在每次 LLM 请求前收集所有上下文信息并拼装为 System Prompt。
    
    架构理念：参考 Claude Code QueryEngine 的 submitMessage() 中
    读取 cwd、commands、tools、mcpClients 等运行时资源的做法。
    """
    
    def __init__(self, workspace_dir: str):
        self.workspace_dir = workspace_dir
    
    async def build_context(self) -> SystemContext:
        """收集所有上下文并返回"""
        ...
```

- [ ] 返回值 `SystemContext(BaseModel)` 包含：
  - `system_prompt_parts`: list[str] — 各段 System Prompt
  - `git_state`: GitState | None
  - `rules`: str | None
  - `memory`: str | None
  - `system_info`: SystemInfo

### 2.2 Git 状态收集器 (`GitContextProvider`)

- [ ] 文件：`backend/app/core/context_builder.py`
- [ ] 使用 GitPython 扫描当前工作目录
- [ ] `class GitState(BaseModel)`:
  - `branch`: str — 当前分支名
  - `is_dirty`: bool — 是否有未提交变更
  - `unstaged_files`: list[str] — 未暂存文件列表
  - `staged_files`: list[str] — 已暂存文件列表
  - `recent_commits`: list[CommitInfo] — 最近 5 条 commit（hash, message, author, date）
  - `remote_url`: str | None — 远程仓库 URL
  - `diff_summary`: str | None — `git diff --stat` 摘要（仅当 is_dirty=True）
- [ ] 错误处理：非 Git 目录时优雅降级，不报错
- [ ] 性能注意：`git diff` 只跑 `--stat`，不加载完整 diff

### 2.3 项目规则加载器 (`RulesProvider`)

- [ ] 扫描 `.minicc/rules.md` 文件
- [ ] 如果不存在，返回 None
- [ ] 规则文件格式约定：
  - Markdown 格式
  - 可以包含代码风格指南、项目架构约定、API 使用规范等
  - 类比 Claude Code 的 `CLAUDE.md`

### 2.4 长期记忆加载器 (`MemoryProvider`)

- [ ] 扫描 `.minicc/memory.md` 文件
- [ ] 如果不存在，返回 None
- [ ] 记忆文件由 Agent 自动维护（Phase 3 实现写入能力）
- [ ] 格式约定：
  - 每个条目以 `- [ ]` 或 `- [x]` 开头（待办/已完成）
  - 可包含项目决策记录、已知问题、折中方案等

### 2.5 系统信息收集器 (`SystemInfoProvider`)

- [ ] `class SystemInfo(BaseModel)`:
  - `os`: str — `platform.platform()`
  - `python_version`: str — `sys.version`
  - `current_time`: str — ISO 8601
  - `timezone`: str — `datetime.now().astimezone().tzinfo`
  - `workspace_dir`: str — 工作目录
  - `hostname`: str (可选)

### 2.6 System Prompt 拼装

- [ ] 将所有上下文按固定模板组合：

```
你是一个极简工程级智能编程助手 MiniCC，运行在 {system_info.os} 上。

## 项目上下文
工作目录: {workspace_dir}
当前时间: {current_time}

## Git 状态
当前分支: {branch}
未提交变更: {unstaged_files_count} 个文件
最近提交: {recent_commits}

## 项目规则
{rules_content}

## 长期记忆
{memory_content}

## 可用工具
{tools_description}

请遵循以下原则：
1. 所有文件操作必须先展示 diff 并获得用户批准
2. Shell 命令执行前必须说明预期效果
3. ...
```

- [ ] 提供一个 `build_system_prompt()` 方法返回最终的字符串
- [ ] System Prompt 长度不超过模型的 Context Window（可配置截断策略）

### 2.7 单元测试

- [ ] 测试 Git 上下文收集（mock GitPython）
- [ ] 测试 rules.md 加载（临时文件）
- [ ] 测试 memory.md 加载
- [ ] 测试 System Prompt 拼装完整性
- [ ] 测试非 Git 目录降级行为

## 3. 验收标准

| # | 检查项 | 验证方式 |
|:-|:-|:-|
| 1 | 在 Git 仓库中能正确读取分支、diff 摘要 | pytest |
| 2 | 非 Git 目录不报错，返回空 GitState | pytest |
| 3 | `.minicc/rules.md` 存在时能正确加载 | pytest |
| 4 | `.minicc/memory.md` 存在时能正确加载 | pytest |
| 5 | System Prompt 包含所有必要部分 | pytest 断言字符串包含 |
| 6 | 构建耗时 < 200ms（避免阻塞主循环） | 性能测试 |

## 4. 参考资源

- [GitPython 文档](https://gitpython.readthedocs.io/)
- [Claude Code CLAUDE.md 文档](https://docs.anthropic.com/en/docs/claude-code/overview)
- 规划文档 §3 Phase 1 任务 1.1
- 参考 xuanyuancode 教程中关于 Context Assembly 的讲解

## 5. 注意事项

- Git 操作必须使用 `subprocess` 或 GitPython 的轻量模式，避免加载整个仓库
- 上下文信息必须限制大小：diff 摘要不超过 2KB，rules 不超过 8KB，memory 不超过 4KB
- 考虑缓存：Git 状态和系统信息可以缓存 30s，减少重复读取
- 如果 rules.md 超大，只取前 100 行并添加截断提示
