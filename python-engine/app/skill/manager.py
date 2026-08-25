"""鎶€鑳藉伐浣滃彴澧炲己: MCP 鍗忚闆嗘垚 + 鍔ㄦ€佹敞鍐?+ 绉熸埛闅旂闄愭祦

鍔熻兘:
1. MCP (Model Context Protocol) 宸ュ叿鍙戠幇涓庤皟鐢?
2. 鎶€鑳藉姩鎬佹敞鍐?鍗歌浇 (鐑洿鏂?
3. 姣忕鎴风嫭绔嬮檺娴?(QPS=50, Burst=100)
4. Trace 闆嗘垚: 姣忔鎶€鑳芥墽琛岃褰?span
"""
from __future__ import annotations

import json
import logging
import os
import shlex
import time
from typing import Any, Optional
from dataclasses import dataclass, field
from enum import Enum

from app.trace import record_span

logger = logging.getLogger(__name__)


class SkillType(str, Enum):
    """鎶€鑳界被鍨?""
    PROMPT = "prompt"              # 鎻愮ず璇嶆ā鏉?
    PYTHON_SCRIPT = "python"       # Python 鑴氭湰
    SHELL_COMMAND = "shell"        # Shell 鍛戒护
    HTTP_REQUEST = "http"          # HTTP 璇锋眰
    MCP_TOOL = "mcp"              # MCP 鍗忚宸ュ叿


class SkillStatus(str, Enum):
    """鎶€鑳界姸鎬?""
    ACTIVE = "active"
    INACTIVE = "inactive"
    ERROR = "error"


@dataclass
class SkillMetadata:
    """鎶€鑳藉厓鏁版嵁"""
    skill_id: str
    name: str
    description: str
    type: SkillType
    version: str = "1.0.0"
    author: str = ""
    tags: list[str] = field(default_factory=list)
    input_schema: dict = field(default_factory=dict)  # JSON Schema
    output_schema: dict = field(default_factory=dict)
    config: dict = field(default_factory=dict)  # 鎶€鑳介厤缃?(濡?prompt 妯℃澘)
    status: SkillStatus = SkillStatus.ACTIVE
    created_at: float = field(default_factory=time.time)
    updated_at: float = 0
    tenant_id: str = ""  # SaaS 瀹夊叏: 绉熸埛闅旂


