"""Agent 运行模式 — 声明式配置表（基于 deepseek-harness agent-presets 语义）

四种模式对应官方 preset：standard(常规) / minimal(极简) / code(PTC) / cordis(创造)。
模式 = ModeConfig（persona 策略 + 工具集 + 特殊能力 + 上下文/压缩开关），
新增模式只需在 _MODE_CONFIGS 加一条条目。mode_overrides.json（可被创造
模式的 mode_edit 工具写入）在加载时叠加覆盖。
"""
from __future__ import annotations

import json
import logging
from dataclasses import dataclass, field
from enum import Enum
from pathlib import Path
from typing import Optional

logger = logging.getLogger(__name__)

# ── 核心工具列表（Token Economy：只暴露这些给 LLM，其余按需激活） ──
CORE_TOOL_NAMES = frozenset({
    "recall",        # 检索记忆（用户偏好/事实）
    "remember",      # 保存新事实
    "skill_list",    # 列出可用技能
    "skill_run",     # 执行技能
    "web_fetch",     # 获取外部信息
    "shell_exec",    # 执行命令
    "execute_python",  # 执行 Python
    "git_status",    # 查看项目状态
    "read_file",     # 读取文件
    "write_file",    # 写入/保存文件
    "grep_files",    # 搜索文件
    "subagent",      # 子 agent 委派（多 agent 协作核心）
})

# 极简模式：仅信息读取 + 编辑 + shell（deepseek minimal 的 persistent-bash + str_replace_editor 语义）
MINIMAL_TOOL_NAMES = frozenset({"read_file", "edit_file", "shell_exec"})

# 模式额外工具
PTC_EXTRA_TOOLS = frozenset({"run_code"})
CREATIVE_EXTRA_TOOLS = frozenset({"mode_list", "mode_edit"})

# 创造模式 persona（deepseek cordis：可读取并定制运行平台的模式与技能定义）
CREATIVE_PERSONA = (
    "You are a coding agent on the MiniCC platform. "
    "You can read and modify the agent mode definitions this platform runs on: "
    "each mode is a declared configuration (persona, tool set, context policy). "
    "Use mode_list to inspect the current modes and mode_edit to adjust them "
    "(e.g. add a tool to minimal, tune a persona). Prefer reading before editing, "
    "and keep changes reversible."
)


class AgentMode(str, Enum):
    NORMAL = "normal"
    MINIMAL = "minimal"
    PTC = "ptc"
    CREATIVE = "creative"


@dataclass(frozen=True)
class ModeConfig:
    mode: AgentMode
    persona: Optional[str] = None       # None = 用现有默认 persona；str = 固定完整 persona
    include_context: bool = True        # 是否注入记忆/skills/RAG/git 上下文段
    include_tools: frozenset = frozenset(CORE_TOOL_NAMES)  # 模式可见工具
    extra_tools: frozenset = frozenset()  # 模式额外注册的工具
    enable_compaction: bool = True      # 是否启用上下文压缩
    compaction: Optional[dict] = None   # SaaS：截断策略配置（strategy/max_messages/max_context_tokens/
                                        #        threshold_ratio/snipe_ratio/tool_result_max_chars 等），
                                        #        租户/模式可手动确认；None = 默认策略


_BASE_MODES: dict[AgentMode, ModeConfig] = {
    AgentMode.NORMAL: ModeConfig(mode=AgentMode.NORMAL),
    AgentMode.MINIMAL: ModeConfig(
        mode=AgentMode.MINIMAL,
        persona="You are a helpful software engineer assistant.",
        include_context=False,
        include_tools=frozenset(MINIMAL_TOOL_NAMES),
        enable_compaction=False,
    ),
    AgentMode.PTC: ModeConfig(
        mode=AgentMode.PTC,
        include_tools=frozenset(CORE_TOOL_NAMES),
        extra_tools=frozenset(PTC_EXTRA_TOOLS),
    ),
    AgentMode.CREATIVE: ModeConfig(
        mode=AgentMode.CREATIVE,
        persona=CREATIVE_PERSONA,
        include_tools=frozenset(CORE_TOOL_NAMES),
        extra_tools=frozenset(CREATIVE_EXTRA_TOOLS),
    ),
}


def _overrides_path() -> Path:
    return Path(__file__).resolve().parent / "mode_overrides.json"


def _load_overrides() -> dict:
    try:
        return json.loads(_overrides_path().read_text(encoding="utf-8"))
    except (FileNotFoundError, json.JSONDecodeError):
        return {}


def _apply_overrides(cfg: ModeConfig, overrides: dict) -> ModeConfig:
    """按 overrides 字段合并（persona/include_context/include_tools/extra_tools/enable_compaction/compaction）。"""
    o = overrides.get(cfg.mode.value)
    if not isinstance(o, dict) or not o:
        return cfg
    return ModeConfig(
        mode=cfg.mode,
        persona=o.get("persona", cfg.persona),
        include_context=o.get("include_context", cfg.include_context),
        include_tools=frozenset(o.get("include_tools", list(cfg.include_tools))),
        extra_tools=frozenset(o.get("extra_tools", list(cfg.extra_tools))),
        enable_compaction=o.get("enable_compaction", cfg.enable_compaction),
        compaction=o.get("compaction", cfg.compaction),
    )


def get_mode_config(mode: Optional[str]) -> ModeConfig:
    """未知/空模式回退 NORMAL；叠加 mode_overrides.json。"""
    try:
        base = _BASE_MODES[AgentMode(mode or "normal")]
    except (ValueError, KeyError):
        base = _BASE_MODES[AgentMode.NORMAL]
    try:
        return _apply_overrides(base, _load_overrides())
    except Exception as e:
        logger.warning("mode overrides load failed: %s", e)
        return base
