# 任务 08：Shell 执行器 ShellExecutor

> **所属阶段**：Phase 2 - 真实工具落地与权限系统 (The Action Layer)
> **对应模块**：模块 6 (Built-in Tools) — Shell 执行
> **预估工时**：3-4 天
> **依赖**：任务 05 (QueryEngine)、任务 06 (Tool Parser)、任务 09 (Permission System)

---

## 1. 任务目标

实现安全的 Shell 命令执行器，支持交互式命令（PTY 伪终端）、超时控制、输出截断、实时流式输出。这是 MiniCC 最有价值但也最危险的工具。

参考 Claude Code 的 BashTool 设计。

## 2. 详细子任务

### 2.1 `ShellExecutorTool` 工具定义

- [ ] 文件：`backend/app/tools/shell_executor.py`

```python
class ShellExecutorInput(BaseModel):
    command: str = Field(
        description="要执行的 Shell 命令",
        max_length=4096,  # 防止超长命令注入
    )
    description: str | None = Field(
        default=None,
        description="AI 对命令意图的自然语言说明（用于审批时展示）",
    )
    timeout: int = Field(
        default=30,
        ge=1,
        le=300,
        description="超时时间（秒）",
    )
    workdir: str | None = Field(
        default=None,
        description="工作目录（默认使用 workspace_dir）",
    )

class ShellExecutorTool(BaseTool):
    name = "bash"
    description = """执行 Shell 命令。支持任意命令，但必须：
    1. 在执行前说明命令的意图
    2. 优先使用非破坏性操作
    3. 删除/覆盖操作必须加倍谨慎"""
    input_schema = ShellExecutorInput
    permission_level = PermissionLevel.EXECUTE  # 最严格
```

### 2.2 核心执行器

- [ ] 使用 `asyncio.create_subprocess_exec`（非 `create_subprocess_shell`）以增强安全性
- [ ] 实际将命令包装为 `["bash", "-c", command]`
- [ ] 重定向：`stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.PIPE`
- [ ] 实时输出流：
  - 逐行读取 `stdout` 和 `stderr`
  - 通过 WebSocket Queue 发送 `tool_call_result` 中的流式块
  - 区分 stdout / stderr（颜色标记）

```python
async def _execute(self, input: ShellExecutorInput) -> ToolResult:
    """执行命令的核心逻辑"""
    process = await asyncio.create_subprocess_exec(
        "bash", "-c", input.command,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
        cwd=input.workdir or self.workspace_dir,
        env=self._get_safe_env(),  # 干净的 env
    )
    
    stdout_lines = []
    stderr_lines = []
    
    try:
        async with asyncio.timeout(input.timeout):
            async for line in process.stdout:
                decoded = line.decode("utf-8", errors="replace")
                stdout_lines.append(decoded)
                # 实时推送（通过 callback）
                await self._on_output(decoded, stream="stdout")
            
            async for line in process.stderr:
                decoded = line.decode("utf-8", errors="replace")
                stderr_lines.append(decoded)
                await self._on_output(decoded, stream="stderr")
    except asyncio.TimeoutError:
        process.kill()
        return ToolResult(
            tool_call_id=input.id,
            output=f"<timeout: {input.timeout}s>\n" + "".join(stdout_lines[-20:]),
            is_error=True
        )
    
    exit_code = await process.wait()
    ...
```

### 2.3 输出截断策略

- [ ] 最大输出长度：`OUTPUT_MAX_CHARS = 100_000`（约 100KB）
- [ ] 超过限制时，保留首尾各 30% 内容，中间用 `... [truncated X lines] ...` 替代
- [ ] 单行最大长度：`LINE_MAX_CHARS = 10_000`
- [ ] 超长单行截断为 `前 5000 字符 ... [truncated] ... 后 5000 字符`

### 2.4 安全环境隔离

- [ ] `_get_safe_env()`:
  - 继承 `PATH`, `HOME`, `USER` 等基本环境变量
  - 移除敏感变量：`API_KEY`, `TOKEN`, `SECRET`, `PASSWORD`
  - 设置 `DEBIAN_FRONTEND=noninteractive` 防止交互式提示卡住
  - 设置 `PAGER=cat` 防止 `less` 等分页器
- [ ] 禁止的命令列表（可选）：
  - `rm -rf /`, `dd if=/dev/zero of=...`, `:(){ :|:& };:` (fork bomb) 等
  - 使用正则匹配简单检测，不依赖黑名单（黑名单总有遗漏）

### 2.5 PTY 支持（交互式命令）

- [ ] 可选：使用 `ptyprocess` 或 `asyncio-pty` 为需要 PTY 的命令分配伪终端
- [ ] 适用场景：`top`, `htop`, `vim` 等需要 TTY 的命令
- [ ] 不适用场景不做强求，检测到需要 PTY 时报错回退

### 2.6 工作目录管理

- [ ] 默认使用 `workspace_dir`
- [ ] 可通过 `workdir` 参数指定子目录
- [ ] 子目录必须在 `workspace_dir` 内（PathValidator 验证）

### 2.7 单元测试

- [ ] 测试简单命令执行 (`echo hello`)
- [ ] 测试超时杀死
- [ ] 测试输出截断
- [ ] 测试 stderr 捕获
- [ ] 测试非零退出码
- [ ] 测试禁止环境变量被移除
- [ ] 测试工作目录验证

## 3. 验收标准

| # | 检查项 | 验证方式 |
|:-|:-|:-|
| 1 | `echo hello` 返回 "hello\n" | pytest |
| 2 | `sleep 100` 在 5s 后被超时杀死 | pytest（timeout=5） |
| 3 | 输出超过 100KB 时被截断并提示 | pytest（生成大输出） |
| 4 | 敏感环境变量不在子进程中暴露 | pytest |
| 5 | 工作目录被限制在 workspace_dir 内 | pytest |
| 6 | stderr 内容被正确捕获 | pytest |
| 7 | 命令退出码被正确返回 | pytest |

## 4. 参考资源

- [asyncio.create_subprocess_exec](https://docs.python.org/3/library/asyncio-subprocess.html)
- Claude Code BashTool 设计（参考 xuanyuancode 教程）
- 规划文档 §3 Phase 2 任务 2.2、§4 Shell 异步交互方案

## 5. 注意事项

- **使用 `exec` 而非 `shell`**：`create_subprocess_shell` 容易受到 Shell 注入攻击
- 超时机制必须可靠：`asyncio.timeout()` 超时后立即 `process.kill()` 并清理
- 大输出截断是必须的前置保护，否则前端会炸
- 不要试图用黑名单阻止所有危险命令——审批流程才是真正的安全屏障
- `timeout` 默认值 30s，用户可通过参数覆盖，但硬上限 300s
