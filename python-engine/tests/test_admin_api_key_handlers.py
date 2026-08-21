"""admin API Key 管理 HTTP handler（真实调用 SmartAPIKeyPool，验证 fail-loud 语义）"""
import json

import pytest

from app.gateway.smart_key_pool import SmartAPIKeyPool


class FakeRequest:
    def __init__(self, path_params=None, body=None, json_raises=False):
        self.path_params = path_params or {}
        self._body = body
        self._json_raises = json_raises

    async def json(self):
        if self._json_raises or self._body is None:
            raise ValueError("no body")
        return self._body


async def make_pool_with_key(provider="openai", key="sk-test-1"):
    pool = SmartAPIKeyPool()
    await pool.add_key(provider, key)
    return pool, pool.get_all_keys()[0]["id"]


def body_of(resp) -> dict:
    return json.loads(resp.body)


# ── admin_update_api_key ──


@pytest.mark.asyncio
async def test_update_status_success():
    from app.main import admin_update_api_key

    pool, key_id = await make_pool_with_key()
    resp = await admin_update_api_key(
        FakeRequest(path_params={"key_id": key_id}, body={"status": "rate_limited"}),
        pool=pool,
    )
    assert resp == {"status": "updated", "id": key_id, "key_status": "rate_limited"}
    assert pool.get_all_keys()[0]["status"] == "rate_limited"


@pytest.mark.asyncio
async def test_update_status_missing_params_400():
    from app.main import admin_update_api_key

    pool, _ = await make_pool_with_key()
    # 缺 status
    resp = await admin_update_api_key(
        FakeRequest(path_params={"key_id": "some-id"}, body={}),
        pool=pool,
    )
    assert resp.status_code == 400
    assert body_of(resp)["error"] == "key id and status are required"


@pytest.mark.asyncio
async def test_update_status_unknown_id_404():
    from app.main import admin_update_api_key

    pool, _ = await make_pool_with_key()
    resp = await admin_update_api_key(
        FakeRequest(path_params={"key_id": "openai-nonexistent"}, body={"status": "active"}),
        pool=pool,
    )
    assert resp.status_code == 404


@pytest.mark.asyncio
async def test_update_status_invalid_status_value_404():
    from app.main import admin_update_api_key

    pool, key_id = await make_pool_with_key()
    resp = await admin_update_api_key(
        FakeRequest(path_params={"key_id": key_id}, body={"status": "not-a-status"}),
        pool=pool,
    )
    assert resp.status_code == 404
    # 状态未被改动
    assert pool.get_all_keys()[0]["status"] == "active"


# ── admin_delete_api_key ──


@pytest.mark.asyncio
async def test_delete_by_path_id_without_body():
    """前端实际调用方式：路径 ID + 无请求体（原实现会 400，本修复的目标场景）"""
    from app.main import admin_delete_api_key

    pool, key_id = await make_pool_with_key()
    resp = await admin_delete_api_key(
        FakeRequest(path_params={"key_id": key_id}),
        pool=pool,
    )
    assert resp == {"status": "deleted", "id": key_id}
    assert pool.get_all_keys() == []


@pytest.mark.asyncio
async def test_delete_by_provider_key_body_compat():
    """兼容旧调用：请求体携带 provider+key"""
    from app.main import admin_delete_api_key

    pool, _ = await make_pool_with_key()
    resp = await admin_delete_api_key(
        FakeRequest(path_params={"key_id": ""}, body={"provider": "openai", "key": "sk-test-1"}),
        pool=pool,
    )
    assert resp["status"] == "deleted"
    assert pool.get_all_keys() == []


@pytest.mark.asyncio
async def test_delete_unknown_id_404():
    from app.main import admin_delete_api_key

    pool, _ = await make_pool_with_key()
    resp = await admin_delete_api_key(
        FakeRequest(path_params={"key_id": "openai-nope"}),
        pool=pool,
    )
    assert resp.status_code == 404


@pytest.mark.asyncio
async def test_delete_no_id_no_body_400():
    from app.main import admin_delete_api_key

    pool, _ = await make_pool_with_key()
    resp = await admin_delete_api_key(FakeRequest(path_params={}), pool=pool)
    assert resp.status_code == 400
    assert "required" in body_of(resp)["error"]


# ── admin_list_api_keys（列表含 id） ──


@pytest.mark.asyncio
async def test_list_keys_includes_stable_id():
    from app.main import admin_list_api_keys

    pool, key_id = await make_pool_with_key()
    resp = await admin_list_api_keys(FakeRequest(), pool=pool)
    assert resp["keys"][0]["id"] == key_id
    assert "stats" in resp
