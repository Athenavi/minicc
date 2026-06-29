"""上下文装配系统 ContextBuilder。

每次 LLM 请求前自动收集 Git 状态、项目规则、长期记忆、系统信息，
拼装为完整的 System Prompt。参考 Claude Code Context Assembly 设计。
"""

from __future__ import annotations

import datetime
import logging
import os
import platform
import subprocess
import sys
from pathlib import Path
from typing import Optional

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


class SystemContext(BaseModel):
    """一次完整的上下文装配结果。"""
    git_state: GitState | None = None
    rules: str | None = None
    memory: str | None = None
    system_info: SystemInfo = Field(default_factory=SystemInfo)

    @property
    def system_prompt_parts(self) -> list[str]:
        parts: list[str] = []
        parts.append(f"You are MiniCC, a minimal engineering-grade AI coding assistant running on {self.system_info.os}.\n")

        # Project context
        project = [
            "## Project Context",
            f"Working directory: {self.system_info.workspace_dir}",
            f"Current time: {self.system_info.current_time}",
            f"Timezone: {self.system_info.timezone}",
        ]
        parts.append("\n".join(project))

        # Git state
        if self.git_state:
            git_lines = ["## Git Status"]
            git_lines.append(f"Branch: {self.git_state.branch}")
            git_lines.append(f"Uncommitted changes: {len(self.git_state.unstaged_files)} file(s)")
            if self.git_state.staged_files:
                git_lines.append(f"Staged: {len(self.git_state.staged_files)} file(s)")
            if self.git_state.recent_commits:
                git_lines.append("Recent commits:")
                for c in self.git_state.recent_commits:
                    git_lines.append(f"  - {c.hash[:8]} {c.message[:60]} ({c.author})")
            if self.git_state.diff_summary:
                git_lines.append(f"\nDiff summary:\n{self.git_state.diff_summary}")
            parts.append("\n".join(git_lines))

        # Project rules
        if self.rules:
            parts.append(f"## Project Rules\n{self.rules}")

        # Long-term memory
        if self.memory:
            parts.append(f"## Long-term Memory\n{self.memory}")

        return parts

    def build_system_prompt(self) -> str:
        return "\n\n".join(self.system_prompt_parts)


# ── Providers ────────────────────────────────────────────


class GitProvider:
    """Git 上下文收集 — 使用 git CLI（轻量，避免加载整个仓库）。"""

    def __init__(self, workspace_dir: str | Path) -> None:
        self.workspace = Path(workspace_dir)

    def collect(self) -> GitState | None:
        """收集 Git 状态。非 Git 目录返回 None。"""
        git_dir = self.workspace / ".git"
        if not git_dir.exists():
            return None

        state = GitState()

        try:
            state.branch = self._run("rev-parse --abbrev-ref HEAD")
            state.remote_url = self._run("remote get-url origin") or None

            # 变更状态
            status = self._run("status --porcelain")
            if status:
                state.is_dirty = True
                for line in status.splitlines():
                    if line.startswith("??") or line.startswith(" M") or line.startswith(" D"):
                        state.unstaged_files.append(line[3:])
                    elif line.startswith("M ") or line.startswith("A ") or line.startswith("D "):
                        state.staged_files.append(line[3:])

            # 最近提交
            log = self._run('log --oneline -5 --format="%h|%s|%an|%ar"')
            if log:
                for line in log.splitlines():
                    parts = line.split("|", 3)
                    if len(parts) == 4:
                        state.recent_commits.append(
                            CommitInfo(hash=parts[0], message=parts[1], author=parts[2], date=parts[3])
                        )

            # Diff 摘要
            if state.is_dirty:
                state.diff_summary = self._run("diff --stat")

        except Exception as exc:
            logger.warning("Git collection failed (non-git dir?): %s", exc)
            return None

        return state

    def _run(self, args: str) -> str:
        result = subprocess.run(
            ["git"] + args.split(),
            capture_output=True, text=True,
            cwd=self.workspace,
            timeout=10,
        )
        return result.stdout.strip()


class RulesProvider:
    """项目规则加载 — 从 .minicc/rules.md 读取。"""

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
    """长期记忆加载 — 从 .minicc/memory.md 读取。"""

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
    """上下文装配系统。

    在每次 LLM 请求前收集所有上下文信息并拼装为 System Prompt。
    """

    def __init__(self, workspace_dir: str) -> None:
        self.workspace_dir = workspace_dir
        self._git_provider = GitProvider(workspace_dir)
        self._rules_provider = RulesProvider(workspace_dir)
        self._memory_provider = MemoryProvider(workspace_dir)

    async def build_context(self) -> SystemContext:
        """收集所有上下文并返回。"""
        git_state = await self._collect_git()
        rules = await self._load_rules()
        memory = await self._load_memory()
        system_info = SystemInfo(workspace_dir=os.path.abspath(self.workspace_dir))

        return SystemContext(
            git_state=git_state,
            rules=rules,
            memory=memory,
            system_info=system_info,
        )

    async def _collect_git(self) -> GitState | None:
        return self._git_provider.collect()

    async def _load_rules(self) -> str | None:
        return self._rules_provider.load()

    async def _load_memory(self) -> str | None:
        return self._memory_provider.load()
