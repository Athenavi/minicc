# 种子填充 AutoCAD 插件 — 成果答辩

## 一、项目概述

### 1.1 项目目标

开发一个 AutoCAD 2027 插件，实现以下功能：
- **Seed 命令**：在任意形状的闭合边界内，用指定矩形种子平铺填充
- **Seed2 命令**：对含圆弧/曲线的不规则边界，用线网格方式填充
- **Statistics 命令**：统计边界内未裁切和裁切种子的数量与面积

### 1.2 开发环境

| 项目 | 说明 |
|------|------|
| AutoCAD 版本 | AutoCAD 2027（26.0） |
| .NET 版本 | .NET 10.0-windows |
| 开发语言 | C# 12.0 |
| 加载方式 | Bundle 自动加载（PackageContents.xml + LISP 引导） |

---

## 二、核心代码逻辑阐述

### 2.1 命令入口（SeedCommands.cs）

```csharp
[CommandMethod("Seed")]
public void SeedCommand()
{
    SeedFillService service = new SeedFillService(doc.Database);
    SeedStatistics stats = service.Execute();
    // 输出统计结果
}
```

三个 AutoCAD 命令均通过 `[CommandMethod]` 特性注册：
- `Seed` → 调用 `service.Execute()`（瓷砖平铺）
- `Seed2` → 调用 `service.Execute2()`（线网格）
- `Statistics` → 调用 `service.GetStatistics(boundaryId)`（统计）

### 2.2 种子参数提取（ExtractSeedParams）

**输入**：种子矩形多段线的 4 个顶点

**算法**：
1. 计算四条边的长度：`d01`, `d12`, `d23`, `d30`
2. 对边取平均：`e1 = (d01 + d23)/2`, `e2 = (d12 + d30)/2`
3. `longSide = max(e1, e2)`, `shortSide = min(e1, e2)`
4. 方向角 = `atan2(V1.Y - V0.Y, V1.X - V0.X)`

**设计理由**：种子可能从任意顶点开始绘制，取对边平均可消除手绘误差。

### 2.3 网格生成（Execute 方法核心）

```csharp
// 计算边界在种子局部坐标中的范围
foreach (var v in boundary.Vertices)
{
    du = (v.X - cx) * dirLongX + (v.Y - cy) * dirLongY;
    dv = (v.X - cx) * dirShortX + (v.Y - cy) * dirShortY;
}

// 计算网格索引范围
uiMin = Floor((minU - longSide/2) / longSide);
uiMax = Ceil((maxU + longSide/2) / longSide);
viMin = Floor((minV - shortSide/2) / shortSide);
viMax = Ceil((maxV + shortSide/2) / shortSide);

// 遍历每个网格单元格
for (int ui = uiMin; ui <= uiMax; ui++)
    for (int vi = viMin; vi <= viMax; vi++)
        处理瓷砖(ui, vi);
```

**设计理由**：以种子中心为原点构建局部坐标系，网格与种子的旋转方向对齐。网格范围"扩展一个瓷砖尺寸"确保边界上的瓷砖不会遗漏。

### 2.4 点包含检测（IsPointInClosedCurve）

```csharp
private bool IsPointInClosedCurve(Point3d point, Polyline pline)
{
    // 从点向右发出一条射线
    Point3d rayEnd = new Point3d(point.X + 1e10, point.Y, 0);
    Line ray = new Line(point, rayEnd);
    Point3dCollection hits = new Point3dCollection();
    pline.IntersectWith(ray, Intersect.OnBothOperands, hits, ...);
    ray.Dispose();
    
    // 奇数个交点 = 在内部
    return (hits.Count % 2) == 1;
}
```

**设计理由**：使用 AutoCAD 原生 `IntersectWith` 而非自定义射线法，因为 AutoCAD 的几何引擎正确处理：
- 圆弧段（Bulge）的精确求交
- 样条曲线（Spline）的精确求交
- 数值精度问题（容差由 AutoCAD 内部处理）

### 2.5 边界瓷砖裁剪（IntersectWith 边裁剪）

```csharp
for (int ei = 0; ei < 4; ei++)
{
    Line edgeLine = new Line(corner[i], corner[(i+1)%4]);
    Point3dCollection hits = new Point3dCollection();
    boundaryPline.IntersectWith(edgeLine, ..., hits, ...);
    
    bool aIn = IsPointInClosedCurve(edgeLine.StartPoint, boundaryPline);
    bool bIn = IsPointInClosedCurve(edgeLine.EndPoint, boundaryPline);
    
    // 根据 aIn/bIn 和 hits 保留边界内线段
    if (aIn && bIn) 保留整条边;
    if (aIn && !bIn) 保留起点→第一个交点;
    if (!aIn && bIn) 保留最末交点→终点;
    if (!aIn && !bIn && hits.Count>=2) 保留入口→出口;
}
```

**设计理由**：
- 每条边独立裁剪，不依赖多边形的整体裁剪算法
- 用参数 t 排序交点，确保沿边方向的顺序正确
- 避免使用 Sutherland-Hodgman（凹多边形有问题）或 Region（爆炸后产生多线段）

### 2.6 Seed2：线网格（Execute2）

