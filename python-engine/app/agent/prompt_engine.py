"""
Prompt Engine — Assembles the system prompt from multiple context sources.

Sources:
  1. Base prompt (agent persona / task-specific system prompt)
  2. CLAUDE.md project-specific instructions
  3. Memory context (from MemoryManager)
  4. Skills context (from SkillStore)
  5. RAG context (from RAGBuilder)
  6. Git context (current branch, status, recent commits)
  7. Tool descriptions
"""
from __future__ import annotations

import logging
import os
import subprocess
from pathlib import Path
from typing import Optional

from app.agent.runtime import AgentTask
from app.memory.manager import MemoryManager
from app.skill.store import SkillStore

logger = logging.getLogger(__name__)

# ---------------------------------------------------------------------------
# Default system prompt template
# ---------------------------------------------------------------------------
DEFAULT_SYSTEM_PROMPT_TEMPLATE = """\
# System Prompt

You are an AI coding assistant with access to a rich set of tools for reading, writing,
searching, and executing code. You are precise, helpful, and proactive.

## Agent Identity & Capabilities

- You can read, write, and edit files in the project workspace.
- You can execute shell commands and scripts.
- You can search across the codebase using grep and glob patterns.
- You can access project memory (facts the user has asked you to remember).
- You can use installed skills to perform specialised tasks.
- You can retrieve relevant context from the project knowledge base (RAG).

{tools_section}

{project_context_section}

{memory_section}

{skills_section}

{rag_section}

{git_section}
"""


