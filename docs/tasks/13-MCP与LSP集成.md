# 任务 13：MCP 与 LSP 扩展集成

> **所属阶段**：Phase 4 - 扩展系统与平台化 (Extensions)
> **对应模块**：模块 8 (Extensions — MCP Client / LSP / Plugins)
> **预估工时**：4-5 天
> **依赖**：任务 05 (QueryEngine)、任务 06 (Tool Parser/ToolRegistry)

---

## 1. 任务目标

接入 MCP (Model Context Protocol) 和 LSP (Language Server Protocol) 生态，实现工具的动态扩展。MCP 允许接入第三方工具服务器，LSP 提供代码智能（跳转定义、查找引用等）。同时实现 Python 插件热加载机制。

## 2. 详细子任务

### 2.1 ExtensionLoader — 扩展加载器骨架

- [ ] 文件：`backend/app/core/extensions.py`

```python
class ExtensionLoader:
    """
    扩展加载器。
    管理 MCP 客户端、LSP 客户端和本地 Python 插件的生命周期。
    """
    
    def __init__(self, tool_registry: ToolRegistry):
        self.tool_registry = tool_registry
        self.mcp_clients: dict[str, MCPClient] = {}
        self.lsp_clients: dict[str, LSPClient] = {}
        self.plugins: dict[str, PluginModule] = {}
    
    async def load_all(self, config: ExtensionsConfig):
        """加载所有配置的扩展"""
        await self.load_mcp_clients(config.mcp_servers)
        await self.load_lsp_clients(config.lsp_configs)
        await self.load_plugins(config.plugin_dirs)
    
    async def shutdown_all(self):
        """优雅关闭所有扩展"""
        for client in self.mcp_clients.values():
            await client.shutdown()
        for client in self.lsp_clients.values():
            await client.shutdown()
```

### 2.2 MCP Client 实现

- [ ] 文件：`backend/app/core/mcp_client.py`

```python
class MCPClient:
    """
    MCP (Model Context Protocol) 客户端。
    支持 Stdio（子进程）和 SSE（HTTP 流）两种传输方式。
    """
    
    def __init__(self, name: str, config: MCPServerConfig):
        self.name = name
        self.config = config
        self.process: asyncio.subprocess.Process | None = None
        self.session: MCPClientSession | None = None
    
    async def connect(self):
        """连接到 MCP 服务器"""
        if self.config.transport == "stdio":
            await self._connect_stdio()
        elif self.config.transport == "sse":
            await self._connect_sse()
    
    async def _connect_stdio(self):
        """
        启动 MCP 服务器子进程。
        通信协议：JSON-RPC over stdin/stdout。
        """
        self.process = await asyncio.create_subprocess_exec(
            *self.config.command,
            stdin=asyncio.subprocess.PIPE,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        # 初始化 MCP session
        self.session = MCPClientSession(
            self.process.stdin,
            self.process.stdout,
        )
        await self.session.initialize()
    
    async def list_tools(self) -> list[MCPTool]:
        """调用 MCP 的 tools/list 获取工具列表"""
        result = await self.session.send_request("tools/list", {})
        return [MCPTool(**t) for t in result["tools"]]
    
    async def call_tool(self, name: str, arguments: dict) -> dict:
        """调用 MCP 工具的 tools/call"""
        result = await self.session.send_request("tools/call", {
            "name": name,
            "arguments": arguments,
        })
        return result
    
    async def shutdown(self):
        """关闭 MCP 连接"""
        if self.session:
            await self.session.close()
        if self.process:
            self.process.terminate()
            try:
                await asyncio.wait_for(self.process.wait(), timeout=5)
            except asyncio.TimeoutError:
                self.process.kill()
```

#### MCP 工具适配器

- [ ] `class MCPToolAdapter(BaseTool)`:
  - 将 MCP tool 定义包装为 MiniCC 的 BaseTool 子类
  - `name`、`description`、`input_schema` 从 MCP 定义自动派生
  - `execute()` 委托给 `MCPClient.call_tool()`
  - 权限等级：默认为 `WRITE`（可配置）

### 2.3 MCP 配置

- [ ] 配置文件格式（支持在 `.minicc/mcp.json` 或全局配置中定义）：

```json
{
  "mcpServers": {
    "github.com/modelcontextprotocol/filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "."],
      "transport": "stdio"
    },
    "playwright": {
      "command": "npx",
      "args": ["-y", "@playwright/mcp"],
      "transport": "stdio"
    },
    "custom-sse-server": {
      "url": "http://localhost:8080/mcp",
      "transport": "sse"
    }
  }
}
```

### 2.4 LSP 客户端实现

- [ ] 文件：`backend/app/core/lsp_client.py`

