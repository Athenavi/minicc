# 轴网生成 Revit 插件 — 成果答辩（代码逻辑阐述）

## 一、项目概述

### 1.1 项目目标

开发 Revit 2026 插件，实时监听详图线（轴线）的创建/修改/删除，自动在交点处打断轴线，形成拓扑干净的轴网网格，确保每个网格边不被其他轴线穿过。

### 1.2 开发环境

| 项目 | 说明 |
|------|------|
| Revit 版本 | Revit 2026 |
| .NET 版本 | .NET Framework 4.7.2 |
| 开发语言 | C# |
| 监听机制 | IUpdater |
| 部署方式 | `.addin` 文件注册 |

---

## 二、架构设计

### 2.1 分层架构

```
┌──────────────────────────────────────────────┐
│ App.cs (IExternalApplication)                │
│  ├─ Ribbon 面板注册（"轴网工具"→"同步轴网"）   │
│  └─ IUpdater 注册（DocumentOpened 事件中触发） │
├──────────────────────┬───────────────────────┤
│ AxisGridUpdater      │ Command.cs            │
│ (IUpdater 实现)      │ (手动同步命令)         │
├──────────────────────┴───────────────────────┤
│ GridSyncService.cs                           │
│  ├─ CollectDetailLines() — 收集视图中的详图线 │
│  ├─ BuildGraph() — 计算交点、构建轴网图      │
│  └─ UpdateNodes() — 创建/更新节点           │
├──────────────────────┬───────────────────────┤
│ IntersectionService  │ AxisGraph.cs          │
│ (求交计算)            │ (图数据结构)          │
└──────────────────────┴───────────────────────┘
```

### 2.2 数据流

```
用户操作 → Revit 触发 IUpdater
  → AxisGridUpdater.Execute()
    → GridSyncService.RebuildGraph()
      → CollectDetailLines() 收集所有详图线
      → BuildGraph() 计算交点、打断轴线、构建图
      → UpdateNodes() 创建/更新 FilledRegion 节点
    → 完成同步
```

---

## 三、核心代码逻辑

### 3.1 IUpdater 监听（AxisGridUpdater.cs）

```csharp
public class AxisGridUpdater : IUpdater
{
    private static bool _isProcessing = false;  // 防循环

    public void Execute(UpdaterData data)
    {
        if (_isProcessing) return;  // 防止自身修改再次触发

        _isProcessing = true;
        try
        {
            // 1. 收集变更的 CurveElement
            // 2. 重建轴网
            GridSyncService service = new GridSyncService(doc);
            service.RebuildGraph(viewPlan);
        }
        finally
        {
            _isProcessing = false;
        }
    }
}
```

**设计要点**：
- `_isProcessing` 静态标志位防止循环变更——打断操作本身会修改详图线，修改会再次触发 Updater，标志位阻止重入
- 使用 `ElementClassFilter(typeof(CurveElement))` 监听所有曲线元素
- 注册时机：文档打开时通过 `DocumentOpened` 事件触发

### 3.2 轴网图数据结构（AxisGraph.cs）

```csharp
public class AxisGraph
{
    public List<XYZ> Nodes { get; set; }           // 所有节点（位置）
    public List<AxisEdge> Edges { get; set; }       // 所有轴线段
    public Dictionary<int, ElementId> NodeRegionIds  // 节点→FilledRegion 映射
    public Dictionary<ElementId, List<int>> DetailLineToEdges  // 详图线→边索引

    public int AddNode(XYZ point)    // 添加节点（去重，容差 0.01 英尺）
    public void AddEdge(AxisEdge)    // 添加边，自动维护映射
    public void RemoveEdgesForDetailLine(ElementId)  // 删除某条线对应的所有边
    public void RemoveOrphanNodes()  // 删除没有边连接的孤立节点
}
```

**数据结构**：拓扑图（无向图），节点和边分别存储，维护双向索引。

### 3.3 轴网图构建（GridSyncService.BuildGraph）

核心算法分为 4 步：

#### Step 1：计算所有交点

```csharp
for (int i = 0; i < detailLines.Count; i++)
{
    Curve ci = detailLines[i].GeometryCurve;
    // 添加端点
    allPoints.Add((ci.GetEndPoint(0), i, "end"));
    allPoints.Add((ci.GetEndPoint(1), i, "end"));

    for (int j = i + 1; j < detailLines.Count; j++)
    {
        Curve cj = detailLines[j].GeometryCurve;
        // Revit 原生求交
        var inters = IntersectionService.FindIntersections(ci, cj);
        foreach (var pt in inters)
        {
            allPoints.Add((pt, i, "cross"));
            allPoints.Add((pt, j, "cross"));
        }
        // 检测 T 形连接（端点落在另一条线上）
        CheckEndpointOnCurve(ci, cj, i, j, allPoints);
        CheckEndpointOnCurve(cj, ci, j, i, allPoints);
    }
}
```

**三种交点类型**：
1. **端点**：每条线的起点和终点（共 2×N 个）
2. **交叉点**：线与线相交（十字交叉）
3. **T 形点**：一条线的端点落在另一条线上（T 形连接）

#### Step 2：在交点处打断

