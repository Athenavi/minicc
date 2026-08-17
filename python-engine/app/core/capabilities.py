"""统一能力注册中心 (Capabilities Registry)

设计目标:
- 所有工作台(对话/Agent/工作流/技能/知识库/插件)统一注册自身能力
- 提供基于语义的能力搜索与发现
- 支持能力组合与链式调用
- SaaS 租户隔离: 每个租户只能使用自己的能力

架构思想:
类比 Docker 的 Registry + Kubernetes 的 Service Discovery
- 工作台启动时注册 capability
- 运行时查询可用能力
- TaskRouter 根据能力动态编排执行路径
"""
from __future__ import annotations

import json
import logging
import time
from typing import Any, Optional, Callable
from dataclasses import dataclass, field
from enum import Enum
from pathlib import PurePosixPath

logger = logging.getLogger(__name__)


class WorkstationType(str, Enum):
    """工作台类型"""
    DIALOGUE = "dialogue"        # 对话工作台 (入口)
    AGENT = "agent"              # Agent 工作台 (智能体)
    WORKFLOW = "workflow"        # 工作流工作台 (DAG 编排)
    SKILL = "skill"             # 技能工作台 (工具执行)
    KNOWLEDGE = "knowledge"     # 知识库工作台 (RAG)
    PLUGIN = "plugin"           # 插件工作台 (扩展)


class CapabilityType(str, Enum):
    """能力类型"""
    TOOL = "tool"                   # 工具型 (如 execute_python)
    SERVICE = "service"            # 服务型 (如 kb_search)
    TEMPLATE = "template"          # 模板型 (如 prompt_template)
    COMPOSITE = "composite"        # 组合型 (多个能力组合)


@dataclass
class CapabilityParam:
    """能力参数定义 (JSON Schema 简化版)"""
    name: str
    type: str  # "string" / "number" / "boolean" / "object" / "array"
    description: str
    required: bool = True
    default: Any = None


@dataclass
class CapabilityResult:
    """能力返回结果定义"""
    fields: list[str]
    format: str = "json"  # "json" / "text" / "binary"


@dataclass
class Capability:
    """统一能力定义
    
    所有工作台的能力都统一为此格式
    包括: 工具、服务、模板、组合能力
    """
    capability_id: str  # 唯一标识,格式: "{workstation}:{name}"
    name: str
    description: str
    workstation_type: WorkstationType
    capability_type: CapabilityType
    
    # 输入输出契约
    input_schema: list[CapabilityParam] = field(default_factory=list)
    output_schema: Optional[CapabilityResult] = None
    
    # 元数据
    version: str = "1.0.0"
    tags: list[str] = field(default_factory=list)  # 用于语义搜索
    author: str = ""
    status: str = "active"  # "active" / "inactive" / "deprecated"
    
    # 租户隔离
    tenant_id: str = ""  # 空字符串表示全局能力
    
    # 性能指标
    avg_latency_ms: int = 0
    success_rate: float = 1.0
    
    # 实际执行函数 (Python 侧)
    _executor: Optional[Callable] = field(default=None, repr=False)
    
    # 组合能力的子能力列表
    sub_capabilities: list[str] = field(default_factory=list)
    
    # 执行耗时统计
    call_count: int = 0
    total_duration_ms: int = 0
    
    @property
    def avg_duration_ms(self) -> int:
        if self.call_count == 0:
            return self.avg_latency_ms
        return self.total_duration_ms // max(self.call_count, 1)
    
    def record_call(self, duration_ms: int, success: bool = True):
        """记录一次调用"""
        self.call_count += 1
        self.total_duration_ms += duration_ms
        # TODO: 更新 success_rate
        

