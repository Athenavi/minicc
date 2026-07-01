# 户型生成 Revit 插件 — 成果答辩（代码逻辑阐述）

## 一、项目概述

### 1.1 项目目标

开发 Revit 2026 插件，从 JSON 文件（网页设计端输出）读取房间数据，在 Revit 中自动创建**墙体、房间、房间标记**三种元素，并按照现实建筑规范处理内外墙厚度、墙体贯通和外墙外扩。

### 1.2 开发环境

| 项目 | 说明 |
|------|------|
| Revit 版本 | Revit 2026 |
| .NET 版本 | .NET Framework 4.7.2 |
| 开发语言 | C# |
| 加载方式 | `.addin` 文件注册 |

### 1.3 用户交互流程

```
Command.Execute()
  │
  ├─ 检查当前视图是否为 ViewPlan
  ├─ 弹出 OpenFileDialog 选择 huxi.json
  ├─ JsonReader 解析 JSON → List<RoomData>
  ├─ FloorPlanService.GenerateFloorPlan()
  │   ├─ 清空已有户型（墙体 + 房间 + 标记 + 行进路线）
  │   ├─ WallService.CreateWalls()  → 创建墙体
  │   └─ RoomService.CreateRoomsAndTags() → 创建房间 + 标记
  │   └─ DrawTravelPath() → 绿色行进路线
  └─ TaskDialog 显示完成信息
```

---

## 二、架构设计

### 2.1 分层架构

```
┌──────────────────────────────────────────┐
│    App.cs (IExternalApplication)         │ ← Ribbon 面板注册
├──────────────────────────────────────────┤
│    Command.cs (IExternalCommand)         │ ← 主命令入口
├──────────────────────────────────────────┤
│    FloorPlanService.cs                   │ ← 流程编排
├─────────────────┬──────────────────┬─────┤
│  WallService.cs  │  RoomService.cs   │ JsonReader.cs │
│  (墙体算法核心)   │  (房间+标记生成)   │ (JSON解析)    │
├─────────────────┴──────────────────┴─────┤
│  Models/RoomData.cs / RoomJson.cs         │ ← 数据模型
│  Services/UnitExtensions.cs               │ ← 单位转换
└──────────────────────────────────────────┘
```

### 2.2 文件结构

```
FloorPlanGenerator/
├── FloorPlanGenerator.csproj
├── FloorPlanGenerator.addin        ← Revit 插件注册文件
├── App.cs                          ← Ribbon 面板 + 按钮
├── Command.cs                      ← 主命令入口
├── Properties/AssemblyInfo.cs
├── Models/
│   ├── RoomData.cs                 ← 房间数据 + 边界线 + BoundaryLine
│   └── RoomJson.cs                 ← JSON 顶层结构
├── Services/
│   ├── JsonReader.cs               ← JSON 手动解析
│   ├── UnitExtensions.cs           ← 毫米↔英尺转换
│   ├── WallService.cs              ← 墙体算法（内外墙判定、合并）
│   ├── RoomService.cs              ← 房间/标记创建/删除
│   └── FloorPlanService.cs         ← 流程编排 + 行进路线
```

---

## 三、核心代码逻辑

### 3.1 命令入口（Command.cs）

```csharp
[Transaction(TransactionMode.Manual)]
public class Command : IExternalCommand
{
    public Result Execute(ExternalCommandData commandData,
        ref string message, ElementSet elements)
    {
        // Step 1: 检查当前视图是否为楼层平面视图
        if (!(doc.ActiveView is ViewPlan))
            return Result.Cancelled;

        // Step 2: 弹出文件选择器选择 huxi.json
        OpenFileDialog → 选择 .json 文件

        // Step 3: 解析 JSON
        JsonReader reader = new JsonReader();
        RoomJson roomJson = reader.ReadFromFile(jsonPath);

        // Step 4: 调用编排服务生成户型
        FloorPlanService service = new FloorPlanService(doc);
        service.GenerateFloorPlan(roomJson.Spaces, viewPlan);

        return Result.Succeeded;
    }
}
```

**设计要点**：
- `[Transaction(TransactionMode.Manual)]` — 手动管理事务，每个操作独立提交
- 先校验视图类型再执行，避免在非平面视图中运行
- 各步骤抛出异常由上层统一捕获

### 3.2 流程编排（FloorPlanService.cs）

`GenerateFloorPlan()` 统筹四个步骤：

