"""跨工作台调度器 (TaskRouter)

设计目标:
- 接收自然语言任务,自动分解为子任务
- 根据 Capabilities Registry 动态匹配最优执行路径
- 编排多工作台协同执行 (对话 → Agent → 工作流 → 技能 → 知识库)
- 统一异常处理与错误恢复

核心思想:
类比操作系统的进程调度器 + 微服务网关
- 能力发现 → 任务拆解 → DAG 构建 → 执行编排 → 结果聚合
"""
from __future__ import annotations

import json
import logging
import time
from typing import Any, Optional
from dataclasses import dataclass, field
from enum import Enum

from app.core.capabilities import (
    get_registry,
    Capability,
    WorkstationType,
    CapabilityType,
)

logger = logging.getLogger(__name__)


class TaskPriority(str, Enum):
    """任务优先级"""
    LOW = "low"
    NORMAL = "normal"
    HIGH = "high"
    CRITICAL = "critical"


@dataclass
class SubTask:
    """子任务"""
    subtask_id: str
    description: str
    capability_id: str  # 匹配到的能力 ID
    parameters: dict[str, Any] = field(default_factory=dict)
    dependencies: list[str] = field(default_factory=list)  # 依赖的其他 subtask_id
    tags: list[str] = field(default_factory=list)  # 意图关键词 (能力语义匹配用)
    workstation_type: Optional[WorkstationType] = None
    status: str = "pending"  # pending/running/completed/failed


@dataclass 
class ExecutedTask:
    """执行的任务 (含结果)"""
    task_id: str
    subtask_id: str
    capability_id: str
    input_params: dict
    output: Any = None
    error: str = ""
    duration_ms: int = 0
    status: str = "completed"  # completed/failed


