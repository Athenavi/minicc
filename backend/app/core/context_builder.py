"""上下文装配系统 ContextBuilder — MiniCC 的分层提示词工程。

参考 Claude Code 的 prompts.ts 设计：
不要把 prompt 理解成"一大段总提示词"，
而是一整套分层装配的提示词系统：

1. 基础身份与行为约束（getSimpleIntroSection）
2. 动态 section：session_guidance / memory / env_info / language
3. 用户自定义 systemPrompt / appendSystemPrompt
4. 工具级 prompt（每个工具自带使用指南）
5. 专项子系统 prompt（记忆筛选、工具总结等）
"""

from __future__ import annotations

import datetime
import logging
import os
import platform
import subprocess
import sys
from pathlib import Path
from typing import Callable, Optional

from pydantic import BaseModel, Field

logger = logging.getLogger("minicc.context")


# ── 数据模型 ─────────────────────────────────────────────


class CommitInfo(BaseModel):
    hash: str
    message: str
    author: str
    date: str


class GitState(BaseModel):
    branch: str = "unknown"
    is_dirty: bool = False
    unstaged_files: list[str] = Field(default_factory=list)
    staged_files: list[str] = Field(default_factory=list)
    recent_commits: list[CommitInfo] = Field(default_factory=list)
    remote_url: str | None = None
    diff_summary: str | None = None


class SystemInfo(BaseModel):
    os: str = Field(default_factory=lambda: platform.platform())
    python_version: str = Field(default_factory=lambda: sys.version.split()[0])
    current_time: str = Field(
        default_factory=lambda: datetime.datetime.now(datetime.timezone.utc).isoformat()
    )
    timezone: str = Field(
        default_factory=lambda: str(datetime.datetime.now().astimezone().tzinfo)
    )
    workspace_dir: str = "."


# ── Prompt Section 系统 ──────────────────────────────────
#
# 对应 Claude Code 的 dynamicSections 数组：
# session_guidance / memory / env_info / language / output_style / mcp_instructions


class PromptSection:
    """一个可动态装配的 prompt 片段。

    每个 section 有：
    - name: 唯一标识符
    - render(): 返回 prompt 文本（或 None 跳过）
    - cache_key: 缓存键（相同内容跳过重复渲染）
    """

    def __init__(self, name: str, render: Callable[[], str | None], cache_key: str = "") -> None:
        self.name = name
        self._render = render
        self._cache_key = cache_key or name

    def render(self) -> str | None:
        return self._render()

    @property
    def cache_key(self) -> str:
        return self._cache_key


class PromptBuilder:
    """提示词装配器 — 将多个 PromptSection 按顺序拼装为最终 System Prompt。

    对应 Claude Code 的 assembleSystemPrompt() 逻辑：
    1. 基础身份描述
    2. 动态 sections
    3. 工具级 prompt
    4. 用户自定义替换/追加
    """

    def __init__(self) -> None:
        self._sections: list[PromptSection] = []
        self._tool_prompts: list[str] = []
        self._custom_system_prompt: str | None = None
        self._append_system_prompt: str | None = None
        self._intro_section: str = ""

    def set_intro(self, text: str) -> None:
        """设置基础身份描述（对应 getSimpleIntroSection）。"""
        self._intro_section = text

    def add_section(self, section: PromptSection) -> None:
        """注册一个动态 section。"""
        self._sections.append(section)

    def add_tool_prompt(self, prompt: str) -> None:
        """注册一个工具级 prompt。"""
        if prompt:
            self._tool_prompts.append(prompt)

    def set_custom_system_prompt(self, prompt: str | None) -> None:
        """设置自定义替换 prompt（对应 --system-prompt）。"""
        self._custom_system_prompt = prompt

    def set_append_system_prompt(self, prompt: str | None) -> None:
        """设置追加 prompt（对应 --append-system-prompt）。"""
        self._append_system_prompt = prompt

    def build(self) -> str:
        """装配完整的 System Prompt。

        如果 customSystemPrompt 已设置，直接返回它（完全替换模式）。
        否则按标准流程拼装。
        """
        if self._custom_system_prompt:
            return self._custom_system_prompt

        parts: list[str] = []

        # 1. 基础身份描述
        if self._intro_section:
            parts.append(self._intro_section)

        # 2. 动态 sections
        for section in self._sections:
            text = section.render()
            if text:
                parts.append(text)

        # 3. 工具级 prompt
        if self._tool_prompts:
            parts.append("## Available Tools\n" + "\n\n".join(self._tool_prompts))

        # 4. 用户追加（append mode）
        if self._append_system_prompt:
            parts.append(self._append_system_prompt)

        return "\n\n".join(parts)