```csharp
for (int i = 0; i < detailLines.Count; i++)
{
    Curve curve = detailLines[i].GeometryCurve;
    
    // 收集该线上的所有断点
    var breakPoints = new List<(XYZ point, double param)>();
    // ... 收集并去重
    
    // 按参数 t 排序（从 0 到 1）
    breakPoints.Sort((a, b) => a.param.CompareTo(b.param));
    
    // 每相邻两个断点构成一条 Edge
    for (int k = 0; k < breakPoints.Count - 1; k++)
    {
        // 计算该段的起终点
        XYZ p1 = curve.Evaluate(t1, false);
        XYZ p2 = curve.Evaluate(t2, false);
        
        // 创建子曲线
        if (curve is Line)
            segment = Line.CreateBound(p1, p2);
        else if (curve is Arc)
            segment = Arc.Create(p1, p2, mid);
        
        // 添加到图的边列表
        graph.AddEdge(new AxisEdge { ... });
    }
}
```

#### Step 3：合并共线边

```csharp
private void MergeCollinearEdges(AxisGraph graph)
{
    // 遍历所有边对，检查是否共享节点且方向相同
    if (共享节点 && 方向点积 > 0.999)
    {
        // 合并为一条边
        // 标记旧边为删除，创建新边
    }
}
```

#### Step 4：清理孤立节点

```csharp
graph.RemoveOrphanNodes();  // 删除没有边连接的节点
```

### 3.4 求交计算（IntersectionService.cs）

```csharp
public static List<XYZ> FindIntersections(Curve curve1, Curve curve2)
{
    // 使用 Revit 原生 Curve.Intersect API
    curve1.Intersect(curve2, out IntersectionResultArray arr);
    // 返回所有交点坐标
}

public static bool IsPointOnCurve(Curve curve, XYZ point)
{
    // 用 curve.Project(point) 检测点是否在曲线上
}

public static double GetParameter(Curve curve, XYZ point)
{
    // 返回点在曲线上的参数化位置
}
```

**支持的曲线组合**：
- 直线 ∩ 直线
- 直线 ∩ 圆弧
- 圆弧 ∩ 圆弧

### 3.5 防循环变更机制

```csharp
// AxisGridUpdater.cs
private static bool _isProcessing = false;

public void Execute(UpdaterData data)
{
    if (_isProcessing) return;  // ① 首次进入: _isProcessing = false → 继续
                                // ② 打断触发递归: _isProcessing = true → 直接返回
    
    _isProcessing = true;
    try
    {
        // 修改文档（打断轴线、创建节点）
        // 这些修改会再次触发 IUpdater
        // 但因 _isProcessing = true，直接返回，不会死循环
    }
    finally
    {
        _isProcessing = false;
    }
}
```

---

## 四、Revit 2026 API 适配

### 4.1 DetailLine 不可用

在 Revit 2026 中，`DetailLine` 类不再是公共 API。改用：

```csharp
// 错误：DetailLine 不可用
collector.OfClass(typeof(DetailLine))

// 正确：使用 CurveElement + 类别过滤
collector.OfClass(typeof(CurveElement))
// 然后用 Category.Id.Value 判断是否为详图线
```

### 4.2 ElementId 变化

```csharp
// Revit 2025 及之前
int id = elementId.IntegerValue;

// Revit 2026
long id = elementId.Value;
```

### 4.3 IUpdater API 变化

```csharp
// Revit 2025 及之前
UpdaterRegistry.AddUpdater(updaterId);

// Revit 2026
UpdaterRegistry.RegisterUpdater(this, document);
```

### 4.4 FilledRegion.Create 签名变化

```csharp
// Revit 2026 需要 IList<CurveLoop>
FilledRegion.Create(doc, typeId, viewId, new List<CurveLoop> { curveLoop });
```

---

## 五、部署与使用

### 5.1 部署

```powershell
# 编译
cd AxisGrid
dotnet build

# 复制到 Revit Addins 目录
copy AxisGrid.addin %appdata%\Autodesk\Revit\Addins\2026\
copy AxisGrid.dll %appdata%\Autodesk\Revit\Addins\2026\
```

### 5.2 使用

1. 启动 Revit 2026，打开楼层平面视图
2. 绘制详图线（轴线）→ IUpdater 自动触发轴网同步
3. 或点击"轴网工具"标签 → "同步轴网"按钮手动触发
4. 观察轴线在交点处被打断，形成网格

### 5.3 调试

运行"同步轴网"命令会弹出调试对话框，显示：
- 视图中 CurveElement 的总数
- 每个 CurveElement 的类别 ID 和名称
- 生成的节点数和轴线数

---

## 六、关键问题与解决方案

| 问题 | 解决方案 |
|------|---------|
| 循环变更（打断触发监听→再触发→死循环） | `_isProcessing` 静态标志位防重入 |
| DetailLine 在 Revit 2026 中不可用 | 改用 `CurveElement` + 类别过滤 |
| 元素类别 ID 无法确定 | 调试命令输出所有 CurveElement 的类别信息 |
| T 形连接的端点不打断自身 | `CheckEndpointOnCurve` 只打断被搭接的线 |
| 共线合并 | 方向点积 > 0.999 + 共享节点检测 |
