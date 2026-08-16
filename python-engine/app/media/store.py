"""媒体资产存储接口与本地实现。

提供 BaseStore 抽象接口和 LocalStore 本地文件系统实现。
通过 create_store() 工厂函数根据环境变量选择后端。
"""
from __future__ import annotations

import json
import logging
import os
import time
import uuid
from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

logger = logging.getLogger(__name__)


@dataclass
class Asset:
    id: str
    name: str
    file_url: str
    type: str = "text"
    category: str = "generated"
    tags: list[str] = field(default_factory=list)
    size: int = 0
    format: str = ""
    width: int = 0
    height: int = 0
    created_at: float = field(default_factory=time.time)


class BaseStore(ABC):
    """存储后端抽象接口。"""

    @abstractmethod
    def write(self, name: str, content: bytes, asset_type: str = "text",
              category: str = "generated", tags: list[str] | None = None,
              fmt: str = "", width: int = 0, height: int = 0) -> Asset:
        ...

    @abstractmethod
    def get(self, asset_id: str) -> Asset | None:
        ...

    @abstractmethod
    def list(self) -> list[Asset]:
        ...


class LocalStore(BaseStore):
    """本地文件系统存储（原 MediaStore）。"""

    def __init__(self, root: str) -> None:
        self._root = Path(root)
        self._root.mkdir(parents=True, exist_ok=True)
        self._index_path = self._root / "metadata.json"
        self._assets: dict[str, Asset] = {}
        self._load()

    def _load(self) -> None:
        if self._index_path.exists():
            try:
                data = json.loads(self._index_path.read_text(encoding="utf-8"))
                for item in data:
                    a = Asset(**item)
                    self._assets[a.id] = a
            except Exception:
                logger.warning("Corrupted metadata.json at %s, starting with empty store", self._index_path)

    def _save(self) -> None:
        data = [
            {
                "id": a.id, "name": a.name, "file_url": a.file_url, "type": a.type,
                "category": a.category, "tags": a.tags, "size": a.size,
                "format": a.format, "width": a.width, "height": a.height,
                "created_at": a.created_at,
            }
            for a in self._assets.values()
        ]
        self._index_path.write_text(json.dumps(data, ensure_ascii=False, indent=2), encoding="utf-8")

    def write(self, name: str, content: bytes, asset_type: str = "text", category: str = "generated", tags: list[str] | None = None, fmt: str = "", width: int = 0, height: int = 0) -> Asset:
        asset_id = uuid.uuid4().hex[:12]
        prefix = asset_id[:8]
        safe_name = name.replace("/", "_").replace("\\", "_")
        rel_path = f"media/generated/{prefix}_{safe_name}"
        out = self._root / rel_path
        out.parent.mkdir(parents=True, exist_ok=True)
        out.write_bytes(content)

        asset = Asset(
            id=asset_id, name=name, file_url=str(out), type=asset_type,
            category=category, tags=tags or [], size=len(content),
            format=fmt, width=width, height=height,
        )
        self._assets[asset_id] = asset
        self._save()
        return asset

    def get(self, asset_id: str) -> Asset | None:
        return self._assets.get(asset_id)

    def list(self) -> list[Asset]:
        return list(self._assets.values())


# 向后兼容别名
MediaStore = LocalStore


def create_store() -> BaseStore:
    """根据环境变量创建存储后端。

    环境变量:
        MEDIA_STORE_BACKEND: "local"（默认）或 "s3"
        MEDIA_STORE_PATH: 本地存储根目录（local 模式）
        S3_BUCKET: S3 桶名（s3 模式，必填）
        S3_PREFIX: S3 对象前缀（s3 模式，默认 "media/"）
        S3_ENDPOINT_URL: 自定义 S3 端点（可选，用于 MinIO 等）
    """
    backend = os.getenv("MEDIA_STORE_BACKEND", "local").lower()

    if backend == "s3":
        from app.media.s3_store import S3Store
        bucket = os.getenv("S3_BUCKET", "")
        if not bucket:
            raise ValueError("S3_BUCKET is required when MEDIA_STORE_BACKEND=s3")
        prefix = os.getenv("S3_PREFIX", "media/")
        endpoint_url = os.getenv("S3_ENDPOINT_URL")
        return S3Store(bucket=bucket, prefix=prefix, endpoint_url=endpoint_url)

    # 默认本地存储
    root = os.getenv("MEDIA_STORE_PATH", os.path.join(".", "data", "media"))
    return LocalStore(root)
