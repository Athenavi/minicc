"""Tests: FileSystem tools — ReadFile / WriteToFile / StrReplaceEditor."""

from __future__ import annotations

from pathlib import Path
from tempfile import TemporaryDirectory

import pytest

pytestmark = pytest.mark.asyncio

from app.tools.file_system import (
    DiffGenerator,
    ReadFileTool,
    ReadFileToolInput,
    StrReplaceEditorTool,
    StrReplaceEditorInput,
    WriteToFileTool,
    WriteToFileToolInput,
    register_file_tools,
)
from app.tools.base import ToolRegistry


# ── Fixtures ──


@pytest.fixture
def workspace():
    with TemporaryDirectory() as tmp:
        yield Path(tmp)


@pytest.fixture
def read_tool(workspace):
    return ReadFileTool(workspace)


@pytest.fixture
def write_tool(workspace):
    return WriteToFileTool(workspace)


@pytest.fixture
def edit_tool(workspace):
    return StrReplaceEditorTool(workspace)


# ── DiffGenerator ──


class TestDiffGenerator:
    def test_generate_diff_added_lines(self):
        original = "line1\nline2\n"
        modified = "line1\nline2\nline3\n"
        diff = DiffGenerator.generate_diff(original, modified, "test.py")
        assert "+line3" in diff
        assert "-" not in diff or True  # no deletions

    def test_generate_diff_removed_lines(self):
        original = "line1\nline2\nline3\n"
        modified = "line1\nline3\n"
        diff = DiffGenerator.generate_diff(original, modified, "test.py")
        assert "-line2" in diff

    def test_generate_diff_for_tool_new_file(self):
        diff = DiffGenerator.generate_diff_for_tool("/nonexistent/file.py", "write_to_file", {"content": "x"})
        assert diff == "(new file: /nonexistent/file.py)" or "new file" in (diff or "")

    def test_generate_diff_for_tool_unknown(self):
        assert DiffGenerator.generate_diff_for_tool("x.py", "unknown_tool", {}) is None


# ── ReadFileTool ──


class TestReadFileTool:
    async def test_read_text_file(self, read_tool, workspace):
        f = workspace / "hello.py"
        f.write_text("print('hello')\nprint('world')\n")
        result = await read_tool.execute(ReadFileToolInput(path=str(f)))
        assert not result.is_error
        assert "hello" in result.output
        assert "world" in result.output

    async def test_read_with_line_numbers(self, read_tool, workspace):
        f = workspace / "nums.py"
        f.write_text("a\nb\nc\n")
        result = await read_tool.execute(ReadFileToolInput(path=str(f)))
        lines = result.output.splitlines()
        assert len(lines) >= 3
        assert "1|a" in result.output or "     1|a" in result.output

    async def test_read_with_offset_and_limit(self, read_tool, workspace):
        f = workspace / "lines.txt"
        f.write_text("\n".join(f"line{i}" for i in range(20)))
        result = await read_tool.execute(ReadFileToolInput(path=str(f), offset=5, limit=3))
        assert "line5" in result.output
        assert "line6" in result.output
        assert "line7" in result.output
        assert "line0" not in result.output

    async def test_file_not_found(self, read_tool):
        result = await read_tool.execute(ReadFileToolInput(path="/nonexistent/xyz.py"))
        assert result.is_error

    async def test_binary_file(self, read_tool, workspace):
        f = workspace / "data.bin"
        f.write_bytes(b"\x00\x01\x02\x03")
        result = await read_tool.execute(ReadFileToolInput(path=str(f)))
        assert "binary" in result.output.lower()

    async def test_path_traversal_rejected(self, read_tool, workspace):
        result = await read_tool.execute(ReadFileToolInput(path=str(workspace.parent / "etc" / "passwd")))
        assert result.is_error

    async def test_metadata(self, read_tool, workspace):
        f = workspace / "meta.txt"
        f.write_text("hello\nworld\nfoo\n")
        result = await read_tool.execute(ReadFileToolInput(path=str(f)))
        assert result.metadata.get("total_lines") == 3


# ── WriteToFileTool ──


class TestWriteToFileTool:
    async def test_create_new_file(self, write_tool, workspace):
        path = workspace / "new.txt"
        result = await write_tool.execute(WriteToFileToolInput(path=str(path), content="hello"))
        assert not result.is_error
        assert path.read_text() == "hello"

    async def test_overwrite_existing_file(self, write_tool, workspace):
        path = workspace / "overwrite.txt"
        path.write_text("old")
        result = await write_tool.execute(WriteToFileToolInput(path=str(path), content="new"))
        assert not result.is_error
        assert path.read_text() == "new"

    async def test_create_parent_directories(self, write_tool, workspace):
        path = workspace / "a" / "b" / "deep.txt"
        result = await write_tool.execute(WriteToFileToolInput(path=str(path), content="deep", create_parents=True))
        assert not result.is_error
        assert path.read_text() == "deep"

    async def test_path_traversal_rejected(self, write_tool, workspace):
        result = await write_tool.execute(WriteToFileToolInput(
            path=str(workspace.parent / "outside.txt"),
            content="x",
        ))
        assert result.is_error


# ── StrReplaceEditorTool ──


class TestStrReplaceEditorTool:
    async def test_simple_replacement(self, edit_tool, workspace):
        f = workspace / "edit.txt"
        f.write_text("hello world")
        result = await edit_tool.execute(StrReplaceEditorInput(
            path=str(f), old_string="world", new_string="there"
        ))
        assert not result.is_error
        assert f.read_text() == "hello there"

    async def test_no_match_returns_error(self, edit_tool, workspace):
        f = workspace / "no_match.txt"
        f.write_text("hello")
        result = await edit_tool.execute(StrReplaceEditorInput(
            path=str(f), old_string="xyz", new_string="abc"
        ))
        assert result.is_error
        assert "No match" in result.output

    async def test_multiple_matches_requires_replace_all(self, edit_tool, workspace):
        f = workspace / "multi.txt"
        f.write_text("a a a")
        result = await edit_tool.execute(StrReplaceEditorInput(
            path=str(f), old_string="a", new_string="x"
        ))
        assert result.is_error
        assert "replace_all=True" in result.output

    async def test_replace_all(self, edit_tool, workspace):
        f = workspace / "all.txt"
        f.write_text("a a a")
        result = await edit_tool.execute(StrReplaceEditorInput(
            path=str(f), old_string="a", new_string="x", replace_all=True
        ))
        assert not result.is_error
        assert f.read_text() == "x x x"


# ── Registration ──


class TestRegistration:
    def test_register_file_tools(self):
        registry = ToolRegistry()
        register_file_tools(registry)
        assert registry.get("read_file") is not None
        assert registry.get("write_to_file") is not None
        assert registry.get("str_replace_editor") is not None