# ── Section 工厂函数 ─────────────────────────────────────


def make_intro_section() -> str:
    """基础身份描述。

    对应 Claude Code 的 getSimpleIntroSection()。
    定义了 Agent 的身份、任务域、工具可用性、安全边界。
    """
    return (
        "You are MiniCC, an interactive AI coding assistant that helps users "
        "with software engineering tasks. Use the instructions below and the "
        "tools available to you to assist the user.\n\n"
        "IMPORTANT: You must NEVER generate or guess URLs unless you are "
        "confident they help with a programming task. Always explain your "
        "reasoning before taking actions."
    )


def make_session_guidance_section(tool_names: list[str] | None = None) -> str:
    """会话指导 section。

    对应 Claude Code 的 getSessionSpecificGuidanceSection()。
    指导模型如何正确使用工具、遵循审批流程。
    """
    lines = [
        "## Session Guidance",
        "",
        "### Tool Usage Rules",
        "- Read a file first before editing it — understand the current code.",
        "- When a dedicated tool exists for a task, use it instead of bash.",
        f"  Available tools: {', '.join(tool_names) if tool_names else 'check /api/tools'}",
        "- For file edits, use str_replace_editor for small, targeted changes.",
        "- For new files or large rewrites, use write_to_file.",
        "- Always explain what a shell command does before running it.",
        "",
        "### Approval Process",
        "- Read operations (read_file, grep, glob): auto-approved.",
        "- Write operations (write_to_file, str_replace_editor): require your approval.",
        "- Execute operations (bash): require strict approval.",
        "- If a tool call is rejected, explain why and suggest alternatives.",
        "",
        "### Output Rules",
        "- Show the diff of every file change.",
        "- After completing a task, summarize what was done.",
        "- If you're unsure about something, ask the user.",
    ]
    return "\n".join(lines)


def make_env_info_section(info: SystemInfo) -> str:
    """环境信息 section。

    对应 Claude Code 的 computeSimpleEnvInfo()。
    """
    return (
        "## Environment\n"
        f"- OS: {info.os}\n"
        f"- Python: {info.python_version}\n"
        f"- Working directory: {info.workspace_dir}\n"
        f"- Current time: {info.current_time}\n"
        f"- Timezone: {info.timezone}"
    )


def make_git_section(git: GitState | None) -> str | None:
    """Git 状态 section。"""
    if not git:
        return None
    lines = ["## Git Status"]
    lines.append(f"Branch: {git.branch}")
    lines.append(f"Uncommitted changes: {len(git.unstaged_files)} file(s)")
    if git.staged_files:
        lines.append(f"Staged: {len(git.staged_files)} file(s)")
    if git.recent_commits:
        lines.append("Recent commits:")
        for c in git.recent_commits:
            lines.append(f"  - {c.hash[:8]} {c.message[:60]} ({c.author})")
    if git.diff_summary:
        lines.append(f"\nDiff summary:\n{git.diff_summary}")
    return "\n".join(lines)


def make_memory_section(memory: str | None) -> str | None:
    """长期记忆 section。"""
    if not memory:
        return None
    return f"## Long-term Memory\n{memory}"


def make_rules_section(rules: str | None) -> str | None:
    """项目规则 section。"""
    if not rules:
        return None
    return f"## Project Rules\n{rules}"


# ── Providers ────────────────────────────────────────────


class GitProvider:
    """Git 上下文收集 — 使用 git CLI。"""

    def __init__(self, workspace_dir: str | Path) -> None:
        self.workspace = Path(workspace_dir)

    def collect(self) -> GitState | None:
        git_dir = self.workspace / ".git"
        if not git_dir.exists():
            return None

        state = GitState()
        try:
            state.branch = self._run("rev-parse --abbrev-ref HEAD")
            state.remote_url = self._run("remote get-url origin") or None
            status = self._run("status --porcelain")
            if status:
                state.is_dirty = True
                for line in status.splitlines():
                    if line.startswith("??") or line.startswith(" M") or line.startswith(" D"):
                        state.unstaged_files.append(line[3:])
                    elif line.startswith("M ") or line.startswith("A ") or line.startswith("D "):
                        state.staged_files.append(line[3:])
            log = self._run('log --oneline -5 --format="%h|%s|%an|%ar"')
            if log:
                for line in log.splitlines():
                    parts = line.split("|", 3)
                    if len(parts) == 4:
                        state.recent_commits.append(CommitInfo(hash=parts[0], message=parts[1], author=parts[2], date=parts[3]))
            if state.is_dirty:
                state.diff_summary = self._run("diff --stat")
        except Exception as exc:
            logger.warning("Git collection failed: %s", exc)
            return None
        return state

    def _run(self, args: str) -> str:
        result = subprocess.run(["git"] + args.split(), capture_output=True, text=True, cwd=self.workspace, timeout=10)
        return result.stdout.strip()


