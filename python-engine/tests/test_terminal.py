"""Tests for persistent_shell — 持久终端状态跨调用。"""
from __future__ import annotations

import sys
import tempfile
from pathlib import Path

import pytest

from app.tools.context import set_tool_context
from app.tools.terminal import persistent_shell, _terminal


@pytest.mark.asyncio
async def test_basic_command_output():
    set_tool_context(session_id="t-sess-1")
    out = await persistent_shell("echo hello-persist")
    assert out.get("error") is None
    assert "hello-persist" in out["output"]
    assert out["persistent"] is True
    await _terminal.close_all()


@pytest.mark.asyncio
async def test_cwd_persists_across_calls(tmp_path):
    """cd 一次后，后续调用在同一目录执行（deepseek persistent-bash 语义）。"""
    set_tool_context(session_id="t-sess-2")
    d = str(tmp_path)
    if sys.platform == "win32":
        await persistent_shell(f"cd /d {d}")
        pwd = await persistent_shell("echo %CD%")
    else:
        await persistent_shell(f"cd {d}")
        pwd = await persistent_shell("pwd")
    assert str(tmp_path) in pwd["output"], f"expected cwd {d}, got {pwd}"
    await _terminal.close_all()


@pytest.mark.asyncio
async def test_session_isolation():
    set_tool_context(session_id="t-sess-a")
    await persistent_shell("echo AA")
    set_tool_context(session_id="t-sess-b")
    out = await persistent_shell("echo BB")
    assert "BB" in out["output"]
    await _terminal.close_all()


@pytest.mark.asyncio
async def test_timeout_resets_shell():
    set_tool_context(session_id="t-sess-3")
    if sys.platform == "win32":
        cmd = "ping -n 30 127.0.0.1"
    else:
        cmd = "sleep 30"
    out = await persistent_shell(cmd, timeout=1)
    assert out["reset"] is True and "timed out" in out["output"]
    # 重置后仍可执行
    out2 = await persistent_shell("echo after-reset")
    assert "after-reset" in out2["output"]
    await _terminal.close_all()


@pytest.mark.asyncio
async def test_exit_code_captured():
    set_tool_context(session_id="t-sess-4")
    ok = await persistent_shell("echo ok")
    assert ok["exit_code"] == 0
    if sys.platform == "win32":
        bad = await persistent_shell("exit /b 7")
    else:
        bad = await persistent_shell("exit 7")
    assert bad["exit_code"] == 7
    await _terminal.close_all()
