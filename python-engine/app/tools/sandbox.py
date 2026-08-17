"""sandbox — per-user 运行环境隔离（A 层：代码级 root/cwd 锁定）

所有 agent 文件/命令工具强制在 `SANDBOX_ROOT/{tenant}/{user}/workspace/` 内
执行，模型无法接触宿主文件系统（解决：思考/输出暴露服务器真实路径）。

- get_sandbox_dir() / workspace_dir()：由 contextvars（user/tenant）推导
- safe_join()：相对路径 clamp 到 workspace，拒绝 ../ 与绝对路径逃逸
"""
from __future__ import annotations

import asyncio
import logging
import os
from pathlib import Path
from typing import Any

logger = logging.getLogger(__name__)


def sandbox_root() -> Path:
    """沙箱根：默认置于进程 cwd 上两级（项目外），避免 workspace 路径
    泄露服务器项目结构（S 安全修复：cwd 泄露项目/python-engine 前缀）。"""
    env = os.environ.get("SANDBOX_ROOT")
    if env:
        return Path(env).resolve()
    # 无论 cwd 是项目根还是子目录（python-engine），上两级都到项目外
    return Path(os.getcwd()).resolve().parent.parent / "minicc-sandbox"


def get_sandbox_dir() -> Path:
    """当前 user 的沙箱根（由 context 推导，自动创建）。"""
    from app.tools.context import get_user_id, get_tenant_id
    tenant = get_tenant_id() or "default"
    user = get_user_id() or "anonymous"
    d = sandbox_root() / tenant / user
    try:
        d.mkdir(parents=True, exist_ok=True)
    except OSError as e:
        logger.warning("sandbox dir create failed: %s", e)
    return d


def workspace_dir() -> Path:
    """agent 文件操作的唯一根目录。"""
    d = get_sandbox_dir() / "workspace"
    try:
        d.mkdir(parents=True, exist_ok=True)
    except OSError as e:
        logger.warning("workspace dir create failed: %s", e)
    return d


def safe_join(rel: str) -> Path:
    """将相对路径 clamp 到 workspace，拒绝逃逸（绝对路径/../）。"""
    base = workspace_dir().resolve()
    target = (base / rel).resolve()
    if target != base and not str(target).startswith(str(base) + os.sep):
        raise ValueError(f"path escapes sandbox: {rel}")
    return target


def sandboxed_env() -> dict[str, str]:
    """执行环境：清理敏感/宿主变量，仅保留基础 PATH，并把用户目录变量
    重定向到沙箱内，防止 `$HOME`/`~` 泄漏宿主路径（S 安全修复）。"""
    allow = {"PATH", "SYSTEMROOT", "WINDIR", "COMSPEC", "TEMP", "TMP", "HOME", "USERPROFILE"}
    env = {k: v for k, v in os.environ.items() if k in allow}
    env["HOME"] = str(workspace_dir())
    env["USERPROFILE"] = str(workspace_dir())
    env["TEMP"] = str(get_sandbox_dir())
    env["TMP"] = str(get_sandbox_dir())
    return env


# 被禁止的 shell 逃逸模式（S 安全修复：cwd 锁定无法阻止命令内绝对路径访问宿主）
# 注意：生产部署为 Linux/alpine（见 Dockerfile），必须覆盖 Unix 绝对路径。
_ESCAPE_PATTERNS = [
    r"[A-Za-z]:[\\/]",        # Windows 盘符绝对路径 (C:\ 或 C:/)
    r"\\\\",                  # UNC 路径
    r"(^|[^A-Za-z0-9_.])(\.\.)[\\/]",  # 父目录跳转 ..\ 或 ../
    r"\bcd\b[^&|;]*[A-Za-z]:[\\/]",   # cd 到绝对路径
    # Unix/Linux 逃逸：绝对路径（/xxx，2+ 字符路径段或单独 /）、~ 家目录、$HOME 变量。
    # 以空格/引号/分号开头界定，避免误伤相对路径（a/b）、URL（http://）
    # 与 Windows cmd 单字符开关（/b /d /s）。
    r"(?:^|[\s\"';])(?:/(?:[A-Za-z0-9_.-]{2,}|$)|~/?|\$\{?HOME\}?\b)",
    # 云元数据 / 内网地址（curl/wget 等 SSRF 常见目标）
    r"(?:^|[\s\"';])(?:curl|wget)\s+https?://(?:169\.254\.169\.254|127\.0\.0\.1|localhost)",
]


def _has_escape(command: str) -> str | None:
    """检测命令是否含逃逸模式，命中返回原因。"""
    import re
    for pat in _ESCAPE_PATTERNS:
        m = re.search(pat, command)
        if m:
            return pat
    return None


async def run_in_sandbox(command: str, timeout: int = 120) -> dict[str, Any]:
    """在沙箱 workspace 内执行命令（shell），cwd 锁定 + 环境清理 + 逃逸拦截。"""
    import asyncio.subprocess

    # 逃逸拦截：绝对路径/父目录访问宿主 → 拒绝（S 安全修复）
    esc = _has_escape(command)
    if esc:
        return {
            "error": f"command blocked: absolute-path / parent-directory access is not allowed in sandbox",
            "reason": esc,
        }

    proc = await asyncio.create_subprocess_shell(
        command,
        cwd=str(workspace_dir()),
        env=sandboxed_env(),
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )
    try:
        stdout, stderr = await asyncio.wait_for(proc.communicate(), timeout=timeout)
    except asyncio.TimeoutError:
        proc.kill()
        await proc.wait()
        return {"error": "timeout", "timeout": timeout}
    return {
        "exit_code": proc.returncode,
        "stdout": stdout.decode("utf-8", errors="replace"),
        "stderr": stderr.decode("utf-8", errors="replace"),
    }
