"""共享 pytest fixtures。"""

from __future__ import annotations

from pathlib import Path
from tempfile import TemporaryDirectory
from typing import Generator

import pytest
from fastapi.testclient import TestClient

from app.main import app, tool_registry
from app.tools.base import BaseTool, ToolRegistry
from app.utils.security import PathValidator


@pytest.fixture
def client() -> Generator[TestClient, None, None]:
    with TestClient(app) as c:
        yield c


@pytest.fixture
def registry() -> ToolRegistry:
    return tool_registry


@pytest.fixture
def temp_workspace() -> Generator[Path, None, None]:
    with TemporaryDirectory() as tmp:
        yield Path(tmp)


@pytest.fixture
def validator(temp_workspace: Path) -> PathValidator:
    return PathValidator(temp_workspace)
