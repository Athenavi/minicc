# 任务 14：Token 预算管理、UI 优化与 Docker 打包

> **所属阶段**：Phase 5 - 打磨与优化 (Polish)
> **对应模块**：全模块 — Budget Manager / UI UX / 打包分发
> **预估工时**：2-3 天
> **依赖**：所有前置任务完成

---

## 1. 任务目标

进行最后的打磨与优化：实现 Token 预算管理防止 Context Window 溢出、优化 UI/UX、适配暗黑模式、添加骨架屏加载动画、打包 Docker 镜像实现一键部署。

## 2. 详细子任务

### 2.1 Token 预算管理器 (BudgetManager)

- [ ] 文件：`backend/app/engine/budget_manager.py`

```python
class BudgetManager:
    """
    Token 预算管理器。
    防止大上下文撑爆 Context Window，控制在预算范围内。
    """
    
    def __init__(
        self,
        max_context_tokens: int = 128_000,
        max_output_tokens: int = 8192,
        max_tool_rounds: int = 25,
        max_budget_usd: float = 0.0,  # 0 = 不限制
    ):
        self.max_context_tokens = max_context_tokens
        self.max_output_tokens = max_output_tokens
        self.max_tool_rounds = max_tool_rounds
        self.max_budget_usd = max_budget_usd
        self.total_input_tokens = 0
        self.total_output_tokens = 0
    
    def estimate_tokens(self, text: str) -> int:
        """粗略估计 token 数（4 字符 ≈ 1 token）"""
        return len(text) // 4
    
    def would_exceed_context(self, messages: list[Message]) -> bool:
        """检查上下文是否即将超限"""
        total = sum(self.estimate_tokens(m.content) for m in messages)
        return total > self.max_context_tokens * 0.8  # 80% 阈值预警
    
    def should_compress(self, messages: list[Message]) -> bool:
        """判断是否需要对消息进行压缩"""
        return self.would_exceed_context(messages)
    
    def compress_messages(self, messages: list[Message]) -> list[Message]:
        """
        上下文压缩策略：
        1. 保留系统提示词（不变）
        2. 保留最近的 N 条消息（完整）
        3. 对中间的消息做摘要压缩
        4. 丢弃最早的非系统消息（如果仍然太长）
        """
        ...
    
    def update_usage(self, input_tokens: int, output_tokens: int, cost_usd: float = 0):
        """更新用量统计"""
        self.total_input_tokens += input_tokens
        self.total_output_tokens += output_tokens
    
    def has_budget_remaining(self) -> bool:
        """检查是否还有预算"""
        if self.max_budget_usd <= 0:
            return True
        return self.total_cost_usd < self.max_budget_usd
    
    def get_usage_report(self) -> UsageReport:
        """生成用量报告"""
        return UsageReport(
            total_input_tokens=self.total_input_tokens,
            total_output_tokens=self.total_output_tokens,
            total_cost_usd=self.total_cost_usd,
            remaining_budget=self.max_budget_usd - self.total_cost_usd,
        )
```

#### 上下文压缩策略实现

- [ ] **策略 1：丢弃+摘要** — 当 Context > 80% 时：
  - 保留 System Prompt
  - 保留最近 5 轮对话（用户消息 + AI 回复）
  - 中间的对话用 `[先前对话摘要: ...]` 替代
- [ ] **策略 2：消息折叠** — 连续的 `tool_result` 消息可合并
- [ ] **策略 3：截断大型文件内容** — 仅保留文件头尾
- [ ] 压缩过程必须通过 `status_update` 通知前端："正在压缩上下文..."

### 2.2 UI/UX 优化

#### 暗黑模式

- [ ] 使用 `next-themes` 提供暗黑/亮色/系统跟随三种模式
- [ ] 所有组件适配暗黑模式（Shadcn 默认支持，但需检查所有自定义组件）
- [ ] 代码语法高亮适配暗黑主题（rehype-highlight 主题切换）
- [ ] Terminal 风格 Shell 卡片适配暗黑模式

#### 骨架屏与加载状态

- [ ] AI 思考中：显示 "🤔 思考中..." 加跳动光标
- [ ] 工具执行中：骨架屏卡片动画（脉冲效果）
- [ ] Shell 执行中：终端光标闪烁动画
- [ ] 文件读取中：行号骨架屏

#### 用户体验细节