class RulesProvider:
    def __init__(self, workspace_dir: str | Path) -> None:
        self.path = Path(workspace_dir) / ".minicc" / "rules.md"

    def load(self, max_lines: int = 100) -> str | None:
        if not self.path.exists():
            return None
        content = self.path.read_text(encoding="utf-8")
        lines = content.splitlines()
        if len(lines) > max_lines:
            lines = lines[:max_lines]
            lines.append(f"\n... [truncated: {len(content.splitlines())} lines total, showing first {max_lines}]")
        return "\n".join(lines)


class MemoryProvider:
    def __init__(self, workspace_dir: str | Path) -> None:
        self.path = Path(workspace_dir) / ".minicc" / "memory.md"

    def load(self, max_lines: int = 50) -> str | None:
        if not self.path.exists():
            return None
        content = self.path.read_text(encoding="utf-8")
        lines = content.splitlines()
        if len(lines) > max_lines:
            lines = lines[:max_lines]
            lines.append(f"\n... [truncated: {len(content.splitlines())} lines total, showing first {max_lines}]")
        return "\n".join(lines)


# ── ContextBuilder ───────────────────────────────────────


class ContextBuilder:
    """上下文装配系统 — 分层提示词生成。

    对应 Claude Code 的 context.ts / prompts.ts：
    - 基础身份描述（intro）
    - 动态 sections（session_guidance, memory, env, git, rules）
    - 工具级 prompts（由外部注入）
    - 用户自定义替换/追加
    """

    def __init__(self, workspace_dir: str) -> None:
        self.workspace_dir = workspace_dir
        self._git_provider = GitProvider(workspace_dir)
        self._rules_provider = RulesProvider(workspace_dir)
        self._memory_provider = MemoryProvider(workspace_dir)

        # 外部注入
        self._tool_prompts: list[str] = []
        self._custom_system_prompt: str | None = None
        self._append_system_prompt: str | None = None

    def add_tool_prompt(self, prompt: str) -> None:
        if prompt:
            self._tool_prompts.append(prompt)

    def set_custom_system_prompt(self, prompt: str | None) -> None:
        self._custom_system_prompt = prompt

    def set_append_system_prompt(self, prompt: str | None) -> None:
        self._append_system_prompt = prompt

    async def build_prompt(
        self,
        tool_names: list[str] | None = None,
    ) -> str:
        """构建完整的 System Prompt。

        对应 Claude Code 的 assembleSystemPrompt() 流程。
        """
        builder = PromptBuilder()

        # 用户自定义替换模式
        if self._custom_system_prompt:
            builder.set_custom_system_prompt(self._custom_system_prompt)
            return builder.build()

        # 1. 基础身份
        builder.set_intro(make_intro_section())

        # 2. 数据收集
        git_state = await self._collect_git()
        rules = await self._load_rules()
        memory = await self._load_memory()
        system_info = SystemInfo(workspace_dir=os.path.abspath(self.workspace_dir))

        # 3. 动态 sections
        builder.add_section(PromptSection("session_guidance", lambda: make_session_guidance_section(tool_names)))
        builder.add_section(PromptSection("env_info", lambda: make_env_info_section(system_info)))
        builder.add_section(PromptSection("git", lambda: make_git_section(git_state)))
        builder.add_section(PromptSection("rules", lambda: make_rules_section(rules)))
        builder.add_section(PromptSection("memory", lambda: make_memory_section(memory)))

        # 4. 工具级 prompts
        for tp in self._tool_prompts:
            builder.add_tool_prompt(tp)

        # 5. 用户追加
        builder.set_append_system_prompt(self._append_system_prompt)

        return builder.build()

    async def _collect_git(self) -> GitState | None:
        return self._git_provider.collect()

    async def _load_rules(self) -> str | None:
        return self._rules_provider.load()

    async def _load_memory(self) -> str | None:
        return self._memory_provider.load()
