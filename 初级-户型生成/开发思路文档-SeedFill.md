# 种子填充 AutoCAD 插件 — 开发思路文档

## 一、需求概述

### 1.1 输入条件

| 要素 | 说明 |
|------|------|
| **边界** | 任意形状的闭合多段线，图层为"边界"。可由直线段和圆弧段组成，面积大小不固定 |
| **种子** | 矩形闭合多段线，图层为"种子"。旋转角度不固定，面积远小于边界，第一个种子完全位于边界内部 |

### 1.2 输出要求

- **种子填充**：用种子矩形平铺整个边界内部区域。完全在内部的瓷砖为完整矩形，碰到边界的瓷砖被裁剪
- **种子统计**：统计未裁剪种子（内部完整矩形）和裁切种子（边界被裁剪的瓷砖）的数量和面积
- **绘图次序**：边界绘图次序位于种子之上，方便选择边界线

### 1.3 命令设计

| 命令 | 功能 |
|------|------|
| `Seed` | 选择边界和种子 → 瓷砖平铺填充（规则边界优先） |
| `Seed2` | 选择边界和种子 → 线网格填充（不规则边界优先） |
| `Statistics` | 选择边界 → 统计种子数据（未裁剪/裁切的个数和面积） |

---

## 二、架构设计

### 2.1 整体架构

```
┌─────────────────────────────────────────────┐
│         SeedCommands.cs (命令入口)           │
│  [CommandMethod("Seed")]                     │
│  [CommandMethod("Seed2")]                    │
│  [CommandMethod("Statistics")]               │
├─────────────────────────────────────────────┤
│         SeedFillService.cs (核心算法)         │
│  Execute()    — 瓷砖平铺填充                  │
│  Execute2()   — 线网格填充                    │
│  GetStatistics() — 统计                       │
├──────────────────┬──────────────────────────┤
│  ClippingService │  SeedResult.cs (模型)     │
│  (裁剪算法)       │  Polygon / Point2D       │
│                   │  SeedStatistics          │
└──────────────────┴──────────────────────────┘
```

### 2.2 文件结构

```
SeedFill/
├── SeedFill.csproj          — 项目文件（.NET 10.0-windows）
├── PackageContents.xml       — AutoCAD Bundle 注册文件
├── Contents/
│   └── SeedFill.lsp          — LISP 引导脚本（自动 NETLOAD）
├── App.cs                    — IExtensionApplication 入口
├── Commands/
│   └── SeedCommands.cs       — AutoCAD 命令注册
├── Models/
│   └── SeedResult.cs         — Polygon、Point2D、SeedStatistics
├── Services/
│   ├── SeedFillService.cs    — 核心填充算法
│   └── ClippingService.cs    — 多边形裁剪（备选方案）
```

---

## 三、核心算法

### 3.1 算法总览

种子填充的核心思路是：

1. **提取种子参数**：从种子矩形中提取长边长度、短边长度和旋转角度
2. **构建种子局部坐标系**：以种子中心为原点，长边方向为 U 轴，短边方向为 V 轴
3. **生成平铺网格**：在边界包围盒内，以 `longSide × shortSide` 为单元格生成网格
4. **分类处理每个瓷砖**：完全在内→保留完整矩形；部分在外→裁剪为线段；完全在外→跳过
5. **写入 AutoCAD**：生成闭合多段线（内部）和 LINE 实体（边界）

### 3.2 种子参数提取

```
种子矩形顶点顺序: V0 → V1 → V2 → V3（首尾闭合）
边 0-1: 长度 = seedWidth,  方向角 = angle
边 1-2: 长度 = seedHeight

长边 longSide = max(seedWidth, seedHeight)
短边 shortSide = min(seedWidth, seedHeight)
```

### 3.3 种子局部坐标系

以种子中心为原点，构建局部坐标系：

```
U 轴方向 = 长边方向（单位向量 dirLong）
V 轴方向 = 短边方向（单位向量 dirShort），垂直于 U 轴

世界坐标 ↔ 局部坐标转换：
  world.X = center.X + u * dirLong.X + v * dirShort.X
  world.Y = center.Y + u * dirLong.Y + v * dirShort.Y
```

### 3.4 网格生成

```
1. 将边界的每个顶点投影到局部坐标系，得 minU/maxU, minV/maxV
2. 计算网格范围：
   uiMin = floor((minU - longSide/2) / longSide)
   uiMax = ceil((maxU + longSide/2) / longSide)
   viMin = floor((minV - shortSide/2) / shortSide)
   viMax = ceil((maxV + shortSide/2) / shortSide)
3. 双层循环遍历每个网格单元格：
   for ui in [uiMin..uiMax]:
     for vi in [viMin..viMax]:
       处理瓷砖 (ui, vi)
```

