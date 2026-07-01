using System;
using System.Collections.Generic;
using System.Linq;
using Autodesk.Revit.DB;
using Autodesk.Revit.DB.Architecture;
using FloorPlanGenerator.Models;

namespace FloorPlanGenerator.Services
{
    /// <summary>
    /// 户型生成编排服务 — 统筹墙体、房间、标记的创建
    /// </summary>
    public class FloorPlanService
    {
        private readonly Document _doc;

        public FloorPlanService(Document doc)
        {
            _doc = doc;
        }

        /// <summary>
        /// 主入口：在指定视图中根据 JSON 数据创建完整户型
        /// </summary>
        public void GenerateFloorPlan(List<RoomData> rooms, ViewPlan viewPlan)
        {
            if (viewPlan == null)
                throw new ArgumentNullException(nameof(viewPlan));

            Level level = viewPlan.GenLevel;
            if (level == null)
                throw new InvalidOperationException("视图的标高无效，请选择有效的楼层平面视图。");

            // 1. 清空已有户型元素（墙体、房间、标记）
            using (Transaction tClear = new Transaction(_doc, "清空已有户型"))
            {
                tClear.Start();
                ClearExistingFloorPlan(viewPlan);
                tClear.Commit();
            }

            // 2. 创建墙体
            WallService wallService = new WallService(_doc);
            List<Wall> walls = null;

            using (Transaction t = new Transaction(_doc, "生成墙体"))
            {
                t.Start();
                walls = wallService.CreateWalls(rooms, level);
                t.Commit();
            }

            if (walls == null || walls.Count == 0)
                throw new InvalidOperationException("未能创建任何墙体！");

            // 3. 创建房间和房间标记
            RoomService roomService = new RoomService(_doc);
            roomService.CreateRoomsAndTags(rooms, level, viewPlan);

            // 4. 绘制绿色行进路线详图线（房间之间）
            using (Transaction tDetail = new Transaction(_doc, "绘制行进路线"))
            {
                tDetail.Start();
                DrawTravelPath(rooms, viewPlan);
                tDetail.Commit();
            }
        }

        /// <summary>
        /// 在每个房间的墙体上绘制绿色行进路线详图线段
        /// 每条边上的线段长度 = 房间周长 × 10% × (该边长 / 周长)
        /// 即总线段长度 = 房间周长的 10%，按边长比例分配到各边
        /// </summary>
        private void DrawTravelPath(List<RoomData> rooms, ViewPlan viewPlan)
        {
            if (rooms == null || rooms.Count == 0) return;

            // 获取或创建绿色详图线样式
            GraphicsStyle greenStyle = GetOrCreateGreenLineStyle(viewPlan);

            foreach (var room in rooms)
            {
                // 获取房间的四条边界线
                var boundaries = room.GetBoundaryLines();

                // 计算房间周长
                double perimeter = 0;
                foreach (var b in boundaries)
                    perimeter += b.Start.DistanceTo(b.End);

                // 总详图线长度 = 周长 × 100%
                double totalLineLength = perimeter;

                foreach (var boundary in boundaries)
                {
                    double wallLength = boundary.Start.DistanceTo(boundary.End);
                    if (wallLength < 0.01) continue;

                    // 该边分得的线段长度 = 总长 × (该边长 / 周长)
                    double segLen = totalLineLength * (wallLength / perimeter);
                    if (segLen < 0.3.MmToFeet()) continue; // 太短不画

                    // 居中计算起点终点
                    double margin = (wallLength - segLen) / 2.0;
                    XYZ dir = (boundary.End - boundary.Start).Normalize();

                    XYZ segStart = boundary.Start + dir * margin;
                    XYZ segEnd = segStart + dir * segLen;

                    // 稍微偏移到墙体外侧，避免与墙线重叠
                    double shift = 0.1.MmToFeet(); // 约 30mm 偏移
                    if (boundary.Orientation == Orientation.Vertical)
                    {
                        double offsetX = boundary.IsLeft ? -shift : shift;
                        segStart = new XYZ(segStart.X + offsetX, segStart.Y, 0);
                        segEnd = new XYZ(segEnd.X + offsetX, segEnd.Y, 0);
                    }
                    else
                    {
                        double offsetY = boundary.IsLeft ? shift : -shift;
                        segStart = new XYZ(segStart.X, segStart.Y + offsetY, 0);
                        segEnd = new XYZ(segEnd.X, segEnd.Y + offsetY, 0);
                    }

                    // 创建绿色详图线段
                    Line segLine = Line.CreateBound(segStart, segEnd);
                    DetailCurve dc = _doc.Create.NewDetailCurve(viewPlan, segLine);
                    dc.LineStyle = greenStyle;
                }
            }
        }

