"""核心本地工具集（Python 端）。

首波迁移：filesystem / shell / search(grep) / web。
后续可继续扩展 memory / pm / skill / workflow 等工具。
"""
from __future__ import annotations

import asyncio
import json
import logging
import os
import glob as globmod
import re
from pathlib import Path
from typing import Any

import httpx

from app.tools.registry import registry

logger = logging.getLogger(__name__)


def _safe_path(path: str, root: str) -> Path:
    """防止路径穿越，返回绝对路径。"""
    base = Path(root).resolve()
    target = (base / path).resolve()
    # 必须加 trailing separator，否则 /app 会误匹配 /appdata
    if target != base and not str(target).startswith(str(base) + os.sep):
        raise ValueError("path escapes root")
    return target


async def read_file(path: str, root: str = ".", offset: int = 0, limit: int = 2000) -> dict[str, Any]:
    target = _safe_path(path, root)
    if not target.exists():
        return {"error": f"file not found: {path}"}
    text = target.read_text(encoding="utf-8", errors="replace")
    lines = text.splitlines()
    page = lines[offset: offset + limit]
    return {
        "path": str(target),
        "total_lines": len(lines),
        "offset": offset,
        "limit": limit,
        "content": "\n".join(page),
    }


async def write_file(path: str, content: str, root: str = ".") -> dict[str, Any]:
    target = _safe_path(path, root)
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(content, encoding="utf-8")
    return {"path": str(target), "bytes": len(content.encode("utf-8"))}


async def execute_command(command: str, cwd: str = ".", timeout: int = 30) -> dict[str, Any]:
    proc = await asyncio.create_subprocess_shell(
        command,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
        cwd=cwd,
    )
    try:
        stdout, stderr = await asyncio.wait_for(proc.communicate(), timeout=timeout)
    except asyncio.TimeoutError:
        proc.kill()
        return {"error": "timeout", "timeout": timeout}
    return {
        "exit_code": proc.returncode,
        "stdout": stdout.decode("utf-8", errors="replace"),
        "stderr": stderr.decode("utf-8", errors="replace"),
    }


async def grep_files(query: str, root: str = ".", glob: str = "", max_results: int = 200) -> dict[str, Any]:
    pattern = re.compile(query)
    matches: list[dict[str, Any]] = []
    base = _safe_path(root, os.getcwd())
    paths = globmod.glob(str(base / "**" / (glob or "*")), recursive=True)
    for p in paths:
        if not os.path.isfile(p):
            continue
        try:
            with open(p, encoding="utf-8", errors="ignore") as f:
                for idx, line in enumerate(f, 1):
                    if pattern.search(line):
                        matches.append({"path": p, "line": idx, "text": line.rstrip("\n")})
                        if len(matches) >= max_results:
                            return {"count": len(matches), "matches": matches}
        except Exception:
            logger.debug("Cannot read file %s, skipping", p)
            continue
    return {"count": len(matches), "matches": matches}


async def search_files(pattern: str, root: str = ".", max_results: int = 200) -> dict[str, Any]:
    base = _safe_path(root, os.getcwd())
    paths = globmod.glob(str(base / "**" / (pattern or "*")), recursive=True)
    files = [p for p in paths if os.path.isfile(p)][:max_results]
    return {"count": len(files), "files": files}


async def execute_python(code: str, timeout: int = 30) -> dict[str, Any]:
    if not code:
        return {"error": "code is required"}
    return await execute_command(command=f"python -c {json.dumps(code)}", timeout=timeout)


async def web_fetch(url: str, max_chars: int = 12000, follow_redirects: bool = True) -> dict[str, Any]:
    async with httpx.AsyncClient(timeout=15, follow_redirects=follow_redirects) as client:
        resp = await client.get(url)
        text = resp.text[:max_chars]
        return {
            "url": str(resp.url),
            "status_code": resp.status_code,
            "content_type": resp.headers.get("content-type", ""),
            "content": text,
        }


# 注册工具（schema 与 Go 侧保持兼容）
registry.register(
    name="read_file",
    description="Read a file with pagination",
    parameters={
        "type": "object",
        "properties": {
            "path": {"type": "string"},
            "offset": {"type": "integer", "default": 0},
            "limit": {"type": "integer", "default": 2000},
        },
        "required": ["path"],
    },
    handler=read_file,
)

registry.register(
    name="write_file",
    description="Write content to a file",
    parameters={
        "type": "object",
        "properties": {
            "path": {"type": "string"},
            "content": {"type": "string"},
        },
        "required": ["path", "content"],
    },
    handler=write_file,
)

registry.register(
    name="shell_exec",
    description="Execute a shell command with timeout",
    parameters={
        "type": "object",
        "properties": {
            "command": {"type": "string"},
            "cwd": {"type": "string", "default": "."},
            "timeout": {"type": "integer", "default": 30},
        },
        "required": ["command"],
    },
    handler=execute_command,
)

registry.register(
    name="grep_files",
    description="Regex search across files",
    parameters={
        "type": "object",
        "properties": {
            "query": {"type": "string"},
            "root": {"type": "string", "default": "."},
            "glob": {"type": "string", "default": ""},
            "max_results": {"type": "integer", "default": 200},
        },
        "required": ["query"],
    },
    handler=grep_files,
)

registry.register(
    name="web_fetch",
    description="Fetch URL content over HTTP/HTTPS",
    parameters={
        "type": "object",
        "properties": {
            "url": {"type": "string"},
            "max_chars": {"type": "integer", "default": 12000},
            "follow_redirects": {"type": "boolean", "default": True},
        },
        "required": ["url"],
    },
    handler=web_fetch,
)


registry.register(
    name="search_files",
    description="Search files by glob pattern",
    parameters={
        "type": "object",
        "properties": {
            "pattern": {"type": "string"},
            "root": {"type": "string", "default": "."},
            "max_results": {"type": "integer", "default": 200},
        },
        "required": ["pattern"],
    },
    handler=search_files,
)

registry.register(
    name="execute_python",
    description="Execute a Python code snippet",
    parameters={
        "type": "object",
        "properties": {
            "code": {"type": "string"},
            "timeout": {"type": "integer", "default": 30},
        },
        "required": ["code"],
    },
    handler=execute_python,
)
