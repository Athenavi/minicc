"""简化版记忆存储（兼容 Go deprecated memory tools）。

用途：
- 支持 remember / recall / forget 工具
- 默认内存存储，可选文件落盘（path）
- 生产环境请替换为 Redis/Milvus 方案
"""
from __future__ import annotations

import json
import logging
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any

logger = logging.getLogger(__name__)


@dataclass
class Fact:
    key: str
    value: str
    source: str
    created_at: float


class MemoryStore:
    def __init__(self, path: str | None = None) -> None:
        self._path = Path(path) if path else None
        self._facts: dict[str, Fact] = {}
        if self._path and self._path.exists():
            self._load()

    # ── persistence ─────────────────────────────────────────────
    def _load(self) -> None:
        try:
            data = json.loads(self._path.read_text(encoding="utf-8"))
            for item in data:
                fact = Fact(**item)
                self._facts[fact.key] = fact
        except Exception as e:
            logger.warning("Failed to load memory store from %s: %s", self._path, e)

    def _save(self) -> None:
        if not self._path:
            return
        self._path.parent.mkdir(parents=True, exist_ok=True)
        data = [
            {"key": f.key, "value": f.value, "source": f.source, "created_at": f.created_at}
            for f in self._facts.values()
        ]
        self._path.write_text(json.dumps(data, ensure_ascii=False, indent=2), encoding="utf-8")

    # ── public API ──────────────────────────────────────────────
    def save(self, key: str, value: str, source: str = "ai") -> Fact:
        fact = Fact(key=key, value=value, source=source, created_at=time.time())
        self._facts[key] = fact
        self._save()
        return fact

    def get(self, key: str) -> Fact | None:
        return self._facts.get(key)

    def delete(self, key: str) -> bool:
        if key in self._facts:
            del self._facts[key]
            self._save()
            return True
        return False

    def all(self) -> list[Fact]:
        return list(self._facts.values())

    def search(self, query: str) -> list[Fact]:
        q = query.lower()
        return [f for f in self._facts.values() if q in f.key.lower() or q in f.value.lower()]


# 全局默认存储（内存），可由应用启动时替换
store = MemoryStore()
