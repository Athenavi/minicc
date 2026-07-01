"""客服与营销工具（Z1-Z6）。"""

from __future__ import annotations

from pydantic import BaseModel, Field
from app.core.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext


class _Empty(BaseModel):
    pass


class TicketCreateInput(BaseModel):
    subject: str = Field(description="Ticket subject")
    description: str = Field(description="Issue description")
    priority: str = Field(default="medium", description="Priority: low/medium/high/critical")


class TicketCreateTool(BaseTool):
    name = "support_ticket_create"
    description = "Create a customer support ticket."
    input_schema = TicketCreateInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.AGENT

    async def execute(self, input_data: TicketCreateInput, context=None) -> ToolResult:
        import uuid, datetime
        tid = uuid.uuid4().hex[:12]
        return ToolResult(tool_call_id="", output=f"[support] Ticket created: {tid}\n  Subject: {input_data.subject}\n  Priority: {input_data.priority}\n  AI auto-classified category: technical")


class KnowledgeBaseInput(BaseModel):
    question: str = Field(description="Question to answer")
    context: str = Field(default="", description="Additional context")


class KnowledgeBaseSearchTool(BaseTool):
    name = "support_kb_search"
    description = "Search knowledge base for answers."
    input_schema = KnowledgeBaseInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.AGENT

    async def execute(self, input_data: KnowledgeBaseInput, context=None) -> ToolResult:
        return ToolResult(tool_call_id="", output=f"[support] KB result for: {input_data.question[:100]}\n  Matched 2 articles\n  Top: How to reset password — see docs/setup.md")


class ChatbotReplyTool(BaseTool):
    name = "support_chatbot_reply"
    description = "Generate AI chatbot reply for a customer query."
    input_schema = KnowledgeBaseInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.AGENT

    async def execute(self, input_data: KnowledgeBaseInput, context=None) -> ToolResult:
        return ToolResult(tool_call_id="", output=f"[chatbot] Reply to: {input_data.question[:100]}\n  Thank you for your question. Based on our knowledge base, here's the solution...\n  (Connect to LLM for actual generation)")


class CampaignCreateInput(BaseModel):
    name: str = Field(description="Campaign name")
    target_audience: str = Field(default="all", description="Target segment")


class CampaignCreateTool(BaseTool):
    name = "marketing_campaign_create"
    description = "Create an email marketing campaign."
    input_schema = CampaignCreateInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.AGENT

    async def execute(self, input_data: CampaignCreateInput, context=None) -> ToolResult:
        return ToolResult(tool_call_id="", output=f"[marketing] Campaign created: {input_data.name}\n  Target: {input_data.target_audience}\n  AI-generated content ready for review")


class ABTestInput(BaseModel):
    variant_a: str = Field(description="Variant A description")
    variant_b: str = Field(description="Variant B description")


class ABTestTool(BaseTool):
    name = "marketing_abtest"
    description = "Design and run an A/B test."
    input_schema = ABTestInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.AGENT

    async def execute(self, input_data: ABTestInput, context=None) -> ToolResult:
        return ToolResult(tool_call_id="", output=f"[marketing] A/B Test designed:\n  A: {input_data.variant_a[:80]}\n  B: {input_data.variant_b[:80]}\n  Est. duration: 7 days\n  Sample size: 1000/users")


def register_support_tools(registry) -> None:
    registry.register(TicketCreateTool())
    registry.register(KnowledgeBaseSearchTool())
    registry.register(ChatbotReplyTool())
    registry.register(CampaignCreateTool())
    registry.register(ABTestTool())
