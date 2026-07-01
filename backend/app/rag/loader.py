"""多源文档加载器 — PDF/HTML/Office/代码/Web。对标 Dify 文档加载。"""

from __future__ import annotations

import logging
from abc import ABC, abstractmethod
from pathlib import Path
from typing import Any, Optional

from pydantic import BaseModel, Field

logger = logging.getLogger("minicc.rag.loader")

SKIP_EXTS = {".pyc", ".pyo", ".so", ".dll", ".dylib", ".exe", ".bin", ".png", ".jpg", ".gif", ".ico"}
SKIP_DIRS = {".git", "node_modules", "__pycache__", ".venv", "venv", ".next", "dist", "build"}


class Document(BaseModel):
    """一个加载的文档。"""
    content: str
    metadata: dict[str, Any] = Field(default_factory=dict)
    source: str = ""


class BaseLoader(ABC):
    """文档加载器基类。"""

    @abstractmethod
    async def load(self, source: str) -> list[Document]:
        ...


class PDFLoader(BaseLoader):
    """PDF 文档加载器 (PyMuPDF)。"""

    async def load(self, source: str) -> list[Document]:
        path = Path(source)
        if not path.exists():
            return [Document(content=f"File not found: {source}", metadata={"error": True}, source=source)]
        try:
            import fitz
            doc = fitz.open(source)
            pages = []
            for i, page in enumerate(doc):
                text = page.get_text()
                if text.strip():
                    pages.append(Document(
                        content=text,
                        metadata={"page": i + 1, "total": len(doc)},
                        source=source,
                    ))
            doc.close()
            logger.info("PDF loaded: %s (%d pages)", path.name, len(pages))
            return pages
        except Exception as exc:
            return [Document(content=f"PDF error: {exc}", metadata={"error": True}, source=source)]


class TextLoader(BaseLoader):
    """纯文本/代码文件加载器。"""

    async def load(self, source: str) -> list[Document]:
        path = Path(source)
        if not path.exists():
            return [Document(content=f"File not found: {source}", metadata={"error": True}, source=source)]
        try:
            content = path.read_text(encoding="utf-8", errors="replace")
            return [Document(content=content, metadata={"size": len(content)}, source=source)]
        except Exception as exc:
            return [Document(content=f"Read error: {exc}", metadata={"error": True}, source=source)]


class CodeDirLoader(BaseLoader):
    """代码目录递归加载器。跳过 SKIP_DIRS/SKIP_EXTS。"""

    async def load(self, source: str) -> list[Document]:
        root = Path(source)
        if not root.exists():
            return [Document(content=f"Directory not found: {source}", metadata={"error": True}, source=source)]

        docs = []
        for i, path in enumerate(root.rglob("*")):
            if i > 5000:
                break
            if path.is_file() and path.suffix not in SKIP_EXTS:
                if any(p in SKIP_DIRS for p in path.parts):
                    continue
                if path.stat().st_size > 1024 * 1024:
                    continue
                try:
                    content = path.read_text(encoding="utf-8", errors="replace")
                    if content.strip():
                        rel = str(path.relative_to(root))
                        docs.append(Document(
                            content=content[:50000],
                            metadata={"path": rel, "size": len(content)},
                            source=rel,
                        ))
                except Exception:
                    continue

        logger.info("Code dir loaded: %s (%d files)", root.name, len(docs))
        return docs


class CompositeLoader(BaseLoader):
    """组合加载器 — 自动根据源类型选择加载器。"""

    def __init__(self) -> None:
        self._loaders: dict[str, BaseLoader] = {
            ".pdf": PDFLoader(),
            ".txt": TextLoader(),
            ".md": TextLoader(),
            ".py": TextLoader(),
            ".js": TextLoader(),
            ".ts": TextLoader(),
            ".tsx": TextLoader(),
            ".jsx": TextLoader(),
            ".json": TextLoader(),
            ".yaml": TextLoader(),
            ".yml": TextLoader(),
            ".toml": TextLoader(),
            ".cfg": TextLoader(),
            ".ini": TextLoader(),
            ".env": TextLoader(),
            ".css": TextLoader(),
            ".html": TextLoader(),
            ".xml": TextLoader(),
        }

    async def load(self, source: str) -> list[Document]:
        path = Path(source)
        if path.is_dir():
            return await CodeDirLoader().load(source)
        loader = self._loaders.get(path.suffix.lower(), TextLoader())
        return await loader.load(source)
