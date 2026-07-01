# Phase O: 多 Agent 协作

> **总预估**：4 周 | **前置**：V0.3 Agent Router

## O1: Agent 会话隔离（1 周）

每个子 Agent 拥有独立的：
- 消息历史（`mutable_messages`）
- LLM Provider 实例（可共享但上下文隔离）
- ToolRegistry 视图（仅可见允许的工具）
- PermissionHandler 实例（独立审批）
- 文件缓存（`_file_cache`）

```
UserSession
  ├── Main Agent（QueryEngine #1）
  │   └── 与用户对话、拆解任务
  │
  ├── Sub-Agent A（QueryEngine #2）
  │   ├── 上下文：模块 A 相关文件
  │   ├── 工具：read_file, write_to_file, bash
  │   └── 目标：实现模块 A
  │
  └── Sub-Agent B（QueryEngine #3）
      ├── 上下文：测试框架
      ├── 工具：read_file, write_to_file, bash
      └── 目标：为模块 A 编写测试
```

## O2: 多 Agent 流水线（1 周）

### 架构师 → 编码员 → 审查员流程

```
[用户需求]
    │
    ▼
┌──────────────┐
│ Architect    │  设计方案、API 接口、模块拆分
│ Agent        │  输出：`architecture.md`
└──────┬───────┘
       │
       ▼
┌──────────────┐   ┌──────────────┐   ┌──────────────┐
│ Coding       │   │ Coding       │   │ Coding       │
│ Agent A      │   │ Agent B      │   │ Agent C      │
│ (模块 1)     │   │ (模块 2)     │   │ (测试)       │
└──────┬───────┘   └──────┬───────┘   └──────┬───────┘
       │                   │                   │
       ▼                   ▼                   ▼
┌─────────────────────────────────────────────────────┐
│ Reviewer Agent                                      │
│ 检查：代码质量、测试覆盖、安全、风格                 │
│ 输出：Review 报告 + 修改建议                         │
└──────────────────────┬──────────────────────────────┘
                       │
                       ▼
              [用户审批/Agents 修改]
```

## O3: 自动 Git 工作流（1 周）

```python
class AutoGitWorkflow:
    async def create_pr(self, agent_id, changes):
        branch = f"ai/{agent_id}/{int(time.time())}"
        self.git.checkout_new_branch(branch)
        self.git.commit(changes, message=f"AI: {agent_id} changes")
        self.git.push(branch)
        return self.github.create_pr(branch, title=f"[AI] {agent_id}")
```

**功能**：
- 每个 Agent 自动创建功能分支
- 自动 Commit（含 AI 生成的消息）
- 自动创建 PR（含 AI 生成的描述）
- CI 触发后等待结果
- PR 被批准后自动 Merge

## O4: 冲突解决（0.5 周）

当多个 Agent 并行修改同一文件时：
1. 检测冲突（行级别）
2. 展示冲突面板（类似 VS Code 合并编辑器）
3. AI 自动建议合并方案
4. 用户手动解决（或一键接受 AI 建议）

## O5: 长期记忆（0.5 周）

- SQLite 存储：项目级知识图谱
- 记忆内容：API 设计决策、命名约定、架构选择
- 跨会话检索：Agent 启动时自动加载
- 向量化存储（复用 V0.3 RAG）

## 验收标准

- [ ] 多 Agent 并行执行不互相干扰
- [ ] 架构→编码→审查流水线工作
- [ ] 自动创建 Git 分支和 PR
- [ ] 冲突检测和解决
- [ ] 跨会话记忆正确加载
- [ ] 所有测试通过
