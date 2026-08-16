"""run_code 工具 — PTC 模式核心（对应 deepseek-harness Code Mode SDK）

模型编写一段 Python 异步程序，通过注入的 ``tools`` 命名空间组合调用多个
工具（``await tools.read_file(...)``、``await tools.shell_exec(...)`` 等），
一次执行多步操作（deepseek: "five round trips become one"）。

- 程序体为 ``async def _main():`` 的函数体
- ``tools`` 是注入的命名空间：每个已注册工具的异步方法，调用
  ``registry.execute(name, args)``，返回规范 JSON；工具失败抛
  ``ToolCallError(name, message)``
- stdout 捕获 → logs；``return`` 值 → result
- 超时/异常 → 结构化 ``{isError, message, logs}`` 返回模型自纠
"""
from __future__ import annotations

import asyncio
import contextlib
import io
import json
import logging
import types
from typing import Any

from app.tools.registry import registry

logger = logging.getLogger(__name__)

DEFAULT_TIMEOUT_SECONDS = 60
MAX_LOG_CHARS = 20_000

# ── 静态安全检查：禁止模型代码直接访问宿主（S 安全修复） ────────────────
# 文件/命令/网络操作必须通过 tools.* 命名空间（工具层有沙箱栅栏）。
# 说明：这是 A 层（代码层）约束——提高绕过门槛；完整隔离仍需部署侧 B 层（容器/低权限）。
DANGEROUS_MODULES = {
    "os", "subprocess", "sys", "ctypes", "socket", "importlib",
    "pathlib", "shutil", "tempfile", "glob", "atexit", "signal", "multiprocessing",
}
DANGEROUS_CALLS = {"open", "exec", "eval", "compile", "__import__", "input", "breakpoint"}
DANGEROUS_ATTRS = ("os.", "subprocess.", "sys.", "socket.", "ctypes.")


def _check_static(code: str) -> str | None:
    """AST 静态检查模型代码：命中危险模块导入/危险调用返回原因，否则 None。"""
    import ast as _ast

    try:
        tree = _ast.parse(code)
    except SyntaxError:
        return None  # 语法错误由执行阶段结构化返回

    for node in _ast.walk(tree):
        # import os / import pathlib / from X import Y
        if isinstance(node, _ast.Import):
            for alias in node.names:
                if alias.name.split(".")[0] in DANGEROUS_MODULES:
                    return f"module '{alias.name}' is not allowed — use tools.* instead"
        if isinstance(node, _ast.ImportFrom):
            if node.module and node.module.split(".")[0] in DANGEROUS_MODULES:
                return f"module '{node.module}' is not allowed — use tools.* instead"
        # open(...) / exec(...) / eval(...) / __import__(...)
        if isinstance(node, _ast.Call):
            fn = node.func
            if isinstance(fn, _ast.Name) and fn.id in DANGEROUS_CALLS:
                return f"call '{fn.id}()' is not allowed — use tools.* instead"
            # os.system / subprocess.run / sys.exit / socket.* 属性调用
            if isinstance(fn, _ast.Attribute) and isinstance(fn.value, _ast.Name):
                attr_path = f"{fn.value.id}.{fn.attr}"
                if attr_path in ("os.system", "os.popen", "os.startfile", "os.remove",
                                 "os.rename", "os.chdir", "os.mkdir", "os.makedirs",
                                 "sys.exit", "subprocess.run", "subprocess.Popen",
                                 "subprocess.call", "socket.socket", "shutil.rmtree",
                                 "shutil.copy", "shutil.move", "pathlib.Path", "open"):
                    return f"call '{attr_path}()' is not allowed — use tools.* instead"
    return None


# ── 运行时守卫（SaaS 安全：堵静态守卫绕过，如 __builtins__['ope'+'n'] 动态构造） ──
# 方案：替换 exec 命名空间的 builtins —— open/exec/eval 等 builtin 全被 stub（raise），
# __import__ 白名单化（仅安全标准库）。模型代码无法拿到 os/subprocess 模块，
# 动态构造 builtin 调用也被 stub 拦截。（settrace/monitoring 对 asyncio 协程与
# builtin 调用均不可靠，实测弃用。）
_SAFE_IMPORTS = frozenset({
    "json", "math", "random", "datetime", "re", "collections",
    "itertools", "asyncio", "typing", "time", "functools", "decimal",
})
_BLOCKED_BUILTINS = frozenset({"open", "exec", "eval", "compile", "input", "breakpoint"})


def _make_blocked_builtin(name: str):
    def _blocked(*_args, **_kwargs):  # noqa: ANN002, ANN003 — 桩函数
        raise RuntimeError(f"blocked by runtime guard: {name}()")
    return _blocked


def _safe_builtins() -> dict:
    """构造受控 builtins：安全库白名单导入 + 危险 builtin 全部 stub。"""
    import builtins as _b
    orig = vars(_b).copy()
    real_import = orig["__import__"]

    def _guarded_import(name, globals=None, locals=None, fromlist=(), level=0):  # noqa: A002
        root = (name or "").split(".")[0]
        if root in _SAFE_IMPORTS:
            return real_import(name, globals, locals, fromlist, level)
        raise RuntimeError(f"blocked by runtime guard: import '{name}'")

    for name in _BLOCKED_BUILTINS:
        orig[name] = _make_blocked_builtin(name)
    orig["__import__"] = _guarded_import
    return orig


class ToolCallError(Exception):
    """模型程序内调用工具失败时抛出的错误（仅携带 toolName + message）。"""

    def __init__(self, tool_name: str, message: str):
        self.tool_name = tool_name
        self.message = message
        super().__init__(f"ToolCallError({tool_name}): {message}")


