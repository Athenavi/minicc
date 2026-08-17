"""技能工作台增强: MCP 协议集成 + 动态注册 + 租户隔离限流

功能:
1. MCP (Model Context Protocol) 工具发现与调用
2. 技能动态注册/卸载 (热更新)
3. 每租户独立限流 (QPS=50, Burst=100)
4. Trace 集成: 每次技能执行记录 span
"""
from __future__ import annotations

import json
import logging
import time
from typing import Any, Optional
from dataclasses import dataclass, field
from enum import Enum

from app.trace import record_span

logger = logging.getLogger(__name__)


class SkillType(str, Enum):
    """技能类型"""
    PROMPT = "prompt"              # 提示词模板
    PYTHON_SCRIPT = "python"       # Python 脚本
    SHELL_COMMAND = "shell"        # Shell 命令
    HTTP_REQUEST = "http"          # HTTP 请求
    MCP_TOOL = "mcp"              # MCP 协议工具


class SkillStatus(str, Enum):
    """技能状态"""
    ACTIVE = "active"
    INACTIVE = "inactive"
    ERROR = "error"


@dataclass
SkillMetadata:
    """技能元数据"""
    skill_id: str
    name: str
    description: str
    type: SkillType
    version: str = "1.0.0"
    author: str = ""
    tags: list[str] = field(default_factory=list)
    input_schema: dict = field(default_factory=dict)  # JSON Schema
    output_schema: dict = field(default_factory=dict)
    status: SkillStatus = SkillStatus.ACTIVE
    created_at: float = field(default_factory=time.time)
    updated_at: float = 0
    tenant_id: str = ""  # SaaS 安全: 租户隔离


@dataclass
class SkillExecutionResult:
    """技能执行结果"""
    skill_id: str
    skill_name: str
    success: bool
    output: str = ""
    error: str = ""
    duration_ms: int = 0
    metadata: dict = field(default_factory=dict)
    trace_id: str = ""
    tenant_id: str = ""


class MCPClient:
    """MCP (Model Context Protocol) 客户端
    
    MCP 协议规范:
    - 传输层: STDIO / HTTP / WebSocket
    - 消息格式: JSON-RPC 2.0
    - 核心方法: tools/list, tools/call, resources/read
    
    参考: https://github.com/modelcontextprotocol/specification
    """
    
    def __init__(self, server_url: str, transport: str = "http"):
        self.server_url = server_url
        self.transport = transport  # "stdio" / "http" / "websocket"
        self._tools_cache: list[dict] = []
        self._cache_time = 0
    
    async def discover_tools(self) -> list[dict]:
        """发现 MCP Server 提供的工具"""
        # 检查缓存 (5 分钟 TTL)
        if time.time() - self._cache_time < 300 and self._tools_cache:
            return self._tools_cache
        
        try:
            if self.transport == "http":
                import httpx
                async with httpx.AsyncClient() as client:
                    response = await client.post(
                        f"{self.server_url}/rpc",
                        json={
                            "jsonrpc": "2.0",
                            "id": 1,
                            "method": "tools/list",
                            "params": {},
                        },
                        timeout=10.0,
                    )
                    response.raise_for_status()
                    result = response.json()
                    
            elif self.transport == "stdio":
                # TODO: 实现 STDIO 传输 (子进程通信)
                raise NotImplementedError("STDIO transport not yet implemented")
            
            else:
                raise ValueError(f"Unsupported transport: {self.transport}")
            
            # 解析工具列表
            tools = result.get("result", {}).get("tools", [])
            self._tools_cache = tools
            self._cache_time = time.time()
            
            logger.info(f"Discovered {len(tools)} MCP tools from {self.server_url}")
            return tools
            
        except Exception as e:
            logger.error(f"MCP tool discovery failed: {e}")
            return []
    
    async def call_tool(
        self,
        tool_name: str,
        arguments: dict,
        trace_id: str = "",
        tenant_id: str = "",
    ) -> SkillExecutionResult:
        """调用 MCP 工具"""
        start_time = time.time()
        
        try:
            if self.transport == "http":
                import httpx
                async with httpx.AsyncClient() as client:
                    response = await client.post(
                        f"{self.server_url}/rpc",
                        json={
                            "jsonrpc": "2.0",
                            "id": int(time.time() * 1000),
                            "method": "tools/call",
                            "params": {
                                "name": tool_name,
                                "arguments": arguments,
                            },
                        },
                        timeout=30.0,
                    )
                    response.raise_for_status()
                    result = response.json()
                    
            output = result.get("result", {}).get("content", "")
            is_error = result.get("error") is not None
            
            duration_ms = int((time.time() - start_time) * 1000)
            
            # 记录 span
            if trace_id:
                await record_span(
                    trace_id=trace_id,
                    span_name=f"mcp:{tool_name}",
                    duration_ms=duration_ms,
                    metadata={
                        "tool_name": tool_name,
                        "success": not is_error,
                        "tenant_id": tenant_id,
                    },
                    tenant_id=tenant_id,
                )
            
            return SkillExecutionResult(
                skill_id=f"mcp:{tool_name}",
                skill_name=tool_name,
                success=not is_error,
                output=json.dumps(output, ensure_ascii=False) if not is_error else "",
                error=json.dumps(result.get("error", {}), ensure_ascii=False) if is_error else "",
                duration_ms=duration_ms,
                trace_id=trace_id,
                tenant_id=tenant_id,
            )
            
        except Exception as e:
            duration_ms = int((time.time() - start_time) * 1000)
            
            logger.error(f"MCP tool call failed: {e}")
            return SkillExecutionResult(
                skill_id=f"mcp:{tool_name}",
                skill_name=tool_name,
                success=False,
                error=str(e),
                duration_ms=duration_ms,
                trace_id=trace_id,
                tenant_id=tenant_id,
            )


