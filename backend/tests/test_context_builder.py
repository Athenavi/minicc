"""Tests: ContextBuilder — layered prompt assembly system."""

from __future__ import annotations

import os
from pathlib import Path
from tempfile import TemporaryDirectory

import pytest

from app.core.context_builder import (
    ContextBuilder,
    GitState,
    MemoryProvider,
    PromptBuilder,
    PromptSection,
    RulesProvider,
    SystemInfo,
    make_env_info_section,
    make_git_section,
    make_intro_section,
    make_session_guidance_section,
)

pytestmark = pytest.mark.asyncio


class TestPromptBuilder:
    def test_basic_assembly(self):
        builder = PromptBuilder()
        builder.set_intro("You are MiniCC.")
        prompt = builder.build()
        assert "MiniCC" in prompt

    def test_with_sections(self):
        builder = PromptBuilder()
        builder.set_intro("Intro.")
        builder.add_section(PromptSection("test", lambda: "## Test\ncontent"))
        prompt = builder.build()
        assert "Intro." in prompt
        assert "Test" in prompt

    def test_custom_replaces_all(self):
        builder = PromptBuilder()
        builder.set_intro("Intro.")
        builder.set_custom_system_prompt("Custom prompt only")
        prompt = builder.build()
        assert prompt == "Custom prompt only"
        assert "Intro." not in prompt

    def test_append_adds_at_end(self):
        builder = PromptBuilder()
        builder.set_intro("Intro.")
        builder.set_append_system_prompt("Extra rules.")
        prompt = builder.build()
        assert "Extra rules." in prompt
        assert prompt.endswith("Extra rules.")

    def test_tool_prompts(self):
        builder = PromptBuilder()
        builder.set_intro("Intro.")
        builder.add_tool_prompt("Tool1: do X")
        builder.add_tool_prompt("Tool2: do Y")
        prompt = builder.build()
        assert "Tool1" in prompt
        assert "Tool2" in prompt
        assert "Available Tools" in prompt

    def test_empty_build(self):
        builder = PromptBuilder()
        prompt = builder.build()
        assert prompt == ""


class TestPromptSections:
    def test_intro_contains_identity(self):
        text = make_intro_section()
        assert "MiniCC" in text
        assert "software engineering" in text

    def test_session_guidance_contains_rules(self):
        text = make_session_guidance_section(["read_file", "bash"])
        assert "Tool Usage" in text
        assert "read_file" in text
        assert "bash" in text
        assert "Approval" in text

    def test_env_info(self):
        info = SystemInfo(workspace_dir="/test")
        text = make_env_info_section(info)
        assert "/test" in text
        assert "Python" in text

    def test_git_section(self):
        git = GitState(branch="main", is_dirty=True, unstaged_files=["a.py"])
        text = make_git_section(git)
        assert text is not None
        assert "main" in text
        assert "1 file" in text

    def test_git_section_none(self):
        assert make_git_section(None) is None

    def test_memory_section(self):
        from app.core.context_builder import make_memory_section
        assert make_memory_section(None) is None
        text = make_memory_section("- [x] done")
        assert text is not None
        assert "done" in text


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


class TestMemoryProvider:
    def test_load_memory(self):
        with TemporaryDirectory() as tmp:
            p = Path(tmp) / ".minicc"
            p.mkdir()
            (p / "memory.md").write_bytes(b"- [x] Module A done\n")
            content = MemoryProvider(tmp).load()
            assert content is not None
            assert "Module A" in content

    def test_no_memory_file_returns_none(self):
        with TemporaryDirectory() as tmp:
            assert MemoryProvider(tmp).load() is None


class TestContextBuilder:
    async def test_build_prompt_non_git_dir(self):
        with TemporaryDirectory() as tmp:
            builder = ContextBuilder(tmp)
            prompt = await builder.build_prompt()
            assert "MiniCC" in prompt
            assert "Session Guidance" in prompt
            assert "Environment" in prompt

    async def test_build_prompt_with_rules_and_memory(self):
        with TemporaryDirectory() as tmp:
            p = Path(tmp) / ".minicc"
            p.mkdir()
            (p / "rules.md").write_bytes(b"# Test Rule")
            (p / "memory.md").write_bytes(b"- [x] Test done")
            builder = ContextBuilder(tmp)
            prompt = await builder.build_prompt()
            assert "Test Rule" in prompt
            assert "Test done" in prompt

    async def test_custom_system_prompt(self):
        with TemporaryDirectory() as tmp:
            builder = ContextBuilder(tmp)
            builder.set_custom_system_prompt("Custom override")
            prompt = await builder.build_prompt()
            assert prompt == "Custom override"
            assert "Session Guidance" not in prompt

    async def test_append_system_prompt(self):
        with TemporaryDirectory() as tmp:
            builder = ContextBuilder(tmp)
            builder.set_append_system_prompt("Extra rule at end.")
            prompt = await builder.build_prompt()
            assert "Extra rule at end." in prompt

    async def test_tool_prompts_included(self):
        with TemporaryDirectory() as tmp:
            builder = ContextBuilder(tmp)
            builder.add_tool_prompt("Read: use for files")
            builder.add_tool_prompt("Bash: use as last resort")
            prompt = await builder.build_prompt()
            assert "Available Tools" in prompt
            assert "use for files" in prompt
            assert "use as last resort" in prompt

    async def test_tool_names_in_session_guidance(self):
        with TemporaryDirectory() as tmp:
            builder = ContextBuilder(tmp)
            prompt = await builder.build_prompt(tool_names=["read_file", "bash"])
            assert "read_file" in prompt
            assert "bash" in prompt
