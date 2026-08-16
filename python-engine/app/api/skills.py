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