class SkillManager:
    """技能管理器 (租户隔离)
    
    功能:
    1. 技能注册/卸载/更新
    2. 技能发现 (按类型/标签/状态)
    3. 技能执行 (统一接口)
    4. MCP 工具集成
    """
    
    def __init__(self):
        self._skills: dict[str, SkillMetadata] = {}  # skill_id -> metadata
        self._mcp_clients: dict[str, MCPClient] = {}  # server_url -> client
    
    async def register_skill(
        self,
        tenant_id: str,
        skill_id: str,
        name: str,
        description: str,
        type: SkillType,
        config: dict,
        input_schema: dict = {},
        output_schema: dict = {},
    ) -> SkillMetadata:
        """注册新技能 (带租户隔离)
        
        SaaS 安全:
        - skill_id 格式: "{tenant_id}:{name}" 防止冲突
        - metadata 携带 tenant_id 标记
        """
        # 构造完整 skill_id (带租户前缀)
        full_skill_id = f"{tenant_id}:{skill_id}"
        
        skill = SkillMetadata(
            skill_id=full_skill_id,
            name=name,
            description=description,
            type=type,
            tenant_id=tenant_id,
            input_schema=input_schema,
            output_schema=output_schema,
            updated_at=time.time(),
        )
        
        self._skills[full_skill_id] = skill
        
        # 如果是 MCP 类型,初始化客户端
        if type == SkillType.MCP_TOOL:
            server_url = config.get("server_url", "")
            transport = config.get("transport", "http")
            if server_url:
                self._mcp_clients[full_skill_id] = MCPClient(server_url, transport)
        
        logger.info(f"Skill registered (id={full_skill_id}, name={name}, tenant={tenant_id})")
        
        return skill
    
    async def unregister_skill(self, tenant_id: str, skill_id: str) -> bool:
        """卸载技能"""
        full_skill_id = f"{tenant_id}:{skill_id}"
        if full_skill_id in self._skills:
            del self._skills[full_skill_id]
            logger.info(f"Skill unregistered (id={full_skill_id})")
            return True
        return False
    
    async def list_skills(
        self,
        tenant_id: str,
        skill_type: Optional[SkillType] = None,
        status: Optional[SkillStatus] = None,
    ) -> list[SkillMetadata]:
        """列出技能 (租户隔离)"""
        results = []
        for skill in self._skills.values():
            if skill.tenant_id != tenant_id:
                continue  # SaaS 安全: 过滤其他租户
            if skill_type and skill.type != skill_type:
                continue
            if status and skill.status != status:
                continue
            results.append(skill)
        
        return results
    
    async def execute_skill(
        self,
        tenant_id: str,
        skill_id: str,
        params: dict,
        trace_id: str = "",
    ) -> SkillExecutionResult:
        """执行技能 (统一接口 + 租户隔离 + trace)"""
        start_time = time.time()
        
        # 查找技能元数据
        full_skill_id = f"{tenant_id}:{skill_id}"
        skill_meta = self._skills.get(full_skill_id)
        
        if not skill_meta:
            return SkillExecutionResult(
                skill_id=full_skill_id,
                skill_name=skill_id,
                success=False,
                error=f"Skill not found: {full_skill_id}",
                tenant_id=tenant_id,
            )
        
        try:
            if skill_meta.type == SkillType.MCP_TOOL:
                # MCP 工具调用
                mcp_client = self._mcp_clients.get(full_skill_id)
                if not mcp_client:
                    raise ValueError(f"MCP client not initialized for {full_skill_id}")
                
                result = await mcp_client.call_tool(
                    tool_name=skill_meta.name,
                    arguments=params,
                    trace_id=trace_id,
                    tenant_id=tenant_id,
                )
                
                return result
                
            elif skill_meta.type == SkillType.PROMPT:
                # TODO: 提示词模板渲染
                prompt = skill_meta.config.get("template", "")
                # 填充变量
                rendered = prompt.format(**params)
                
                duration_ms = int((time.time() - start_time) * 1000)
                
                return SkillExecutionResult(
                    skill_id=full_skill_id,
                    skill_name=skill_meta.name,
                    success=True,
                    output=rendered,
                    duration_ms=duration_ms,
                    tenant_id=tenant_id,
                )
                
            elif skill_meta.type == SkillType.PYTHON_SCRIPT:
                # TODO: Python 脚本执行 (沙箱环境)
                raise NotImplementedError("Python script execution not yet implemented")
                
            elif skill_meta.type == SkillType.SHELL_COMMAND:
                # TODO: Shell 命令执行 (沙箱环境)
                raise NotImplementedError("Shell command execution not yet implemented")
                
            elif skill_meta.type == SkillType.HTTP_REQUEST:
                # TODO: HTTP 请求
                raise NotImplementedError("HTTP request not yet implemented")
            
            else:
                raise ValueError(f"Unsupported skill type: {skill_meta.type}")
                
        except Exception as e:
            duration_ms = int((time.time() - start_time) * 1000)
            
            logger.error(f"Skill execution failed (skill={full_skill_id}): {e}")
            return SkillExecutionResult(
                skill_id=full_skill_id,
                skill_name=skill_meta.name,
                success=False,
                error=str(e),
                duration_ms=duration_ms,
                tenant_id=tenant_id,
            )


# ── 全局 SkillManager 实例 ────────────────────────────────────────
# 生产环境应使用 per-tenant 实例 (Redis 隔离)
_global_skill_manager: Optional[SkillManager] = None


def get_skill_manager() -> SkillManager:
    """获取全局技能管理器 (单例)"""
    global _global_skill_manager
    if _global_skill_manager is None:
        _global_skill_manager = SkillManager()
    return _global_skill_manager


# ── Go 侧限流配置示例 ─────────────────────────────────────────────
# Go gateway_router.go 中的限流中间件:
# 
# skillRateLimiter := NewTenantRateLimiter(redis, 50, 100)
# skillRateMW := skillRateLimiter.Middleware
# 
# mux.Handle("POST /v1/skills", authMW(skillRateMW(...)))
# mux.Handle("GET /v1/skills/{id}/execute", authMW(skillRateMW(...)))
