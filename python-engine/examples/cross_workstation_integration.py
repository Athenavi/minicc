"""端到端集成示例: 对话 → Agent → 工作流 → 技能 → 知识库 → 返回结果

这是一个完整的使用场景演示,展示六大工作台如何协同工作。

场景: 用户在对话框输入 "帮我分析 workspace/gantt-chart.md 文件,提取任务依赖关系,生成 Mermaid 甘特图代码"

执行流程:
1. 对话工作台接收请求
2. Agent Hub 拆解为 3 个子任务:
   a. 读取文件 (Skill: read_file)
   b. 分析内容 (Agent: Analyzer + Knowledge Base 检索类似案例)
   c. 生成代码 (Skill: execute_python)
3. 工作流编排执行顺序 (DAG: a → b → c)
4. 聚合结果并返回
"""

from __future__ import annotations

import json
import asyncio
from typing import Any


# ── 模拟能力执行函数 ────────────────────────────────────────────────
async def mock_read_file(path: str, max_chars: int = 10000) -> dict:
    """模拟读取文件"""
    return {
        "content": f"""# 项目甘特图

## 阶段 1: 需求分析 (第1-2周)
- 用户需求调研
- 竞品分析
- 技术可行性评估

## 阶段 2: 系统设计 (第3-4周)  
- 架构设计
- 数据库建模
- API 接口定义

## 阶段 3: 开发实现 (第5-8周)
- 前端开发 (依赖: 阶段 2)
- 后端开发 (依赖: 阶段 2)
- 集成测试

## 阶段 4: 部署上线 (第9-10周)
- UAT 测试
- 生产部署
- 监控配置""",
        "encoding": "utf-8",
        "size_bytes": 1024,
    }


async def mock_kb_search(query: str, top_k: int = 5) -> dict:
    """模拟知识库搜索"""
    return {
        "results": [
            {
                "document_id": "kb_mermaid_best_practices",
                "content": "Mermaid 甘特图语法示例:\ngantt\n    title 项目计划\n    section 阶段1\n    需求分析 :2024-01-01, 14d",
                "score": 0.92,
            },
            {
                "document_id": "kb_task_dependency_patterns", 
                "content": "任务依赖关系提取模式:使用 '依赖:' 标注后续任务的依赖前置",
                "score": 0.87,
            },
        ],
        "count": 2,
    }


async def mock_execute_python(code: str, timeout: int = 30) -> dict:
    """模拟 Python 代码执行"""
    # 模拟代码执行结果
    mermaid_output = """gantt
    title 项目开发计划
    dateFormat  YYYY-MM-DD
    section 需求分析
    用户需求调研       :a1, 2024-01-01, 7d
    竞品分析          :a2, after a1, 5d
    技术评估          :a3, after a2, 2d
    
    section 系统设计
    架构设计          :b1, after a3, 5d
    数据库建模        :b2, after b1, 3d
    API 定义         :b3, after b1, 4d
    
    section 开发实现
    前端开发          :c1, after b2, 14d
    后端开发          :c2, after b3, 14d
    集成测试         :c3, after c1, 7d"""
    
    return {
        "output": mermaid_output,
        "exit_code": 0,
        "error": "",
    }