```csharp
public void GenerateFloorPlan(List<RoomData> rooms, ViewPlan viewPlan)
{
    // 1. 清空已有户型
    Transaction tClear → ClearExistingFloorPlan()

    // 2. 创建墙体（独立事务）
    Transaction tWalls → WallService.CreateWalls(rooms, level)

    // 3. 创建房间和标记（独立事务）
    RoomService.CreateRoomsAndTags(rooms, level, viewPlan)

    // 4. 绘制绿色行进路线（独立事务）
    Transaction tDetail → DrawTravelPath(rooms, viewPlan)
}
```

**设计理由**：每个步骤使用独立事务，实现原子性。若某一步失败，不影响已完成的操作。

**清空逻辑** `ClearExistingFloorPlan()`：
1. 删除行进路线详图线（按线样式名称过滤）
2. 删除房间和标记（调用 RoomService.DeleteExistingRooms）
3. 删除墙体（按视图可见性过滤）

### 3.3 JSON 解析（JsonReader.cs）

手动正则解析，不依赖 `System.Web.Extensions`：

```
ReadFromFile(path) → Parse(jsonContent)
  → 正则提取 "Level"
  → 提取 "Spaces" 数组内容
  → 分割每个房间对象 { }
  → 对每个对象提取 CentroidX, CentroidY, Bay, Depth, Name
```

**设计理由**：Revit 插件通常引用 .NET Framework 4.x，`System.Web.Extensions` 需要额外配置。手动解析减少外部依赖，稳定可靠。

### 3.4 墙体核心算法（WallService.cs）

五步算法：

```
CreateWalls(rooms, level)
  │
  ├─ Step 1: 为每个房间生成 4 条边界线（左/右/下/上）
  │           标记所属房间索引和名称
  │
  ├─ Step 2: 判断内外墙
  │   DetermineInteriorExterior(boundaries)
  │   └─ 两两比较不同房间的边界线
  │      共线且重叠 → 标记为内墙（共享）
  │      否则 → 外墙（独有）
  │
  ├─ Step 3: 外墙外扩 100mm
  │   ExtendExteriorWalls(boundaries)
  │   └─ 左墙→左移, 右墙→右移, 上墙→上移, 下墙→下移
  │
  ├─ Step 4: 合并共线重叠墙体
  │   MergeCollinearLines(boundaries)
  │   └─ 按 (方向, 厚度, 固定坐标) 分组
  │      每组内合并一维区间
  │
  └─ Step 5: 创建 Revit 墙体元素
      CreateWallElements(mergedLines, level)
      └─ 内墙→100mm, 外墙→200mm
```

#### Step 2：内外墙判定（`DetermineInteriorExterior`）

```csharp
for (int i = 0; i < boundaries.Count; i++)
    for (int j = i + 1; j < boundaries.Count; j++)
        // 只比较不同房间 + 同方向
        if (bi.RoomIndex != bj.RoomIndex
            && bi.Orientation == bj.Orientation
            && LinesAreCollinearAndOverlap(bi, bj))
        {
            bi.IsExterior = false;  // 共享→内墙
            bj.IsExterior = false;
        }
```

**共线重叠判断**（`LinesAreCollinearAndOverlap`）：
- 垂直墙：比较 X 坐标（容差 0.01 英尺 ≈ 3mm），检查 Y 区间重叠
- 水平墙：比较 Y 坐标，检查 X 区间重叠

#### Step 3：外墙外扩（`ExtendExteriorWalls`）

| 墙方向 | IsLeft | 操作 |
|--------|--------|------|
| 垂直 | true（左墙） | X 坐标减小 100mm |
| 垂直 | false（右墙） | X 坐标增大 100mm |
| 水平 | true（上墙） | Y 坐标增大 100mm |
| 水平 | false（下墙） | Y 坐标减小 100mm |

#### Step 4：共线合并（`MergeCollinearLines`）

```
按 (方向, 厚度, 固定坐标轴) 分组：
  垂直组：(X坐标, 厚度) → 收集 Y 区间 → 合并重叠 → 创建合并后的墙线
  水平组：(Y坐标, 厚度) → 收集 X 区间 → 合并重叠 → 创建合并后的墙线

区间合并算法（MergeIntervals）：
  1. 按 Min 排序
  2. 遍历，若当前区间与上个区间重叠 → 扩展 Max
  3. 否则 → 开始新区间
```

#### Step 5：创建墙体（`CreateWallElements`）

