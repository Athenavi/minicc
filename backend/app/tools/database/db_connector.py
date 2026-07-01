"""数据库连接器 — 通用 SQL 数据库工具。"""

from __future__ import annotations

from typing import Any, Optional

from pydantic import BaseModel, Field

from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext


class DbQueryInput(BaseModel):
    connection_string: str = Field(description="SQLAlchemy connection string (e.g. sqlite:///db.sqlite3)")
    query: str = Field(description="SQL query to execute")


class DbQueryTool(BaseTool):
    name = "db_query"
    description = "Execute a SQL query against a database."
    input_schema = DbQueryInput
    permission_level = PermissionLevel.EXECUTE
    category = ToolCategory.WEB

    async def execute(self, input_data: DbQueryInput, context: ToolUseContext | None = None) -> ToolResult:
        try:
            from sqlalchemy import create_engine, text
        except ImportError:
            return ToolResult(tool_call_id="", output="[db] SQLAlchemy not installed.", is_error=True)
        try:
            engine = create_engine(input_data.connection_string)
            with engine.connect() as conn:
                result = conn.execute(text(input_data.query))
                if result.returns_rows:
                    rows = [dict(row._mapping) for row in result]
                    output_lines = [f"[db] {len(rows)} row(s) returned"]
                    for r in rows[:50]:
                        output_lines.append(str(r))
                    return ToolResult(tool_call_id="", output="\n".join(output_lines), metadata={"rows": len(rows)})
                conn.commit()
                return ToolResult(tool_call_id="", output=f"[db] Query executed (no rows returned)")
        except Exception as exc:
            return ToolResult(tool_call_id="", output=f"[db] Error: {exc}", is_error=True)


class DbTablesInput(BaseModel):
    connection_string: str = Field(description="SQLAlchemy connection string")


class DbListTablesTool(BaseTool):
    name = "db_list_tables"
    description = "List all tables in a database."
    input_schema = DbTablesInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.WEB

    async def execute(self, input_data: DbTablesInput, context: ToolUseContext | None = None) -> ToolResult:
        from sqlalchemy import create_engine, inspect
        try:
            engine = create_engine(input_data.connection_string)
            inspector = inspect(engine)
            tables = inspector.get_table_names()
            return ToolResult(tool_call_id="", output=f"[db] Tables ({len(tables)}):\n" + "\n".join(f"  • {t}" for t in tables))
        except Exception as exc:
            return ToolResult(tool_call_id="", output=f"[db] Error: {exc}", is_error=True)


def register_db_tools(registry) -> None:
    registry.register(DbQueryTool())
    registry.register(DbListTablesTool())
