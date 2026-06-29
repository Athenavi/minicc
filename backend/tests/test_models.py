"""测试：核心 Pydantic 模型。"""

from __future__ import annotations

from datetime import datetime, timezone

import pytest
from pydantic import ValidationError

from app.models.chat import ContentBlock, Message, Role
from app.models.permission import PermissionLevel, PermissionRequest
from app.models.session import SessionState
from app.models.tool import ToolCall, ToolResult


class TestChatModels:
    def test_role_enum_values(self):
        assert Role.system.value == "system"
        assert Role.user.value == "user"
        assert Role.assistant.value == "assistant"
        assert Role.tool.value == "tool"

    def test_content_block_text(self):
        block = ContentBlock(type="text", text="hello")
        assert block.text == "hello"
        assert block.type == "text"

    def test_content_block_tool_use(self):
        block = ContentBlock(
            type="tool_use",
            id="toolu_abc",
            name="read_file",
            input={"path": "main.py"},
        )
        assert block.id == "toolu_abc"
        assert block.input == {"path": "main.py"}

    def test_message_user(self):
        msg = Message(role=Role.user, content="hello")
        assert msg.role == Role.user
        assert msg.content == "hello"
        assert isinstance(msg.created_at, datetime)

    def test_message_with_blocks(self):
        blocks = [
            ContentBlock(type="text", text="thinking..."),
            ContentBlock(type="tool_use", id="t1", name="bash", input={"command": "ls"}),
        ]
        msg = Message(role=Role.assistant, content=blocks)
        assert len(msg.content) == 2
        assert msg.content[0].text == "thinking..."

    def test_message_invalid_role(self):
        with pytest.raises(ValidationError):
            Message(role="invalid", content="x")  # type: ignore


class TestToolModels:
    def test_tool_call_defaults(self):
        tc = ToolCall(id="tc_1", name="read_file", type="file_read")
        assert tc.status == "pending"
        assert tc.input == {}

    def test_tool_call_full(self):
        tc = ToolCall(
            id="tc_2",
            name="bash",
            type="bash",
            input={"command": "ls -la"},
            status="running",
        )
        assert tc.status == "running"

    def test_tool_result(self):
        tr = ToolResult(tool_call_id="tc_1", output="hello\nworld")
        assert not tr.is_error
        assert tr.output == "hello\nworld"

    def test_tool_result_error(self):
        tr = ToolResult(tool_call_id="tc_2", output="not found", is_error=True)
        assert tr.is_error


class TestSessionModels:
    def test_session_defaults(self):
        sess = SessionState(session_id="sess_1")
        assert sess.messages == []
        assert sess.pending_tool_calls == []

    def test_session_with_messages(self):
        msg = Message(role=Role.user, content="hello")
        sess = SessionState(session_id="sess_2", messages=[msg])
        assert len(sess.messages) == 1


class TestPermissionModels:
    def test_permission_level_values(self):
        assert PermissionLevel.READ.value == "read"
        assert PermissionLevel.WRITE.value == "write"
        assert PermissionLevel.EXECUTE.value == "execute"

    def test_permission_request_pending(self):
        req = PermissionRequest(
            id="pr_1",
            tool_name="bash",
            tool_input={"command": "ls"},
            level=PermissionLevel.EXECUTE,
            reason="list files",
        )
        assert req.status == "pending"
        assert req.level == PermissionLevel.EXECUTE

    def test_permission_request_with_diff(self):
        req = PermissionRequest(
            id="pr_2",
            tool_name="write_to_file",
            tool_input={"path": "x.py", "content": "x"},
            level=PermissionLevel.WRITE,
            reason="create file",
            diff_preview="+ new file",
        )
        assert req.diff_preview == "+ new file"
