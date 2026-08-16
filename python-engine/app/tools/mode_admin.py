"""mode_admin 工具 — 创造模式（对应 deepseek-harness cordis preset）

让 agent 能检查并定制本平台的 agent 模式定义：
- mode_list: 列出全部模式的当前生效配置（含 overrides 叠加后结果）
- mode_edit: 按字段合并写入 mode_overrides.json（persona / include_context /
  include_tools / extra_tools / enable_compaction），runtime 加载时叠加覆盖

写入是显式的、可逆的：mode_edit 返回修改后完整 diff；mode_overrides.json
可被删除以回退默认。与 deepseek cordis 的 preset 创作同语义（轻量实现）。
"""
from __future__ import annotations

import json
import logging
from typing import Any

from app.agent.modes import (
    _BASE_MODES,
    _overrides_path,
    AgentMode,
)
from app.tools.registry import registry

logger = logging.getLogger(__name__)

EDITABLE_FIELDS = {"persona", "include_context", "include_tools", "extra_tools", "enable_compaction"}
VALID_MODES = {m.value for m in AgentMode}


def _mode_config_dict(mode_value: str) -> dict[str, Any]:
    from app.agent.modes import get_mode_config
    cfg = get_mode_config(mode_value)
    return {
        "mode": cfg.mode.value,
        "persona": cfg.persona,
        "include_context": cfg.include_context,
        "include_tools": sorted(cfg.include_tools),
        "extra_tools": sorted(cfg.extra_tools),
        "enable_compaction": cfg.enable_compaction,
    }


async def mode_list() -> dict[str, Any]:
    """List all agent modes with their effective (overrides-applied) configuration."""
    return {"modes": {m.value: _mode_config_dict(m.value) for m in AgentMode}}


async def mode_edit(mode: str, patch: dict[str, Any]) -> dict[str, Any]:
    """Merge *patch* fields into the mode's overrides.

    Allowed fields: persona, include_context, include_tools, extra_tools,
    enable_compaction. Writes mode_overrides.json; returns the updated mode.
    """
    if mode not in VALID_MODES:
        return {"error": f"unknown mode '{mode}'; valid: {sorted(VALID_MODES)}"}
    if not isinstance(patch, dict) or not patch:
        return {"error": "patch is required"}

    unknown = set(patch) - EDITABLE_FIELDS
    if unknown:
        return {"error": f"unsupported fields: {sorted(unknown)}; allowed: {sorted(EDITABLE_FIELDS)}"}

    path = _overrides_path()
    overrides: dict[str, Any] = {}
    try:
        overrides = json.loads(path.read_text(encoding="utf-8")) if path.exists() else {}
    except json.JSONDecodeError:
        return {"error": "mode_overrides.json is corrupt; fix or remove it before editing"}

    current = overrides.get(mode, {})
    for field, value in patch.items():
        if field in ("include_tools", "extra_tools"):
            if not isinstance(value, list) or not all(isinstance(v, str) for v in value):
                return {"error": f"{field} must be a list of tool name strings"}
            current[field] = value
        elif field == "persona":
            if not isinstance(value, str):
                return {"error": "persona must be a string"}
            current[field] = value
        elif field == "include_context" or field == "enable_compaction":
            if not isinstance(value, bool):
                return {"error": f"{field} must be a boolean"}
            current[field] = value
        else:
            return {"error": f"unsupported field: {field}"}

    overrides[mode] = current
    path.write_text(json.dumps(overrides, ensure_ascii=False, indent=2), encoding="utf-8")
    logger.info("mode_edit: mode=%s patch=%s", mode, patch)

    return {
        "updated": True,
        "mode": mode,
        "effective": _mode_config_dict(mode),
        "note": "Changes apply to the next agent run; delete mode_overrides.json to reset.",
    }


# ── Register ─────────────────────────────────────────────────────
registry.register(
    name="mode_list",
    description="List all agent modes (normal/minimal/ptc/creative) with their effective configuration: persona, context injection, tool set, compaction.",
    parameters={"type": "object", "properties": {}},
    handler=mode_list,
)

registry.register(
    name="mode_edit",
    description=(
        "Edit an agent mode definition by merging fields into mode_overrides.json. "
        "Fields: persona (str), include_context (bool), include_tools (list[str]), "
        "extra_tools (list[str]), enable_compaction (bool). Changes apply to the "
        "next agent run; delete mode_overrides.json to reset to defaults. "
        "Read mode_list first."
    ),
    parameters={
        "type": "object",
        "properties": {
            "mode": {
                "type": "string",
                "enum": sorted(VALID_MODES),
                "description": "Mode to edit",
            },
            "patch": {
                "type": "object",
                "description": "Fields to merge",
                "additionalProperties": True,
            },
        },
        "required": ["mode", "patch"],
    },
    handler=mode_edit,
)