        /// <summary>
        /// 获取或创建绿色的"行进路线"详图线样式
        /// </summary>
        private GraphicsStyle GetOrCreateGreenLineStyle(ViewPlan viewPlan)
        {
            string styleName = "行进路线";

            // 查找已有的自定义线样式
            FilteredElementCollector collector = new FilteredElementCollector(_doc);
            var existingStyle = collector
                .OfClass(typeof(GraphicsStyle))
                .Cast<GraphicsStyle>()
                .FirstOrDefault(gs => gs.Name == styleName
                    && gs.GraphicsStyleCategory != null
                    && gs.GraphicsStyleCategory.CategoryType == CategoryType.Annotation);

            if (existingStyle != null)
                return existingStyle;

            // 在详图项目类别下创建新的子类别（绿色）
            Categories categories = _doc.Settings.Categories;
            Category detailCategory = categories.get_Item(BuiltInCategory.OST_DetailComponents);

            Category newSubCategory = _doc.Settings.Categories.NewSubcategory(
                detailCategory, styleName);

            // 设置绿色 + 加粗线宽（填满内墙缝隙）
            newSubCategory.LineColor = new Color(0, 200, 0); // 绿色
            newSubCategory.SetLineWeight(8, GraphicsStyleType.Projection); // 加粗

            // 获取对应的 GraphicsStyle
            collector = new FilteredElementCollector(_doc);
            return collector
                .OfClass(typeof(GraphicsStyle))
                .Cast<GraphicsStyle>()
                .FirstOrDefault(gs => gs.Name == styleName);
        }

        /// <summary>
        /// 清空视图中已有的户型元素（墙体、房间、房间标记）
        /// </summary>
        public void ClearExistingFloorPlan(ViewPlan viewPlan)
        {
            // 删除行进路线详图线
            DeleteTravelPathLines(viewPlan);

            // 收集当前视图中可见的所有墙体
            FilteredElementCollector wallCollector = new FilteredElementCollector(_doc, viewPlan.Id);
            var walls = wallCollector
                .OfClass(typeof(Wall))
                .Cast<Wall>()
                .ToList();

            // 删除房间和标记
            RoomService roomService = new RoomService(_doc);
            roomService.DeleteExistingRooms(viewPlan);

            // 删除墙体
            if (walls.Count > 0)
            {
                _doc.Delete(walls.Select(w => w.Id).ToList());
            }
        }

        /// <summary>
        /// 删除视图中已有的行进路线详图线
        /// </summary>
        private void DeleteTravelPathLines(ViewPlan viewPlan)
        {
            string styleName = "行进路线";

            // 查找行进路线线样式
            FilteredElementCollector collector = new FilteredElementCollector(_doc, viewPlan.Id);
            var detailLines = collector
                .OfClass(typeof(CurveElement))
                .Cast<CurveElement>()
                .Where(dl =>
                {
                    if (dl.LineStyle == null) return false;
                    return dl.LineStyle.Name == styleName;
                })
                .ToList();

            if (detailLines.Count > 0)
            {
                _doc.Delete(detailLines.Select(d => d.Id).ToList());
            }
        }

        /// <summary>
        /// 检查视图中是否已有户型
        /// </summary>
        public bool HasFloorPlan(ViewPlan viewPlan)
        {
            FilteredElementCollector collector = new FilteredElementCollector(_doc, viewPlan.Id);
            return collector
                .OfClass(typeof(Wall))
                .GetElementCount() > 0;
        }
    }
}