@dataclass
class SkillExecutionResult:
    """鎶€鑳芥墽琛岀粨鏋?""
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
    """MCP (Model Context Protocol) 瀹㈡埛绔?
    
    MCP 鍗忚瑙勮寖:
    - 浼犺緭灞? STDIO / HTTP / WebSocket
    - 娑堟伅鏍煎紡: JSON-RPC 2.0
    - 鏍稿績鏂规硶: tools/list, tools/call, resources/read
    
    鍙傝€? https://github.com/modelcontextprotocol/specification
    """
    
    def __init__(self, server_url: str, transport: str = "http"):
        self.server_url = server_url
        self.transport = transport  # "stdio" / "http" / "websocket"
        self._tools_cache: list[dict] = []
        self._cache_time = 0
    
    async def discover_tools(self) -> list[dict]:
        """鍙戠幇 MCP Server 鎻愪緵鐨勫伐鍏?""
        # 妫€鏌ョ紦瀛?(5 鍒嗛挓 TTL)
        if time.time() - self._cache_time < 300 and self._tools_cache:
            return self._tools_cache

        # Fail loud: 涓嶆敮鎸佺殑浼犺緭鏂瑰紡蹇呴』鏄惧紡鎶涢敊銆?
        if self.transport == "stdio":
            response = await self._stdio_rpc("tools/list", {}, timeout=10.0)
            tools = response.get("result", {}).get("tools", [])
            self._tools_cache = tools
            self._cache_time = time.time()
            logger.info(f"Discovered {len(tools)} MCP tools via STDIO from {self.server_url}")
            return tools

        if self.transport != "http":
            raise ValueError(f"Unsupported transport: {self.transport}")
        
        try:
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
            
            # 瑙ｆ瀽宸ュ叿鍒楄〃
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
        """璋冪敤 MCP 宸ュ叿"""
        start_time = time.time()
        
        # Fail loud: 涓嶆敮鎸佺殑浼犺緭鏂瑰紡鏄惧紡鎶涢敊銆?
        if self.transport == "stdio":
            response = await self._stdio_rpc(
                "tools/call",
                {"name": tool_name, "arguments": arguments},
                timeout=30.0,
            )
            output = response.get("result", {}).get("content", "")
            is_error = response.get("error") is not None

            duration_ms = int((time.time() - start_time) * 1000)

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
                error=json.dumps(response.get("error", {}), ensure_ascii=False) if is_error else "",
                duration_ms=duration_ms,
                trace_id=trace_id,
                tenant_id=tenant_id,
            )

        if self.transport != "http":
            raise ValueError(f"Unsupported transport: {self.transport}")
        
        try:
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
            
            # 璁板綍 span
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

    # 鈹€鈹€ STDIO 浼犺緭 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

    async def _stdio_rpc(
        self,
        method: str,
        params: dict,
        timeout: float = 30.0,
    ) -> dict[str, Any]:
        """閫氳繃 STDIO 瀛愯繘绋嬪彂閫?JSON-RPC 2.0 璇锋眰銆?

        MCP STDIO 浼犺緭瑙勮寖:
        - server_url 瀛楁澶嶇敤涓鸿鎵ц鐨勫懡浠わ紙濡?"node server.js"锛?
        - 姣忚涓€涓?JSON-RPC 娑堟伅锛岄€氳繃 stdin/stdout 閫氫俊
        - 鍏堝彂閫?initialize 鎻℃墜锛屽啀鍙戦€佸疄闄呰姹?

        璁捐鍐崇瓥:
        - 姣忔 RPC 璇锋眰 spawn 鏂板瓙杩涚▼锛堢畝鍗曞彲闈狅紝閬垮厤鐢熷懡鍛ㄦ湡绠＄悊澶嶆潅搴︼級
        - discover_tools 鏈?5 鍒嗛挓缂撳瓨锛屼笉浼氶绻?spawn
        - 瓒呮椂 kill + stderr 鎴柇 + fail-loud
        """
        import asyncio

        cmd = shlex.split(self.server_url, posix=os.name != "nt")
        if not cmd:
            raise ValueError(
                "STDIO transport: server_url must be a command to execute "
                "(e.g. 'node /path/to/mcp-server.js')"
            )

        # S 瀹夊叏淇锛歁CP STDIO 鍛戒护蹇呴』杩囩櫧鍚嶅崟(PLUGIN_COMMAND_ALLOWLIST)锛?
        # 闃?server_url 鍙閰嶇疆/娉ㄥ叆鎺у埗鏃朵换鎰忓懡浠ゆ墽琛屻€傛湭閰嶇疆鍒?fail-close 鎷掔粷銆?
        from app.tools.ssrf import command_allowed
        if not command_allowed(cmd[0]):
            raise ValueError(
                f"STDIO transport: command not allowed: {cmd[0]} "
                "(set PLUGIN_COMMAND_ALLOWLIST)"
            )

        # 鏋勯€?initialize + 瀹為檯璇锋眰涓ゆ潯娑堟伅
        init_req = {
            "jsonrpc": "2.0",
            "id": 0,
            "method": "initialize",
            "params": {
                "protocolVersion": "2024-11-05",
                "capabilities": {},
                "clientInfo": {"name": "chiron-mcp-client", "version": "1.0.0"},
            },
        }
        actual_req = {
            "jsonrpc": "2.0",
            "id": 1,
            "method": method,
            "params": params,
        }
        payload = (
            json.dumps(init_req) + "\n" + json.dumps(actual_req) + "\n"
        ).encode()

        try:
            proc = await asyncio.create_subprocess_exec(
                *cmd,
                stdin=asyncio.subprocess.PIPE,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
            )
        except FileNotFoundError:
            raise RuntimeError(
                f"STDIO transport: command not found: {cmd[0]}"
            )
        except Exception as e:
            raise RuntimeError(
                f"STDIO transport: failed to spawn process: {e}"
            )

        try:
            stdout, stderr = await asyncio.wait_for(
                proc.communicate(payload),
                timeout=timeout,
            )
        except asyncio.TimeoutError:
            proc.kill()
            await proc.wait()
            raise TimeoutError(
                f"MCP STDIO '{method}' timed out after {timeout}s"
            )

        # 浠?stdout 瑙ｆ瀽 JSON-RPC 鍝嶅簲锛堝彇 id=1 鐨勯偅鏉★級
        lines = stdout.decode(errors="replace").strip().splitlines()
        response: dict[str, Any] | None = None
        for line in reversed(lines):
            line = line.strip()
            if not line:
                continue
            try:
                parsed = json.loads(line)
            except json.JSONDecodeError:
                continue
            if isinstance(parsed, dict) and parsed.get("id") == 1:
                response = parsed
                break

        if response is None:
            err_snippet = stderr.decode(errors="replace")[:500]
            raise RuntimeError(
                f"MCP STDIO '{method}': no JSON-RPC response with id=1. "
                f"stdout_lines={len(lines)}, stderr={err_snippet}"
            )

        return response


