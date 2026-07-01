using System;
using System.Collections.Generic;
using System.Linq;
using Autodesk.Revit.DB;
using FloorPlanGenerator.Models;

namespace FloorPlanGenerator.Services
{
    /// <summary>
    /// 墙体生成与合并核心算法
    /// </summary>
    public class WallService
    {
        private readonly Document _doc;

        // 外墙外扩量（需求：超出边界线 100mm）
        private const double ExteriorExtensionMm = 100.0;

        public WallService(Document doc)
        {
            _doc = doc;
        }

        /// <summary>
        /// 主入口：根据所有房间数据生成墙体
        /// </summary>
        public List<Wall> CreateWalls(List<RoomData> rooms, Level level)
        {
            // Step 1: 为每个房间生成 4 条边界线，标记所属房间
            var allBoundaries = new List<BoundaryLine>();
            for (int i = 0; i < rooms.Count; i++)
            {
                var lines = rooms[i].GetBoundaryLines();
                foreach (var line in lines)
                {
                    line.RoomIndex = i;
                    line.RoomName = rooms[i].Name;
                }
                allBoundaries.AddRange(lines);
            }

            // Step 2: 判断内外墙
            DetermineInteriorExterior(allBoundaries);

            // Step 3: 外墙外扩
            ExtendExteriorWalls(allBoundaries);

            // Step 4: 按方向+厚度分组，同组内合并重叠线段
            var mergedLines = MergeCollinearLines(allBoundaries);

            // Step 5: 在 Revit 中创建墙体
            return CreateWallElements(mergedLines, level);
        }

        /// <summary>
        /// 判断每条边界线是内墙还是外墙。
        /// 如果另一房间有共线且重叠的边界 → 内墙（共享）；否则外墙（独有）。
        /// </summary>
        private void DetermineInteriorExterior(List<BoundaryLine> boundaries)
        {
            for (int i = 0; i < boundaries.Count; i++)
            {
                var bi = boundaries[i];

                for (int j = i + 1; j < boundaries.Count; j++)
                {
                    var bj = boundaries[j];

                    // 只比较不同房间的墙
                    if (bi.RoomIndex == bj.RoomIndex) continue;

                    // 只比较同方向的墙
                    if (bi.Orientation != bj.Orientation) continue;

                    // 检查是否共线且重叠
                    if (LinesAreCollinearAndOverlap(bi, bj))
                    {
                        // 两个房间共享此墙 → 内墙
                        bi.IsExterior = false;
                        bj.IsExterior = false;
                    }
                }
            }
        }

        /// <summary>
        /// 判断两条边界线是否共线且有重叠
        /// </summary>
        private bool LinesAreCollinearAndOverlap(BoundaryLine a, BoundaryLine b)
        {
            double tolerance = 0.01; // 约 3mm 的容差

            if (a.Orientation == Orientation.Vertical)
            {
                // 垂直墙：X 坐标必须相同
                if (Math.Abs(a.Start.X - b.Start.X) > tolerance) return false;

                // Y 方向必须有重叠区间
                double aMin = Math.Min(a.Start.Y, a.End.Y);
                double aMax = Math.Max(a.Start.Y, a.End.Y);
                double bMin = Math.Min(b.Start.Y, b.End.Y);
                double bMax = Math.Max(b.Start.Y, b.End.Y);

                return aMin <= bMax - tolerance && bMin <= aMax - tolerance;
            }
            else // Horizontal
            {
                // 水平墙：Y 坐标必须相同
                if (Math.Abs(a.Start.Y - b.Start.Y) > tolerance) return false;

                // X 方向必须有重叠区间
                double aMin = Math.Min(a.Start.X, a.End.X);
                double aMax = Math.Max(a.Start.X, a.End.X);
                double bMin = Math.Min(b.Start.X, b.End.X);
                double bMax = Math.Max(b.Start.X, b.End.X);

                return aMin <= bMax - tolerance && bMin <= aMax - tolerance;
            }
        }

