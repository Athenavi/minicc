"""Media tools 注册到本地工具注册表。

实现对标 Go `internal/tools/media.go` 注册的两个工具：
- media_create：创建媒体资产（文本/CSV/代码等）
- image_generate：生成图片（AI 生成失败时降级为 SVG 占位图）

默认使用本地 MediaStore；后续可接入 S3 + DB。
"""
from __future__ import annotations

import os
import re
import base64
import hashlib
from typing import Any
from xml.sax.saxutils import escape as xml_escape

from app.tools.registry import registry
from app.media.store import create_store

_store = create_store()


def _sanitize_filename(prompt: str) -> str:
    name = re.sub(r"[^a-zA-Z0-9 _\-]", "", prompt).strip()
    name = re.sub(r"\s+", "_", name)
    return (name[:48] or "image")


def _generate_svg(prompt: str, width: int, height: int) -> bytes:
    # 渐变背景 + 居中文字（与 Go 侧 fallback 一致）
    h = hashlib.md5(prompt.encode()).hexdigest()[:6]
    c1, c2 = f"#{h}", f"#{h[::-1]}"
    lines = []
    words = prompt.split()
    line = ""
    for w in words:
        if len(line) + len(w) + 1 > 30:
            lines.append(line)
            line = w
        else:
            line = f"{line} {w}".strip()
    if line:
        lines.append(line)

    tspans = ""
    start_y = max(40, height // 2 - 10 * len(lines))
    for i, ln in enumerate(lines):
        tspans += f'<tspan x="{width//2}" y="{start_y + i*22}">{xml_escape(ln)}</tspan>'

    svg = f'''<svg xmlns="http://www.w3.org/2000/svg" width="{width}" height="{height}">
  <defs>
    <linearGradient id="bg" x1="0%" y1="0%" x2="100%" y2="100%">
      <stop offset="0%" stop-color="{c1}"/>
      <stop offset="100%" stop-color="{c2}"/>
    </linearGradient>
  </defs>
  <rect width="100%" height="100%" fill="url(#bg)"/>
  <text font-family="Arial, sans-serif" font-size="18" fill="white" text-anchor="middle">{tspans}</text>
</svg>'''
    return svg.encode("utf-8")


# ── media_create ──────────────────────────────────────────────
async def media_create(name: str, content: str, type: str = "text", category: str = "generated", tags: list[str] | None = None) -> dict[str, Any]:
    if not name or not content:
        return {"error": "name and content are required"}
    data = content.encode("utf-8")
    asset = _store.write(name=name, content=data, asset_type=type, category=category, tags=tags or [])
    return {
        "output": f"Media asset '{name}' created ({asset.size} bytes)",
        "id": asset.id, "name": asset.name, "type": asset.type,
        "category": asset.category, "file_url": asset.file_url, "size": asset.size,
    }


# ── image_generate ────────────────────────────────────────────
async def image_generate(prompt: str = "Generated Image", width: int = 800, height: int = 600, category: str = "generated") -> dict[str, Any]:
    width = max(64, min(width, 4096))
    height = max(64, min(height, 4096))

    # 当前无 AI provider，直接走 SVG 降级；后续可注入 ImageGateway
    svg = _generate_svg(prompt, width, height)
    name = _sanitize_filename(prompt) + ".svg"
    asset = _store.write(name=name, content=svg, asset_type="image", category=category, fmt="image/svg+xml", width=width, height=height)

    return {
        "output": f"Image generated: {name} ({asset.size} bytes)",
        "id": asset.id, "name": name, "type": "image", "format": "image/svg+xml",
        "width": width, "height": height, "category": category,
        "file_url": asset.file_url, "size": asset.size,
    }


# ── 注册 ──────────────────────────────────────────────────────
registry.register(
    name="media_create",
    description="Create a media asset (text, CSV, code, etc.).",
    parameters={
        "type": "object",
        "properties": {
            "name": {"type": "string"},
            "content": {"type": "string"},
            "type": {"type": "string", "default": "text"},
            "category": {"type": "string", "default": "generated"},
            "tags": {"type": "array", "items": {"type": "string"}, "default": []},
        },
        "required": ["name", "content"],
    },
    handler=media_create,
)

registry.register(
    name="image_generate",
    description="Generate an image from a text prompt (SVG placeholder; AI generation can be added later).",
    parameters={
        "type": "object",
        "properties": {
            "prompt": {"type": "string", "default": "Generated Image"},
            "width": {"type": "integer", "default": 800},
            "height": {"type": "integer", "default": 600},
            "category": {"type": "string", "default": "generated"},
        },
    },
    handler=image_generate,
)
