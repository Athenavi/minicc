"""Glob 工具集 — 文件模式匹配搜索。"""
from __future__ import annotations

import os
from pathlib import Path
from typing import Any

from app.tools.registry import registry


async def glob_files(pattern: str, root: str = ".") -> dict[str, Any]:
    """Find files matching a glob pattern (supports **, *, ?, []).

    Returns a list of matching file paths with their sizes.
    """
    base = Path(root).resolve()
    # pathlib.Path.glob already supports **, *, ?, []
    matches: list[dict[str, Any]] = []
    try:
        for p in sorted(base.glob(pattern)):
            if p.is_file():
                try:
                    size = p.stat().st_size
                except OSError:
                    size = 0
                matches.append({
                    "path": str(p),
                    "size": size,
                })
    except (ValueError, OSError) as exc:
        return {"error": str(exc), "count": 0, "files": []}

    return {"count": len(matches), "files": matches}


# ---------------------------------------------------------------------------
# 注册
# ---------------------------------------------------------------------------
registry.register(
    name="glob_files",
    description="Find files matching a glob pattern (supports **, *, ?, [])",
    parameters={
        "type": "object",
        "properties": {
            "pattern": {
                "type": "string",
                "description": 'Glob pattern, e.g. "**/*.py", "src/**/*.go"',
            },
            "root": {
                "type": "string",
                "description": "Root directory to search from",
                "default": ".",
            },
        },
        "required": ["pattern"],
    },
    handler=glob_files,
)