```python
class LSPClient:
    """
    LSP (Language Server Protocol) 客户端。
    提供代码智能查询能力。
    """
    
    def __init__(self, language: str, command: list[str]):
        self.language = language
        self.command = command
        self.process = None
        self.connection = None
    
    async def start(self):
        """启动 LSP 服务器进程并初始化"""
        ...
    
    async def go_to_definition(self, file_path: str, line: int, col: int) -> list[Location]:
        """跳转到定义"""
        ...
    
    async def find_references(self, file_path: str, line: int, col: int) -> list[Location]:
        """查找引用"""
        ...
    
    async def hover(self, file_path: str, line: int, col: int) -> str | None:
        """悬停文档"""
        ...
    
    async def shutdown(self):
        """关闭 LSP 连接"""
        ...
```

#### LSP 工具包装

- [ ] `class LSPTool(BaseTool)`:
  - `lsp_go_to_definition`
  - `lsp_find_references`
  - `lsp_hover`
  - `lsp_completion`（代码补全）
  - 权限等级全部为 READ（自动允许）

### 2.5 插件热加载

- [ ] 文件：`backend/app/core/plugin_loader.py`

```python
class PluginLoader:
    """
    Python 插件热加载器。
    扫描目录，动态导入 Tool 子类并注册到 ToolRegistry。
    """
    
    def __init__(self, plugin_dirs: list[Path]):
        self.plugin_dirs = plugin_dirs
    
    async def load_plugins(self, registry: ToolRegistry):
        """加载所有插件"""
        for plugin_dir in self.plugin_dirs:
            if not plugin_dir.exists():
                continue
            for file in plugin_dir.glob("*.py"):
                if file.name.startswith("_"):
                    continue
                await self._load_single_plugin(file, registry)
    
    async def _load_single_plugin(self, file: Path, registry: ToolRegistry):
        """加载单个插件文件"""
        # 动态导入
        spec = importlib.util.spec_from_file_location(file.stem, file)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        
        # 查找 BaseTool 子类
        for attr_name in dir(module):
            attr = getattr(module, attr_name)
            if (isinstance(attr, type) and 
                issubclass(attr, BaseTool) and 
                attr is not BaseTool):
                tool_instance = attr()
                registry.register(tool_instance)
                logger.info(f"Loaded plugin tool: {tool_instance.name}")
    
    def watch_for_changes(self, registry: ToolRegistry):
        """监听文件变化，自动重载（使用 watchfiles）"""
        ...
```

### 2.6 扩展配置模型

- [ ] `class ExtensionsConfig(BaseModel)`:
  - `mcp_servers`: dict[str, MCPServerConfig]
  - `lsp_configs`: dict[str, LSPConfig]
  - `plugin_dirs`: list[str] = ["~/.minicc/plugins"]

### 2.7 与 QueryEngine 集成

- [ ] `ExtensionLoader` 在 QueryEngine 初始化时加载所有扩展
- [ ] MCP 工具注册到 `ToolRegistry`，与其他原生工具无差别对待
- [ ] LSP 工具仅在需要时实例化（按语言惰性启动）
- [ ] 扩展加载失败不应影响核心功能运行

### 2.8 单元测试

- [ ] 测试 MCP Client Stdio 连接和工具列表获取
- [ ] 测试 MCP 工具适配器包装
- [ ] 测试 LSP 客户端连接和查询
- [ ] 测试插件热加载
- [ ] 测试扩展加载失败时的降级行为

## 3. 验收标准

| # | 检查项 | 验证方式 |
|:-|:-|:-|
| 1 | MCP Stdio 服务器可连接并列出工具 | 集成测试（mock stdio） |
| 2 | MCP 工具可被 LLM 调用并返回结果 | 端到端测试 |
| 3 | LSP 可返回 `go_to_definition` 结果 | 集成测试（mock LSP） |
| 4 | Python 插件目录中的 Tool 子类被自动注册 | pytest |
| 5 | 插件热重载在文件修改后生效 | 文件修改后验证 |
| 6 | MCP 服务器断开后不影响主循环 | 模拟 MCP 崩溃 |
| 7 | 所有扩展可在运行时动态启停 | 集成测试 |

## 4. 参考资源

- [MCP 规范](https://modelcontextprotocol.io/)
- [MCP Python SDK](https://github.com/modelcontextprotocol/python-sdk)
- [LSP 规范](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/)
- [pygls — Python LSP 客户端](https://github.com/openlawlibrary/pygls)
- Claude Code MCP/LSP 集成设计（参考 xuanyuancode 教程扩展能力篇）
- Reasonix MCP 插件设计（参考 Reasonix Guide）
- 规划文档 §3 Phase 4 任务 4.1-4.3

## 5. 注意事项

- MCP/JSON-RPC 通信需要严格的超时控制（默认 30s）
- LSP 服务器启动较慢，建议惰性初始化（首次调用时启动）
- 插件热加载使用 `importlib` 的 `reload()` 需要谨慎处理状态
- MCP 工具与原生工具在 ToolRegistry 中一视同仁——LLM 不需要知道工具来源
- 安全注意：MCP 服务器命令不应包含用户可控的未转义参数
