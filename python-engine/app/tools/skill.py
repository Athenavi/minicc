"""Skill tools (skill_list / skill_install / skill_generate / skill_discover) 注册到本地工具注册表。

实现对标 Go `internal/skill/tools.go`，使用磁盘 SkillStore 作为后端。
"""
from __future__ import annotations

import json
import os
from typing import Any

from app.tools.registry import registry
from app.skill.store import SkillStore, SkillDef

_skill_root = os.getenv("SKILL_STORE_PATH", os.path.join(".", "data", "skills"))
_store = SkillStore(_skill_root)


async def skill_list() -> dict[str, Any]:
    skills = _store.list()
    if not skills:
        return {"output": "No skills installed.", "count": 0, "skills": []}
    lines = [f"  - {s.name}: {s.description} (v{s.version}, {s.exec_type})" for s in skills]
    return {"output": "\n".join(lines), "count": len(skills), "skills": [s.to_dict() for s in skills]}


async def skill_install(url: str = "", file: str = "", inline: str = "") -> dict[str, Any]:
    if not url and not file and not inline:
        return {"error": "one of url, file, or inline is required"}

    try:
        if inline:
            data = json.loads(inline)
        elif file:
            from app.tools.core import _safe_path
            safe_file = _safe_path(file, _skill_root)
            data = json.loads(safe_file.read_text(encoding="utf-8"))
        else:
            import httpx
            async with httpx.AsyncClient(timeout=20) as client:
                resp = await client.get(url)
                resp.raise_for_status()
                data = resp.json()
    except Exception as e:
        return {"error": f"failed to load skill definition: {e}"}

    exec_cfg = data.get("exec", {})
    skill = SkillDef(
        name=data["name"],
        description=data.get("description", ""),
        version=data.get("version", "0.1.0"),
        author=data.get("author", ""),
        tags=data.get("tags", []),
        exec_type=exec_cfg.get("type", "prompt"),
        source=exec_cfg.get("source", ""),
        parameters=data.get("parameters", []),
    )
    _store.save(skill)
    return {"output": f"Skill installed: {skill.name} (v{skill.version})\n{json.dumps(skill.to_dict(), ensure_ascii=False, indent=2)}", "skill": skill.name, "version": skill.version}


async def skill_generate(description: str, install: bool = False) -> dict[str, Any]:
    if not description:
        return {"error": "description is required"}

    name = description.strip().lower().replace(" ", "_")[:32] or "generated_skill"
    skill = SkillDef(
        name=name,
        description=description,
        version="0.1.0",
        exec_type="prompt",
        source=f"Generate a concise prompt-based skill for: {description}",
    )
    result: dict[str, Any] = {
        "output": f"Generated skill definition:\n{json.dumps(skill.to_dict(), ensure_ascii=False, indent=2)}",
        "skill": skill.to_dict(),
        "name": skill.name,
        "type": skill.exec_type,
    }

    if install:
        try:
            _store.save(skill)
            result["installed"] = True
            result["output"] = f"Generated and installed skill: {skill.name}\n{json.dumps(skill.to_dict(), ensure_ascii=False, indent=2)}"
        except Exception as e:
            result["install_error"] = str(e)

    return result


async def skill_discover(url: str = "") -> dict[str, Any]:
    results: list[dict[str, Any]] = []

    if url:
        try:
            import httpx
            async with httpx.AsyncClient(timeout=20) as client:
                resp = await client.get(url)
                resp.raise_for_status()
                items = resp.json()
                installed_names = {s.name for s in _store.list()}
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
            return {"error": f"discover remote failed: {e}"}
    else:
        for s in _store.list():
            results.append({
                "name": s.name,
                "description": s.description,
                "version": s.version,
                "author": s.author,
                "source": s.source,
                "installed": True,
            })

    lines = [f"  [{'✓' if r['installed'] else ' '}] {r['name']}: {r['description']} (v{r['version']})" for r in results]
    return {"output": f"Available skills ({len(results)})\n" + "\n".join(lines), "count": len(results), "results": results}


async def skill_run(name: str, params: str = "{}") -> dict[str, Any]:
    """执行一个已安装的技能，渲染提示词模板并返回结果"""
    skill = _store.get(name)
    if not skill:
        return {"error": f"Skill not found: {name}"}

    if skill.exec_type != "prompt":
        return {"error": f"Unsupported skill type: {skill.exec_type}"}

    try:
        param_dict = json.loads(params) if isinstance(params, str) else params
    except json.JSONDecodeError:
        param_dict = {"input": params}

    # 简单模板渲染：替换 {key} 为参数值
    source = skill.source
    for key, value in param_dict.items():
        source = source.replace("{" + key + "}", str(value))

    return {
        "output": source,
        "skill": name,
        "rendered": True,
    }


registry.register(
    name="skill_list",
    description="List all installed skills with their descriptions and versions.",
    parameters={"type": "object", "properties": {}},
    handler=skill_list,
)

registry.register(
    name="skill_install",
    description="Install a skill from a URL, local file path, or inline JSON definition.",
    parameters={
        "type": "object",
        "properties": {
            "url": {"type": "string", "default": ""},
            "file": {"type": "string", "default": ""},
            "inline": {"type": "string", "default": ""},
        },
    },
    handler=skill_install,
)

registry.register(
    name="skill_generate",
    description="Generate a new skill from a natural language description and optionally install it.",
    parameters={
        "type": "object",
        "properties": {
            "description": {"type": "string"},
            "install": {"type": "boolean", "default": False},
        },
        "required": ["description"],
    },
    handler=skill_generate,
)

registry.register(
    name="skill_discover",
    description="Discover available skills from local directory or remote index.",
    parameters={
        "type": "object",
        "properties": {
            "url": {"type": "string", "default": ""},
        },
    },
    handler=skill_discover,
)

registry.register(
    name="skill_run",
    description="Execute an installed skill: render its prompt template with provided parameters and return the result.",
    parameters={
        "type": "object",
        "properties": {
            "name": {"type": "string", "description": "Name of the installed skill"},
            "params": {"type": "string", "default": "{}", "description": "JSON string of parameters to inject into the skill template"},
        },
        "required": ["name"],
    },
    handler=skill_run,
)