```csharp
foreach (var line in mergedLines)
{
    // 过滤极小线段（< 10mm）
    Wall wall = Wall.Create(doc, line.GetLine(),
        GetWallType(thicknessMm).Id, level.Id,
        10.0, 0, false, false);
}
```

**墙类型管理**（`GetWallType`）：
1. 按名称查找已有类型（"外墙-200mm" / "内墙-100mm"）
2. 不存在则从系统默认墙类型复制
3. 修改复合结构第一层厚度为新值

### 3.5 房间与标记（RoomService.cs）

```csharp
foreach (var roomData in roomsData)
{
    // 在房间中心点创建房间
    XYZ point = new XYZ(
        roomData.CentroidX.MmToFeet(),
        roomData.CentroidY.MmToFeet(), 0);
    Room room = doc.Create.NewRoom(level, new UV(point.X, point.Y));

    // 设置房间名称（支持中文）
    room.get_Parameter(BuiltInParameter.ROOM_NAME).Set(roomData.Name);

    // 创建房间标记
    RoomTag tag = doc.Create.NewRoomTag(
        new LinkElementId(room.Id),
        new UV(point.X, point.Y), viewPlan.Id);
}
```

**删除逻辑**（`DeleteExistingRooms`）：
1. 收集视图中的 `SpatialElementTag`（房间标记）
2. 收集文档中同标高的 `SpatialElement`（房间）
3. 统一删除

### 3.6 行进路线绘制（FloorPlanService.DrawTravelPath）

在每个房间的墙体上绘制绿色加粗详图线：

```
对每个房间：
  1. 获取 4 条边界线，计算周长
  2. 每条边分得的线段长度 = 总长 × (该边长 / 周长)
  3. 居中放置，偏移到墙体外侧（避免重叠）
  4. 创建绿色 DetailCurve（线样式="行进路线"）
```

**线样式管理**（`GetOrCreateGreenLineStyle`）：
1. 查找已存在的"行进路线"线样式
2. 不存在则在 `OST_DetailComponents` 下创建子类别
3. 设置颜色 = 绿色 (0, 200, 0)，线宽 = 8（加粗）

---

## 四、数据结构

### 4.1 RoomData

```csharp
public class RoomData
{
    public double CentroidX { get; set; }  // 房间中心 X (mm)
    public double CentroidY { get; set; }  // 房间中心 Y (mm)
    public string Name { get; set; }       // 房间名称
    public double Bay { get; set; }        // 开间 (mm)
    public double Depth { get; set; }      // 进深 (mm)

    public List<BoundaryLine> GetBoundaryLines() { }
        // → [左墙, 右墙, 下墙, 上墙]
}
```

### 4.2 BoundaryLine

```csharp
public class BoundaryLine
{
    public XYZ Start { get; set; }
    public XYZ End { get; set; }
    public Orientation Orientation { get; set; }  // Horizontal / Vertical
    public bool IsLeft { get; set; }
    // Vertical: true=左墙, false=右墙
    // Horizontal: true=上墙, false=下墙
    public int RoomIndex { get; set; }
    public string RoomName { get; set; }
    public bool IsExterior { get; set; } = true;  // 外墙?
    public double ThicknessMm => IsExterior ? 200.0 : 100.0;
}
```

### 4.3 RoomJson

```csharp
public class RoomJson
{
    public string Level { get; set; }
    public List<RoomData> Spaces { get; set; }
}
```

### 4.4 Orientation 枚举

```csharp
public enum Orientation { Horizontal, Vertical }
```

---

## 五、关键技术点

| 技术点 | 说明 |
|--------|------|
| **内外墙判定** | 两两比较不同房间的边界线，共线且重叠→内墙（100mm），否则→外墙（200mm） |
| **外墙外扩** | 外墙边界线向外侧偏移 100mm，按左右上下方向分别处理 |
| **墙体合并** | 按方向+厚度分组→区间排序→合并重叠→每个合并区间创建一条连续墙 |
| **事务管理** | 每个操作独立事务（清空/建墙/建房间），实现原子性 |
| **手动 JSON 解析** | 正则表达式手动解析，不依赖第三方库 |
| **单位转换** | 毫米→英尺（除以 304.8），定义 `MmToFeet()` 扩展方法 |
| **行进路线** | 绿色加粗详图线，按边长比例分配总长度，偏移到墙体外侧 |
| **清空已有户型** | 按元素类型和线样式精确过滤删除，支持重复生成 |
