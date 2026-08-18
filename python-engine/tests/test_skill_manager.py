"""技能管理器 (app/skill/manager.py) fail loud 回归测试。

意图:
- 未实现的 MCP 传输方式必须显式抛错,不得吞掉异常返回空列表/假结果
- PROMPT 技能必须完成真实的模板渲染 (config 已在注册时持久化)
- 未实现的技能类型执行必须返回明确的失败结果
"""
from __future__ import annotations

import pytest

import app.main  # noqa: F401 — 初始化 app 包，避免循环导入
from app.skill.manager import MCPClient, SkillManager, SkillType

TENANT = "tenant-skill-test"


async def test_stdio_transport_discovery_fails_loud():
    """STDIO 传输未实现: 必须抛 NotImplementedError。

    若被吞掉返回 [],调用方无法区分"未实现"和"服务器没有工具"。
    """
    client = MCPClient(server_url="http://localhost:1", transport="stdio")
    with pytest.raises(NotImplementedError, match="STDIO"):
        await client.discover_tools()


async def test_stdio_transport_call_fails_loud():
    client = MCPClient(server_url="http://localhost:1", transport="stdio")
    with pytest.raises(NotImplementedError, match="STDIO"):
        await client.call_tool(tool_name="any", arguments={})


async def test_unsupported_transport_fails_loud():
    client = MCPClient(server_url="http://localhost:1", transport="carrier-pigeon")
    with pytest.raises(ValueError, match="Unsupported transport"):
        await client.discover_tools()


async def test_prompt_skill_renders_template():
    """PROMPT 技能必须真实渲染模板变量 (注册时的 config 必须持久化)。"""
    manager = SkillManager()
    await manager.register_skill(
        tenant_id=TENANT,
        skill_id="greet",
        name="greet",
        description="问候模板",
        type=SkillType.PROMPT,
        config={"template": "你好, {name}!"},
    )
    result = await manager.execute_skill(TENANT, "greet", {"name": "世界"})
    assert result.success is True
    assert result.output == "你好, 世界!"


async def test_unimplemented_skill_types_return_explicit_error():
    """python/shell/http 技能类型未实现: 执行必须返回明确失败,不得静默成功。"""
    manager = SkillManager()
    for skill_type in (SkillType.PYTHON_SCRIPT, SkillType.SHELL_COMMAND, SkillType.HTTP_REQUEST):
        await manager.register_skill(
            tenant_id=TENANT,
            skill_id=f"s-{skill_type.value}",
            name=f"s-{skill_type.value}",
            description="t",
            type=skill_type,
            config={},
        )
        result = await manager.execute_skill(TENANT, f"s-{skill_type.value}", {})
        assert result.success is False
        assert "not yet implemented" in result.error


async def test_skill_tenant_isolation():
    """其他租户不得看到/执行不属于自己的技能。"""
    manager = SkillManager()
    await manager.register_skill(
        tenant_id="tenantA", skill_id="priv", name="priv",
        description="t", type=SkillType.PROMPT, config={"template": "x"},
    )
    assert await manager.list_skills("tenantB") == []
    result = await manager.execute_skill("tenantB", "priv", {})
    assert result.success is False
    assert "not found" in result.error.lower()


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
