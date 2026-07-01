# M4: 多 Tab 编辑 + M5: Diff 编辑器

> **M4 预估**：0.5 周 | **M5 预估**：0.5 周 | **前置**：M1

## M4: 多 Tab 编辑

`frontend/src/components/editor/EditorTabs.tsx`：
- Tab 栏（类似 VS Code）：文件名 + 图标 + 关闭按钮
- 被修改文件在文件名前显示圆点（unsaved indicator）
- 拖拽重排 Tab 顺序
- 右键菜单：关闭 / 关闭其他 / 关闭所有
- 最大 Tab 数限制（默认 20），超过时 LRU 淘汰
- Tab 持久化到 localStorage（会话恢复）

**状态管理**：
```typescript
interface EditorState {
  openFiles: string[];          // 已打开文件路径列表
  activeFile: string | null;    // 当前激活文件
  fileContents: Record<string, string>;  // 文件内容缓存
  dirtyFiles: Set<string>;      // 未保存文件集合
}
```

## M5: Diff 编辑器

`frontend/src/components/editor/DiffEditor.tsx`：
- 基于 Monaco `DiffEditor` 组件
- 左侧：原始内容 | 右侧：AI 修改后内容
- 逐行高亮（绿=新增，红=删除，蓝=修改）
- 导航箭头（跳转到下一个/上一个变更）
- Accept / Reject 按钮（逐块或全部）
- 与 AI 聊天联动：AI 输出变更 → Diff 展示 → 用户确认

**AI 变更流程**：
```
AI 建议修改 → Diff 编辑器展示 → 用户检查每个变更
  → Accept All → 写入文件
  → Reject All → 丢弃
  → 逐块操作 → 混合模式
```

## 验收标准

- [ ] 多文件 Tab 正常切换
- [ ] 未保存标记正确显示
- [ ] Tab 可关闭/重排
- [ ] 会话刷新后 Tab 恢复
- [ ] Diff 编辑器显示 AI 变更
- [ ] Accept/Reject 逐块操作
- [ ] 所有现有测试通过