        /// <summary>
        /// 外墙向外扩展 100mm（边界线朝户型外侧延长）
        /// - 左墙：向左延长（X 减小）
        /// - 右墙：向右延长（X 增大）
        /// - 下墙：向下延长（Y 减小）
        /// - 上墙：向上延长（Y 增大）
        /// </summary>
        private void ExtendExteriorWalls(List<BoundaryLine> boundaries)
        {
            double ext = ExteriorExtensionMm.MmToFeet();

            foreach (var b in boundaries)
            {
                if (!b.IsExterior) continue;

                if (b.Orientation == Orientation.Vertical)
                {
                    if (b.IsLeft) // 左墙：向左扩
                    {
                        b.Start = new XYZ(b.Start.X - ext, b.Start.Y, 0);
                        b.End = new XYZ(b.End.X - ext, b.End.Y, 0);
                    }
                    else // 右墙：向右扩
                    {
                        b.Start = new XYZ(b.Start.X + ext, b.Start.Y, 0);
                        b.End = new XYZ(b.End.X + ext, b.End.Y, 0);
                    }
                }
                else // Horizontal
                {
                    if (b.IsLeft) // 上墙：向上扩
                    {
                        b.Start = new XYZ(b.Start.X, b.Start.Y + ext, 0);
                        b.End = new XYZ(b.End.X, b.End.Y + ext, 0);
                    }
                    else // 下墙：向下扩
                    {
                        b.Start = new XYZ(b.Start.X, b.Start.Y - ext, 0);
                        b.End = new XYZ(b.End.X, b.End.Y - ext, 0);
                    }
                }
            }
        }

        /// <summary>
        /// 按方向+厚度分组，同组内合并共线重叠线段。
        /// 合并后每个线段用一个 BoundaryLine 表示（取最小起点的最大终点）。
        /// </summary>
        private List<BoundaryLine> MergeCollinearLines(List<BoundaryLine> boundaries)
        {
            // 按 (方向, 厚度, 固定坐标轴) 分组
            var verticalGroups = boundaries
                .Where(b => b.Orientation == Orientation.Vertical)
                .GroupBy(b => new { Key = Math.Round(b.Start.X, 4), Thick = b.ThicknessFeet })
                .ToList();

            var horizontalGroups = boundaries
                .Where(b => b.Orientation == Orientation.Horizontal)
                .GroupBy(b => new { Key = Math.Round(b.Start.Y, 4), Thick = b.ThicknessFeet })
                .ToList();

            var merged = new List<BoundaryLine>();

            foreach (var group in verticalGroups)
            {
                double x = group.Key.Key;
                bool isExterior = group.Any(b => b.IsExterior);

                // 收集所有 Y 区间
                var intervals = group.Select(b =>
                {
                    double yMin = Math.Min(b.Start.Y, b.End.Y);
                    double yMax = Math.Max(b.Start.Y, b.End.Y);
                    return (Min: yMin, Max: yMax);
                }).OrderBy(i => i.Min).ToList();

                // 合并重叠区间
                var mergedIntervals = MergeIntervals(intervals);

                foreach (var iv in mergedIntervals)
                {
                    var line = new BoundaryLine
                    {
                        Start = new XYZ(x, iv.Min, 0),
                        End = new XYZ(x, iv.Max, 0),
                        Orientation = Orientation.Vertical,
                        IsExterior = isExterior
                    };
                    merged.Add(line);
                }
            }

            foreach (var group in horizontalGroups)
            {
                double y = group.Key.Key;
                bool isExterior = group.Any(b => b.IsExterior);

                // 收集所有 X 区间
                var intervals = group.Select(b =>
                {
                    double xMin = Math.Min(b.Start.X, b.End.X);
                    double xMax = Math.Max(b.Start.X, b.End.X);
                    return (Min: xMin, Max: xMax);
                }).OrderBy(i => i.Min).ToList();

                var mergedIntervals = MergeIntervals(intervals);

                foreach (var iv in mergedIntervals)
                {
                    var line = new BoundaryLine
                    {
                        Start = new XYZ(iv.Min, y, 0),
                        End = new XYZ(iv.Max, y, 0),
                        Orientation = Orientation.Horizontal,
                        IsExterior = isExterior
                    };
                    merged.Add(line);
                }
            }

            return merged;
        }

