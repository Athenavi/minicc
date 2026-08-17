"""Graph/Workflow API endpoints — CRUD + execution with PostgreSQL persistence."""
from __future__ import annotations

import asyncio
import datetime
import json
import logging
import time
import uuid
from typing import Any, Optional

from fastapi import APIRouter, Depends, HTTPException, Query
from pydantic import BaseModel

from app.db import get_pool
from app.main import get_gateway
from app.workflow.engine import run_workflow, get_instance

logger = logging.getLogger(__name__)

router = APIRouter(tags=["graphs"])

# 保存后台任务引用，防止被 GC 回收导致工作流静默丢失（asyncio.create_task 必须持有引用）
_background_tasks: set[asyncio.Task] = set()


class GraphCreateRequest(BaseModel):
    id: Optional[str] = None
    name: str
    graph_json: Any = {}
    user_id: Optional[str] = None


class GraphExecuteRequest(BaseModel):
    name: str = "workflow"
    nodes: list[dict[str, Any]] = []
    edges: list[dict[str, Any]] = []
    initial_state: dict[str, Any] = {}


@router.get("/v1/graphs")
async def list_graphs(user_id: Optional[str] = Query(None, alias="user_id")):
    """List all graphs, optionally filtered by user_id."""
    try:
        pool = get_pool()
        if user_id:
            rows = await pool.fetch(
                """SELECT id, name, COALESCE(user_id::VARCHAR,'') as user_id, graph_json, created_at, updated_at
                   FROM workflow_graphs WHERE user_id::VARCHAR = $1
                   ORDER BY updated_at DESC NULLS LAST, created_at DESC LIMIT 100""",
                str(user_id),
            )
        else:
            rows = await pool.fetch(
                """SELECT id, name, COALESCE(user_id::VARCHAR,'') as user_id, graph_json, created_at, updated_at
                   FROM workflow_graphs
                   ORDER BY updated_at DESC NULLS LAST, created_at DESC LIMIT 100"""
            )
        results = [dict(r) for r in rows]
        for r in results:
            if isinstance(r.get("graph_json"), str):
                r["graph_json"] = json.loads(r["graph_json"])
            r["created_at"] = r["created_at"].isoformat() if r.get("created_at") else ""
            r["updated_at"] = r["updated_at"].isoformat() if r.get("updated_at") else ""
        return {"success": True, "data": results}
    except Exception as e:
        logger.warning("graph list failed: %s", e)
        return {"success": False, "error": str(e), "data": []}


@router.post("/v1/graphs")
async def create_graph(body: GraphCreateRequest, user_id: str = Query("", alias="user_id")):
    """Create or update a graph definition."""
    import uuid as _uuid

    # 优先使用请求体中的 user_id，再回退到查询参数
    effective_user_id = body.user_id or user_id
    if not effective_user_id:
        raise HTTPException(status_code=400, detail="user_id is required")

    graph_id = body.id or f"g-{_uuid.uuid4().hex[:10]}"
    now = datetime.datetime.now(datetime.timezone.utc)

    graph_json = body.graph_json
    if isinstance(graph_json, str):
        try:
            graph_json = json.loads(graph_json)
        except json.JSONDecodeError:
            pass

    record = {
        "id": graph_id,
        "name": body.name,
        "user_id": effective_user_id,
        "graph_json": graph_json,
        "created_at": now.isoformat(),
        "updated_at": now.isoformat(),
    }

    try:
        pool = get_pool()
        await pool.execute(
            """INSERT INTO workflow_graphs (id, name, user_id, graph_json, created_at, updated_at)
               VALUES ($1, $2, $3, $4, $5, $6)
               ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, graph_json = EXCLUDED.graph_json, updated_at = EXCLUDED.updated_at""",
            graph_id, body.name, effective_user_id or None, json.dumps(graph_json), now, now,
        )
    except Exception as e:
        logger.error("graph insert failed: %s", e)
        raise HTTPException(status_code=500, detail="failed to save graph")

    return {"success": True, "data": record}


@router.get("/v1/graphs/{graph_id}")
async def get_graph(graph_id: str):
    """Get a graph by ID."""
    try:
        pool = get_pool()
        row = await pool.fetchrow(
            """SELECT id, name, COALESCE(user_id,'') as user_id, graph_json, created_at, updated_at
               FROM workflow_graphs WHERE id = $1""",
            graph_id,
        )
        if row:
            record = dict(row)
            if isinstance(record.get("graph_json"), str):
                record["graph_json"] = json.loads(record["graph_json"])
            record["created_at"] = record["created_at"].isoformat() if record.get("created_at") else ""
            record["updated_at"] = record["updated_at"].isoformat() if record.get("updated_at") else ""
            return {"success": True, "data": record}
    except Exception as e:
        logger.warning("graph get failed: %s", e)

    raise HTTPException(status_code=404, detail="graph not found")


@router.delete("/v1/graphs/{graph_id}")
async def delete_graph(graph_id: str):
    """Delete a graph by ID."""
    if not graph_id or graph_id.strip() == "":
        raise HTTPException(status_code=400, detail="graph_id is required")
    try:
        pool = get_pool()
        result = await pool.execute("DELETE FROM workflow_graphs WHERE id = $1", graph_id)
        if result == "DELETE 0":
            raise HTTPException(status_code=404, detail="graph not found")
    except HTTPException:
        raise
    except Exception as e:
        logger.error("graph delete failed: %s", e)
        raise HTTPException(status_code=500, detail="failed to delete graph")

    return {"success": True, "message": f"Graph {graph_id} deleted"}


