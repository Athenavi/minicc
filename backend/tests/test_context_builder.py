"""Tests: ContextBuilder context assembly system."""
from __future__ import annotations

import os
from pathlib import Path
from tempfile import TemporaryDirectory

import pytest

from app.core.context_builder import (
    ContextBuilder,
    GitState,
    MemoryProvider,
    RulesProvider,
    SystemContext,
    SystemInfo,
)


class TestSystemContext:
    def test_system_prompt_contains_basics(self):
        ctx = SystemContext()
        prompt = ctx.build_system_prompt()
        assert "MiniCC" in prompt

    def test_system_prompt_with_git(self):
        git = GitState(branch="main", is_dirty=True, unstaged_files=["test.py"])
        ctx = SystemContext(git_state=git)
        prompt = ctx.build_system_prompt()
        assert "Git" in prompt
        assert "main" in prompt

    def test_system_prompt_with_rules(self):
        ctx = SystemContext(rules="- use 4-space indent")
        prompt = ctx.build_system_prompt()
        assert "Rules" in prompt or "rules" in prompt
        assert "4-space" in prompt

    def test_system_prompt_with_memory(self):
        ctx = SystemContext(memory="- [x] DB design done")
        prompt = ctx.build_system_prompt()
        assert "Memory" in prompt or "memory" in prompt
        assert "DB design" in prompt


class TestRulesProvider:
    def test_load_rules(self):
        with TemporaryDirectory() as tmp:
            p = Path(tmp) / ".minicc"
            p.mkdir()
            (p / "rules.md").write_bytes(b"# Rules\n\n- Use TypeScript\n")
            content = RulesProvider(tmp).load()
            assert content is not None
            assert "TypeScript" in content

    def test_no_rules_file_returns_none(self):
        with TemporaryDirectory() as tmp:
            assert RulesProvider(tmp).load() is None

    def test_rules_truncation(self):
        with TemporaryDirectory() as tmp:
            p = Path(tmp) / ".minicc"
            p.mkdir()
            (p / "rules.md").write_text("\n".join(f"line {i}" for i in range(200)))
            content = RulesProvider(tmp).load(max_lines=10)
            assert content is not None
            assert "truncated" in content
            assert content.count("\n") <= 11


class TestMemoryProvider:
    def test_load_memory(self):
        with TemporaryDirectory() as tmp:
            p = Path(tmp) / ".minicc"
            p.mkdir()
            (p / "memory.md").write_bytes(b"- [x] Module A done\n- [ ] Module B pending\n")
            content = MemoryProvider(tmp).load()
            assert content is not None
            assert "Module A" in content

    def test_no_memory_file_returns_none(self):
        with TemporaryDirectory() as tmp:
            assert MemoryProvider(tmp).load() is None


class TestContextBuilder:
    @pytest.mark.asyncio
    async def test_build_context_non_git_dir(self):
        with TemporaryDirectory() as tmp:
            ctx = await ContextBuilder(tmp).build_context()
            assert ctx.git_state is None
            assert ctx.rules is None
            assert ctx.memory is None
            assert isinstance(ctx.system_info, SystemInfo)
            assert ctx.system_info.workspace_dir == os.path.abspath(tmp)

    @pytest.mark.asyncio
    async def test_build_context_with_rules_and_memory(self):
        with TemporaryDirectory() as tmp:
            p = Path(tmp) / ".minicc"
            p.mkdir()
            (p / "rules.md").write_bytes(b"# Test Rule")
            (p / "memory.md").write_bytes(b"- [x] Test done")
            ctx = await ContextBuilder(tmp).build_context()
            assert ctx.rules is not None
            assert "Test Rule" in ctx.rules
            assert ctx.memory is not None
            assert "Test done" in ctx.memory

    @pytest.mark.asyncio
    async def test_build_system_prompt(self):
        with TemporaryDirectory() as tmp:
            p = Path(tmp) / ".minicc"
            p.mkdir()
            (p / "rules.md").write_bytes(b"- use 2-space indent")
            ctx = await ContextBuilder(tmp).build_context()
            prompt = ctx.build_system_prompt()
            assert "MiniCC" in prompt
            assert "2-space" in prompt