        /// <summary>
        /// 合并一维重叠区间
        /// </summary>
        private List<(double Min, double Max)> MergeIntervals(IEnumerable<(double Min, double Max)> intervals)
        {
            var sorted = intervals.OrderBy(i => i.Min).ToList();
            var result = new List<(double Min, double Max)>();

            if (sorted.Count == 0) return result;

            double curMin = sorted[0].Min;
            double curMax = sorted[0].Max;

            for (int i = 1; i < sorted.Count; i++)
            {
                double tolerance = 0.001;
                if (sorted[i].Min <= curMax + tolerance) // 有重叠或相接
                {
                    curMax = Math.Max(curMax, sorted[i].Max);
                }
                else
                {
                    result.Add((curMin, curMax));
                    curMin = sorted[i].Min;
                    curMax = sorted[i].Max;
                }
            }
            result.Add((curMin, curMax));
            return result;
        }

        /// <summary>
        /// 在 Revit 中创建墙体元素
        /// </summary>
        private List<Wall> CreateWallElements(List<BoundaryLine> mergedLines, Level level)
        {
            var walls = new List<Wall>();

            foreach (var line in mergedLines)
            {
                // 过滤极小线段（小于 10mm）
                double length = line.Start.DistanceTo(line.End);
                if (length < 0.03.MmToFeet()) continue; // 约 10mm

                Line revitLine = line.GetLine();

                // 创建墙（默认墙高 10 英尺 ≈ 3048mm）
                Wall wall = Wall.Create(
                    _doc,
                    revitLine,
                    GetWallType(line.ThicknessMm).Id,
                    level.Id,
                    10.0, // 高度 10 英尺
                    0, // 底部偏移
                    false, // Flip
                    false  // Structural
                );

                walls.Add(wall);
            }

            return walls;
        }

        /// 获取对应厚度的墙类型。
        /// 如果不存在，则从系统复制创建，并设置正确的厚度。
        /// </summary>
        private WallType GetWallType(double thicknessMm)
        {
            string typeName = thicknessMm >= 190 ? "外墙-200mm" : "内墙-100mm";
            double thicknessFeet = thicknessMm.MmToFeet();

            // 查找是否已有
            FilteredElementCollector collector = new FilteredElementCollector(_doc);
            var existingType = collector
                .OfClass(typeof(WallType))
                .Cast<WallType>()
                .FirstOrDefault(wt => wt.Name == typeName);

            if (existingType != null)
                return existingType;

            // 复制系统默认墙类型
            var defaultWallType = collector
                .OfClass(typeof(WallType))
                .Cast<WallType>()
                .FirstOrDefault(wt => wt.Kind == WallKind.Basic);

            if (defaultWallType == null)
                throw new InvalidOperationException("Revit 项目中没有任何基本墙类型！");

            WallType newType = defaultWallType.Duplicate(typeName) as WallType;
            if (newType == null)
                throw new InvalidOperationException("无法创建墙类型：" + typeName);

            // 设置墙类型的实际厚度（修改复合结构的层厚）
            CompoundStructure cs = newType.GetCompoundStructure();
            if (cs != null)
            {
                IList<CompoundStructureLayer> layers = cs.GetLayers();
                if (layers != null && layers.Count > 0)
                {
                    // 修改第一层（主结构层）的厚度
                    CompoundStructureLayer firstLayer = layers[0];
                    layers[0] = new CompoundStructureLayer(
                        thicknessFeet,
                        firstLayer.Function,
                        firstLayer.MaterialId);
                    cs.SetLayers(layers);
                    newType.SetCompoundStructure(cs);
                }
            }

            return newType;
        }
    }
}
