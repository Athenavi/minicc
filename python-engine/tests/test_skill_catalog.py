"""Tests for skill_catalog — 持久技能目录注入。"""
from __future__ import annotations

import pytest

from app.tools.registry import registry
from app.tools.skill_catalog import CATALOG_MARKER, inject_skill_catalog


class TestSkillCatalog:
    @pytest.mark.asyncio
    async def test_injects_when_skills_exist(self, monkeypatch):
        async def fake_skill_list(*_args):
            return {"skills": [{"name": "git-helper", "description": "Git 操作辅助"}]}
        monkeypatch.setattr(registry, "get", lambda name: type("T", (), {"handler": fake_skill_list})())
        msgs = [{"role": "user", "content": "hi"}]
        out = await inject_skill_catalog(msgs)
        assert len(out) == 2
        assert CATALOG_MARKER in out[0]["content"]
        assert "git-helper" in out[0]["content"]

    @pytest.mark.asyncio
    async def test_no_skills_no_injection(self, monkeypatch):
        async def empty_list():
            return {"skills": []}
        monkeypatch.setattr(registry, "get", lambda name: type("T", (), {"handler": empty_list})())
        msgs = [{"role": "user", "content": "hi"}]
        out = await inject_skill_catalog(msgs)
        assert len(out) == 1

    @pytest.mark.asyncio
    async def test_skip_when_already_injected(self, monkeypatch):
        async def fake_skill_list(*_args):
            return {"skills": [{"name": "s1", "description": "d"}]}
        monkeypatch.setattr(registry, "get", lambda name: type("T", (), {"handler": fake_skill_list})())
        msgs = [{"role": "user", "content": f"{CATALOG_MARKER}\n- s1: d\n</available_skills>"}]
        out = await inject_skill_catalog(msgs)
        assert len(out) == 1  # 不重复注入

    @pytest.mark.asyncio
    async def test_no_skill_tool_no_injection(self, monkeypatch):
        monkeypatch.setattr(registry, "get", lambda name: None)
        out = await inject_skill_catalog([{"role": "user", "content": "hi"}])
        assert len(out) == 1
