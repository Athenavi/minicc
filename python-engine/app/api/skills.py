"""Skill API endpoints — CRUD + discover, backed by SkillStore."""
from __future__ import annotations

import json
import re
from typing import Any

from fastapi import APIRouter, HTTPException
from pydantic import BaseModel

from app.skill.store import SkillStore, SkillDef

router = APIRouter(tags=["skills"])

# Shared store instance (same directory as tools/skill.py)
import os
_skill_root = os.getenv("SKILL_STORE_PATH", os.path.join(".", "data", "skills"))
store = SkillStore(_skill_root)


@router.get("/v1/skills")
async def list_skills() -> dict[str, Any]:
    skills = store.list()
    return {
        "skills": [s.to_dict() for s in skills],
        "count": len(skills),
    }


class SkillInstallRequest(BaseModel):
    url: str = ""
    file: str = ""
    inline: str = ""


@router.post("/v1/skills/{name}/register")
async def register_skill(name: str) -> dict[str, Any]:
    """将能力注册中心的技能注册为本地技能（SkillStore），供对话/Agent 工具链调用。

    前端 SkillMarketCard 调用 POST /v1/skills/{capabilityId}/register；
    能力注册中心查询不到时返回 404，避免假注册。
    """
    from app.core.capabilities import get_registry

    cap = get_registry().get(name)
    if cap is None:
        raise HTTPException(status_code=404, detail=f"capability not found: {name}")

    existing = [s for s in store.list() if s.name == name]
    if existing:
        return {"success": True, "skill": name, "registered": True}

    skill = SkillDef(
        name=cap.name or name,
        description=cap.description or f"Skill {name}",
        version="0.1.0",
        exec_type="prompt",
        source=f"Use the {name} capability. {cap.description or ''}".strip(),
    )
    store.save(skill)
    return {
        "success": True,
        "skill": skill.name,
        "registered": True,
        "description": skill.description,
    }


@router.post("/v1/skills/install")
async def install_skill(body: SkillInstallRequest) -> dict[str, Any]:
    if not body.url and not body.file and not body.inline:
        raise HTTPException(status_code=400, detail="provide url, file, or inline")

    try:
        if body.inline:
            data = json.loads(body.inline)
        elif body.file:
            from app.tools.core import _safe_path
            skill_root = os.getenv("SKILL_STORE_PATH", os.path.join(".", "data", "skills"))
            safe_file = _safe_path(body.file, skill_root)
            data = json.loads(safe_file.read_text(encoding="utf-8"))
        else:
            import httpx
            async with httpx.AsyncClient(timeout=20) as client:
                resp = await client.get(body.url)
                resp.raise_for_status()
                data = resp.json()
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"failed to load skill definition: {e}")

    exec_cfg = data.get("exec", {})
    skill_name = data.get("name", "")
    if not skill_name:
        raise HTTPException(status_code=400, detail="skill definition missing 'name'")
    skill = SkillDef(
        name=skill_name,
        description=data.get("description", ""),
        version=data.get("version", "0.1.0"),
        author=data.get("author", ""),
        tags=data.get("tags", []),
        exec_type=exec_cfg.get("type", "prompt"),
        source=exec_cfg.get("source", ""),
        parameters=data.get("parameters", []),
    )
    store.save(skill)
    return {"skill": skill.to_dict(), "message": f"Skill '{skill.name}' installed"}


class SkillGenerateRequest(BaseModel):
    description: str
    auto_install: bool = False


@router.post("/v1/skills/generate")
async def generate_skill(body: SkillGenerateRequest) -> dict[str, Any]:
    if not body.description:
        raise HTTPException(status_code=400, detail="description is required")

    name = body.description.strip().lower().replace(" ", "_")[:32] or "generated_skill"
    skill = SkillDef(
        name=name,
        description=body.description,
        version="0.1.0",
        exec_type="prompt",
        source=f"Generate a concise prompt-based skill for: {body.description}",
    )
    result: dict[str, Any] = {"skill": skill.to_dict(), "message": "Skill generated"}

    if body.auto_install:
        store.save(skill)
        result["message"] = f"Skill '{skill.name}' generated and installed"

    return result


@router.delete("/v1/skills/{name}")
async def delete_skill(name: str) -> dict[str, str]:
    if not re.match(r"^[a-zA-Z0-9_.-]+$", name):
        raise HTTPException(status_code=400, detail="invalid skill name")
    if not store.get(name):
        raise HTTPException(status_code=404, detail=f"Skill '{name}' not found")
    store.delete(name)
    return {"message": f"Skill '{name}' deleted"}


class SkillToggleRequest(BaseModel):
    enabled: bool = True


@router.put("/v1/skills/{name}")
async def toggle_skill(name: str, body: SkillToggleRequest) -> dict[str, Any]:
    """启用/停用技能（停用后不进 agent 目录、不可运行）。"""
    if not re.match(r"^[a-zA-Z0-9_.-]+$", name):
        raise HTTPException(status_code=400, detail="invalid skill name")
    skill = store.get(name)
    if not skill:
        raise HTTPException(status_code=404, detail=f"Skill '{name}' not found")
    skill.enabled = body.enabled
    store.save(skill)
    return {
        "skill": skill.to_dict(),
        "message": f"Skill '{name}' {'已启用' if body.enabled else '已停用'}",
    }


class SkillRunRequest(BaseModel):
    params: dict[str, Any] = {}


@router.post("/v1/skills/{name}/run")
async def run_skill(name: str, body: SkillRunRequest) -> dict[str, Any]:
    """运行技能（校验启用状态后调 skill_run）。"""
    if not re.match(r"^[a-zA-Z0-9_.-]+$", name):
        raise HTTPException(status_code=400, detail="invalid skill name")
    skill = store.get(name)
    if not skill:
        raise HTTPException(status_code=404, detail=f"Skill '{name}' not found")
    if not skill.enabled:
        raise HTTPException(status_code=400, detail=f"Skill '{name}' 已停用")

    import json as _json
    from app.tools.skill import skill_run

    return await skill_run(name, _json.dumps(body.params))


@router.get("/v1/skills/discover")
async def discover_skills(url: str = "") -> dict[str, Any]:
    results: list[dict[str, Any]] = []

    if url:
        try:
            import httpx
            async with httpx.AsyncClient(timeout=20) as client:
                resp = await client.get(url)
                resp.raise_for_status()
                items = resp.json()
                installed_names = {s.name for s in store.list()}
                for item in items:
                    name = item.get("name", "")
                    results.append({
                        "name": name,
                        "description": item.get("description", ""),
                        "version": item.get("version", ""),
                        "author": item.get("author", ""),
                        "source": item.get("source", url),
                        "installed": name in installed_names,
                    })
        except Exception as e:
            raise HTTPException(status_code=500, detail=f"discover remote failed: {e}")
    else:
        for s in store.list():
            results.append({
                "name": s.name,
                "description": s.description,
                "version": s.version,
                "author": s.author,
                "source": s.source,
                "installed": True,
            })

    return {"local": results, "remote": []}
