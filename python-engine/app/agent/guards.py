"""guards — agent 输入/工具/输出三栅栏。

对照实现：
- OpenHands SecurityAnalyzer（工具调用前风险分级）
- NeMo Guardrails（input rails / execution rails / output rails）
- OpenAI Agents SDK guardrails（input guardrail / tool guardrail / output guardrail）

设计：
- InputGuard   输入栅栏：用户消息注入模式检测 → 违规拒绝该轮（guardrail_blocked）
- ToolGuard    工具栅栏：每次工具调用前三态（block 拒绝 / sanitize 替换 / confirm 确认）
- OutputGuard  输出栅栏：流式 text/thinking 逐 chunk 脱敏（宿主路径/secret 替换）+ 计数阈值
"""
from __future__ import annotations

import re
from dataclasses import dataclass, field
from typing import Any

# ═══════════════════════════════ 输入栅栏 ═══════════════════════════════

# Prompt Injection 模式（与 Go 网关 internal/api/security.go 对齐）
INJECTION_PATTERNS: list[str] = [
    r"(?i)ignore\s+(all\s+)?(previous|above|prior|earlier)\s+instructions",
    r"(?i)forget\s+(everything|all|previous)",
    r"(?i)you\s+are\s+(now|free|not\s+bound|unrestricted)",
    r"(?i)system\s+prompt",
    r"(?i)NEW\s+INSTRUCTIONS?",
    r"(?i)disregard\s+(all|previous|above)",
    r"(?i)override\s+(safety|instructions|rules)",
    r"(?i)act\s+as\s+if\s+you\s+(have|are)\s+no\s+restrictions",
    r"(?i)pretend\s+you\s+are\s+(not|an?\s+unrestricted)",
    r"(?i)bypass\s+(all|safety|content)\s+(filters?|restrictions?|rules?)",
]
_INJECTION_RES: list[re.Pattern] = [re.compile(p) for p in INJECTION_PATTERNS]


class InputGuard:
    """输入栅栏：检测用户消息中的注入模式。"""

    def check(self, text: str) -> str | None:
        """违规返回匹配的模式，否则返回 None。"""
        for pat in _INJECTION_RES:
            m = pat.search(text or "")
            if m:
                return pat.pattern
        return None


# ═══════════════════════════════ 工具栅栏 ═══════════════════════════════

# 参数中禁止出现的 secret 模式（照 OpenAI Agents SDK `block_secrets` 示例）
SECRET_PARAM_PATTERNS: list[str] = [
    r"sk-[a-zA-Z0-9]{16,}",
    r"(?i)(api[_-]?key|password|passwd|secret|token)\s*[:=]\s*['\"]?[A-Za-z0-9_\-\.]{12,}",
    r"(?i)(AKIA|ASIA)[A-Z0-9]{16}",
]
_SECRET_RES: list[re.Pattern] = [re.compile(p) for p in SECRET_PARAM_PATTERNS]

# 工具参数中禁止的宿主逃逸模式（绝对路径/盘符/父目录跳转）
TOOL_ARG_ESCAPE_PATTERNS: list[str] = [
    r"[A-Za-z]:[\\/]",                          # Windows 盘符绝对路径
    r"(^|[^A-Za-z0-9_.])(\.\.)[\\/]",           # 父目录跳转
]
_ARG_ESCAPE_RES: list[re.Pattern] = [re.compile(p) for p in TOOL_ARG_ESCAPE_PATTERNS]

# 需要用户确认的危险工具
DANGEROUS_TOOLS: frozenset[str] = frozenset({
    "shell_exec", "execute_command", "persistent_shell", "execute_python",
    "run_code", "browser_navigate", "browser_click", "browser_type",
    "browser_screenshot", "browser_close", "browser_refresh",
    "web_fetch", "web_search", "skill_install",
})


@dataclass
class ToolVerdict:
    """工具调用三态裁决。"""
    action: str  # "block" | "sanitize" | "confirm" | "allow"
    reason: str = ""
    sanitized_args: dict[str, Any] | None = None
    risk_level: str = "low"


