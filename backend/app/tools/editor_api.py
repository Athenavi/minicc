"""AI 编辑器 API — 内联补全 + Natural Edit。"""

from __future__ import annotations

from typing import Optional

from fastapi import APIRouter
from pydantic import BaseModel, Field

router = APIRouter(prefix="/api/editor", tags=["editor"])


class CompletionRequest(BaseModel):
    path: str = Field(default="", description="File path")
    prefix: str = Field(description="Text before cursor")
    suffix: str = Field(default="", description="Text after cursor")
    language: str = Field(default="plaintext", description="Language ID")


class NaturalEditRequest(BaseModel):
    path: str = Field(description="File path")
    selected_range: dict = Field(description="Selected range: {start: {line, col}, end: {line, col}}")
    instruction: str = Field(description="Natural language edit instruction")
    code_context: str = Field(description="Selected code")


@router.post("/complete")
async def inline_complete(req: CompletionRequest):
    """内联代码补全 — 模拟 LLM 补全。"""
    # 简单规则：括号/引号配对
    completions = {
        "(": ")", "[": "]", "{": "}", '"': '"', "'": "'",
    }

    last_char = req.prefix.strip()[-1:] if req.prefix.strip() else ""

    if last_char in completions:
        return {"completion": completions[last_char]}

    # Python 补全
    if req.language == "python":
        if req.prefix.rstrip().endswith(":"):
            return {"completion": "\n    pass"}
        if req.prefix.rstrip().endswith("def "):
            return {"completion": "function_name():\n    pass"}
        if req.prefix.rstrip().endswith("class "):
            return {"completion": "ClassName:\n    pass"}
        if req.prefix.rstrip().endswith("import "):
            return {"completion": "os, sys, json"}
        if req.prefix.rstrip().endswith("from "):
            return {"completion": "typing import "}
        if "async def" in req.prefix:
            return {"completion": ":\n    "}
        if req.prefix.strip().endswith("return "):
            return {"completion": "None"}
        if req.prefix.strip().endswith("="):
            return {"completion": " "}

    # TypeScript/JavaScript 补全
    elif req.language in ("typescript", "javascript"):
        if req.prefix.rstrip().endswith("=>"):
            return {"completion": " {"}
        if req.prefix.rstrip().endswith("function "):
            return {"completion": "name() {\n    "}
        if req.prefix.rstrip().endswith("const ") or req.prefix.rstrip().endswith("let ") or req.prefix.rstrip().endswith("var "):
            return {"completion": "name = "}

    return {"completion": ""}


@router.post("/natural-edit")
async def natural_edit(req: NaturalEditRequest):
    """Natural Edit — 将自然语言指令转换为代码修改。"""
    return {
        "suggestions": [
            f"# AI suggestion based on: {req.instruction}\n"
            f"# This feature requires Phase N3 Natural Edit integration with LLM.\n"
            f"# Selected code ({len(req.code_context)} chars):\n"
            f"{req.code_context[:200]}"
        ],
        "explanation": f"Natural Edit: '{req.instruction}'. Connect LLM to generate actual suggestions.",
    }
