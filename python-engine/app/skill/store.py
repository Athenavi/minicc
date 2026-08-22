"""Skill Store（磁盘持久化，兼容 Go SkillStore 语义）。

存储目录下每个 `.skill.json` 文件对应一个 SkillDef。

## 多租户 / 用户级隔离（用户级目录隔离）

- 根目录解析规则（`SkillStore(root=...)` 显式传入优先）：
  1. 显式 `root` → 直接使用，不做身份解析（兼容旧调用方 / 测试注入）；
  2. 未传 root 时，base = `SKILL_STORE_PATH` 环境变量或 `data/skills`：
     - 同时缺失 tenant_id/user_id（未登录 / 系统任务）→ `base/_shared/`；
     - 有 tenant_id → `base/{tenant_id}/`（可再叠加 `{user_id}`）；
     - 有 user_id → `base/{tenant_id}/{user_id}/`（tenant 缺失则 `base/{user_id}/`）。
- 身份段（tenant_id / user_id）只允许 `[A-Za-z0-9][A-Za-z0-9_.-]{0,127}`，
  其余一律拒绝（`ValueError`），防 `../` 路径穿越导致跨租户目录逃逸。
- 兼容迁移：首次访问 `_shared` 时，若 `_shared` 为空但旧全局目录（base 本身）
  存在 `.skill.json`，将旧目录内容复制进 `_shared`（只复制不移动，幂等；
  线程锁保护并发）。旧全局技能因此自动变为“共享技能”，行为与迁移前一致。

API 层（app/api/skills.py）与运行时工具链（app/tools/skill.py）共用此解析规则：
API 从 query 参数（网关注入）取身份，工具链从 app.tools.context（contextvars）取身份。
"""
from __future__ import annotations

import json
import os
import re
import shutil
import threading
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Union

# 共享目录名（无身份时的回退目录）
SHARED_DIR = "_shared"
# 默认技能根（环境变量 SKILL_STORE_PATH 可覆盖 base）
DEFAULT_SKILL_ROOT = os.path.join(".", "data", "skills")
# 身份段白名单：防路径穿越（不允许 / \ .. 空格等）
_IDENTITY_SEGMENT_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$")

_migration_lock = threading.Lock()

PathLike = Union[str, Path]


def _safe_segment(value: str, label: str) -> str:
    """校验并规范化身份段；非法输入抛 ValueError（防路径穿越）。"""
    v = str(value or "").strip()
    if not v:
        return ""
    if not _IDENTITY_SEGMENT_RE.match(v):
        raise ValueError(f"invalid {label} segment: {value!r}")
    return v


def _migrate_legacy_to_shared(base: Path, shared: Path) -> None:
    """兼容迁移：_shared 为空但旧全局目录有技能时，把旧目录内容复制进 _shared。

    只复制不移动（非破坏性）；幂等（_shared 非空即跳过）；线程锁防并发重复复制。
    """
    if not base.is_dir():
        return
    with _migration_lock:
        if any(shared.glob("*.skill.json")):
            return
        legacy_files = [p for p in base.glob("*.skill.json") if p.is_file()]
        if not legacy_files:
            return
        shared.mkdir(parents=True, exist_ok=True)
        for p in legacy_files:
            shutil.copy2(p, shared / p.name)
        # 旧目录里可能的子目录一并复制（排除 _shared 自身，防递归）
        for d in base.iterdir():
            if d.is_dir() and d.name != SHARED_DIR:
                shutil.copytree(d, shared / d.name, dirs_exist_ok=True)


def resolve_skill_root(
    root: PathLike | None = None,
    *,
    tenant_id: str = "",
    user_id: str = "",
) -> Path:
    """解析技能存储根目录。

    显式 root 优先；否则按身份解析（见模块 docstring）。身份段非法时抛 ValueError。
    """
    if root:
        return Path(root)
    base = Path(os.getenv("SKILL_STORE_PATH", DEFAULT_SKILL_ROOT))
    tid = _safe_segment(tenant_id, "tenant_id")
    uid = _safe_segment(user_id, "user_id")
    if not tid and not uid:
        # 身份缺失（未登录 / 系统任务）→ 共享目录，并触发旧全局目录迁移
        shared = base / SHARED_DIR
        _migrate_legacy_to_shared(base, shared)
        return shared
    parts: list[Path] = [base]
    if tid:
        parts.append(Path(tid))
    if uid:
        parts.append(Path(uid))
    return Path(*parts)


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
    enabled: bool = True

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
            "enabled": self.enabled,
        }


class SkillStore:
    def __init__(
        self,
        root: PathLike | None = None,
        *,
        tenant_id: str = "",
        user_id: str = "",
    ) -> None:
        """按身份解析存储目录；显式 root 优先（见 resolve_skill_root）。"""
        self._root = resolve_skill_root(root, tenant_id=tenant_id, user_id=user_id)
        self._root.mkdir(parents=True, exist_ok=True)

    @property
    def root(self) -> Path:
        """当前 store 的物理根目录（显式 root 或按身份解析后的目录）。"""
        return self._root

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
            enabled=data.get("enabled", True),
        )
