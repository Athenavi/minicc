# 轴网生成 Revit 插件 — 开发思路文档

## 一、需求概述

### 1.1 核心目标

在 Revit 2026 中开发一个插件，让用户像画普通详图线一样绘制"结构轴线"，插件**实时自动**将所有相交/共线的轴线在交点处打断，形成首尾相连的"网格"，保证每个网格边不被其他轴线穿过。

### 1.2 需求来源

建筑行业的结构专业在建筑轴网基础上进行二次处理：
- 打断现有建筑轴网 → 在交点处分成若干小段
- 补充自己的轴线 → 新增轴线也参与打断
- 形成 N 个网格 → 每个网格的边上没有其他轴线穿越

### 1.3 关键词汇

| 术语 | 说明 |
|------|------|
| **轴线** | Revit 详图线（DetailLine / CurveElement），仅支持直线和圆弧 |
| **节点** | 轴线之间的连接点（用 FilledRegion 表示） |
| **网格** | 由节点和线段构成的拓扑干净的平面图 |
| **IUpdater** | Revit 实时监听机制，监听详图线的创建/修改/删除 |

---

## 二、架构设计

### 2.1 整体架构

```
┌──────────────────────────────────────────────────┐
│             Revit 2026 Application               │
├──────────────────────────────────────────────────┤
│         App.cs (IExternalApplication)            │
│   ┌─ Ribbon 面板注册（"轴网工具"标签）            │
│   └─ IUpdater 注册（监听 CurveElement 变更）      │
├──────────────────┬───────────────────────────────┤
│  AxisGridUpdater │  Command.cs                   │
│  (IUpdater)      │  (手动同步命令/调试)           │
├──────────────────┴───────────────────────────────┤
│              GridSyncService.cs                  │
│   ┌─ CollectDetailLines — 收集视图中的详图线      │
│   ├─ BuildGraph — 计算交点、构建轴网图            │
│   ├─ SplitLines — 在交点处打断轴线               │
│   └─ UpdateNodes — 创建/更新节点                │
├──────────────────┬───────────────────────────────┤
│ IntersectionService │  AxisGraph.cs             │
│ (求交计算)         │  (图数据结构)               │
└──────────────────┴───────────────────────────────┘
```

### 2.2 文件结构

```
AxisGrid/
├── AxisGrid.csproj            ← .NET Framework 4.7.2
├── AxisGrid.addin             ← Revit 插件注册
├── App.cs                     ← IExternalApplication 入口
├── Command.cs                 ← 手动同步命令
├── Properties/AssemblyInfo.cs
├── Models/
│   └── AxisGraph.cs           ← 图数据结构（节点、边、拓扑关系）
├── Services/
│   ├── IntersectionService.cs ← 求交计算
│   └── GridSyncService.cs     ← 核心算法
└── Updaters/
    └── AxisGridUpdater.cs     ← IUpdater 实时监听
```

---

## 三、核心算法

### 3.1 轴网打断规则

每当用户新画一条详图线，或移动/修改它，系统自动检查并处理：

#### 情况 A：十字交叉
```
线 A ────╂────   线 A ────┐ ┌────
         ╂       →        │ │
线 B ────╂────   线 B ────┘ └────
```
两条线都在交点处打断，各变成两段，生成一个节点。

#### 情况 B：T 形连接
```
线 A ──────────   线 A ────┐ ──────
         ╂              →    │
线 B ───┘             线 B ─┘
```
被搭接的线 A 在搭接处打断；线 B 的端点就是节点，不再打断。

#### 情况 C：共线端接
```
线 A ──────┐
           ╂       →  自然形成一个节点，各自保持独立
线 B ──────┘
```

### 3.2 轴网图构建（BuildGraph）