@router.post("/v1/graphs/{graph_id}/execute")
async def execute_graph(
    graph_id: str,
    body: GraphExecuteRequest,
    gateway=Depends(get_gateway),
    user_id: str = Query("", alias="user_id"),
):
    """提交图执行：落库 pending 后后台任务执行，立即返回 instance_id（前端轮询状态）。"""
    graph_json = None

    # 尝试从 PostgreSQL 加载
    try:
        pool = get_pool()
        row = await pool.fetchrow(
            "SELECT graph_json FROM workflow_graphs WHERE id = $1", graph_id,
        )
        if row:
            graph_json = row["graph_json"]
            if isinstance(graph_json, str):
                graph_json = json.loads(graph_json)
    except Exception as e:
        logger.warning("graph load for execute failed: %s", e)

    # Fallback to request body
    if not graph_json:
        graph_json = {"name": body.name or graph_id, "nodes": body.nodes, "edges": body.edges}

    graph_name = graph_json.get("name") or graph_id
    instance_id = f"wf_{uuid.uuid4().hex[:10]}"
    now = datetime.datetime.now(datetime.timezone.utc)

    # 落库 running（后台任务完成后更新）
    try:
        pool = get_pool()
        await pool.execute(
            """INSERT INTO workflow_instances (id, user_id, workflow_id, workflow_name, status, results, created_at, updated_at)
               VALUES ($1, $2, $3, $4, 'running', '{}', $5, $5)""",
            instance_id, user_id, graph_id, graph_name, now,
        )
    except Exception as e:
        logger.warning("workflow instance insert failed: %s", e)

    task = asyncio.create_task(_run_and_store(instance_id, graph_json, gateway, body.initial_state, user_id))
    _background_tasks.add(task)
    task.add_done_callback(_background_tasks.discard)

    return {"instance_id": instance_id, "status": "running", "workflow": graph_name}


async def _run_and_store(
    instance_id: str,
    graph_json: dict,
    gateway: Any,
    initial_state: dict[str, Any],
    user_id: str = "",
) -> None:
    """后台执行工作流并把最终结果写回 workflow_instances。"""
    # 工具沙箱上下文：create_task 不继承 contextvars，必须在任务内显式设置
    # （MCP 插件工具/技能等按当前用户过滤，S 安全修复）
    from app.tools.context import set_tool_context
    set_tool_context(session_id=instance_id, user_id=user_id or "", tenant_id=user_id or "")

    status = "error"
    results_json = "{}"
    error_text = ""
    try:
        instance = await run_workflow(graph_json, gateway, initial_state, instance_id=instance_id)
        if instance.status == "completed":
            status = "completed"
        else:
            error_text = "workflow execution failed"
        results_json = json.dumps({
            nid: {"status": nr.status, "output": nr.output}
            for nid, nr in instance.results.items()
        })
    except Exception as e:  # noqa: BLE001
        logger.error("workflow background execution failed: %s", e)
        error_text = str(e)

    try:
        pool = get_pool()
        await pool.execute(
            """UPDATE workflow_instances SET status = $1, results = $2, error = $3, updated_at = NOW()
               WHERE id = $4""",
            status, results_json, error_text or None, instance_id,
        )
    except Exception as e:
        logger.warning("workflow instance update failed: %s", e)


@router.get("/v1/workflows/instances")
async def list_instances(user_id: str = Query("", alias="user_id")):
    """持久化的工作流执行历史（按用户）。"""
    try:
        pool = get_pool()
        if user_id:
            rows = await pool.fetch(
                """SELECT id, workflow_id, workflow_name, status, results, COALESCE(error,'') as error, created_at, updated_at
                   FROM workflow_instances WHERE user_id = $1
                   ORDER BY created_at DESC LIMIT 100""",
                user_id,
            )
        else:
            rows = await pool.fetch(
                """SELECT id, workflow_id, workflow_name, status, results, COALESCE(error,'') as error, created_at, updated_at
                   FROM workflow_instances ORDER BY created_at DESC LIMIT 100"""
            )
        out = []
        for r in rows:
            d = dict(r)
            if isinstance(d.get("results"), str):
                try:
                    d["results"] = json.loads(d["results"])
                except json.JSONDecodeError:
                    d["results"] = {}
            d["created_at"] = d["created_at"].isoformat() if d.get("created_at") else ""
            d["updated_at"] = d["updated_at"].isoformat() if d.get("updated_at") else ""
            out.append(d)
        return {"success": True, "data": out}
    except Exception as e:
        logger.warning("workflow instances list failed: %s", e)
        return {"success": False, "error": str(e), "data": []}


@router.get("/v1/workflows/{instance_id}/status")
async def workflow_status(instance_id: str):
    """获取工作流执行状态（内存实例优先，落库记录回退）。"""
    inst = get_instance(instance_id)
    if inst:
        return {
            "instance_id": inst.instance_id,
            "workflow": inst.graph_name,
            "status": inst.status,
            "results": {nid: {"status": nr.status, "output": nr.output} for nid, nr in inst.results.items()},
        }
    try:
        pool = get_pool()
        row = await pool.fetchrow(
            """SELECT id, workflow_name, status, results, COALESCE(error,'') as error
               FROM workflow_instances WHERE id = $1""",
            instance_id,
        )
        if row:
            results = row["results"]
            if isinstance(results, str):
                try:
                    results = json.loads(results)
                except json.JSONDecodeError:
                    results = {}
            return {
                "instance_id": row["id"],
                "workflow": row["workflow_name"],
                "status": row["status"],
                "results": results,
                "error": row["error"] or "",
            }
    except Exception as e:
        logger.warning("workflow status db fallback failed: %s", e)
    raise HTTPException(status_code=404, detail="workflow instance not found")