class SkillManager:
    """鎶€鑳界鐞嗗櫒 (绉熸埛闅旂)
    
    鍔熻兘:
    1. 鎶€鑳芥敞鍐?鍗歌浇/鏇存柊
    2. 鎶€鑳藉彂鐜?(鎸夌被鍨?鏍囩/鐘舵€?
    3. 鎶€鑳芥墽琛?(缁熶竴鎺ュ彛)
    4. MCP 宸ュ叿闆嗘垚
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
        """娉ㄥ唽鏂版妧鑳?(甯︾鎴烽殧绂?
        
        SaaS 瀹夊叏:
        - skill_id 鏍煎紡: "{tenant_id}:{name}" 闃叉鍐茬獊
        - metadata 鎼哄甫 tenant_id 鏍囪
        """
        # 鏋勯€犲畬鏁?skill_id (甯︾鎴峰墠缂€)
        full_skill_id = f"{tenant_id}:{skill_id}"
        
        skill = SkillMetadata(
            skill_id=full_skill_id,
            name=name,
            description=description,
            type=type,
            tenant_id=tenant_id,
            config=config,
            input_schema=input_schema,
            output_schema=output_schema,
            updated_at=time.time(),
        )
        
        self._skills[full_skill_id] = skill
        
        # 濡傛灉鏄?MCP 绫诲瀷,鍒濆鍖栧鎴风
        if type == SkillType.MCP_TOOL:
            server_url = config.get("server_url", "")
            transport = config.get("transport", "http")
            if server_url:
                self._mcp_clients[full_skill_id] = MCPClient(server_url, transport)
        
        logger.info(f"Skill registered (id={full_skill_id}, name={name}, tenant={tenant_id})")
        
        return skill
    
    async def unregister_skill(self, tenant_id: str, skill_id: str) -> bool:
        """鍗歌浇鎶€鑳?""
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
        """鍒楀嚭鎶€鑳?(绉熸埛闅旂)"""
        results = []
        for skill in self._skills.values():
            if skill.tenant_id != tenant_id:
                continue  # SaaS 瀹夊叏: 杩囨护鍏朵粬绉熸埛
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
        """鎵ц鎶€鑳?(缁熶竴鎺ュ彛 + 绉熸埛闅旂 + trace)"""
        start_time = time.time()
        
        # 鏌ユ壘鎶€鑳藉厓鏁版嵁
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
                # MCP 宸ュ叿璋冪敤
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
                # 鎻愮ず璇嶆ā鏉挎覆鏌?(config 宸插湪 register_skill 鏃跺瓨鍏ュ厓鏁版嵁)
                prompt = skill_meta.config.get("template", "")
                # 濉厖鍙橀噺
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
                return await self._execute_python_script(
                    skill_meta, params, full_skill_id, tenant_id, trace_id, start_time,
                )

            elif skill_meta.type == SkillType.SHELL_COMMAND:
                return await self._execute_shell_command(
                    skill_meta, params, full_skill_id, tenant_id, trace_id, start_time,
                )

            elif skill_meta.type == SkillType.HTTP_REQUEST:
                return await self._execute_http_request(
                    skill_meta, params, full_skill_id, tenant_id, trace_id, start_time,
                )
            
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


    # 鈹€鈹€ Python 鑴氭湰鎵ц (娌欑) 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

    async def _execute_python_script(
        self,
        skill_meta: SkillMetadata,
        params: dict,
        full_skill_id: str,
        tenant_id: str,
        trace_id: str,
        start_time: float,
    ) -> SkillExecutionResult:
        """鍦ㄥ畨鍏ㄦ矙绠变腑鎵ц Python 鑴氭湰鎶€鑳姐€?

        澶嶇敤 run_code 妯″潡鐨勯潤鎬?AST 妫€鏌?+ 杩愯鏃?builtins 瀹堝崼锛?
        纭繚鑴氭湰鏃犳硶璁块棶瀹夸富鏂囦欢绯荤粺/缃戠粶/瀛愯繘绋嬨€?

        config 瀛楁:
            code:     Python 鑴氭湰婧愮爜锛堝嚱鏁颁綋锛屽彲寮曠敤 params 鍙橀噺锛?
            timeout:  瓒呮椂绉掓暟锛堥粯璁?60锛屼笂闄?300锛?
        """
        from app.tools.run_code import _check_static, _safe_builtins, _render_result

        code = skill_meta.config.get("code", "")
        timeout = int(skill_meta.config.get("timeout", 60))

        if not code.strip():
            raise ValueError("skill config missing 'code' field")

        # 闈欐€佸畨鍏ㄦ鏌ワ細AST 鎵弿绂佹鍗遍櫓妯″潡/璋冪敤
        denied = _check_static(code)
        if denied:
            raise ValueError(f"python script blocked by static guard: {denied}")

        # 灏?params 娉ㄥ叆涓鸿剼鏈彲鐢ㄧ殑鍙橀噺
        import io
        import contextlib
        import asyncio

        ns: dict[str, Any] = {
            "params": params,
            "__builtins__": _safe_builtins(),
        }

        # 鍖呰涓?async 鍑芥暟锛堜笌 run_code 璇箟涓€鑷达級
        body = "\n".join("    " + line if line.strip() else line for line in code.splitlines())
        src = f"async def _skill_main():\n{body}\n"

        log_buf = io.StringIO()
        try:
            exec(compile(src, f"<skill:{skill_meta.name}>", "exec"), ns)
            main_fn = ns["_skill_main"]

            async def _run():
                with contextlib.redirect_stdout(log_buf):
                    result = await main_fn()
                return result

            result = await asyncio.wait_for(_run(), timeout=timeout)
            output = json.dumps(
                {"result": _render_result(result), "logs": log_buf.getvalue()[-5000:]},
                ensure_ascii=False,
                default=str,
            )
            success = True
        except asyncio.TimeoutError:
            output = ""
            raise ValueError(f"python script timed out after {timeout}s")
        except RuntimeError as e:
            if "blocked by runtime guard" in str(e):
                raise ValueError(f"python script blocked by runtime guard: {e}")
            raise
        finally:
            log_buf.close()

        duration_ms = int((time.time() - start_time) * 1000)

        if trace_id:
            await record_span(
                trace_id=trace_id,
                span_name=f"skill:python:{skill_meta.name}",
                duration_ms=duration_ms,
                metadata={"tenant_id": tenant_id, "success": True},
                tenant_id=tenant_id,
            )

        return SkillExecutionResult(
            skill_id=full_skill_id,
            skill_name=skill_meta.name,
            success=success,
            output=output,
            duration_ms=duration_ms,
            trace_id=trace_id,
            tenant_id=tenant_id,
        )

    # 鈹€鈹€ Shell 鍛戒护鎵ц (娌欑) 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

    async def _execute_shell_command(
        self,
        skill_meta: SkillMetadata,
        params: dict,
        full_skill_id: str,
        tenant_id: str,
        trace_id: str,
        start_time: float,
    ) -> SkillExecutionResult:
        """鍦ㄥ畨鍏ㄦ矙绠变腑鎵ц Shell 鍛戒护鎶€鑳姐€?

        澶嶇敤 sandbox 妯″潡鐨?run_in_sandbox锛歝wd 閿佸畾 + 鐜娓呯悊 +
        閫冮€告嫤鎴?+ 鍛戒护鐧藉悕鍗曘€?

        config 瀛楁:
            command:  鍛戒护妯℃澘锛堝彲鐢?{var} 寮曠敤 params锛?
            timeout:  瓒呮椂绉掓暟锛堥粯璁?120锛?
        """
        from app.tools.sandbox import run_in_sandbox

        command_template = skill_meta.config.get("command", "")
        timeout = int(skill_meta.config.get("timeout", 120))

        if not command_template.strip():
            raise ValueError("skill config missing 'command' field")

        # 瀹夊叏鍦板～鍏呮ā鏉垮彉閲忥紙浠?str.format锛屼笉鎵ц浠绘剰浠ｇ爜锛?
        try:
            command = command_template.format(**params)
        except KeyError as e:
            raise ValueError(f"missing parameter in command template: {e}")
        except Exception as e:
            raise ValueError(f"command template render failed: {e}")

        result = await run_in_sandbox(command, timeout=timeout)

        duration_ms = int((time.time() - start_time) * 1000)

        if "error" in result:
            output = json.dumps(result, ensure_ascii=False)
            success = False
        else:
            output = json.dumps(result, ensure_ascii=False)
            success = result.get("exit_code", -1) == 0

        if trace_id:
            await record_span(
                trace_id=trace_id,
                span_name=f"skill:shell:{skill_meta.name}",
                duration_ms=duration_ms,
                metadata={"tenant_id": tenant_id, "exit_code": result.get("exit_code")},
                tenant_id=tenant_id,
            )

        return SkillExecutionResult(
            skill_id=full_skill_id,
            skill_name=skill_meta.name,
            success=success,
            output=output,
            error=result.get("error", ""),
            duration_ms=duration_ms,
            trace_id=trace_id,
            tenant_id=tenant_id,
        )

    # 鈹€鈹€ HTTP 璇锋眰鎵ц 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

    async def _execute_http_request(
        self,
        skill_meta: SkillMetadata,
        params: dict,
        full_skill_id: str,
        tenant_id: str,
        trace_id: str,
        start_time: float,
    ) -> SkillExecutionResult:
        """鎵ц HTTP 璇锋眰鎶€鑳斤紙甯?SSRF 闃叉姢锛夈€?

        澶嶇敤 ssrf 妯″潡鐨?assert_safe_url锛氳В鏋愮洰鏍?host锛?
        鎷掔粷鍐呯綉/绉佹湁/淇濈暀 IP 娈点€?

        config 瀛楁:
            url:             璇锋眰 URL 妯℃澘锛堝彲鐢?{var} 寮曠敤 params锛?
            method:          HTTP 鏂规硶锛堥粯璁?GET锛?
            headers:         璇锋眰澶?dict锛堝彲閫夛級
            body_template:   璇锋眰浣撴ā鏉匡紙鍙€夛級
            timeout:         瓒呮椂绉掓暟锛堥粯璁?30锛?
            expected_status: 鏈熸湜鐨勫搷搴旂姸鎬佺爜锛堝彲閫夛紝鐢ㄤ簬鏍￠獙锛?
        """
        from app.tools.ssrf import assert_safe_url

        try:
            import httpx
        except ImportError:
            raise ValueError("httpx not installed 鈥?cannot execute HTTP request skill")

        url_template = skill_meta.config.get("url", "")
        method = skill_meta.config.get("method", "GET").upper()
        headers = skill_meta.config.get("headers", {})
        body_template = skill_meta.config.get("body_template", "")
        timeout = int(skill_meta.config.get("timeout", 30))
        expected_status = skill_meta.config.get("expected_status")

        if not url_template.strip():
            raise ValueError("skill config missing 'url' field")

        # 瀹夊叏鍦板～鍏呮ā鏉?
        try:
            url = url_template.format(**params)
        except KeyError as e:
            raise ValueError(f"missing parameter in url template: {e}")
        except Exception as e:
            raise ValueError(f"url template render failed: {e}")

        body = ""
        if body_template:
            try:
                body = body_template.format(**params)
            except KeyError as e:
                raise ValueError(f"missing parameter in body template: {e}")
            except Exception as e:
                raise ValueError(f"body template render failed: {e}")

        # SSRF 闃叉姢锛氭嫆缁濆唴缃?绉佹湁鍦板潃
        assert_safe_url(url)

        async with httpx.AsyncClient(timeout=timeout) as client:
            response = await client.request(
                method,
                url,
                headers=headers,
                content=body if body else None,
            )

        duration_ms = int((time.time() - start_time) * 1000)

        success = response.is_success
        if expected_status is not None:
            success = response.status_code == expected_status

        output = json.dumps(
            {
                "status_code": response.status_code,
                "headers": dict(response.headers),
                "body": response.text[:10000],
            },
            ensure_ascii=False,
        )

        if trace_id:
            await record_span(
                trace_id=trace_id,
                span_name=f"skill:http:{skill_meta.name}",
                duration_ms=duration_ms,
                metadata={
                    "tenant_id": tenant_id,
                    "url": url,
                    "method": method,
                    "status_code": response.status_code,
                },
                tenant_id=tenant_id,
            )

        return SkillExecutionResult(
            skill_id=full_skill_id,
            skill_name=skill_meta.name,
            success=success,
            output=output,
            duration_ms=duration_ms,
            trace_id=trace_id,
            tenant_id=tenant_id,
        )


# 鈹€鈹€ 鍏ㄥ眬 SkillManager 瀹炰緥 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
# 鐢熶骇鐜搴斾娇鐢?per-tenant 瀹炰緥 (Redis 闅旂)
_global_skill_manager: Optional[SkillManager] = None


def get_skill_manager() -> SkillManager:
    """鑾峰彇鍏ㄥ眬鎶€鑳界鐞嗗櫒 (鍗曚緥)"""
    global _global_skill_manager
    if _global_skill_manager is None:
        _global_skill_manager = SkillManager()
    return _global_skill_manager


# 鈹€鈹€ Go 渚ч檺娴侀厤缃ず渚?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
# Go gateway_router.go 涓殑闄愭祦涓棿浠?
# 
# skillRateLimiter := NewTenantRateLimiter(redis, 50, 100)
# skillRateMW := skillRateLimiter.Middleware
# 
# mux.Handle("POST /v1/skills", authMW(skillRateMW(...)))
# mux.Handle("GET /v1/skills/{id}/execute", authMW(skillRateMW(...)))