class CapabilitiesRegistry:
    """统一能力注册中心
    
    功能:
    1. 能力注册/注销/更新
    2. 基于关键词/标签的能力搜索
    3. 能力依赖分析 (组合型能力)
    4. 租户隔离 (自动过滤)
    
    使用示例:
        registry = CapabilitiesRegistry()
        
        # 注册能力
        await registry.register(Capability(
            capability_id="skill:execute_python",
            name="Execute Python Code",
            description="在沙箱中执行 Python 代码并返回结果",
            workstation_type=WorkstationType.SKILL,
            capability_type=CapabilityType.TOOL,
            input_schema=[
                CapabilityParam("code", "string", "Python 代码", required=True),
                CapabilityParam("timeout", "number", "执行超时(秒)", required=False, default=30),
            ],
            tags=["python", "sandbox", "execution"],
            tenant_id="tenant_001",
            _executor=execute_python_executor,
        ))
        
        # 搜索能力
        results = await registry.search("执行 Python 代码")
        
        # 获取能力详情
        cap = await registry.get_by_id("skill:execute_python")
    """
    
    def __init__(self):
        self._capabilities: dict[str, Capability] = {}  # capability_id -> Capability
        self._index_by_tags: dict[str, list[str]] = {}  # tag -> [capability_id]
        self._index_by_description: list[tuple[str, str]] = []  # [(capability_id, desc)]
    
    async def register(self, capability: Capability) -> None:
        """注册能力"""
        # 生成完整 ID (带租户前缀)
        full_id = f"{capability.tenant_id}:{capability.capability_id}" if capability.tenant_id else capability.capability_id
        
        # 检查 ID 冲突
        if full_id in self._capabilities:
            existing = self._capabilities[full_id]
            if existing.version != capability.version:
                logger.warning(
                    "Capability version conflict: %s v%s vs v%s",
                    full_id, existing.version, capability.version,
                )
            # 更新
            capability._executor = existing._executor  # 保留执行函数
            self._capabilities[full_id] = capability
            logger.info("Capability updated: %s v%s", full_id, capability.version)
        else:
            self._capabilities[full_id] = capability
            logger.info("Capability registered: %s v%s (%s)", full_id, capability.version, 
                       capability.workstation_type.value)
        
        # 更新标签索引
        for tag in capability.tags:
            self._index_by_tags.setdefault(tag, []).append(full_id)
        
        # 更新描述索引 (简单分词)
        desc_words = self._tokenize(capability.description + " " + " ".join(capability.tags))
        for word in desc_words:
            self._index_by_description.append((word, full_id))
    
    async def unregister(self, capability_id: str, tenant_id: str = "") -> bool:
        """注销能力"""
        full_id = f"{tenant_id}:{capability_id}" if tenant_id else capability_id
        if full_id in self._capabilities:
            del self._capabilities[full_id]
            logger.info("Capability unregistered: %s", full_id)
            return True
        return False
    
    async def get_by_id(self, capability_id: str, tenant_id: str = "") -> Optional[Capability]:
        """根据 ID 获取能力"""
        full_id = f"{tenant_id}:{capability_id}" if tenant_id else capability_id
        return self._capabilities.get(full_id)
    
    async def list_by_workstation(
        self,
        workstation_type: WorkstationType,
        tenant_id: str = "",
        status: Optional[str] = None,
    ) -> list[Capability]:
        """列出某工作台的所有能力"""
        results = []
        for cap in self._capabilities.values():
            if cap.workstation_type != workstation_type:
                continue
            if tenant_id and cap.tenant_id != tenant_id:
                continue
            if status and cap.status != status:
                continue
            results.append(cap)
        return results
    
    async def search(
        self,
        query: str,
        tenant_id: str = "",
        workstation_type: Optional[WorkstationType] = None,
        capability_type: Optional[CapabilityType] = None,
        limit: int = 10,
    ) -> list[Capability]:
        """搜索能力 (基于关键词 + 标签 + 描述)
        
        类似搜索引擎,根据相关性打分排序
        """
        query_words = self._tokenize(query)
        scores: dict[str, float] = {}
        
        for cap_id, cap in self._capabilities.items():
            # 租户隔离
            if tenant_id and cap.tenant_id != tenant_id:
                continue
            
            # 工作台类型过滤
            if workstation_type and cap.workstation_type != workstation_type:
                continue
            
            # 能力类型过滤
            if capability_type and cap.capability_type != capability_type:
                continue
            
            score = 0
            
            # 精确匹配 capability_id
            if query.lower() in cap.capability_id.lower():
                score += 10
            
            # 标签匹配
            for tag in cap.tags:
                if tag in query_words:
                    score += 5
            
            # 描述匹配
            for word in query_words:
                if word in cap.description.lower():
                    score += 3
            
            # 名称匹配
            for word in query_words:
                if word in cap.name.lower():
                    score += 7
            
            if score > 0:
                scores[cap_id] = score
        
        # 按分数排序
        sorted_ids = sorted(scores.keys(), key=lambda x: scores[x], reverse=True)[:limit]
        return [self._capabilities[cid] for cid in sorted_ids if cid in self._capabilities]
    
    async def find_best_match(
        self,
        intent: str,
        tenant_id: str,
        available_workstations: list[WorkstationType] = None,
    ) -> Optional[Capability]:
        """找到最匹配意图的能力
        
        用于 TaskRouter 自动编排
        """
        caps = await self.search(
            query=intent,
            tenant_id=tenant_id,
            workstation_type=None,  # 搜索所有工作台
            limit=5,
        )
        
        if not caps:
            return None
        
        # 返回分数最高的
        best = caps[0]
        
        # 如果有可用工作台限制,优先选择在工作台上执行
        if available_workstations:
            for cap in caps:
                if cap.workstation_type in available_workstations:
                    return cap
        
        return best
    
    async def resolve_dependencies(self, capability_id: str, tenant_id: str = "") -> list[Capability]:
        """解析组合能力的依赖链"""
        cap = await self.get_by_id(capability_id, tenant_id)
        if not cap or cap.capability_type != CapabilityType.COMPOSITE:
            return [cap] if cap else []
        
        # BFS 遍历子能力
        resolved = []
        queue = list(cap.sub_capabilities)
        visited = set()
        
        while queue:
            sub_id = queue.pop(0)
            if sub_id in visited:
                continue
            visited.add(sub_id)
            
            sub_cap = await self.get_by_id(sub_id, tenant_id)
            if sub_cap:
                resolved.append(sub_cap)
                if sub_cap.sub_capabilities:
                    queue.extend(sub_cap.sub_capabilities)
        
        return resolved
    
    def _tokenize(self, text: str) -> list[str]:
        """简单分词器 (中英文通用)"""
        import re
        # 提取所有单词和中文词语 (简化版)
        words = re.findall(r'[a-zA-Z0-9]+|[\u4e00-\u9fff]+', text.lower())
        return [w for w in words if len(w) > 1]