```csharp
// 生成平行线（火力覆盖）
for (double v = -hS; v >= -farExt; v -= shortS)
    hLines.Add(长边方向平行线);
for (double u = -hL; u >= -farExt; u -= longS)  
    vLines.Add(短边方向平行线);

// 每条线独立裁剪（精准施策）
foreach (var line in allLines)
{
    boundaryPline.IntersectWith(line, ..., hits, ...);
    // 在交点处分割线段，保留边界内部分
}
```

**设计理由**：对含圆弧的不规则边界，瓷砖裁剪可能产生过多小线段。线网格方式每条线独立裁剪，保证结果为干净的直线段。

### 2.7 统计（GetStatistics）

```csharp
public SeedStatistics GetStatistics(ObjectId boundaryId)
{
    // 读取边界用于空间过滤
    Polygon boundary = PolylineToPolygon(boundaryId);
    
    // 遍历 ResultLayer 上的实体
    foreach (ObjectId id in ms)
    {
        if (pline.Closed && 中心点在边界内)
            未裁切计数++;
        if (line && 中点在边界内)
            裁切计数++;
    }
}
```

**设计理由**：
- 用边界 Polygon 做空间过滤，排除之前不同边界的遗留实体
- 闭合多段线 → 未裁切（内部完整瓷砖）
- LINE → 裁切（边界裁剪线段）
- 中点在边界内才计数，避免边界外的实体被计入

---

## 三、数据结构

### 3.1 Point2D

```csharp
public struct Point2D
{
    public double X { get; set; }
    public double Y { get; set; }
    
    public static Point2D operator +(Point2D a, Point2D b);
    public static Point2D operator -(Point2D a, Point2D b);
    public double Length { get; }
}
```

二维点结构，支持向量加减和长度计算。用于内部几何运算。

### 3.2 Polygon

```csharp
public class Polygon
{
    public List<Point2D> Vertices { get; set; }
    public bool IsClipped { get; set; }
    
    public double Area { get; }     // 多边形面积（鞋带公式）
    public double SignedArea { get; } // 有符号面积（判断顺/逆时针）
    public void EnsureCCW();        // 标准化为逆时针
}
```

多边形模型，支持面积计算和方向标准化。`SignedArea` 用于判断顶点顺序，`EnsureCCW()` 确保 Sutherland-Hodgman 算法正确性。

### 3.3 SeedStatistics

```csharp
public class SeedStatistics
{
    public int UncutCount { get; set; }  // 未裁切种子数
    public double UncutArea { get; set; } // 未裁切种子总面积
    public int CutCount { get; set; }    // 裁切种子数
    public double CutArea { get; set; }  // 裁切种子总面积
    public int TotalCount => UncutCount + CutCount;
    public double TotalArea => UncutArea + CutArea;
}
```

统计结果模型，三组数据涵盖填充结果的所有指标。

---

## 四、关键问题与解决方案

### 4.1 边界顶点方向不确定

**问题**：AutoCAD 多段线可以是顺时针或逆时针，而 Sutherland-Hodgman 裁剪算法要求边界为逆时针。

**解决**：`Polygon.EnsureCCW()` 用有符号面积判断方向，若为顺时针则 `Vertices.Reverse()`。

### 4.2 圆弧边界处理

**问题**：边界含圆弧段时，`PointInPolygon` 精度不够，瓷砖分类错误。

**解决**：用 AutoCAD 原生 `IntersectWith` 替代自定义射线法。AutoCAD 引擎精确处理圆弧/曲线求交。

### 4.3 多次运行数据积累

**问题**：每次运行 Seed/Seed2 旧实体未清除，导致 Statistics 统计数据膨胀。

**解决**：`ClearResultLayer()` 在每次运行前删除 ResultLayer 上的所有实体。

### 4.4 线段数 vs 瓷砖数混淆

**问题**：一个边界瓷砖可产生 1~4 条线段，统计时被当作多个裁切种子。

**解决**：新增 `cutTiles` 计数器，每个边界瓷砖**只计一次**，而非按线段数计算。

### 4.5 不规则边界填充不完整

**问题**：BFS（广度优先扩散）在不规则边界上探索不完整。

**解决**：改用"全网格"方式——计算边界包围盒，生成覆盖所有网格位置的全部瓷砖，不依赖探索路径。

---

## 五、部署方式

### 5.1 Bundle 结构

```
%appdata%\Autodesk\ApplicationPlugins\SeedFill.bundle\
├── PackageContents.xml       ← AutoCAD 自动发现
└── Contents\
    ├── SeedFill.dll          ← 编译好的插件
    ├── SeedFill.lsp          ← 启动时自动 NETLOAD
    └── SeedFill.pdb          ← 调试符号
```

### 5.2 编译部署

```powershell
cd SeedFill
dotnet build    # 编译 + 自动复制到 Bundle 目录
```

### 5.3 使用流程

```
1. 画闭合多段线作为边界 → 图层设为"边界"
2. 画矩形作为种子 → 图层设为"种子"
3. 输入命令:
   Seed      → 瓷砖填充（规则边界）
   Seed2     → 线网格填充（不规则边界）
   Statistics → 选择边界 → 查看统计
```
