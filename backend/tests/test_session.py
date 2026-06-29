"""Tests: SQLiteStore, TaskManager, SessionManager."""

from __future__ import annotations

import asyncio
from datetime import datetime, timezone
from pathlib import Path
from tempfile import TemporaryDirectory

import pytest

from app.engine.session import SessionManager
from app.engine.task_manager import TaskManager
from app.models.chat import Message, Role
from app.models.session import SessionState
from app.utils.redis_client import RedisClient
from app.utils.sqlite_store import SQLiteStore

pytestmark = pytest.mark.asyncio


# ── SQLiteStore ──


class TestSQLiteStore:
    async def test_save_and_get_session(self):
        with TemporaryDirectory() as tmp:
            store = SQLiteStore(str(Path(tmp) / "test.db"))
            ok = await store.connect()
            assert ok
            sess = SessionState(session_id="s1")
            await store.save_session(sess)
            loaded = await store.get_session("s1")
            assert loaded is not None
            assert loaded.session_id == "s1"
            await store.disconnect()

    async def test_save_and_get_messages(self):
        with TemporaryDirectory() as tmp:
            store = SQLiteStore(str(Path(tmp) / "test.db"))
            await store.connect()
            msg = Message(role=Role.user, content="hello")
            await store.save_message("s1", msg)
            msgs = await store.get_messages("s1")
            assert len(msgs) == 1
            assert msgs[0].content == "hello"
            await store.disconnect()

    async def test_list_sessions(self):
        with TemporaryDirectory() as tmp:
            store = SQLiteStore(str(Path(tmp) / "test.db"))
            await store.connect()
            for i in range(3):
                await store.save_session(SessionState(session_id=f"s{i}"))
            sessions = await store.list_sessions()
            assert len(sessions) >= 3
            await store.disconnect()

    async def test_approval_log(self):
        with TemporaryDirectory() as tmp:
            store = SQLiteStore(str(Path(tmp) / "test.db"))
            await store.connect()
            await store.log_approval("s1", "bash", "approve")
            await store.disconnect()

    async def test_unknown_session(self):
        with TemporaryDirectory() as tmp:
            store = SQLiteStore(str(Path(tmp) / "test.db"))
            await store.connect()
            got = await store.get_session("nonexistent")
            assert got is None
            await store.disconnect()


# ── RedisClient (memory-mode, no actual Redis) ──


class TestRedisClient:
    async def test_connect_no_redis(self):
        client = RedisClient("redis://localhost:16379/0")
        ok = await client.connect()
        # Should fail gracefully
        assert not ok

    async def test_fallback_when_unavailable(self):
        client = RedisClient("redis://localhost:16379/0")
        await client.connect()
        assert not client.available
        # Operations should not raise
        assert await client.get_session_state("x") is None
        assert await client.get_recent_messages("x") == []
        await client.delete_session("x")
        await client.disconnect()


# ── SessionManager (with SQLite only) ──


class TestSessionManager:
    async def test_save_and_resume(self):
        with TemporaryDirectory() as tmp:
            store = SQLiteStore(str(Path(tmp) / "test.db"))
            await store.connect()
            redis = RedisClient("redis://localhost:16379/0")
            await redis.connect()
            mgr = SessionManager(redis, store)

            sess = SessionState(session_id="sm1")
            msg = Message(role=Role.user, content="test message")
            await mgr.save_message("sm1", msg)
            await mgr.save_session(sess)

            # Resume from SQLite (Redis unavailable)
            resumed = await mgr.resume_session("sm1")
            assert resumed is not None
            assert resumed.session_id == "sm1"

            # Should have the saved message
            msgs = await store.get_messages("sm1")
            assert len(msgs) >= 1
            assert msgs[-1].content == "test message"

            await store.disconnect()

    async def test_resume_nonexistent(self):
        with TemporaryDirectory() as tmp:
            store = SQLiteStore(str(Path(tmp) / "test.db"))
            await store.connect()
            redis = RedisClient("redis://localhost:16379/0")
            mgr = SessionManager(redis, store)
            resumed = await mgr.resume_session("nonexistent")
            assert resumed is None
            await store.disconnect()


# ── TaskManager ──


class TestTaskManager:
    async def test_register_and_active(self):
        tm = TaskManager()
        assert tm.get_active_count() == 0

        async def dummy():
            await asyncio.sleep(0.1)

        task = await tm.start_session_task("t1", dummy())
        assert tm.is_session_active("t1")
        await task
        assert not tm.is_session_active("t1")

    async def test_cancel_session(self):
        tm = TaskManager()
        async def long_running():
            try:
                await asyncio.sleep(100)
            except asyncio.CancelledError:
                pass

        await tm.start_session_task("t2", long_running())
        assert tm.is_session_active("t2")
        ok = await tm.cancel_session("t2")
        assert ok
        assert not tm.is_session_active("t2")

    async def test_cancel_all(self):
        tm = TaskManager()
        started = asyncio.Event()

        async def runner():
            started.set()
            try:
                await asyncio.sleep(100)
            except asyncio.CancelledError:
                pass

        await tm.start_session_task("a", runner())
        await tm.start_session_task("b", runner())
        # Give tasks time to start
        await asyncio.sleep(0.05)
        assert tm.get_active_count() == 2
        await tm.cancel_all()
        assert tm.get_active_count() == 0

    async def test_cleanup_stale(self):
        tm = TaskManager()
        count = await tm.cleanup_stale(max_idle_minutes=1)
        assert count == 0
