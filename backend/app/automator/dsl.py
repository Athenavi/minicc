"""自动化 DSL — 工作流定义、解析与变量系统。"""

from __future__ import annotations

import re
from datetime import datetime, timezone
from typing import Any, Optional

from pydantic import BaseModel, Field


class WorkflowStep(BaseModel):
    """单个工作流步骤。"""
    id: str = Field(description="步骤唯一标识")
    tool: str = Field(description="工具名，如 browser.navigate")
    params: dict[str, Any] = Field(default_factory=dict)
    timeout: int = Field(default=60, ge=1, le=3600)
    retry: int = Field(default=0, ge=0, le=5)
    on_error: Optional[dict] = None  # {action: "skip"|"stop"|"notify", message: "..."}


class WorkflowCondition(BaseModel):
    """条件分支。"""
    condition: str = Field(description="条件表达式")
    then: list[WorkflowStep] = Field(description="条件满足时执行")
    else_: Optional[list[WorkflowStep]] = Field(default=None, alias="else")


class WorkflowLoop(BaseModel):
    """循环。"""
    over: str = Field(description="遍历对象引用")
    as_: str = Field(description="循环变量名")
    steps: list[WorkflowStep] = Field(description="循环体")


class WorkflowTrigger(BaseModel):
    """触发器。"""
    type: str = Field(description="cron | manual | event")
    cron: Optional[str] = None
    event: Optional[str] = None


class WorkflowDefinition(BaseModel):
    """完整工作流定义。"""
    name: str
    description: Optional[str] = None
    version: str = "1.0"
    trigger: Optional[WorkflowTrigger] = None
    variables: dict[str, Any] = Field(default_factory=dict)
    steps: list[WorkflowStep | WorkflowCondition | WorkflowLoop] = Field(...)


class VariableResolver:
    """变量解析器。处理 {{ .xxx.yyy }} 表达式。"""

    BUILTINS = {
        "date.YYYYMMDD": lambda: datetime.now(timezone.utc).strftime("%Y%m%d"),
        "date.YYYY-MM-DD": lambda: datetime.now(timezone.utc).strftime("%Y-%m-%d"),
        "date.HHmmss": lambda: datetime.now(timezone.utc).strftime("%H%M%S"),
        "env": lambda: {},
    }

    def __init__(self, context: dict[str, Any]) -> None:
        self._ctx = context

    def resolve(self, template: str) -> str:
        """将 {{ .xxx.yyy }} 替换为实际值。"""
        def _replace(m: re.Match) -> str:
            path = m.group(1).strip()
            return str(self._get_value(path))
        return re.sub(r"\{\{\s*(\.?[\w.]+)\s*\}\}", _replace, template)

    def resolve_params(self, params: dict) -> dict:
        """解析参数字典中的所有变量引用。"""
        result = {}
        for k, v in params.items():
            if isinstance(v, str):
                result[k] = self.resolve(v)
            elif isinstance(v, dict):
                result[k] = self.resolve_params(v)
            elif isinstance(v, list):
                result[k] = [self.resolve(item) if isinstance(item, str) else item for item in v]
            else:
                result[k] = v
        return result

    def _get_value(self, path: str) -> Any:
        if path.startswith("."):
            path = path[1:]
        parts = path.split(".")

        # Builtins
        if path in self.BUILTINS:
            val = self.BUILTINS[path]
            return val() if callable(val) else val

        # Context traversal
        val = self._ctx
        for part in parts:
            if isinstance(val, dict):
                val = val.get(part, f"{{{{.{path}}}}}")
            else:
                return f"{{{{.{path}}}}}"
        return val if val is not None else ""
