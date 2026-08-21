"""SmartAPIKeyPool 按稳定 ID 的管理操作（更新状态 / 删除）"""
import pytest

from app.gateway.smart_key_pool import KeyStatus, SmartAPIKeyPool


def make_pool() -> SmartAPIKeyPool:
    pool = SmartAPIKeyPool()
    return pool


@pytest.mark.asyncio
async def test_key_id_stable_and_no_key_leak():
    """ID 稳定（同 provider+key 同 ID）、不同 key 不同 ID、不包含完整 key 明文"""
    pool = make_pool()
    await pool.add_key("openai", "sk-secret-key-value")
    a = pool.get_all_keys()[0]["id"]
    await pool.add_key("openai", "sk-secret-key-value")
    b = pool.get_all_keys()[1]["id"]
    await pool.add_key("openai", "sk-other-key")
    c = pool.get_all_keys()[2]["id"]

    assert a == b  # 稳定
    assert a != c  # 不同 key 不同 ID
    assert "sk-secret-key-value" not in a  # 不泄露明文
    assert a.startswith("openai-")


@pytest.mark.asyncio
async def test_get_all_keys_includes_id():
    pool = make_pool()
    await pool.add_key("anthropic", "sk-ant-1", remark="prod")
    keys = pool.get_all_keys()
    assert len(keys) == 1
    assert keys[0]["id"]
    assert keys[0]["provider"] == "anthropic"
    assert keys[0]["remark"] == "prod"


@pytest.mark.asyncio
async def test_update_key_status_to_rate_limited():
    pool = make_pool()
    await pool.add_key("openai", "sk-1")
    key_id = pool.get_all_keys()[0]["id"]

    ok = await pool.update_key_status(key_id, "rate_limited")
    assert ok is True
    assert pool.get_all_keys()[0]["status"] == "rate_limited"
    stats = pool.get_stats()
    assert stats["rate_limited"] == 1
    assert stats["active"] == 0


@pytest.mark.asyncio
async def test_update_key_status_invalid_status_returns_false():
    pool = make_pool()
    await pool.add_key("openai", "sk-1")
    key_id = pool.get_all_keys()[0]["id"]

    ok = await pool.update_key_status(key_id, "bogus-status")
    assert ok is False
    assert pool.get_all_keys()[0]["status"] == "active"  # 未被改动


@pytest.mark.asyncio
async def test_update_key_status_unknown_id_returns_false():
    pool = make_pool()
    await pool.add_key("openai", "sk-1")
    ok = await pool.update_key_status("openai-nonexistent", "active")
    assert ok is False


@pytest.mark.asyncio
async def test_update_to_active_resets_circuit_breaker():
    """熔断中的 key 手动恢复 active 时应重置熔断器，否则 get_key 仍被拦截"""
    pool = make_pool()
    await pool.add_key("openai", "sk-1")
    key_id = pool.get_all_keys()[0]["id"]

    # 打开熔断
    for _ in range(5):
        await pool.report_failure("sk-1", "boom")
    assert pool.get_all_keys()[0]["status"] == "circuit_open"

    ok = await pool.update_key_status(key_id, "active")
    assert ok is True
    got = await pool.get_key("openai")
    assert got == "sk-1"  # 熔断已重置，key 恢复可用


@pytest.mark.asyncio
async def test_remove_key_by_id():
    pool = make_pool()
    await pool.add_key("openai", "sk-1")
    await pool.add_key("openai", "sk-2")
    key_id = pool.get_all_keys()[0]["id"]

    ok = await pool.remove_key_by_id(key_id)
    assert ok is True
    remaining = pool.get_all_keys()
    assert len(remaining) == 1
    assert remaining[0]["id"] != key_id


@pytest.mark.asyncio
async def test_remove_key_by_id_cleans_empty_provider_pool():
    pool = make_pool()
    await pool.add_key("openai", "sk-1")
    key_id = pool.get_all_keys()[0]["id"]

    ok = await pool.remove_key_by_id(key_id)
    assert ok is True
    assert pool.get_all_keys() == []
    # 空 provider 池被清理（stats.providers 不再出现 openai）
    assert "openai" not in pool.get_stats()["providers"]


@pytest.mark.asyncio
async def test_remove_key_by_id_unknown_returns_false():
    pool = make_pool()
    await pool.add_key("openai", "sk-1")
    assert await pool.remove_key_by_id("openai-nope") is False
    assert await pool.remove_key_by_id("") is False
