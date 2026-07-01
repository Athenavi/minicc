# J1：ReactFlow 可视化画布

> **所属阶段**：Phase J - 可视化编排器
> **预估工时**：5 天

---

## 1. 任务目标

实现 Dify 风格的拖拽式工作流设计器。使用 ReactFlow 构建可视化画布，支持拖拽节点、连线、缩放、撤销/重做。

## 2. 详细子任务

### 2.1 ReactFlow 集成

```bash
npm i reactflow @reactflow/core @reactflow/controls @reactflow/minimap
```

### 2.2 画布组件

```tsx
// frontend/src/components/studio/GraphCanvas.tsx

const GraphCanvas: React.FC = () => {
  // ReactFlow 画布
  // - 拖拽添加节点
  // - 连线（source → target）
  // - 缩放/平移
  // - 缩略图
  // - 撤销/重做 (undo/redo)
}
```

### 2.3 自定义节点

- [ ] `LLMNode` — LLM 配置节点
- [ ] `ToolNode` — 工具调用节点
- [ ] `AgentNode` — 子 Agent 节点
- [ ] `RAGNode` — 知识检索节点
- [ ] `CodeNode` — 代码执行节点
- [ ] `ConditionNode` — 条件判断节点
- [ ] `InputNode` / `OutputNode` — 输入输出

### 2.4 节点样式

- 每种节点类型不同颜色（LLM=蓝，Tool=绿，Agent=紫，RAG=橙）
- 节点状态指示器（idle/running/done/error）
- 端口样式（source=右，target=左）

### 2.5 交互功能

- [ ] 拖拽左侧面板 → 画布添加节点
- [ ] 点击节点打开配置面板
- [ ] 连线自动对齐
- [ ] 撤销/重做（Ctrl+Z/Ctrl+Shift+Z）
- [ ] 导出为 JSON/DSL
- [ ] 导入 JSON/DSL

### 2.6 测试

- [ ] 节点拖拽添加
- [ ] 节点连接
- [ ] 节点删除
- [ ] 撤销/重做
- [ ] 导出/导入
