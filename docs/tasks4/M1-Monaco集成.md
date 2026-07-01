# M1: Monaco Editor 集成

> **预估**：1 周 | **前置**：无 | **产出**：Monaco Editor 嵌入 Next.js

## 目标

将 Monaco Editor（VS Code 同源编辑器）集成到 MiniCC 前端中，使其成为 AI 与用户共同编辑代码的核心界面。

## 实现步骤

### 1. 安装 Monaco 依赖

```bash
cd frontend
npm install @monaco-editor/react monaco-editor
```

### 2. 创建 Editor 组件

`frontend/src/components/editor/MonacoEditor.tsx`：
- 封装 `@monaco-editor/react` 的 `Editor` 组件
- 支持 `language`（自动检测文件类型）
- 支持 `theme`（亮/暗模式同步）
- 支持 `value` / `onChange` 双向绑定
- 支持 `readOnly` 模式（AI 编辑时锁定）

### 3. 创建编辑器页面

`frontend/src/app/editor/page.tsx`：
- 左侧文件浏览器（见 M2）
- 中央编辑器面板
- 右侧 AI 聊天面板（复用现有组件）
- 底部状态栏（光标位置、语言、编码）

### 4. 主题同步

- 从 Tailwind `dark` class 同步到 Monaco `vs-dark` 主题
- 自定义 Monaco 主题色（匹配 MiniCC 品牌色）

### 5. 文件类型自动检测

根据文件扩展名自动设置 `language`：
- `.py` → `python`
- `.ts` / `.tsx` → `typescript`
- `.js` / `.jsx` → `javascript`
- `.md` → `markdown`
- `.json` → `json`
- `.yaml` / `.yml` → `yaml`
- `.toml` → `plaintext` + TOML 语法扩展

## 验收标准

- [ ] Monaco Editor 在 `/editor` 页面中正常渲染
- [ ] 语法高亮按文件类型工作
- [ ] 亮/暗模式自动切换
- [ ] 编辑器与聊天面板同屏布局
- [ ] 光标位置显示在状态栏
- [ ] 所有现有测试仍然通过

## 参考

- [Monaco Editor React](https://github.com/suren-atoyan/monaco-react)
- [Monaco Editor API](https://microsoft.github.io/monaco-editor/api/index.html)
- VS Code 源码中的 `editor` 目录
