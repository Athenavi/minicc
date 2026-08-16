import pytest
from httpx import AsyncClient, ASGITransport

from app.main import create_app
from app.main import get_gateway


def _mock_gateway():
    from unittest.mock import MagicMock
    from app.gateway.provider import ChatResponse

    gw = MagicMock()

    async def fake_stream(**_kwargs):
        yield ChatResponse(content="", finish_reason="stop")

    gw.chat_stream = fake_stream
    return gw


@pytest.mark.asyncio
async def test_list_tools_returns_tools_key():
    app = create_app()
    async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as ac:
        resp = await ac.get("/v1/tools")
    assert resp.status_code == 200
    body = resp.json()
    assert "tools" in body
    assert isinstance(body["tools"], list)


@pytest.mark.asyncio
async def test_execute_tool_requires_name():
    app = create_app()
    async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as ac:
        resp = await ac.post("/v1/tools/execute", json={"input": {}})
    assert resp.status_code == 422


@pytest.mark.asyncio
async def test_execute_tool_returns_output_for_known_tool():
    app = create_app()
    async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as ac:
        resp = await ac.post("/v1/tools/execute", json={"name": "shell_exec", "input": {"command": "echo ok"}})
    assert resp.status_code == 200
    body = resp.json()
    assert body.get("exit_code") == 0
    assert "ok" in body.get("stdout", "")


@pytest.mark.asyncio
async def test_execute_workflow_returns_instance():
    app = create_app()
    app.dependency_overrides[get_gateway] = _mock_gateway
    payload = {
        "name": "wf-test",
        "nodes": [
            {"id": "input_1", "label": "Input", "node_type": "input"},
            {"id": "output_1", "label": "Output", "node_type": "output"},
        ],
        "edges": [{"source_id": "input_1", "target_id": "output_1"}],
        "initial_state": {"input": "hello"},
    }
    try:
        async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as ac:
            resp = await ac.post("/v1/graphs/demo/execute", json=payload)
        assert resp.status_code == 200
        body = resp.json()
        assert body["status"] in {"completed", "error"}
        assert "instance_id" in body
    finally:
        app.dependency_overrides.pop(get_gateway, None)


@pytest.mark.asyncio
async def test_workflow_status_returns_instance():
    app = create_app()
    app.dependency_overrides[get_gateway] = _mock_gateway
    payload = {
        "name": "wf-status",
        "nodes": [
            {"id": "input_1", "label": "Input", "node_type": "input"},
            {"id": "output_1", "label": "Output", "node_type": "output"},
        ],
        "edges": [{"source_id": "input_1", "target_id": "output_1"}],
        "initial_state": {"input": "hello"},
    }
    try:
        async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as ac:
            run_resp = await ac.post("/v1/graphs/demo/execute", json=payload)
            instance_id = run_resp.json()["instance_id"]
            status_resp = await ac.get(f"/v1/workflows/{instance_id}/status")
        assert status_resp.status_code == 200
        body = status_resp.json()
        assert body["instance_id"] == instance_id
        assert body["status"] in {"completed", "error"}
    finally:
        app.dependency_overrides.pop(get_gateway, None)


@pytest.mark.asyncio
async def test_workflow_status_missing_returns_404():
    app = create_app()
    async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as ac:
        resp = await ac.get("/v1/workflows/not-exist/status")
    assert resp.status_code == 404


@pytest.mark.asyncio
async def test_list_agents_returns_agents():
    app = create_app()
    async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as ac:
        resp = await ac.get("/v1/agents")
    assert resp.status_code == 200
    body = resp.json()
    assert "agents" in body
    assert isinstance(body["agents"], list)
    assert len(body["agents"]) >= 1


@pytest.mark.asyncio
async def test_dispatch_agent_returns_dispatched():
    app = create_app()
    async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as ac:
        resp = await ac.post("/v1/agents/dispatch", json={"task": "summarize doc", "agent_type": "knowledge"})
    assert resp.status_code == 200
    body = resp.json()
    assert body.get("agent_type") == "knowledge"
    assert body.get("status") == "dispatched"