class PromptEngine:
    """Assembles the system prompt from multiple context sources."""

    def __init__(
        self,
        memory_manager: Optional[MemoryManager] = None,
        skill_store: Optional[SkillStore] = None,
        rag_builder=None,  # RAGBuilder is optional and loosely typed to avoid circular imports
    ):
        self._memory_manager = memory_manager
        self._skill_store = skill_store
        self._rag_builder = rag_builder

    # ------------------------------------------------------------------
    # Public API
    # ------------------------------------------------------------------

    async def assemble(self, task: AgentTask, tools: list) -> str:
        """Assemble the full system prompt for the given *task* and *tools*.

        If the task already carries a non-empty ``system_prompt`` it is used as
        the base persona; otherwise the default template is used.
        """
        root = self._resolve_root(task)

        # Gather all sections in parallel is unnecessary – they are cheap and
        # some are synchronous.  We just collect them sequentially.
        tools_section = self._format_tools(tools)
        project_context_section = self._load_claude_md(root)
        memory_section = await self._get_memory_context(task.user_id, task.content)
        skills_section = await self._get_skills_context(task.content)
        rag_section = await self._get_rag_context(task)
        git_section = self._get_git_context(root)

        # If the task provides its own system prompt, use it as the base.
        if task.system_prompt and task.system_prompt.strip():
            parts = [task.system_prompt.strip()]
            if tools_section:
                parts.append(f"\n## Available Tools\n\n{tools_section}")
            if project_context_section:
                parts.append(f"\n## Project Context (CLAUDE.md)\n\n{project_context_section}")
            if memory_section:
                parts.append(f"\n## Relevant Memories\n\n{memory_section}")
            if skills_section:
                parts.append(f"\n## Relevant Skills\n\n{skills_section}")
            if rag_section:
                parts.append(f"\n## Knowledge Base Context\n\n{rag_section}")
            if git_section:
                parts.append(f"\n## Git Context\n\n{git_section}")
            return "\n".join(parts)

        # Otherwise use the default template.
        return DEFAULT_SYSTEM_PROMPT_TEMPLATE.format(
            tools_section=tools_section,
            project_context_section=project_context_section,
            memory_section=memory_section,
            skills_section=skills_section,
            rag_section=rag_section,
            git_section=git_section,
        )

    # ------------------------------------------------------------------
    # Context loaders
    # ------------------------------------------------------------------

    def _load_claude_md(self, root: str) -> str:
        """Load CLAUDE.md from the project *root* directory.

        Returns the file contents as a string, or an empty string if the file
        does not exist.
        """
        candidates = ["CLAUDE.md", "claude.md", "Claude.md"]
        for name in candidates:
            path = Path(root) / name
            if path.is_file():
                try:
                    return path.read_text(encoding="utf-8").strip()
                except Exception as exc:
                    logger.warning("Failed to read %s: %s", path, exc)
                    return ""
        return ""

    def _get_git_context(self, root: str) -> str:
        """Return a short summary of the current git state in *root*.

        Includes the current branch name, a brief status, and the last 5
        commits.  Returns an empty string if the directory is not a git repo
        or git is unavailable.
        """
        parts: list[str] = []

        def _run(args: list[str]) -> str:
            try:
                result = subprocess.run(
                    args,
                    cwd=root,
                    capture_output=True,
                    text=True,
                    encoding="utf-8",
                    errors="replace",
                    timeout=5,
                )
                return result.stdout.strip()
            except Exception:
                return ""

        # Current branch
        branch = _run(["git", "rev-parse", "--abbrev-ref", "HEAD"])
        if branch:
            parts.append(f"**Branch:** {branch}")

        # Status (short)
        status = _run(["git", "status", "--short"])
        if status:
            parts.append(f"**Status:**\n```\n{status}\n```")

        # Recent commits
        log = _run(["git", "log", "--oneline", "-5"])
        if log:
            parts.append(f"**Recent commits:**\n```\n{log}\n```")

        if not parts:
            return ""

        return "\n\n".join(parts)

    async def _get_memory_context(self, user_id: str, query: str) -> str:
        """Retrieve relevant memories for the given *query*."""
        if self._memory_manager is None:
            return ""

        try:
            memories = await self._memory_manager.query_memory(
                tenant_id="default",
                user_id=user_id,
                query=query,
                top_k=5,
            )
        except Exception as exc:
            logger.warning("Memory query failed: %s", exc)
            return ""

        if not memories:
            return ""

        lines: list[str] = []
        for mem in memories:
            content = mem.get("content", "")
            relevance = mem.get("relevance", 0)
            mem_type = mem.get("memory_type", "unknown")
            lines.append(f"- [{mem_type}] {content} (relevance: {relevance:.2f})")
        return "\n".join(lines)

    async def _get_skills_context(self, query: str) -> str:
        """Return a summary of installed skills that may be relevant to *query*."""
        if self._skill_store is None:
            return ""

        try:
            skills = self._skill_store.list()
        except Exception as exc:
            logger.warning("Skill listing failed: %s", exc)
            return ""

        if not skills:
            return ""

        # Simple relevance: include all skills (a production version could
        # embed the query and rank by similarity).
        lines: list[str] = []
        for skill in skills:
            tags = ", ".join(skill.tags) if skill.tags else ""
            tag_str = f" [{tags}]" if tags else ""
            lines.append(f"- **{skill.name}**{tag_str}: {skill.description}")
        return "\n".join(lines)

    async def _get_rag_context(self, task: AgentTask) -> str:
        """Retrieve relevant documents from the RAG knowledge base."""
        if self._rag_builder is None:
            return ""

        # RAGBuilder.query needs a kb_id.  We use the tenant_id as a
        # convention; a production system would resolve this differently.
        kb_id = task.tenant_id or "default"
        try:
            results = await self._rag_builder.query(
                kb_id=kb_id,
                query=task.content,
                top_k=3,
                threshold=0.5,
            )
        except Exception as exc:
            logger.warning("RAG query failed: %s", exc)
            return ""

        if not results:
            return ""

        lines: list[str] = []
        for doc in results:
            snippet = doc.get("content", doc.get("text", ""))
            if len(snippet) > 500:
                snippet = snippet[:500] + "…"
            score = doc.get("score", 0)
            lines.append(f"- (score {score:.2f}) {snippet}")
        return "\n".join(lines)

    # ------------------------------------------------------------------
    # Formatting helpers
    # ------------------------------------------------------------------

    def _format_tools(self, tools: list) -> str:
        """Format tool definitions into a readable prompt section.

        Each *tool* is expected to be a dict with at least ``name`` and
        ``description`` keys (OpenAI-style or the project's internal format).
        """
        if not tools:
            return ""

        lines: list[str] = []
        for tool in tools:
            # Support both OpenAI function-calling format and flat dicts.
            if "function" in tool:
                func = tool["function"]
                name = func.get("name", "unknown")
                desc = func.get("description", "")
            else:
                name = tool.get("name", "unknown")
                desc = tool.get("description", "")

            # Truncate overly long descriptions for the system prompt.
            if len(desc) > 300:
                desc = desc[:300] + "…"

            lines.append(f"- **{name}**: {desc}")

        return "\n".join(lines)

    # ------------------------------------------------------------------
    # Internal helpers
    # ------------------------------------------------------------------

    @staticmethod
    def _resolve_root(task: AgentTask) -> str:
        """Determine the project root directory from the task context.

        Uses the ``PROMPT_ENGINE_ROOT`` env-var if set (useful in tests),
        otherwise falls back to the current working directory.
        """
        return os.environ.get("PROMPT_ENGINE_ROOT", os.getcwd())