class TaskRouter:
    """跨工作台任务路由器
    
    核心流程:
    1. Intent Understanding → 理解用户意图
    2. Task Decomposition → 拆解为子任务
    3. Capability Matching → 为每个子任务匹配能力
    4. DAG Construction → 构建执行依赖图
    5. Execution Planning → 生成执行计划 (并行优化)
    6. Result Aggregation → 聚合输出
    """
    
    def __init__(self):
        self.registry = get_registry()
        self._execution_history: list[ExecutedTask] = []  # 最近 100 条
    
    async def route_task(
        self,
        user_input: str,
        tenant_id: str,
        context: dict[str, Any] = None,
        priority: TaskPriority = TaskPriority.NORMAL,
        trace_id: str = "",
    ) -> dict[str, Any]:
        """路由并执行任务 (完整流程)
        
        Args:
            user_input: 用户自然语言输入
            tenant_id: 租户 ID
            context: 上下文 (包含历史对话、共享状态)
            priority: 优先级
            trace_id: 链路追踪 ID
            
        Returns:
            {
                "task_id": "xxx",
                "status": "completed",
                "output": {...},
                "subtasks": [...],
                "trace_id": "xxx",
                "duration_ms": 1234,
            }
        """
        start_time = time.time()
        task_id = f"task_{int(time.time())}_{tenant_id[:8]}"
        
        logger.info(
            "Routing task (task_id=%s, tenant=%s, priority=%s, trace_id=%s)",
            task_id, tenant_id, priority.value, trace_id,
        )
        
        try:
            # ── 阶段 1: 意图理解 ─────────────────────────────────────
            intent = await self._understand_intent(user_input, context or {})
            
            # ── 阶段 2: 任务拆解 ─────────────────────────────────────
            subtasks = await self._decompose_task(intent, tenant_id)
            
            if not subtasks:
                return {
                    "task_id": task_id,
                    "status": "no_match",
                    "output": {"error": "未找到匹配的能力"},
                    "trace_id": trace_id,
                    "duration_ms": int((time.time() - start_time) * 1000),
                }
            
            # ── 阶段 3: 能力匹配 & DAG 构建 ─────────────────────────
            matched_tasks = await self._match_capabilities(subtasks, tenant_id)
            
            # ── 阶段 4: 执行计划生成 (拓扑排序) ─────────────────────
            execution_order = self._topological_sort(matched_tasks)
            
            # ── 阶段 5: 执行 (按序,支持并行) ────────────────────────
            results = await self._execute_tasks(
                execution_order,
                tenant_id=tenant_id,
                trace_id=trace_id,
            )
            
            # ── 阶段 6: 结果聚合 ─────────────────────────────────────
            final_output = await self._aggregate_results(
                results, intent, user_input
            )
            
            total_duration = int((time.time() - start_time) * 1000)
            
            result = {
                "task_id": task_id,
                "status": "completed",
                "output": final_output,
                "subtasks": [
                    {
                        "subtask_id": t.subtask_id,
                        "capability_id": t.capability_id,
                        "status": t.status,
                        "duration_ms": t.duration_ms,
                    }
                    for t in results
                ],
                "trace_id": trace_id,
                "total_duration_ms": total_duration,
                "priority": priority.value,
            }
            
            logger.info(
                "Task completed (task_id=%s, subtasks=%d, duration=%dms)",
                task_id, len(results), total_duration,
            )
            
            return result
            
        except Exception as e:
            logger.error(f"Task routing failed: {e}", exc_info=True)
            return {
                "task_id": task_id,
                "status": "error",
                "output": {"error": str(e)},
                "trace_id": trace_id,
                "duration_ms": int((time.time() - start_time) * 1000),
            }
    
    async def _understand_intent(
        self,
        user_input: str,
        context: dict,
    ) -> dict[str, Any]:
        """理解用户意图
        
        策略:
        1. 尝试调用 LLM (如果 gateway 可用)
        2. 降级到关键词提取 (LLM 不可用时)
        """
        # 尝试使用 LLM 进行意图识别
        try:
            return await self._llm_understand_intent(user_input, context)
        except Exception as e:
            logger.warning(f"LLM intent recognition failed, falling back to keywords: {e}")
            # 降级到关键词提取
            return {
                "keywords": self._extract_keywords(user_input),
                "entities": self._extract_entities(user_input),
                "action": self._infer_action(user_input),
                "fallback": True,
            }
    
    async def _llm_understand_intent(
        self,
        user_input: str,
        context: dict,
    ) -> dict[str, Any]:
        """使用 LLM 理解用户意图 (生产级实现)"""
        from app.gateway.router import get_gateway
        
        gateway = get_gateway()
        if not gateway:
            raise RuntimeError("Gateway not available for LLM intent recognition")
        
        prompt = f"""Analyze the user's intent and return a structured JSON response.

User input: "{user_input}"

Context: {json.dumps(context, ensure_ascii=False) if context else "None"}

Please provide:
1. action: One of ["analyze", "generate_code", "search", "create_workflow", "manage_knowledge", "general"]
2. keywords: List of important keywords/entities
3. entities: Extracted entities (files, functions, data)
4. complexity: ["simple", "moderate", "complex"] - How many steps needed
5. recommended_workstations: List of workstation types that should handle this task
6. description: Brief description of what the user wants to achieve

Return ONLY valid JSON without markdown formatting."""
        
        messages = [
            {
                "role": "system",
                "content": "You are a task intent analyzer. Your job is to understand user requests and structure them for automated processing. Return only JSON, no explanations."
            },
            {"role": "user", "content": prompt}
        ]
        
        # 调用 LLM
        response_text = ""
        async for chunk in gateway.chat_stream(messages=messages, model="gpt-4o-mini"):
            if chunk.content:
                response_text += chunk.content
        
        # 解析 LLM 响应
        import re
        # 清理可能的 markdown 格式
        response_text = re.sub(r'```json\s*|\s*```', '', response_text.strip())
        
        try:
            intent = json.loads(response_text)
            intent["fallback"] = False
            logger.info(f"LLM intent recognized: action={intent.get('action')}, complexity={intent.get('complexity')}")
            return intent
        except json.JSONDecodeError:
            logger.warning(f"Failed to parse LLM response as JSON, using fallback")
            raise ValueError("Invalid LLM response format")
    
    async def _decompose_task(
        self,
        intent: dict,
        tenant_id: str,
    ) -> list[SubTask]:
        """将意图拆解为子任务
        
        策略:
        1. 如果使用 LLM (fallback=False),直接使用推荐的分解
        2. 否则降级到基于动作的规则分解
        """
        action = intent.get("action", "")
        keywords = intent.get("keywords", [])
        complexity = intent.get("complexity", "simple")
        fallback = intent.get("fallback", True)
        
        # 如果是简单任务,直接单任务处理
        if complexity == "simple":
            return [
                SubTask(
                    subtask_id=f"sub_0_default",
                    description=user_input if 'user_input' in locals() else intent.get("description", ""),
                    capability_id="",
                    tags=keywords,
                )
            ]
        
        if fallback:
            # 降级到基于动作的分解
            return await self._rule_based_decomposition(action, keywords)
        
        # LLM 提供了推荐的工作台,可以在这里做更细粒度的分解
        recommended_workstations = intent.get("recommended_workstations", [])
        
        # TODO: 根据 LLM 推荐的站台做高级编排
        # 简化版: 仍然使用动作判断,但可以考虑站台偏好
        return await self._rule_based_decomposition(action, keywords)
    
    async def _rule_based_decomposition(
        self,
        action: str,
        keywords: list[str],
    ) -> list[SubTask]:
        """基于动作的任务分解 (降级策略)"""
        user_input = intent.get("description", "") if isinstance(intent, dict) else ""
        
        # 基于动作拆解
        if action == "analyze":
            return [
                SubTask(
                    subtask_id=f"sub_0_read",
                    description="读取文件内容",
                    capability_id="skill:read_file",
                    tags=keywords,
                ),
                SubTask(
                    subtask_id=f"sub_1_analyze",
                    description="分析内容并提取关键信息",
                    capability_id="agent:analyzer",  # 需要注册
                    dependencies=["sub_0_read"],
                    tags=keywords,
                ),
            ]
        
        elif action == "generate_code":
            return [
                SubTask(
                    subtask_id=f"sub_0_parse",
                    description="解析需求",
                    capability_id="agent:requirement_parser",
                    tags=keywords,
                ),
                SubTask(
                    subtask_id=f"sub_1_write",
                    description="编写代码",
                    capability_id="skill:execute_python",
                    dependencies=["sub_0_parse"],
                    tags=keywords,
                ),
            ]
        
        else:
            # 默认: 单任务
            return [
                SubTask(
                    subtask_id=f"sub_0_default",
                    description=user_input,
                    capability_id="",  # 待匹配
                    tags=keywords,
                )
            ]
    
    async def _rule_based_decomposition(
        self,
        action: str,
        keywords: list[str],
    ) -> list[SubTask]:
        """基于动作的任务分解 (降级策略)"""
        # 找到 user_input 从上下文中
        user_input = "Execute task"
        
        # 基于动作拆解
        if action == "analyze":
            return [
                SubTask(
                    subtask_id=f"sub_0_read",
                    description="读取文件内容",
                    capability_id="skill:read_file",
                    tags=keywords,
                ),
                SubTask(
                    subtask_id=f"sub_1_analyze",
                    description="分析内容并提取关键信息",
                    capability_id="agent:analyzer",  # 需要注册
                    dependencies=["sub_0_read"],
                    tags=keywords,
                ),
            ]
        
        elif action == "generate_code":
            return [
                SubTask(
                    subtask_id=f"sub_0_parse",
                    description="解析需求",
                    capability_id="agent:requirement_parser",
                    tags=keywords,
                ),
                SubTask(
                    subtask_id=f"sub_1_write",
                    description="编写代码",
                    capability_id="skill:execute_python",
                    dependencies=["sub_0_parse"],
                    tags=keywords,
                ),
            ]
        
        elif action == "search":
            return [
                SubTask(
                    subtask_id=f"sub_0_search",
                    description="搜索相关信息",
                    capability_id="knowledge:kb_search",
                    tags=keywords,
                ),
            ]
        
        else:
            # 默认: 单任务
            return [
                SubTask(
                    subtask_id=f"sub_0_default",
                    description=user_input,
                    capability_id="",  # 待匹配
                    tags=keywords,
                )
            ]
    
    async def _match_capabilities(
        self,
        subtasks: list[SubTask],
        tenant_id: str,
    ) -> list[SubTask]:
        """为每个子任务匹配能力"""
        matched = []
        
        for subtask in subtasks:
            if subtask.capability_id:
                # 精确匹配
                cap = await self.registry.get_by_id(subtask.capability_id, tenant_id)
                if cap:
                    subtask.capability_id = cap.capability_id
                    subtask.workstation_type = cap.workstation_type
                    matched.append(subtask)
                    continue
            
            # 语义搜索匹配
            query = subtask.description + " " + " ".join(subtask.tags)
            caps = await self.registry.search(
                query=query,
                tenant_id=tenant_id,
                limit=3,
            )
            
            if caps:
                best_cap = caps[0]
                subtask.capability_id = best_cap.capability_id
                subtask.workstation_type = best_cap.workstation_type
                matched.append(subtask)
                logger.info(
                    "Matched capability for subtask %s: %s",
                    subtask.subtask_id, best_cap.name,
                )
            else:
                logger.warning(f"No capability matched for subtask: {subtask.description}")
        
        return matched
    
    def _topological_sort(self, tasks: list[SubTask]) -> list[SubTask]:
        """拓扑排序 (基于依赖关系)"""
        task_map = {t.subtask_id: t for t in tasks}
        in_degree: dict[str, int] = {t.subtask_id: 0 for t in tasks}
        adj: dict[str, list[str]] = {}
        
        for t in tasks:
            adj[t.subtask_id] = []
            for dep in t.dependencies:
                if dep in task_map:
                    adj[dep].append(t.subtask_id)
                    in_degree[t.subtask_id] += 1
        
        # BFS
        queue = [tid for tid, deg in in_degree.items() if deg == 0]
        sorted_tasks = []
        
        while queue:
            current = queue.pop(0)
            sorted_tasks.append(task_map[current])
            
            for neighbor in adj[current]:
                in_degree[neighbor] -= 1
                if in_degree[neighbor] == 0:
                    queue.append(neighbor)
        
        return sorted_tasks
    
    async def _execute_tasks(
        self,
        tasks: list[SubTask],
        tenant_id: str,
        trace_id: str = "",
    ) -> list[ExecutedTask]:
        """执行所有子任务 (有依赖的按序执行,无依赖可并行)"""
        import asyncio
        
        results = []
        completed_tasks: dict[str, ExecutedTask] = {}
        
        # 分组: 按依赖层级
        layers = self._group_by_dependencies(tasks)
        
        for layer_idx, layer in enumerate(layers):
            logger.info(f"Executing layer {layer_idx + 1}/{len(layers)}: {[t.subtask_id for t in layer]}")
            
            # 并行执行同一层级的任务
            coroutines = [
                self._execute_single_task(task, tenant_id, trace_id, completed_tasks)
                for task in layer
            ]
            
            layer_results = await asyncio.gather(*coroutines, return_exceptions=True)
            
            for result in layer_results:
                if isinstance(result, Exception):
                    logger.error(f"Task execution failed: {result}")
                elif isinstance(result, ExecutedTask):
                    completed_tasks[result.subtask_id] = result
                    results.append(result)
        
        return results
    
    def _group_by_dependencies(self, tasks: list[SubTask]) -> list[list[SubTask]]:
        """按依赖关系分组 (同一组内可并行)"""
        task_map = {t.subtask_id: t for t in tasks}
        grouped = []
        resolved = set()
        
        while len(resolved) < len(tasks):
            # 找出所有依赖都已 resolved 的任务
            layer = []
            for t in tasks:
                if t.subtask_id not in resolved:
                    if all(dep in resolved for dep in t.dependencies):
                        layer.append(t)
            
            if not layer:
                # 循环依赖,全部加入
                layer = [t for t in tasks if t not in resolved]
            
            grouped.append(layer)
            for t in layer:
                resolved.add(t.subtask_id)
        
        return grouped
    
    async def _execute_single_task(
        self,
        task: SubTask,
        tenant_id: str,
        trace_id: str,
        completed_tasks: dict[str, ExecutedTask],
    ) -> ExecutedTask:
        """执行单个子任务"""
        cap = await self.registry.get_by_id(task.capability_id, tenant_id)
        if not cap:
            return ExecutedTask(
                task_id="",
                subtask_id=task.subtask_id,
                capability_id=task.capability_id,
                input_params=task.parameters,
                error=f"Capability not found: {task.capability_id}",
                status="failed",
            )
        
        executor = cap._executor
        if not executor:
            return ExecutedTask(
                task_id="",
                subtask_id=task.subtask_id,
                capability_id=task.capability_id,
                input_params=task.parameters,
                error=f"No executor registered for: {task.capability_id}",
                status="failed",
            )
        
        # 注入依赖任务的输出
        input_params = task.parameters.copy()
        for dep_id in task.dependencies:
            if dep_id in completed_tasks and completed_tasks[dep_id].output:
                input_params["$previous_output"] = completed_tasks[dep_id].output
        
        # 执行
        start_time = time.time()
        try:
            if asyncio.iscoroutinefunction(executor):
                output = await executor(**input_params)
            else:
                output = executor(**input_params)
            
            duration_ms = int((time.time() - start_time) * 1000)
            
            # 记录 span
            from app.trace import record_span
            await record_span(
                trace_id=trace_id,
                span_name=f"task:{cap.name}",
                duration_ms=duration_ms,
                metadata={
                    "subtask_id": task.subtask_id,
                    "workstation": cap.workstation_type.value,
                    "tenant_id": tenant_id,
                },
                tenant_id=tenant_id,
            )
            
            # 更新能力统计
            cap.record_call(duration_ms, success=True)
            
            return ExecutedTask(
                task_id="",
                subtask_id=task.subtask_id,
                capability_id=cap.capability_id,
                input_params=input_params,
                output=output,
                duration_ms=duration_ms,
                status="completed",
            )
            
        except Exception as e:
            duration_ms = int((time.time() - start_time) * 1000)
            
            cap.record_call(duration_ms, success=False)
            
            return ExecutedTask(
                task_id="",
                subtask_id=task.subtask_id,
                capability_id=cap.capability_id,
                input_params=input_params,
                error=str(e),
                duration_ms=duration_ms,
                status="failed",
            )
    
    async def _aggregate_results(
        self,
        results: list[ExecutedTask],
        intent: dict,
        original_input: str,
    ) -> dict[str, Any]:
        """聚合子任务输出"""
        # TODO: 根据意图类型定制聚合逻辑
        return {
            "subtasks_completed": sum(1 for r in results if r.status == "completed"),
            "subtasks_failed": sum(1 for r in results if r.status == "failed"),
            "outputs": [
                {
                    "subtask_id": r.subtask_id,
                    "capability_id": r.capability_id,
                    "output": r.output if r.status == "completed" else None,
                    "error": r.error if r.status == "failed" else None,
                }
                for r in results
            ],
        }
    
    def _extract_keywords(self, text: str) -> list[str]:
        """提取关键词 (简化版)"""
        import re
        words = re.findall(r'[a-zA-Z0-9]+', text.lower())
        return [w for w in words if len(w) > 2]
    
    def _extract_entities(self, text: str) -> dict:
        """提取实体 (简化版)"""
        # TODO: 使用 NER 模型
        return {}
    
    def _infer_action(self, text: str) -> str:
        """推断动作"""
        text_lower = text.lower()
        if "分析" in text_lower or "analyze" in text_lower:
            return "analyze"
        elif "生成" in text_lower or "generate" in text_lower or "写代码" in text_lower:
            return "generate_code"
        elif "搜索" in text_lower or "search" in text_lower:
            return "search"
        else:
            return "general"


# ── 便捷函数 ────────────────────────────────────────────────────────
async def quick_execute(
    user_input: str,
    tenant_id: str,
    trace_id: str = "",
) -> dict[str, Any]:
    """快捷执行 API (Go 侧代理)"""
    router = TaskRouter()
    return await router.route_task(
        user_input=user_input,
        tenant_id=tenant_id,
        trace_id=trace_id,
    )