# ── 全局单例 ───────────────────────────────────────────────────────
_global_registry: Optional[CapabilitiesRegistry] = None


def get_registry() -> CapabilitiesRegistry:
    """获取全局能力注册中心 (单例模式)"""
    global _global_registry
    if _global_registry is None:
        _global_registry = CapabilitiesRegistry()
    return _global_registry


# ── 预注册标准能力 ─────────────────────────────────────────────────
async def preload_default_capabilities(registry: CapabilitiesRegistry = None):
    """预加载默认能力 (系统启动时执行)"""
    reg = registry or get_registry()
    
    # 1. 文件读取技能 (Skill 工作台)
    from app.tools.file import file_read
    await reg.register(Capability(
        capability_id="skill:read_file",
        name="Read File Content",
        description="读取文件内容并返回文本",
        workstation_type=WorkstationType.SKILL,
        capability_type=CapabilityType.TOOL,
        input_schema=[
            CapabilityParam("path", "string", "文件路径", required=True),
            CapabilityParam("max_chars", "number", "最大字符数", required=False, default=10000),
        ],
        output_schema=CapabilityResult(fields=["content", "encoding", "size_bytes"]),
        tags=["file", "read", "io"],
        _executor=file_read,
    ))
    
    # 2. Python 执行技能 (Skill 工作台)
    from app.tools.python import python_run
    await reg.register(Capability(
        capability_id="skill:execute_python",
        name="Execute Python Code",
        description="在沙箱环境中执行 Python 代码并返回输出",
        workstation_type=WorkstationType.SKILL,
        capability_type=CapabilityType.TOOL,
        input_schema=[
            CapabilityParam("code", "string", "Python 代码", required=True),
            CapabilityParam("timeout", "number", "执行超时(秒)", required=False, default=30),
            CapabilityParam("workspace", "string", "工作目录", required=False, default="/tmp"),
        ],
        output_schema=CapabilityResult(fields=["output", "error", "exit_code"]),
        tags=["python", "sandbox", "execution", "coding"],
        _executor=python_run,
    ))
    
    # 3. 知识库搜索服务 (Knowledge 工作台)
    from app.tools.kb import kb_search
    await reg.register(Capability(
        capability_id="knowledge:kb_search",
        name="Search Knowledge Base",
        description="在知识库中检索相关文档片段",
        workstation_type=WorkstationType.KNOWLEDGE,
        capability_type=CapabilityType.SERVICE,
        input_schema=[
            CapabilityParam("query", "string", "查询关键词", required=True),
            CapabilityParam("top_k", "number", "返回结果数", required=False, default=5),
            CapabilityParam("threshold", "number", "相似度阈值", required=False, default=0.7),
        ],
        output_schema=CapabilityResult(fields=["results", "count"]),
        tags=["knowledge", "rag", "search", "vector"],
        _executor=kb_search,
    ))
    
    # 4. LLM 对话服务 (Agent 工作台)
    from app.gateway.router import GatewayRouter
    # 注意: LLM 的执行函数需要注入 gateway,此处暂不注册
    
    logger.info(f"Preloaded {len([c for c in reg._capabilities.values()])} default capabilities")


# ── Go 侧 API 路由示例 ─────────────────────────────────────────────
"""
Go gateway_router.go 中的路由:

// 能力注册表 API (供前端查询可用车)
mux.Handle("GET /v1/capabilities", authMW(http.HandlerFunc(capabilityListHandler)))
mux.Handle("POST /v1/capabilities/search", authMW(http.HandlerFunc(capabilitySearchHandler)))
mux.Handle("GET /v1/capabilities/{id}", authMW(http.HandlerFunc(capabilityGetHandler)))

// 快捷执行 API (前端发送自然语言,系统自动匹配能力执行)
mux.Handle("POST /v1/quick-execute", authMW(http.HandlerFunc(quickExecuteHandler)))
"""
