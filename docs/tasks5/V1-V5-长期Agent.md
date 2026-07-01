# V1-V5: 长期自主 Agent

> **总预估**：4 周 | **前置**：Phase U

## V1: 长期记忆 2.0（1 周）

`backend/app/memory/long_term_memory.py`：

```python
class LongTermMemory:
    """向量化记忆 + 知识图谱（升级 V0.3 RAG）。"""

    async def store(self, item: MemoryItem) -> None:
        """存储记忆片段：
        - 向量化（复用 V0.3 embedder）
        - 时间戳 + 上下文标签
        - 关联到知识图谱节点
        """

    async def recall(self, query: str, context: dict = None) -> list[MemoryItem]:
        """检索相关记忆：
        - 语义搜索（向量相似度）
        - 时间衰减（近期优先）
        - 关联推理（图谱遍历）
        """
```

## V2: 目标管理（1 周）

`backend/app/agents/goal_manager.py`：

```python
class GoalManager:
    """设定 OKR → 拆解为每日任务。"""

    async def set_goal(self, objective: str, key_results: list[str]) -> Goal:
        """用户设定 OKR → AI 拆解为：
        - 周计划（里程碑）
        - 日任务（可执行步骤）
        - 依赖关系
        """

    async def get_daily_tasks(self) -> list[Task]:
        """生成今日待办。"""
```

## V3: 自主执行循环（1 周）

`backend/app/agents/autonomous_loop.py`：

```python
class AutonomousLoop:
    """长期自主执行的 Plan-Do-Check-Act 循环。"""

    async def run(self, goal: Goal) -> None:
        """loop:
           1. PLAN: 检查进度 → 规划今日任务
           2. DO:   执行任务（编码/测试/部署）
           3. CHECK: 验证结果（测试/Review）
           4. ACT:   调整计划 → 记录进度
        """
```

## V4: 进度报告（0.5 周）

- AI 自动生成日报（完成/阻塞/明日计划）
- 周报（进度百分比、关键决策、风险）
- 推送到 Slack/企业微信/邮件

## V5: 决策请求（0.5 周）

- 遇到关键决策时暂停
- 生成决策文档（选项、影响分析、推荐）
- 等待用户确认后继续
- 超时策略：24h 无响应 → 执行推荐选项

### 验收标准
- [ ] 记忆跨会话正确检索
- [ ] OKR 自动拆解为日任务
- [ ] 自主执行循环连续运行 > 7 天
- [ ] 日报推送正常
- [ ] 决策暂停等待用户确认
- [ ] 120 测试通过
