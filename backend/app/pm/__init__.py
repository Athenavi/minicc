"""Phase R: AI 产品经理 — 需求与规划工具套件。"""

from __future__ import annotations

import json
from typing import Any, Optional

from pydantic import BaseModel, Field

from app.core.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext


# ── R1: PRD 生成器 ──

class PRDInput(BaseModel):
    description: str = Field(description="Product description in natural language")
    context: Optional[str] = Field(default="", description="Additional context (tech stack, team size, deadline)")


class PRDGeneratorTool(BaseTool):
    name = "prd_generate"
    description = "Generate a structured Product Requirements Document from a natural language description."
    input_schema = PRDInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.AGENT

    async def execute(self, input_data: PRDInput, context: ToolUseContext | None = None) -> ToolResult:
        desc = input_data.description
        ctx = input_data.context or ""

        # AI-generated PRD structure (simulated — real version uses LLM)
        features = self._extract_features(desc)
        prd = f"""# PRD: {desc[:60]}...

## Product Overview
- **Background**: {desc[:200]}
- **Target Users**: Developers and end-users
- **Core Value**: {features[0] if features else desc[:100]}

## Feature List
| Priority | Feature | Description | Acceptance Criteria |
|:---------|:--------|:------------|:-------------------|
| P0 | {features[0] if len(features) > 0 else 'Core functionality'} | Primary feature | Working end-to-end |
| P1 | {features[1] if len(features) > 1 else 'Secondary feature'} | Enhancement | Edge cases handled |
| P2 | {features[2] if len(features) > 2 else 'Polish'} | UX improvements | Smooth experience |

## User Stories
- As a user, I want {features[0] if features else desc[:60]}, So that I can accomplish my goal

## Non-functional Requirements
- Performance: Response < 200ms
- Security: Authentication required
- Availability: 99.9%
"""
        return ToolResult(tool_call_id="", output=prd, metadata={"feature_count": len(features)})

    def _extract_features(self, text: str) -> list[str]:
        """Simple keyword-based feature extraction (placeholder for LLM)."""
        keywords = ["登录", "注册", "管理", "搜索", "导出", "导入", "报表", "通知", "权限", "支付",
                    "login", "register", "manage", "search", "export", "import", "report", "notify", "payment"]
        found = [kw for kw in keywords if kw.lower() in text.lower()]
        return found[:5] or [text[:60]]


# ── R2: 技术方案生成 ──

class TechDesignInput(BaseModel):
    prd: str = Field(description="PRD content")
    tech_stack: Optional[str] = Field(default="", description="Preferred tech stack")


class TechDesignTool(BaseTool):
    name = "tech_design"
    description = "Generate technical design from PRD: architecture, API, data models."
    input_schema = TechDesignInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.AGENT

    async def execute(self, input_data: TechDesignInput, context: ToolUseContext | None = None) -> ToolResult:
        design = f"""# Technical Design

## System Architecture
```mermaid
graph TD
    A[Client] --> B[API Gateway]
    B --> C[Service Layer]
    C --> D[Database]
    C --> E[Cache]
```

## Data Models
```python
class User(BaseModel):
    id: str
    name: str
    email: str
    created_at: datetime

class Item(BaseModel):
    id: str
    title: str
    description: str
    owner_id: str
```

## API Design
```yaml
openapi: 3.0.0
paths:
  /api/users:
    get: # List users
    post: # Create user
  /api/items:
    get: # List items
    post: # Create item
```

## Directory Structure
```
project/
├── backend/
│   ├── app/
│   │   ├── api/       # Routes
│   │   ├── models/    # Data models
│   │   ├── services/  # Business logic
│   │   └── core/      # Config, auth
│   └── tests/
├── frontend/
│   ├── src/
│   │   ├── components/
│   │   └── pages/
│   └── public/
└── docker-compose.yml
```
"""
        return ToolResult(tool_call_id="", output=design)


# ── R3: 任务拆解引擎 ──

class TaskDecomposeInput(BaseModel):
    prd: str = Field(description="PRD content")
    design: Optional[str] = Field(default="", description="Tech design content")


class TaskDecomposeTool(BaseTool):
    name = "task_decompose"
    description = "Decompose PRD + tech design into executable Graph tasks."
    input_schema = TaskDecomposeInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.AGENT

    async def execute(self, input_data: TaskDecomposeInput, context: ToolUseContext | None = None) -> ToolResult:
        tasks = """# Task Breakdown (V0.3 Graph Compatible)

## Phase 1: Foundation
| ID | Task | Priority | Depends On | Est. Hours |
|:---|:-----|:---------|:-----------|:-----------|
| T1 | Project scaffold | P0 | — | 2 |
| T2 | Database models | P0 | T1 | 4 |
| T3 | API endpoints | P0 | T2 | 8 |
| T4 | Frontend setup | P0 | T1 | 2 |

## Phase 2: Features
| ID | Task | Priority | Depends On | Est. Hours |
|:---|:-----|:---------|:-----------|:-----------|
| T5 | User auth | P0 | T3 | 8 |
| T6 | CRUD operations | P0 | T3, T4 | 12 |
| T7 | Search | P1 | T6 | 6 |

## Phase 3: Polish
| ID | Task | Priority | Depends On | Est. Hours |
|:---|:-----|:---------|:-----------|:-----------|
| T8 | Tests | P1 | T5, T6 | 8 |
| T9 | Documentation | P2 | T8 | 4 |
| T10 | Deployment | P0 | T8 | 2 |

## Graph Definition (for V0.3 executor)
```json
{
  "name": "project",
  "entry_point": "T1",
  "nodes": [
    {"id": "T1", "label": "Scaffold", "node_type": "tool"},
    {"id": "T2", "label": "Database", "node_type": "tool"},
    {"id": "T3", "label": "API", "node_type": "tool"}
  ],
  "edges": [
    {"source_id": "T1", "target_id": "T2"},
    {"source_id": "T2", "target_id": "T3"}
  ]
}
```
"""
        return ToolResult(tool_call_id="", output=tasks)


# ── R4: 需求验证 ──

class ValidateInput(BaseModel):
    description: str = Field(description="Original requirement description")
    prd: Optional[str] = Field(default="", description="Generated PRD to validate")


class RequirementValidateTool(BaseTool):
    name = "requirement_validate"
    description = "Validate requirements by asking clarifying questions about ambiguous points."
    input_schema = ValidateInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.AGENT

    async def execute(self, input_data: ValidateInput, context: ToolUseContext | None = None) -> ToolResult:
        questions = """# Requirement Validation

## ✅ Clear Points
- Core functionality is well-defined

## ❓ Questions (need your input):
1. **Target users**: Who is the primary audience? (end-users / developers / admins)
2. **Auth method**: Password-based, OAuth, or SSO?
3. **Data storage**: SQL or NoSQL preferred?
4. **Deployment**: Cloud (AWS/GCP) or self-hosted?
5. **Timeline**: When is the MVP deadline?

## Suggested Next Steps
1. Answer the questions above
2. Run `prd_generate` again with clarified requirements
3. Run `tech_design` to generate architecture
4. Run `task_decompose` to get executable tasks
"""
        return ToolResult(tool_call_id="", output=questions)


def register_pm_tools(registry) -> None:
    registry.register(PRDGeneratorTool())
    registry.register(TechDesignTool())
    registry.register(TaskDecomposeTool())
    registry.register(RequirementValidateTool())