async def _dispatch(name: str, args: dict[str, Any]) -> Any:
    """调用一个已注册工具，失败抛 ToolCallError。"""
    result = await registry.execute(name, args)
    if isinstance(result, dict) and result.get("error"):
        raise ToolCallError(name, str(result["error"]))
    return result


def _build_sdk() -> types.SimpleNamespace:
    """构造 tools 命名空间：每个注册工具一个 async 方法。"""
    ns = types.SimpleNamespace()

    async def _make_tool(name: str, *args: Any, **kwargs: Any) -> Any:
        # 兼容 tools.name(args_dict) 与 tools.name(k=v) 两种调用
        if len(args) == 1 and isinstance(args[0], dict) and not kwargs:
            params: dict[str, Any] = args[0]
        else:
            params = kwargs
        return await _dispatch(name, params)

    for tool_name in registry.list_names():
        setattr(ns, tool_name, _make_tool.__get__(tool_name, None))
    return ns


def sdk_usage_text() -> str:
    """生成注入 prompt 的 SDK 用法说明（确定性、按当前注册表）。"""
    names = ", ".join(sorted(registry.list_names()))
    return (
        "In the run_code program, a `tools` namespace is available: "
        "each registered tool is an async callable, e.g. "
        "`result = await tools.read_file(path='src/main.py')` or "
        "`result = await tools.grep_files(pattern='TODO', path='src')`. "
        "Tool failures raise ToolCallError(name, message). "
        "Return a JSON-serializable value from the program; captured stdout "
        "appears in logs. Available tools: " + names
    )


def _render_result(value: Any) -> Any:
    """规范化返回值（json-serializable）。"""
    if value is None or isinstance(value, (str, int, float, bool)):
        return value
    try:
        return json.loads(json.dumps(value, ensure_ascii=False, default=str))
    except Exception:
        return str(value)


async def run_code(code: str, description: str = "", timeout: int = DEFAULT_TIMEOUT_SECONDS) -> dict[str, Any]:
    """Execute a Python async program against the tool SDK.

    Parameters
    ----------
    code:
        Body of ``async def _main():``. May use the injected ``tools``
        namespace and ``return`` a JSON-serializable value.
    description:
        Short summary of what the program does (for the model's own bookkeeping).
    """
    if not code.strip():
        return {"error": "code is required"}
    timeout = max(1, min(timeout, 300))

    # 静态安全检查（S 安全修复：禁止模型代码直接访问宿主文件/命令/网络）
    denied = _check_static(code)
    if denied:
        return {
            "isError": True,
            "message": f"program blocked by static guard: {denied}",
            "logs": "",
        }

    # 缩进程序体并包装为 async 函数
    body = "\n".join("    " + line if line.strip() else line for line in code.splitlines())
    # SaaS 安全：运行时守卫 — 受控 builtins（白名单 import + 危险 builtin stub）
    ns: dict[str, Any] = {"tools": _build_sdk(), "ToolCallError": ToolCallError, "__builtins__": _safe_builtins()}
    src = f"async def _main():\n{body}\n"

    log_buf = io.StringIO()
    try:
        exec(compile(src, "<run_code>", "exec"), ns)  # noqa: S102 — 模型沙箱语义由部署策略约束
        main_fn = ns["_main"]
        async def _run():
            with contextlib.redirect_stdout(log_buf):
                result = await main_fn()
            return result
        result = await asyncio.wait_for(_run(), timeout=timeout)
    except RuntimeError as e:
        # 运行时守卫拦截（SaaS 安全：堵静态守卫绕过，如 __builtins__['ope'+'n'] 动态构造）
        if "blocked by runtime guard" in str(e):
            return {
                "isError": True,
                "message": str(e),
                "logs": log_buf.getvalue()[-MAX_LOG_CHARS:],
            }
        return {
            "isError": True,
            "message": f"program raised RuntimeError: {e}",
            "logs": log_buf.getvalue()[-MAX_LOG_CHARS:],
        }
    except ToolCallError as e:
        return {
            "isError": True,
            "message": f"tool call failed: {e.tool_name} — {e.message}",
            "logs": log_buf.getvalue()[-MAX_LOG_CHARS:],
        }
    except asyncio.TimeoutError:
        return {"isError": True, "message": f"run_code timed out after {timeout}s", "logs": log_buf.getvalue()[-MAX_LOG_CHARS:]}
    except SyntaxError as e:
        return {"isError": True, "message": f"program syntax error: {e}", "logs": ""}
    except Exception as e:  # noqa: BLE001 — 模型程序异常按结构化错误返回
        return {"isError": True, "message": f"program raised {type(e).__name__}: {e}", "logs": log_buf.getvalue()[-MAX_LOG_CHARS:]}

    return {
        "logs": log_buf.getvalue()[-MAX_LOG_CHARS:],
        "result": _render_result(result),
    }


# ── Register ─────────────────────────────────────────────────────
registry.register(
    name="run_code",
    description=(
        "Write a Python async program that composes multiple tool calls into one step. "
        "The program body runs as `async def _main()`, with an injected `tools` namespace: "
        "`await tools.<name>(**args)` calls any registered tool and returns its JSON result; "
        "tool failures raise ToolCallError. Capture stdout goes to logs; `return` a "
        "JSON-serializable value as the result. Use this to batch several file/shell/search "
        "operations into a single round trip.\n\n" + sdk_usage_text()
    ),
    parameters={
        "type": "object",
        "properties": {
            "code": {
                "type": "string",
                "description": "Body of `async def _main():` — Python code using the tools namespace",
            },
            "description": {
                "type": "string",
                "description": "Short summary of what the program does",
            },
            "timeout": {
                "type": "integer",
                "description": "Timeout in seconds (default 60, max 300)",
                "default": 60,
            },
        },
        "required": ["code"],
    },
    handler=run_code,
)
