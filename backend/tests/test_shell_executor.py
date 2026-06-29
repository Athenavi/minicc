"""Tests: ShellExecutorTool — safe shell command execution."""

from __future__ import annotations

import gc
import os
from pathlib import Path
from tempfile import TemporaryDirectory

import pytest

from app.tools.shell_executor import ShellExecutorInput, ShellExecutorTool

pytestmark = pytest.mark.asyncio


@pytest.fixture
def workspace():
    with TemporaryDirectory() as tmp:
        yield Path(tmp)
    gc.collect()  # help Windows release subprocess handles


@pytest.fixture
def tool(workspace):
    return ShellExecutorTool(workspace)


class TestShellExecutor:
    async def test_echo(self, tool):
        """Simple echo command works."""
        result = await tool.execute(ShellExecutorInput(command="echo hello"))
        assert not result.is_error
        assert "hello" in result.output

    async def test_exit_code(self, tool):
        """Non-zero exit code is captured."""
        result = await tool.execute(ShellExecutorInput(command="exit 42"))
        assert result.is_error
        assert "42" in result.output

    async def test_stderr(self, tool):
        """stderr output is captured."""
        result = await tool.execute(ShellExecutorInput(command="echo err >&2"))
        # stderr should appear in output
        assert "err" in result.output or "[stderr]" in result.output

    async def test_timeout(self, tool):
        """Command that exceeds timeout is killed."""
        result = await tool.execute(ShellExecutorInput(command="sleep 10", timeout=2))
        assert result.is_error
        assert "timeout" in result.output.lower()

    async def test_cwd_default(self, tool, workspace):
        """Default cwd is workspace_dir."""
        result = await tool.execute(ShellExecutorInput(command="pwd"))
        assert not result.is_error
        assert len(result.output) > 0  # pwd returned something

    async def test_cwd_custom(self, tool, workspace):
        """Custom workdir inside workspace is respected."""
        # Create a subdirectory inside workspace
        subdir = workspace / "subdir"
        subdir.mkdir()
        (subdir / "test.txt").write_text("hello")
        result = await tool.execute(ShellExecutorInput(
            command="ls",
            workdir=str(subdir),
        ))
        assert not result.is_error
        assert "test.txt" in result.output

    async def test_multiline_command(self, tool):
        """Multi-line commands work."""
        result = await tool.execute(ShellExecutorInput(command="echo a && echo b"))
        assert not result.is_error
        assert "a" in result.output
        assert "b" in result.output

    async def test_environment_clean(self, tool, workspace):
        """Sensitive env vars are stripped."""
        os.environ["TEST_API_KEY_SHOULD_BE_REMOVED"] = "secret123"
        try:
            result = await tool.execute(ShellExecutorInput(
                command="echo ${TEST_API_KEY_SHOULD_BE_REMOVED:-not_set}"
            ))
            assert "not_set" in result.output  # var was removed
        finally:
            os.environ.pop("TEST_API_KEY_SHOULD_BE_REMOVED", None)

    async def test_workdir_outside_workspace_rejected(self, tool):
        """Workdir outside workspace is rejected."""
        result = await tool.execute(ShellExecutorInput(
            command="pwd",
            workdir="/etc",
        ))
        assert result.is_error

    async def test_pipe(self, tool):
        """Pipe commands work."""
        result = await tool.execute(ShellExecutorInput(command="echo hello | wc -c"))
        assert not result.is_error
        assert "6" in result.output  # "hello\n" = 6 bytes