class ToolGuard:
    """工具栅栏：每次工具调用前评估，返回三态裁决。

    优先级：secret 参数 → block；命令逃逸 → sanitize（替换为安全值）或 block；
    危险工具 → confirm；其余 → allow。
    """

    def evaluate(self, tool_name: str, args: dict[str, Any]) -> ToolVerdict:
        # 1. secret 参数 → 直接拒绝
        for key, value in (args or {}).items():
            if isinstance(value, str):
                for pat in _SECRET_RES:
                    if pat.search(value):
                        return ToolVerdict(
                            "block",
                            reason=f"argument '{key}' may contain a secret (matched {pat.pattern})",
                            risk_level="high",
                        )

        # 2. 命令/路径逃逸 → 替换或拒绝
        sanitized: dict[str, Any] | None = None
        for key in ("command", "path", "root", "cwd"):
            value = args.get(key)
            if not isinstance(value, str) or not value:
                continue
            if _ARG_ESCAPE_RES and any(r.search(value) for r in _ARG_ESCAPE_RES):
                # 绝对路径/父目录跳转：无法安全替换的 shell 命令 → 拒绝；
                # path 字段替换为沙箱相对路径由 sandbox.safe_join 兜底 → 直接 block（提示 agent 用相对路径）
                return ToolVerdict(
                    "block",
                    reason=f"argument '{key}' contains an absolute path or parent-directory escape",
                    risk_level="high",
                )

        # 3. 危险工具 → 需要用户确认
        if tool_name in DANGEROUS_TOOLS:
            return ToolVerdict("confirm", reason=f"tool '{tool_name}' requires user approval", risk_level="high")

        return ToolVerdict("allow", risk_level="low")


# ═══════════════════════════════ 输出栅栏 ═══════════════════════════════

# 宿主路径/敏感信息模式（出现即替换）
HOST_PATH_PATTERNS: list[str] = [
    r"[A-Za-z]:[\\/](?:[^\\/\"'\s<>]+[\\/])*",   # Windows 绝对路径（C:\a\b\...）
    r"python-engine[\\/]",
]
_HOST_PATH_RES: list[re.Pattern] = [re.compile(p) for p in HOST_PATH_PATTERNS]

OUT_SECRET_PATTERNS: list[str] = [
    r"sk-[a-zA-Z0-9]{16,}",
    r"(?i)(api[_-]?key|password|passwd|secret|token)\s*[:=]\s*['\"]?[A-Za-z0-9_\-\.]{12,}",
]
_OUT_SECRET_RES: list[re.Pattern] = [re.compile(p) for p in OUT_SECRET_PATTERNS]

# 占位符
HOST_PATH_PLACEHOLDER = "[host-path]"
SECRET_PLACEHOLDER = "[redacted]"


class OutputGuard:
    """输出栅栏：流式逐 chunk 脱敏 + 计数阈值。

    - 宿主绝对路径 → `[host-path]`
    - secret → `[redacted]`
    - 替换命中次数超过 ``max_hits`` 后 ``blocked`` 置位（调用方应截断输出）。
    """

    def __init__(self, max_hits: int = 10):
        self.max_hits = max_hits
        self.hits: list[str] = []  # 记录命中原因
        self._blocked = False

    @property
    def blocked(self) -> bool:
        return self._blocked

    def sanitize(self, text: str) -> str:
        """对文本做脱敏替换；命中计数超过阈值后置 blocked。"""
        if not text:
            return text
        for pat in _HOST_PATH_RES:
            text = pat.sub(HOST_PATH_PLACEHOLDER, text)
        for pat in _OUT_SECRET_RES:
            text = pat.sub(SECRET_PLACEHOLDER, text)
        # 计数阈值（近似：出现占位符即算一次命中）
        for _ in range(text.count(HOST_PATH_PLACEHOLDER) + text.count(SECRET_PLACEHOLDER)):
            self.hits.append("leak")
        if len(self.hits) >= self.max_hits:
            self._blocked = True
        return text

    def reset(self) -> None:
        self.hits.clear()
        self._blocked = False
