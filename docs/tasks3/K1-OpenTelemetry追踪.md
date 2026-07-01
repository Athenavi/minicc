# K1：OpenTelemetry 全链路追踪

> **所属阶段**：Phase K - 可观测性
> **预估工时**：3 天

---

## 1. 任务目标

实现类 LangSmith 的全链路追踪系统：每个 Graph 执行、每个节点调用、每次 LLM 请求都可追踪。支持 OpenTelemetry 标准导出。

## 2. 详细子任务

### 2.1 追踪器核心

```python
# backend/app/observability/tracer.py

class Tracer:
    """
    全链路追踪器。每个 Graph 执行创建一个 Trace，
    每个节点创建一个 Span，记录耗时/输入/输出/错误。
    """
    
    @contextmanager
    def span(self, name: str, context: dict = None):
        """创建子 Span。"""
        ...
    
    def add_event(self, name: str, attributes: dict = None):
        """在 Span 中添加事件。"""
        ...
```

### 2.2 追踪数据

- [ ] `Trace`: graph_id, status, total_duration, token_usage, node_count
- [ ] `Span`: node_id, node_type, duration, input, output, error
- [ ] `Event`: event_type, timestamp, data

### 2.3 存储

- [ ] SQLite 存储 Trace/Span/Event
- [ ] 按时间/状态/类型查询
- [ ] 自动清理（保留最近 7 天）

### 2.4 Graph 集成

- [ ] GraphExecutor 自动创建 Trace
- [ ] 每个节点创建 Span
- [ ] 节点失败记录 error
- [ ] LLM 调用记录 token 用量

### 2.5 API 端点

- `GET /api/traces` — 追踪列表
- `GET /api/traces/{id}` — 追踪详情
- `GET /api/traces/{id}/tree` — Span 树视图

## 3. 验收标准

| # | 检查项 | 验证方式 |
|:-|:-------|:---------|
| 1 | Graph 执行创建 Trace | 查询 API 可见 |
| 2 | 每个节点创建 Span | Span 树完整 |
| 3 | 失败节点记录 error | Span 含 error 信息 |
| 4 | 支持按时间查询 | 筛选最近 24h |
