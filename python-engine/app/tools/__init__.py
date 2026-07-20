"""工具模块

包含：
- SystemToolClient / ToolDiscovery（兼容旧 Go 调用链）
- 本地 Python 工具注册表（registry）与核心工具实现（core / memory / pm / skill / graph / agent / browser / media）
"""
from app.tools.client import SystemToolClient
from app.tools.discovery import ToolDiscovery
from app.tools.registry import ToolRegistry, registry
import app.tools.core  # noqa: F401
import app.tools.memory  # noqa: F401
import app.tools.pm  # noqa: F401
import app.tools.skill  # noqa: F401
import app.tools.graph  # noqa: F401
import app.tools.agent  # noqa: F401
import app.tools.browser  # noqa: F401
import app.tools.media  # noqa: F401
import app.tools.edit_file  # noqa: F401
import app.tools.glob_tools  # noqa: F401
import app.tools.git_tools  # noqa: F401

__all__ = ["SystemToolClient", "ToolDiscovery", "ToolRegistry", "registry"]
