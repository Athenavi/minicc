import mimetypes
from pathlib import Path
from typing import Optional

from fastapi import APIRouter, HTTPException
from pydantic import BaseModel, Field

router = APIRouter(prefix="/api/editor", tags=["editor"])

WORKSPACE = Path.cwd()
SKIP_EXTS = {".pyc", ".pyo", ".so", ".dll", ".dylib", ".exe", ".bin", ".png", ".jpg", ".gif", ".ico", ".svg"}
SKIP_DIRS = {".git", "node_modules", "__pycache__", ".venv", "venv", ".next", "dist", "build", ".reasonix"}


def _safe(path: str) -> Path:
    """路径沙箱 — 阻止路径遍历。"""
    p = (WORKSPACE / path).resolve()
    if not str(p).startswith(str(WORKSPACE.resolve())):
        raise HTTPException(403, "Path traversal blocked")
    return p


class WriteRequest(BaseModel):
    path: str = Field(description="File path relative to workspace")
    content: str = Field(description="File content")


@router.get("/files")
async def list_files():
    """列出工作区目录树。"""
    workspace = Path.cwd()
    files = _build_tree(workspace)
    return {"files": files, "root": workspace.name}


@router.get("/read")
async def read_file(path: str):
    """读取文件内容。"""
    safe = _safe(path)
    if not safe.exists():
        raise HTTPException(404, f"File not found: {path}")
    if safe.is_dir():
        raise HTTPException(400, f"Path is a directory: {path}")
    if safe.stat().st_size > 1024 * 1024:
        raise HTTPException(413, "File too large (>1MB)")
    try:
        content = safe.read_text(encoding="utf-8", errors="replace")
        return {"content": content, "path": path, "size": len(content)}
    except Exception as exc:
        raise HTTPException(500, f"Read error: {exc}")


@router.post("/write")
async def write_file(req: WriteRequest):
    """写入文件内容。"""
    safe = _safe(req.path)
    safe.parent.mkdir(parents=True, exist_ok=True)
    try:
        # 原子写入：tmp + rename
        tmp = safe.with_suffix(safe.suffix + ".tmp")
        tmp.write_text(req.content, encoding="utf-8")
        tmp.rename(safe)
        return {"path": req.path, "size": len(req.content), "status": "saved"}
    except Exception as exc:
        raise HTTPException(500, f"Write error: {exc}")


@router.post("/create")
async def create_file(req: WriteRequest):
    """创建新文件。"""
    safe = _safe(req.path)
    if safe.exists():
        raise HTTPException(409, f"File already exists: {req.path}")
    safe.parent.mkdir(parents=True, exist_ok=True)
    safe.write_text(req.content or "", encoding="utf-8")
    return {"path": req.path, "status": "created"}


@router.delete("/delete")
async def delete_file(path: str):
    """删除文件或空目录。"""
    safe = _safe(path)
    if not safe.exists():
        raise HTTPException(404, f"Not found: {path}")
    if safe.is_dir():
        safe.rmdir()
    else:
        safe.unlink()
    return {"path": path, "status": "deleted"}


@router.post("/rename")
async def rename_file(old_path: str, new_path: str):
    """重命名/移动文件。"""
    old_safe = _safe(old_path)
    new_safe = _safe(new_path)
    if not old_safe.exists():
        raise HTTPException(404, f"Not found: {old_path}")
    new_safe.parent.mkdir(parents=True, exist_ok=True)
    old_safe.rename(new_safe)
    return {"old_path": old_path, "new_path": new_path, "status": "renamed"}


@router.get("/search")
async def search_files(query: str):
    """搜索文件名。"""
    workspace = Path.cwd()
    results = []
    for path in workspace.rglob(f"*{query}*"):
        if path.name.startswith("."):
            continue
        if any(skip in path.parts for skip in SKIP_DIRS):
            continue
        if path.is_file() and path.suffix not in SKIP_EXTS:
            rel = str(path.relative_to(workspace))
            results.append({"path": rel, "name": path.name, "ext": path.suffix})
    return {"results": results[:50], "total": len(results)}


def _build_tree(dir_path: Path) -> list[dict]:
    """构建目录树（仅前 3 层，大目录截断）。"""
    entries = []
    try:
        children = sorted(dir_path.iterdir(), key=lambda x: (not x.is_dir(), x.name.lower()))
    except PermissionError:
        return entries

    for i, child in enumerate(children):
        if i > 200:
            break
        skip = child.name.startswith(".") or child.name in SKIP_DIRS or child.suffix in SKIP_EXTS
        if child.is_dir():
            if child.name in SKIP_DIRS:
                continue
            sub = _build_tree(child) if child.name[0] != "." else []
            entries.append({"name": child.name + "/", "path": str(child.relative_to(Path.cwd())), "type": "dir", "children": sub[:100]})
        elif not skip:
            entries.append({"name": child.name, "path": str(child.relative_to(Path.cwd())), "type": "file"})
    return entries