# ── 完整的跨工作台协同流程 ──────────────────────────────────────────
async def run_end_to_end_example():
    """运行端到端示例"""
    print("=" * 80)
    print("跨工作台深度整合 - 端到端示例")
    print("=" * 80)
    
    from app.core.capabilities import get_registry, preload_default_capabilities
    from app.core.task_router import TaskRouter, TaskPriority
    
    tenant_id = "tenant_demo"
    trace_id = "e2e_demo_001"
    
    # ── 步骤 1: 注册所有工作台的能力 ──────────────────────────────
    print("\n[步骤 1] 注册工作台能力...")
    
    registry = get_registry()
    await preload_default_capabilities(registry)
    
    # 注册额外的能力
    from app.core.capabilities import Capability, CapabilityParam, CapabilityResult, WorkstationType, CapabilityType
    
    # 知识库搜索能力
    await registry.register(Capability(
        capability_id="knowledge:kb_search",
        name="Search Knowledge Base",
        description="在知识库中检索相关文档和最佳实践",
        workstation_type=WorkstationType.KNOWLEDGE,
        capability_type=CapabilityType.SERVICE,
        input_schema=[
            CapabilityParam("query", "string", "查询关键词", required=True),
            CapabilityParam("top_k", "number", "返回结果数", required=False, default=5),
        ],
        output_schema=CapabilityResult(fields=["results", "count"]),
        tags=["knowledge", "rag", "search"],
        tenant_id=tenant_id,
        _executor=lambda **kwargs: asyncio.create_task(mock_kb_search(**kwargs)),
    ))
    
    print(f"✓ 已注册 {len(registry._capabilities)} 个能力")
    
    # ── 步骤 2: 用户输入 (对话工作台) ─────────────────────────────
    print("\n[步骤 2] 用户输入:")
    user_input = "帮我分析 workspace/gantt-chart.md 文件,提取任务依赖关系,生成 Mermaid 甘特图代码"
    print(f"  User: {user_input}")
    
    # ── 步骤 3: TaskRouter 自动编排 ──────────────────────────────
    print("\n[步骤 3] TaskRouter 开始编排...")
    
    router = TaskRouter()
    
    # 执行完整流程
    result = await router.route_task(
        user_input=user_input,
        tenant_id=tenant_id,
        priority=TaskPriority.HIGH,
        trace_id=trace_id,
    )
    
    # ── 步骤 4: 输出执行结果 ─────────────────────────────────────
    print("\n[步骤 4] 执行结果:")
    print(f"  任务 ID: {result['task_id']}")
    print(f"  状态: {result['status']}")
    print(f"  总耗时: {result['total_duration_ms']}ms")
    print(f"  子任务数: {len(result['subtasks'])}")
    
    print("\n  子任务执行详情:")
    for subtask in result['subtasks']:
        status_icon = "✅" if subtask['status'] == "completed" else "❌"
        print(f"    {status_icon} [{subtask['capability_id']}] {subtask['duration_ms']}ms")
    
    # ── 步骤 5: 最终输出 ─────────────────────────────────────────
    print("\n[步骤 5] 最终输出 (Mermaid 甘特图):")
    outputs = result.get('output', {}).get('outputs', [])
    for output_item in outputs:
        if output_item.get('output'):
            print(f"\n```{json.dumps(output_item['output'], ensure_ascii=False)[:200]}...")
    
    return result


# ── 模拟对话工作台的完整处理流程 ──────────────────────────────────────
class MockDialogueHandler:
    """模拟对话工作台处理器"""
    
    async def handle_user_message(self, message: str, tenant_id: str) -> dict:
        """处理用户消息"""
        from app.core.task_router import quick_execute
        
        # 调用 TaskRouter 自动编排
        result = await quick_execute(
            user_input=message,
            tenant_id=tenant_id,
            trace_id=f"dialogue_{int(__import__('time').time())}",
        )
        
        # 格式化返回给用户
        if result['status'] == 'completed':
            outputs = result['output'].get('outputs', [])
            final_answer = self._extract_final_answer(outputs)
            
            return {
                "reply": final_answer,
                "trace_id": result['trace_id'],
                "metadata": {
                    "subtasks_completed": result['output']['subtasks_completed'],
                    "duration_ms": result['total_duration_ms'],
                }
            }
        else:
            return {
                "reply": f"抱歉,执行失败: {result['output'].get('error', '未知错误')}",
                "trace_id": result.get('trace_id', ''),
            }
    
    def _extract_final_answer(self, outputs: list) -> str:
        """从输出中提取最终答案"""
        # 查找最后一个成功的输出 (通常是生成结果)
        for output in reversed(outputs):
            if output.get('output'):
                return output['output']
        return "未生成有效输出"


# ── 运行入口 ────────────────────────────────────────────────────────
if __name__ == "__main__":
    import sys
    
    print("跨工作台深度整合 - 端到端集成测试")
    print("=" * 80)
    
    async def main():
        # 1. 运行完整示例
        await run_end_to_end_example()
        
        # 2. 模拟对话交互
        print("\n" + "=" * 80)
        print("模拟对话工作台交互")
        print("=" * 80)
        
        dialogue = MockDialogueHandler()
        result = await dialogue.handle_user_message(
            message="帮我分析 gantt-chart.md 并生成甘特图代码",
            tenant_id="tenant_demo",
        )
        
        print(f"\n用户: 帮我分析 gantt-chart.md 并生成甘特图代码")
        print(f"\n系统: {result['reply'][:200]}...")
        print(f"\n元数据: {result['metadata']}")
    
    asyncio.run(main())
