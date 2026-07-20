"""Skill Store（磁盘持久化，兼容 Go SkillStore 语义）。

存储目录下每个 `.skill.json` 文件对应一个 SkillDef。
"""
from __future__ import annotations

import json
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any


@dataclass
class SkillDef:
    name: str
    description: str
    version: str = "0.1.0"
    author: str = ""
    tags: list[str] = field(default_factory=list)
    exec_type: str = "prompt"
    source: str = ""
    parameters: list[dict[str, Any]] = field(default_factory=list)
    installed_at: float = field(default_factory=time.time)

    def to_dict(self) -> dict[str, Any]:
        return {
            "name": self.name,
            "description": self.description,
            "version": self.version,
            "author": self.author,
            "tags": self.tags,
            "exec": {"type": self.exec_type, "source": self.source},
            "parameters": self.parameters,
            "installed_at": self.installed_at,
        }


class SkillStore:
    def __init__(self, root: str) -> None:
        self._root = Path(root)
        self._root.mkdir(parents=True, exist_ok=True)

    def _path(self, name: str) -> Path:
        return self._root / f"{name}.skill.json"

    def list(self) -> list[SkillDef]:
        out: list[SkillDef] = []
        for p in sorted(self._root.glob("*.skill.json")):
            try:
                out.append(self._load(p))
            except Exception:
                continue
        return out

    def get(self, name: str) -> SkillDef | None:
        p = self._path(name)
        if not p.exists():
            return None
        try:
            return self._load(p)
        except Exception:
            return None

    def save(self, skill: SkillDef) -> None:
        p = self._path(skill.name)
        p.write_text(json.dumps(skill.to_dict(), ensure_ascii=False, indent=2), encoding="utf-8")

    def delete(self, name: str) -> bool:
        p = self._path(name)
        if p.exists():
            p.unlink()
            return True
        return False

    def _load(self, p: Path) -> SkillDef:
        data = json.loads(p.read_text(encoding="utf-8"))
        exec_cfg = data.get("exec", {})
        return SkillDef(
            name=data["name"],
            description=data.get("description", ""),
            version=data.get("version", "0.1.0"),
            author=data.get("author", ""),
            tags=data.get("tags", []),
            exec_type=exec_cfg.get("type", "prompt"),
            source=exec_cfg.get("source", ""),
            parameters=data.get("parameters", []),
            installed_at=data.get("installed_at", 0),
        )
