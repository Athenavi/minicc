# G1：Graph 定义与 Schema

> **所属阶段**：Phase G - StateGraph 引擎
> **预估工时**：5 天
> **依赖**：V0.2 `automator/` 模块

---

## 1. 任务目标

实现 LangGraph 风格的图定义系统：Nodes（节点）+ Edges（边）+ StateGraph（状态图），支持条件分支、并行节点、循环。

## 2. 详细子任务

### 2.1 核心数据结构

```python
# backend/app/graph/graph.py

class GraphNode(BaseModel):
    """图节点。"""
    id: str
    label: str
    node_type: Literal["llm", "tool", "agent", "code", "condition", "input", "output"]
    config: dict[str, Any] = {}
    timeout: int = 120

class GraphEdge(BaseModel):
    """图边。"""
    source_id: str
    target_id: str
    condition: Optional[str] = None  # 条件表达式
    label: Optional[str] = None

class StateGraph(BaseModel):
    """完整状态图定义。"""
    id: str = ""
    name: str
    description: Optional[str] = None
    nodes: list[GraphNode]
    edges: list[GraphEdge]
    entry_point: str  # 起始节点 ID
    state_schema: dict[str, Any] = {}  # Pydantic schema dict
```

### 2.2 GraphBuilder

- [ ] `add_node(id, label, type, config)` — 添加节点
- [ ] `add_edge(source_id, target_id, condition=None)` — 添加边
- [ ] `set_entry_point(node_id)` — 设置入口
- [ ] `compile()` — 编译图：拓扑排序、校验连通性、检测循环
- [ ] `validate()` — 校验：节点可达、边目标存在、条件表达式合法

### 2.3 图编译（拓扑排序）

- [ ] 使用 Kahn 算法拓扑排序
- [ ] 检测循环依赖
- [ ] 识别并行分支（多个出边）
- [ ] 检测死节点（不可达节点）

### 2.4 测试

- [ ] 创建简单线性图
- [ ] 创建条件分支图
- [ ] 创建并行节点图
- [ ] 检测循环依赖
- [ ] 编译校验

## 3. 验收标准

| # | 检查项 | 验证方式 |
|:-|:-------|:---------|
| 1 | 可定义多节点图 | 创建 5 节点图 |
| 2 | 拓扑排序正确 | 验证执行顺序 |
| 3 | 条件边可路由 | 条件满足走 A 分支，否则走 B |
| 4 | 循环检测 | 图含循环时报错 |
| 5 | 并行分支识别 | 识别出 2 条并行路径 |
