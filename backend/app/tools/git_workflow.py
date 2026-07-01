"""自动 Git 工作流工具 — 分支/提交/PR/Merge。"""

from __future__ import annotations

import asyncio
import logging
import tempfile
from pathlib import Path
from typing import Optional

from pydantic import BaseModel, Field

from app.core.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext

logger = logging.getLogger("minicc.git")


async def _run_git(*args: str, cwd: Path | None = None) -> str:
    """异步执行 git 命令。"""
    process = await asyncio.create_subprocess_exec(
        "git", *args,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
        cwd=cwd or Path.cwd(),
    )
    stdout, stderr = await asyncio.wait_for(process.communicate(), timeout=30)
    if process.returncode != 0:
        logger.warning("git %s: %s", args[0], stderr.decode().strip())
    return stdout.decode().strip()


class GitCreateBranchInput(BaseModel):
    branch_name: str = Field(description="New branch name")
    base_branch: str = Field(default="main", description="Base branch")


class GitCommitInput(BaseModel):
    message: str = Field(description="Commit message")
    files: Optional[list[str]] = Field(default=None, description="Files to commit (empty = all)")


class GitCreatePRInput(BaseModel):
    title: str = Field(description="PR title")
    body: Optional[str] = Field(default="", description="PR description")
    head: str = Field(description="Source branch")
    base: str = Field(default="main", description="Target branch")


class GitCreateBranchTool(BaseTool):
    name = "git_create_branch"
    description = "Create a new branch for AI agent work."
    input_schema = GitCreateBranchInput
    permission_level = PermissionLevel.EXECUTE
    category = ToolCategory.AGENT

    async def execute(self, input_data: GitCreateBranchInput, context: ToolUseContext | None = None) -> ToolResult:
        await _run_git("checkout", input_data.base_branch)
        await _run_git("pull", "origin", input_data.base_branch, "--ff-only")
        await _run_git("checkout", "-b", input_data.branch_name)
        return ToolResult(tool_call_id="", output=f"[git] Created branch: {input_data.branch_name} (from {input_data.base_branch})")


class GitCommitTool(BaseTool):
    name = "git_commit"
    description = "Stage and commit changes."
    input_schema = GitCommitInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.AGENT

    async def execute(self, input_data: GitCommitInput, context: ToolUseContext | None = None) -> ToolResult:
        if input_data.files:
            await _run_git("add", *input_data.files)
        else:
            await _run_git("add", "-A")
        diff = await _run_git("diff", "--cached", "--stat")
        await _run_git("commit", "-m", input_data.message)
        return ToolResult(
            tool_call_id="",
            output=f"[git] Committed: {input_data.message}\n{diff}",
        )


class GitCreatePRTool(BaseTool):
    name = "git_create_pr"
    description = "Create a pull request via GitHub CLI (gh)."
    input_schema = GitCreatePRInput
    permission_level = PermissionLevel.EXECUTE
    category = ToolCategory.AGENT

    async def execute(self, input_data: GitCreatePRInput, context: ToolUseContext | None = None) -> ToolResult:
        await _run_git("push", "origin", input_data.head)
        try:
            result = await _run_git("push", "origin", input_data.head)
            if "everything up-to-date" in result.lower():
                return ToolResult(tool_call_id="", output="[git] Branch already up to date, no push needed.")

            # Try gh CLI first
            gh_result = await asyncio.create_subprocess_exec(
                "gh", "pr", "create",
                "--title", input_data.title,
                "--body", input_data.body or "",
                "--head", input_data.head,
                "--base", input_data.base,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
            )
            stdout, stderr = await gh_result.communicate()
            if gh_result.returncode == 0:
                return ToolResult(tool_call_id="", output=f"[git] PR created: {stdout.decode().strip()}")
            else:
                return ToolResult(
                    tool_call_id="",
                    output=f"[git] Branch pushed ({input_data.head}). Create PR manually: https://github.com/.../compare/{input_data.base}...{input_data.head}",
                )
        except Exception as exc:
            return ToolResult(
                tool_call_id="",
                output=f"[git] Push done. Create PR at: https://github.com/.../compare/{input_data.base}...{input_data.head}",
            )


class GitStatusTool(BaseTool):
    name = "git_status"
    description = "Show current git status."
    input_schema = type("_", (), {"model_config": None})()
    permission_level = PermissionLevel.READ
    category = ToolCategory.AGENT

    async def execute(self, input_data: BaseModel, context: ToolUseContext | None = None) -> ToolResult:
        branch = await _run_git("rev-parse", "--abbrev-ref", "HEAD")
        status = await _run_git("status", "--short")
        log = await _run_git("log", "--oneline", "-5")
        lines = [f"[git] Branch: {branch}", f"  Recent commits:\n{log}" if log else ""]
        if status:
            lines.append(f"  Uncommitted ({len(status.splitlines())} files):\n{status[:500]}")
        else:
            lines.append("  Clean working tree")
        return ToolResult(tool_call_id="", output="\n".join(lines))


def register_git_tools(registry) -> None:
    registry.register(GitCreateBranchTool())
    registry.register(GitCommitTool())
    registry.register(GitCreatePRTool())
    registry.register(GitStatusTool())
