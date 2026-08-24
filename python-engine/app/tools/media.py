"""Media tools 注册到本地工具注册表。

实现对标 Go `internal/tools/media.go` 注册的两个工具：
- media_create：创建媒体资产（文本/CSV/代码等）
- image_generate：生成图片（AI 生成失败时降级为 SVG 占位图）

默认使用本地 MediaStore；后续可接入 S3 + DB。
"""
from __future__ import annotations

import os
import re
from typing import Any

from app.tools.registry import registry
from app.media.store import create_store

_store = create_store()


def _sanitize_filename(prompt: str) -> str:
    name = re.sub(r"[^a-zA-Z0-9 _\-]", "", prompt).strip()
    name = re.sub(r"\s+", "_", name)
    return (name[:48] or "image")


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
    """真实生成图片；未配置 provider 时明确报错（S 修复：移除假的 SVG 占位，
    不再虚报生成成功）。配置 IMAGE_GEN_API_URL(+IMAGE_GEN_API_KEY) 接入生成服务。"""
    width = max(64, min(width, 4096))
    height = max(64, min(height, 4096))

    api_url = os.getenv("IMAGE_GEN_API_URL", "").strip()
    api_key = os.getenv("IMAGE_GEN_API_KEY", "").strip()
    if not api_url:
        return {"error": "image generation not available: IMAGE_GEN_API_URL not configured"}

    import httpx
    try:
        headers = {"Authorization": f"Bearer {api_key}"} if api_key else {}
        async with httpx.AsyncClient(timeout=120) as client:
            resp = await client.post(
                api_url, json={"prompt": prompt, "width": width, "height": height}, headers=headers
            )
            resp.raise_for_status()
            data = resp.content
    except Exception as e:  # noqa: BLE001 — 生成失败如实返回，不伪造成功
        return {"error": f"image generation failed: {e}"}

    name = _sanitize_filename(prompt) + ".png"
    asset = _store.write(name=name, content=data, asset_type="image", category=category, fmt="image/png", width=width, height=height)

    return {
        "output": f"Image generated: {name} ({asset.size} bytes)",
        "id": asset.id, "name": name, "type": "image", "format": "image/png",
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
    description="Generate an image from a text prompt (requires IMAGE_GEN_API_URL; fails loudly when unconfigured).",
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
