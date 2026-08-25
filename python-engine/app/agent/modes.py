"""Agent 杩愯妯″紡 鈥?澹版槑寮忛厤缃〃锛堝熀浜?deepseek-harness agent-presets 璇箟锛?
鍥涚妯″紡瀵瑰簲瀹樻柟 preset锛歴tandard(甯歌) / minimal(鏋佺畝) / code(PTC) / cordis(鍒涢€?銆?妯″紡 = ModeConfig锛坧ersona 绛栫暐 + 宸ュ叿闆?+ 鐗规畩鑳藉姏 + 涓婁笅鏂?鍘嬬缉寮€鍏筹級锛?鏂板妯″紡鍙渶鍦?_MODE_CONFIGS 鍔犱竴鏉℃潯鐩€俶ode_overrides.json锛堝彲琚垱閫?妯″紡鐨?mode_edit 宸ュ叿鍐欏叆锛夊湪鍔犺浇鏃跺彔鍔犺鐩栥€?"""
from __future__ import annotations

import json
import logging
from dataclasses import dataclass, field
from enum import Enum
from pathlib import Path
from typing import Optional

logger = logging.getLogger(__name__)

# 鈹€鈹€ 鏍稿績宸ュ叿鍒楄〃锛圱oken Economy锛氬彧鏆撮湶杩欎簺缁?LLM锛屽叾浣欐寜闇€婵€娲伙級 鈹€鈹€
CORE_TOOL_NAMES = frozenset({
    "recall",        # 妫€绱㈣蹇嗭紙鐢ㄦ埛鍋忓ソ/浜嬪疄锛?    "remember",      # 淇濆瓨鏂颁簨瀹?    "skill_list",    # 鍒楀嚭鍙敤鎶€鑳?    "skill_run",     # 鎵ц鎶€鑳?    "web_fetch",     # 鑾峰彇澶栭儴淇℃伅
    "shell_exec",    # 鎵ц鍛戒护
    "execute_python",  # 鎵ц Python
    "git_status",    # 鏌ョ湅椤圭洰鐘舵€?    "read_file",     # 璇诲彇鏂囦欢
    "write_file",    # 鍐欏叆/淇濆瓨鏂囦欢
    "grep_files",    # 鎼滅储鏂囦欢
    "subagent",      # 瀛?agent 濮旀淳锛堝 agent 鍗忎綔鏍稿績锛?})

# 鏋佺畝妯″紡锛氫粎淇℃伅璇诲彇 + 缂栬緫 + shell锛坉eepseek minimal 鐨?persistent-bash + str_replace_editor 璇箟锛?MINIMAL_TOOL_NAMES = frozenset({"read_file", "edit_file", "shell_exec"})

# 妯″紡棰濆宸ュ叿
PTC_EXTRA_TOOLS = frozenset({"run_code"})
CREATIVE_EXTRA_TOOLS = frozenset({"mode_list", "mode_edit"})

# 鍒涢€犳ā寮?persona锛坉eepseek cordis锛氬彲璇诲彇骞跺畾鍒惰繍琛屽钩鍙扮殑妯″紡涓庢妧鑳藉畾涔夛級
CREATIVE_PERSONA = (
    "You are a coding agent on the Chiron platform. "
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
    persona: Optional[str] = None       # None = 鐢ㄧ幇鏈夐粯璁?persona锛泂tr = 鍥哄畾瀹屾暣 persona
    include_context: bool = True        # 鏄惁娉ㄥ叆璁板繂/skills/RAG/git 涓婁笅鏂囨
    include_tools: frozenset = frozenset(CORE_TOOL_NAMES)  # 妯″紡鍙宸ュ叿
    extra_tools: frozenset = frozenset()  # 妯″紡棰濆娉ㄥ唽鐨勫伐鍏?    enable_compaction: bool = True      # 鏄惁鍚敤涓婁笅鏂囧帇缂?    compaction: Optional[dict] = None   # SaaS锛氭埅鏂瓥鐣ラ厤缃紙strategy/max_messages/max_context_tokens/
                                        #        threshold_ratio/snipe_ratio/tool_result_max_chars 绛夛級锛?                                        #        绉熸埛/妯″紡鍙墜鍔ㄧ‘璁わ紱None = 榛樿绛栫暐


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
    """鎸?overrides 瀛楁鍚堝苟锛坧ersona/include_context/include_tools/extra_tools/enable_compaction/compaction锛夈€?""
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
    """鏈煡/绌烘ā寮忓洖閫€ NORMAL锛涘彔鍔?mode_overrides.json銆?""
    try:
        base = _BASE_MODES[AgentMode(mode or "normal")]
    except (ValueError, KeyError):
        base = _BASE_MODES[AgentMode.NORMAL]
    try:
        return _apply_overrides(base, _load_overrides())
    except Exception as e:
        logger.warning("mode overrides load failed: %s", e)
        return base