### 3.5 瓷砖分类与处理

对每个瓷砖，用 AutoCAD 原生 `Curve.IntersectWith` 做点包含检测：

```
对瓷砖的 4 个角：
  从角点向右发出一条射线（LINE）
  boundaryPline.IntersectWith(ray) → 数交点
  奇数个交点 → 在边界内
  偶数个交点 → 在边界外

结果：
  4 个角全在内部 → 完整瓷砖 → 闭合 Polyline（未裁剪）
  部分在内部     → 边裁剪为 LINE（裁切）
  全在外部       → 跳过
```

### 3.6 边界瓷砖的边裁剪

对每个与边界相交的瓷砖：

```
对每条边 (corner[i] → corner[(i+1)%4]):
  1. 创建临时 LINE 实体
  2. boundaryPline.IntersectWith(line) → 获取交点
  3. 判断起点和终点是否在边界内
  4. 根据内外状态保留边界内的线段段：
     - 起点在内、终点在内 → 保留整条边
     - 起点在内、终点在外 → 保留起点→第一个交点
     - 起点在外、终点在内 → 保留最末交点→终点
     - 都在外但有交点 → 保留入口→出口段
  5. 将保留的线段加入 boundarySegments
```

### 3.7 Seed2：线网格填充（适用于不规则边界）

对含圆弧、样条曲线的复杂边界，改用线网格方式：

```
1. 从种子边出发，生成平行于长边的线，间隔 = shortSide
2. 从种子边出发，生成平行于短边的线，间隔 = longSide  
3. 每条线延伸到 farExt（边界范围的 2 倍）
4. 用 boundaryPline.IntersectWith(line) 精确裁剪每条线
5. 保留下来的线段为 LINE 实体
```

### 3.8 统计逻辑

```
GetStatistics(ObjectId boundaryId):
  1. 读取边界 Polygon，用于空间过滤
  2. 遍历模型空间所有实体
  3. 筛选 ResultLayer 上的实体
  4. 对闭合多段线 → 取中心点 → 在边界内 → 未裁切种子
  5. 对 LINE → 取中点 → 在边界内 → 裁切种子
```

---

## 四、关键技术点

### 4.1 AutoCAD 原生几何引擎

核心几何操作全部使用 AutoCAD 原生 API：

| 操作 | API | 优势 |
|------|-----|------|
| 点包含检测 | `Curve.IntersectWith(ray, ...)` | 处理任意曲线，无精度问题 |
| 线段裁剪 | `Curve.IntersectWith(line, ...)` | 精确求交，支持圆弧/样条 |
| 空间过滤 | `Entity.Layer` 属性 | 快速筛选目标实体 |

### 4.2 图层管理

| 图层名 | 用途 | 可见性 |
|--------|------|--------|
| 边界 | 用户绘制的边界线 | 可见 |
| 种子 | 用户绘制的种子矩形 | 可见 |
| 种子填充结果 | 生成的瓷砖/线段 | 可见 |
| 种子填充边框 | 复制的隐藏边界 | 初始隐藏 |

### 4.3 Bundle 自动加载

```
AutoCAD 启动
  → 扫描 %appdata%\ApplicationPlugins\SeedFill.bundle\
  → 读取 PackageContents.xml
  → 自动加载 Contents\SeedFill.lsp
  → LISP 执行 NETLOAD 加载 SeedFill.dll
  → [CommandMethod] 注册 Seed、Seed2、Statistics 命令
```

### 4.4 每次运行自动清空

`ClearResultLayer()` 在每次运行 Seed/Seed2 前删除 `种子填充结果` 图层上的所有旧实体，防止多次运行积累导致数据错误。

---

## 五、算法演进历史

| 版本 | 方法 | 问题 |
|------|------|------|
| v1 | 自定义 Sutherland-Hodgman 裁剪 | 凹多边形产生鬼影边 |
| v2 | AutoCAD Region.BooleanOperation | 爆炸后产生小线段 |
| v3 | BFS 从种子扩散 | 不规则边界探索不完整 |
| v4 | 全网格 + 手动线段求交 | 曲线边界精度不够 |
| v5 | AutoCAD Hatch 引擎 | 填充图案非独立线条 |
| v6 | **Curve.IntersectWith 原生裁剪** ✅ | 正确处理所有边界类型 |