- [ ] 空状态：首次使用时显示欢迎信息 + 示例提示
- [ ] 输入框自动获取焦点（`autoFocus`）
- [ ] Shift+Enter 换行，Enter 发送
- [ ] 消息加载中禁止重复提交（防抖）
- [ ] 会话标题自动生成（取第一条消息的前 30 字）
- [ ] 侧边栏会话列表按时间倒序排列
- [ ] 删除会话功能

### 2.3 Docker 打包与一键部署

#### `Dockerfile`（后端）

```dockerfile
FROM python:3.14.6-slim

WORKDIR /app

COPY backend/pyproject.toml .
RUN pip install uv && uv sync --no-dev

COPY backend/app ./app

EXPOSE 8000

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
```

- [ ] 多阶段构建：builder 阶段装依赖，最终阶段只复制必要文件
- [ ] 健康检查：`HEALTHCHECK CMD curl -f http://localhost:8000/health`

#### `Dockerfile`（前端）

```dockerfile
# 构建阶段
FROM node:22-alpine AS builder
WORKDIR /app
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ .
RUN npm run build

# 运行阶段
FROM nginx:alpine
COPY --from=builder /app/out /usr/share/nginx/html
COPY frontend/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 3000
```

#### `docker-compose.yml`

```yaml
version: "3.9"
services:
  redis:
    image: redis:7-alpine
    ports: ["6379:6379"]
  
  backend:
    build:
      context: .
      dockerfile: backend/Dockerfile
    ports: ["8000:8000"]
    environment:
      - MINICC_REDIS_URL=redis://redis:6379/0
      - MINICC_LLM_API_KEY=${ANTHROPIC_API_KEY}
    volumes:
      - .:/workspace  # 挂载工作目录
    depends_on:
      - redis
  
  frontend:
    build:
      context: .
      dockerfile: frontend/Dockerfile
    ports: ["3000:80"]
    depends_on:
      - backend
```

### 2.4 开发环境配置 (`docker-compose.dev.yml`)

- [ ] 后端开发模式：`--reload` 热重载
- [ ] 前端开发模式：Next.js 开发服务器（HMR）
- [ ] 可选 VectorDB 服务（Chroma/PGVector，为未来语义搜索预留）

### 2.5 性能优化

- [ ] 后端：添加响应压缩中间件（`gzip`）
- [ ] 后端：配置 `uvicorn` 的 `workers` 数量（`WEB_CONCURRENCY` 环境变量）
- [ ] 前端：静态资源 CDN 缓存配置（`Cache-Control` header）
- [ ] 前端：Next.js 图片优化（`next/image`）
- [ ] 前端：代码分割（dynamic import）

### 2.6 文档与可观测性

- [ ] 后端 `/docs` (Swagger) 和 `/redoc` 文档可用
- [ ] 结构化日志（JSON 格式）便于日志聚合
- [ ] 关键指标暴露（Prometheus `/metrics` 端点）
- [ ] 启动脚本：`scripts/dev.sh` 一键启动前后端

## 3. 验收标准

| # | 检查项 | 验证方式 |
|:-|:-|:-|
| 1 | 上下文超 80% 时自动触发压缩，不报错 | 长对话测试 |
| 2 | Token 用量统计正确，budget 限制生效 | 测试带 budget 限制的会话 |
| 3 | 暗黑模式切换无闪烁、所有组件适配 | 视觉检查 |
| 4 | 骨架屏在工具执行时正确显示 | 视觉检查 |
| 5 | `docker-compose up` 一键启动所有服务 | 执行测试 |
| 6 | 访问 `http://localhost:3000` 可正常使用 | 端到端测试 |
| 7 | 后端 Swagger 文档可访问 | 浏览器访问 `/docs` |
| 8 | Stop 按钮防抖有效，不会重复提交 | 多次快速点击测试 |

## 4. 参考资源

- [Docker Compose 文档](https://docs.docker.com/compose/)
- [next-themes](https://github.com/pacocoursey/next-themes)
- [rehype-highlight](https://github.com/rehypejs/rehype-highlight)
- Claude Code Token Budget 管理理念
- 规划文档 §3 Phase 5 任务 5.1-5.3

## 5. 注意事项

- Token 估算使用 4 字符 ≈ 1 token 的规则（实际因模型而异，但不影响压缩触发时机）
- 上下文压缩必须在主循环**之前**完成——不要在 LLM 调用途中压缩
- Docker 镜像中不包含 API Key——通过环境变量注入
- 前端静态导出（`next export`）后配合 nginx 实现高性能部署
- 暗黑模式使用 `prefers-color-scheme` 媒体查询作为默认值，避免闪烁