```
输入：所有 DetailLine 集合
输出：AxisGraph（节点列表 + 边列表）

Step 1: 计算所有交点
  - 每条线的起点/终点 → 端点节点
  - 线与线的交叉点 → 十字交点
  - 端点落在另一条线上 → T 形交点

Step 2: 按线分组断点，排序参数化位置
  - 对每条线，收集所有断点（端点 + 交点）
  - 按参数 t 排序（从 0 到 1）
  - 每相邻两个断点构成一条 Edge

Step 3: 合并共线边
  - 共享节点 + 方向相同 → 合并为一条边

Step 4: 清理孤立节点
  - 没有 Edge 连接的节点被删除
```

### 3.3 交点计算（IntersectionService）

```
Curve.Intersect(curve1, curve2, out resultArray)
  → SetComparisonResult.Overlap → 有交点
  → 遍历 resultArray 获取交点坐标

曲线类型支持：
  - 直线-直线：解析几何求交
  - 直线-圆弧：Revit 原生求交
  - 圆弧-圆弧：Revit 原生求交
  - 端点落在曲线上：Project 函数检测
```

### 3.4 IUpdater 实时监听

```csharp
public void Execute(UpdaterData data)
{
    if (_isProcessing) return;  // 防循环
    
    _isProcessing = true;
    try
    {
        // 1. 收集所有变更的 CurveElement
        // 2. 重建轴网图
        // 3. 打断轴线（删除旧线，创建新线段）
        // 4. 更新节点
    }
    finally
    {
        _isProcessing = false;
    }
}
```

**循环变更防止**：通过 `_isProcessing` 静态标志位，打断操作中再次触发的 Updater 直接返回。

### 3.5 节点管理

节点用 FilledRegion（填充区域）表示：
- 圆形，直径约 8mm
- 每个节点对应一个 FilledRegion
- 轴网重建时，旧节点删除，新节点创建
- 颜色：黑色或自定义

---

## 四、关键技术点

### 4.1 Revit 2026 API 变化

| API | 变化 |
|-----|------|
| `DetailLine` | 在 Revit 2026 中不可直接使用，需用 `CurveElement` + 类别过滤 |
| `ElementId.IntegerValue` | 不存在，改用 `.Value`（返回 `long`） |
| `UpdaterRegistry.AddUpdater` | 被 `RegisterUpdater` 替代 |
| `ChangePriority.LinesOrPatterns` | 不存在，改用 `Annotations` |

### 4.2 曲线类型支持

仅支持两种曲线类型：
- **Line**：直线，由起点和终点定义
- **Arc**：圆弧，由起点、终点、中点定义

通过 `curve.GetType()` 区分，不同类型的打断和合并逻辑不同（共线的判断方法不同）。

### 4.3 防循环变更

```csharp
private static bool _isProcessing = false;

public void Execute(UpdaterData data)
{
    if (_isProcessing) return;
    _isProcessing = true;
    try { /* 修改文档 */ }
    finally { _isProcessing = false; }
}
```

### 4.4 插件部署

```
AxisGrid.addin → %appdata%\Autodesk\Revit\Addins\2026\
AxisGrid.dll  → 同目录
```

`.addin` 文件内容：
```xml
<?xml version="1.0" encoding="utf-8"?>
<RevitAddIns>
  <AddIn Type="Application">
    <Name>轴网生成</Name>
    <Assembly>AxisGrid.dll</Assembly>
    <AddInId>A1B2C3D4-E5F6-7890-ABCD-1234567890AB</AddInId>
    <FullClassName>AxisGrid.App</FullClassName>
    <VendorId>AxisGrid</VendorId>
  </AddIn>
</RevitAddIns>
```

---

## 五、开发的进一步计划

当前已完成：
- [x] 项目结构和编译通过
- [x] IUpdater 框架
- [x] 轴网图数据结构
- [x] 交点计算
- [x] 轴网图构建（内存中）

待完成：
- [ ] 实际打断轴线（在 Revit 中删除旧线、创建新线段）
- [ ] 节点创建/更新
- [ ] 增量更新（只处理变更的部分，提高性能）
- [ ] 测试各种场景（十字、T 形、共线、移动、删除）
- [ ] 录屏演示
