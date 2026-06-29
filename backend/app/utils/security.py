"""路径安全沙箱 — 防止越权访问项目根目录之外的文件。"""

from __future__ import annotations

from pathlib import Path


class PathValidator:
    """路径安全沙箱。

    确保所有文件操作限制在 workspace_dir 内，防止路径遍历攻击。
    """

    def __init__(self, workspace_dir: str | Path) -> None:
        self.workspace_dir = Path(workspace_dir).resolve()

    def validate(self, path: str | Path, mode: str = "read") -> Path:
        """验证路径是否合法，返回解析后的绝对路径。

        Raises:
            PermissionError: 路径不在 workspace_dir 内或涉及敏感目录。
        """
        resolved = self._resolve(path)

        # 检查是否在 workspace 内
        try:
            resolved.relative_to(self.workspace_dir)
        except ValueError:
            raise PermissionError(
                f"路径 '{resolved}' 不在工作目录 '{self.workspace_dir}' 内"
            )

        # 阻止直接操作 .git
        if ".git" in resolved.parts:
            raise PermissionError(f"不允许直接操作 .git 目录: {resolved}")

        return resolved

    @staticmethod
    def _resolve(path: str | Path) -> Path:
        """安全解析路径 — 处理符号链接和 .. 遍历。"""
        return Path(path).resolve()
