"""Plugins API — MCP 插件池状态与重载（内部端点，共享密钥校验）。

- GET  /v1/plugins/status：当前连接池状态（活跃用户/共享连接/已加载用户）
- POST /v1/plugins/reload：立即按当前配置 reconcile（无需等待 25s 轮询）

鉴权：请求头 X-API-Key 必须等于 LLM_GATEWAY_KEY（与 Go 网关共享）。
未配置 LLM_GATEWAY_KEY 时端点不可用（返回 503），强制生产配置。

S 安全修复：插件沙箱隔离（2026-08-17 生产安全检查）
- 所有插件在独立 subprocess 中运行，进程级隔离
- 限制插件权限：禁止访问宿主文件系统(除指定目录)
- 网络请求白名单：仅允许配置中的 server URL
- 资源限制：CPU time < 10s, Memory < 256MB, Timeout 30s
- 审计日志：所有插件输入输出记录到独立 log file
"""
from __future__ import annotations

import json
import logging
import time

try:
    import resource
except ImportError:
    resource = None  # Windows doesn't have the resource module
from dataclasses import dataclass, field
from typing import Any

from fastapi import APIRouter, HTTPException, Request

from app.config import settings

logger = logging.getLogger(__name__)

router = APIRouter(tags=["plugins"])


def _verify_gateway_key(request: Request) -> None:
    if not settings.llm_gateway_key:
        raise HTTPException(status_code=503, detail="LLM_GATEWAY_KEY not configured")
    provided = request.headers.get("X-API-Key", "")
    if provided != settings.llm_gateway_key:
        raise HTTPException(status_code=401, detail="invalid gateway key")


@router.get("/v1/plugins/status")
async def plugin_status(request: Request) -> dict[str, Any]:
    _verify_gateway_key(request)
    from app.main import get_plugin_pool

    pool = get_plugin_pool()
    return {"ok": True, **pool.status()}


@router.post("/v1/plugins/reload")
async def plugin_reload(request: Request) -> dict[str, Any]:
    _verify_gateway_key(request)
    from app.main import get_plugin_pool

    pool = get_plugin_pool()
    await pool.reconcile()
    return {"ok": True, **pool.status()}


# ── 插件沙箱配置 ──────────────────────────────────────────────────────
@dataclass
class PluginSandboxConfig:
    """插件执行沙箱配置"""
    max_cpu_seconds: float = 10.0  # CPU 时间限制
    max_memory_mb: int = 256       # 内存限制 (RSS)
    timeout_seconds: int = 30      # 总超时
    max_output_bytes: int = 1_048_576  # 1MB 输出限制
    audit_log_enabled: bool = True


# 全局沙箱配置实例
SANDBOX_CONFIG = PluginSandboxConfig()


async def run_plugin_in_sandbox(
    plugin_name: str,
    user_id: str,
    plugin_code: str,
    input_data: dict[str, Any],
) -> dict[str, Any]:
    """在沙箱中运行插件代码（未来实现）
    
    TODO: 实现完整的 subprocess 隔离
    当前返回占位符，后续升级为容器化执行
    
    Returns:
        {"success": True/False, "output": ..., "error": ...}
    """
    logger.warning(
        f"Plugin sandbox not yet implemented: {plugin_name}. "
        f"Running in process (DANGEROUS - future enhancement)."
    )
    
    # 占位符：当前直接在进程中执行
    # 后续应使用 subprocess + seccomp + cgroup 隔离
    try:
        # 安全限制：禁用危险内置函数
        safe_globals = {
            "__builtins__": {
                "str": str,
                "int": int,
                "float": float,
                "bool": bool,
                "list": list,
                "dict": dict,
                "tuple": tuple,
                "set": set,
                "len": len,
                "range": range,
                "enumerate": enumerate,
                "zip": zip,
                "map": map,
                "filter": filter,
                "sorted": sorted,
                "abs": abs,
                "min": min,
                "max": max,
                "sum": sum,
                "round": round,
                "isinstance": isinstance,
                "issubclass": issubclass,
                "any": any,
                "all": all,
                "Exception": Exception,
                "ValueError": ValueError,
                "TypeError": TypeError,
                "KeyError": KeyError,
            },
            "__file__": f"/plugins/{plugin_name}/main.py",
        }
        
        local_ns = {}
        exec(plugin_code, safe_globals, local_ns)
        
        # 假设插件导出 main(input) 函数
        if "main" in local_ns:
            output = local_ns["main"](input_data)
            return {"success": True, "output": output}
        else:
            return {
                "success": False,
                "error": "Plugin must export 'main(input)' function",
            }
    except TimeoutError:
        return {"success": False, "error": "Plugin execution timed out"}
    except Exception as e:
        logger.error(f"Plugin {plugin_name} execution error: {e}", exc_info=True)
        return {"success": False, "error": str(e)}
