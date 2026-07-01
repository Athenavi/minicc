using System;
using System.Collections.Generic;
using Autodesk.AutoCAD.ApplicationServices;
using Autodesk.AutoCAD.DatabaseServices;
using Autodesk.AutoCAD.EditorInput;
using Autodesk.AutoCAD.Geometry;
using Autodesk.AutoCAD.Runtime;
using SeedFill.Models;

namespace SeedFill.Services
{
    /// <summary>
    /// 种子填充核心算法
    /// </summary>
    public class SeedFillService
    {
        private const string BoundaryLayer = "边界";
        private const string SeedLayer = "种子";
        private const string ResultLayer = "种子填充结果";
        private const string HiddenBoundaryLayer = "种子填充边框";

        private readonly Database _db;

        public SeedFillService(Database db)
        {
            _db = db;
        }

        /// <summary>
        /// 主入口：执行种子填充
        /// 平铺瓷砖，内部瓷砖=闭合多段线(uncut)，边界瓷砖=LINE(cut)
        /// </summary>
        public SeedStatistics Execute()
        {
            ClearResultLayer();
            Document doc = Application.DocumentManager.MdiActiveDocument;
            Editor ed = doc.Editor;

            // 1. 选择边界
            PromptEntityOptions peo = new PromptEntityOptions("\n请选择边界多段线（图层：边界）");
            peo.SetRejectMessage("请选择闭合多段线");
            peo.AddAllowedClass(typeof(Polyline), true);
            PromptEntityResult per = ed.GetEntity(peo);
            if (per.Status != PromptStatus.OK) return null;

            ObjectId boundaryId = per.ObjectId;
            Polygon boundary = PolylineToPolygon(boundaryId);
            if (boundary == null) { ed.WriteMessage("\n边界无效！"); return null; }
            boundary.EnsureCCW(); // 确保逆时针，Sutherland-Hodgman 要求

            // 2. 选择种子
            peo.Message = "\n请选择种子矩形（图层：种子）";
            per = ed.GetEntity(peo);
            if (per.Status != PromptStatus.OK) return null;

            ObjectId seedId = per.ObjectId;
            Polygon seedPoly = PolylineToPolygon(seedId);
            if (seedPoly == null) { ed.WriteMessage("\n种子无效！"); return null; }

            // 3. 提取种子矩形的参数
            double seedWidth, seedHeight;
            double angle;
            ExtractSeedParams(seedPoly, out seedWidth, out seedHeight, out angle);

            if (seedWidth < 1e-6 || seedHeight < 1e-6)
            { ed.WriteMessage("\n种子尺寸无效！"); return null; }

            ed.WriteMessage($"\n种子尺寸: {seedWidth:F2} x {seedHeight:F2}, 旋转角: {angle * 180 / Math.PI:F1}°");

            // 4. 平铺瓷砖：以种子中心到边界最远点为半径，平铺覆盖整个区域
            var results = new List<Polygon>();
            SeedStatistics stats = new SeedStatistics();

            double cosA = Math.Cos(angle);
            double sinA = Math.Sin(angle);

            // 确定长边和短边方向
            double longSide, shortSide;
            double dirLongX, dirLongY;     // 长边方向（单位向量）
            double dirShortX, dirShortY;   // 短边方向（单位向量）
            if (seedWidth >= seedHeight)
            {
                longSide = seedWidth; shortSide = seedHeight;
                dirLongX = cosA; dirLongY = sinA;
                dirShortX = -sinA; dirShortY = cosA;
            }
            else
            {
                longSide = seedHeight; shortSide = seedWidth;
                dirShortX = cosA; dirShortY = sinA;
                dirLongX = -sinA; dirLongY = cosA;
            }

            // 计算种子中心（4个顶点的平均）
            double cx = 0, cy = 0;
            foreach (var v in seedPoly.Vertices) { cx += v.X; cy += v.Y; }
            cx /= seedPoly.Vertices.Count; cy /= seedPoly.Vertices.Count;
            Point2D seedCenter = new Point2D(cx, cy);

            // 4. 全网格生成：覆盖整个外边框的包围盒
            double minU = double.MaxValue, maxU = double.MinValue;
            double minV = double.MaxValue, maxV = double.MinValue;
            foreach (var v in boundary.Vertices)
            {
                double du = (v.X - cx) * dirLongX + (v.Y - cy) * dirLongY;
                double dv = (v.X - cx) * dirShortX + (v.Y - cy) * dirShortY;
                if (du < minU) minU = du; if (du > maxU) maxU = du;
                if (dv < minV) minV = dv; if (dv > maxV) maxV = dv;
            }

            double hL = longSide / 2;
            double hS = shortSide / 2;
            int uiMin = (int)Math.Floor((minU - hL) / longSide);
            int uiMax = (int)Math.Ceiling((maxU + hL) / longSide);
            int viMin = (int)Math.Floor((minV - hS) / shortSide);
            int viMax = (int)Math.Ceiling((maxV + hS) / shortSide);

            ed.WriteMessage($"\n种子 {longSide:F2} x {shortSide:F2}, 网格 {uiMax - uiMin + 1}x{viMax - viMin + 1}");
            ed.WriteMessage($"\n正在生成种子...");

            int count = 0;
            var boundarySegments = new List<(Point2D, Point2D)>();

            // 读取边界 Polyline（用于 AutoCAD 原生 IntersectWith 检测）
            Polyline boundaryPline = null;
            using (var tr = _db.TransactionManager.StartTransaction())
            {
                boundaryPline = tr.GetObject(boundaryId, OpenMode.ForRead) as Polyline;
                tr.Commit();
            }
            bool hasBoundaryPline = (boundaryPline != null);
            int cutTiles = 0; // 裁切瓷砖数（按瓷砖计数，非线段计数）

            for (int ui = uiMin; ui <= uiMax; ui++)
            {
                for (int vi = viMin; vi <= viMax; vi++)
                {
                    count++;
                    if (count % 500 == 0) ed.WriteMessage(".");

                    double u0 = -hL + ui * longSide;
                    double u1 = -hL + (ui + 1) * longSide;
                    double v0 = -hS + vi * shortSide;
                    double v1 = -hS + (vi + 1) * shortSide;

                    Point2D[] corners = new Point2D[4];
                    corners[0] = LocalToWorld(u0, v0, seedCenter, dirLongX, dirLongY, dirShortX, dirShortY);
                    corners[1] = LocalToWorld(u1, v0, seedCenter, dirLongX, dirLongY, dirShortX, dirShortY);
                    corners[2] = LocalToWorld(u1, v1, seedCenter, dirLongX, dirLongY, dirShortX, dirShortY);
                    corners[3] = LocalToWorld(u0, v1, seedCenter, dirLongX, dirLongY, dirShortX, dirShortY);

                    // 用 AutoCAD 原生 IntersectWith 检测
                    bool anyInside = false, allInside = true;
                    if (hasBoundaryPline)
                    {
                        foreach (var c in corners)
                        {
                            bool inside = IsPointInClosedCurve(new Point3d(c.X, c.Y, 0), boundaryPline);
                            if (inside) anyInside = true; else allInside = false;
                        }
                        if (!anyInside)
                        {
                            Line testLine = new Line(
                                new Point3d(corners[0].X, corners[0].Y, 0),
                                new Point3d(corners[2].X, corners[2].Y, 0));
                            Point3dCollection pts = new Point3dCollection();
                            boundaryPline.IntersectWith(testLine, Intersect.OnBothOperands, pts, IntPtr.Zero, IntPtr.Zero);
                            testLine.Dispose();
                            anyInside = (pts.Count > 0);
                        }
                    }
                    else
                    {
                        foreach (var c in corners)
                        {
                            bool inside = PointInPolygon(c, boundary);
                            if (inside) anyInside = true; else allInside = false;
                        }
                    }
                    if (!anyInside) continue;

                    if (allInside)
                    {
                        // 完全在边界内部 → 闭合多段线（uncut）
                        Polygon tile = new Polygon { Vertices = new List<Point2D>(corners) };
                        results.Add(tile);
                        stats.UncutCount++;
                        stats.UncutArea += tile.Area;
                    }
                    else if (hasBoundaryPline)
                    {
                        // 与边界相交 → 用 IntersectWith 裁剪每条边，结果存为 LINE（cut）
                        int segsBefore = boundarySegments.Count;
                        for (int ei = 0; ei < 4; ei++)
                        {
                            Point2D ea = corners[ei];
                            Point2D eb = corners[(ei + 1) % 4];
                            Line edgeLine = new Line(
                                new Point3d(ea.X, ea.Y, 0), new Point3d(eb.X, eb.Y, 0));

                            Point3dCollection hits = new Point3dCollection();
                            boundaryPline.IntersectWith(edgeLine, Intersect.OnBothOperands, hits, IntPtr.Zero, IntPtr.Zero);
                            bool aIn = IsPointInClosedCurve(edgeLine.StartPoint, boundaryPline);
                            bool bIn = IsPointInClosedCurve(edgeLine.EndPoint, boundaryPline);

                            if (aIn && bIn) boundarySegments.Add((ea, eb));
                            else if (aIn && !bIn)
                            {
                                if (hits.Count > 0)
                                {
                                    double minT = double.MaxValue; int idx = 0;
                                    for (int h = 0; h < hits.Count; h++)
                                    { double t = edgeLine.GetParameterAtPoint(hits[h]); if (t < minT) { minT = t; idx = h; } }
                                    boundarySegments.Add((ea, new Point2D(hits[idx].X, hits[idx].Y)));
                                }
                            }
                            else if (!aIn && bIn)
                            {
                                if (hits.Count > 0)
                                {
                                    double maxT = -1; int idx = 0;
                                    for (int h = 0; h < hits.Count; h++)
                                    { double t = edgeLine.GetParameterAtPoint(hits[h]); if (t > maxT) { maxT = t; idx = h; } }
                                    boundarySegments.Add((new Point2D(hits[idx].X, hits[idx].Y), eb));
                                }
                            }
                            else if (hits.Count >= 2)
                            {
                                var sorted = new List<(double t, Point3d pt)>();
                                foreach (Point3d hp in hits) sorted.Add((edgeLine.GetParameterAtPoint(hp), hp));
                                sorted.Sort((a, b) => a.t.CompareTo(b.t));
                                boundarySegments.Add((
                                    new Point2D(sorted[0].pt.X, sorted[0].pt.Y),
                                    new Point2D(sorted[sorted.Count - 1].pt.X, sorted[sorted.Count - 1].pt.Y)));
                            }
                            edgeLine.Dispose();
                        }
                        // 该瓷砖至少产生一条裁剪线段 → 计为一个裁切种子
                        if (boundarySegments.Count > segsBefore)
                        {
                            cutTiles++;
                        }
                    }
                }
            }

            // 写入图形（先清空旧结果）
            ClearResultLayer();
            int cutLineCount = WritePolygonsToDrawing(results) + WriteSegmentsToDrawing(boundarySegments);
            stats.CutCount = cutTiles;
            stats.CutArea = cutTiles * (longSide * shortSide);

            ed.WriteMessage($"\n完成！未裁剪={stats.UncutCount} 裁切={stats.CutCount} 总线段={cutLineCount}");
            return stats;
        }

        /// <summary>
        /// Seed2：创建线网格，使用 ClippingService.Clip 进行裁剪
        /// 同时为内部完整网格单元格创建闭合多段线
        /// </summary>
        public int Execute2()
        {
            ClearResultLayer();
            Document doc = Application.DocumentManager.MdiActiveDocument;
            Editor ed = doc.Editor;

            // 1. 选择边界
            PromptEntityOptions peo = new PromptEntityOptions("\n请选择边界多段线（图层：边界）");
            peo.SetRejectMessage("请选择闭合多段线");
            peo.AddAllowedClass(typeof(Polyline), true);
            PromptEntityResult per = ed.GetEntity(peo);
            if (per.Status != PromptStatus.OK) return 0;

            ObjectId boundaryId = per.ObjectId;
            Polygon boundary = PolylineToPolygon(boundaryId);
            if (boundary == null) { ed.WriteMessage("\n边界无效！"); return 0; }
            boundary.EnsureCCW();

            // 2. 选择种子
            peo.Message = "\n请选择种子矩形（图层：种子）";
            per = ed.GetEntity(peo);
            if (per.Status != PromptStatus.OK) return 0;

            ObjectId seedId = per.ObjectId;
            Polygon seedPoly = PolylineToPolygon(seedId);
            if (seedPoly == null) { ed.WriteMessage("\n种子无效！"); return 0; }

            // 3. 提取种子参数
            double seedWidth, seedHeight;
            double angle;
            ExtractSeedParams(seedPoly, out seedWidth, out seedHeight, out angle);

            if (seedWidth < 1e-6 || seedHeight < 1e-6)
            { ed.WriteMessage("\n种子尺寸无效！"); return 0; }

            ed.WriteMessage($"\n种子尺寸: {seedWidth:F2} x {seedHeight:F2}, 旋转角: {angle * 180 / Math.PI:F1}°");

            double cosA = Math.Cos(angle);
            double sinA = Math.Sin(angle);

            double longSide, shortSide;
            double dirLongX, dirLongY;
            double dirShortX, dirShortY;
            if (seedWidth >= seedHeight)
            {
                longSide = seedWidth; shortSide = seedHeight;
                dirLongX = cosA; dirLongY = sinA;
                dirShortX = -sinA; dirShortY = cosA;
            }
            else
            {
                longSide = seedHeight; shortSide = seedWidth;
                dirShortX = cosA; dirShortY = sinA;
                dirLongX = -sinA; dirLongY = cosA;
            }

            // 种子中心
            double cx = 0, cy = 0;
            foreach (var v in seedPoly.Vertices) { cx += v.X; cy += v.Y; }
            cx /= seedPoly.Vertices.Count; cy /= seedPoly.Vertices.Count;
            Point2D seedCenter = new Point2D(cx, cy);

            // 计算边界包围盒（在种子局部坐标系中）
            double minU = double.MaxValue, maxU = double.MinValue;
            double minV = double.MaxValue, maxV = double.MinValue;
            foreach (var v in boundary.Vertices)
            {
                double du = (v.X - cx) * dirLongX + (v.Y - cy) * dirLongY;
                double dv = (v.X - cx) * dirShortX + (v.Y - cy) * dirShortY;
                if (du < minU) minU = du; if (du > maxU) maxU = du;
                if (dv < minV) minV = dv; if (dv > maxV) maxV = dv;
            }

            double hL = longSide / 2;
            double hS = shortSide / 2;
            int uiMin = (int)Math.Floor((minU - hL) / longSide);
            int uiMax = (int)Math.Ceiling((maxU + hL) / longSide);
            int viMin = (int)Math.Floor((minV - hS) / shortSide);
            int viMax = (int)Math.Ceiling((maxV + hS) / shortSide);

            ed.WriteMessage($"\n网格 {uiMax - uiMin + 1}x{viMax - viMin + 1}，正在生成线网格...");

            // 收集线段（局部坐标）
            var hLines = new List<(Point2D a, Point2D b)>(); // 水平方向线（沿长边）
            var vLines = new List<(Point2D a, Point2D b)>(); // 垂直方向线（沿短边）
            var interiorCells = new List<List<Point2D>>();    // 内部完整单元格（闭合多段线）

            // 读取边界 Polyline
            Polyline boundaryPline = null;
            using (var tr = _db.TransactionManager.StartTransaction())
            {
                boundaryPline = tr.GetObject(boundaryId, OpenMode.ForRead) as Polyline;
                tr.Commit();
            }
            bool hasBoundaryPline = (boundaryPline != null);

            int lineCount = 0;

            // 生成水平网格线（沿长边方向）
            for (int vi = viMin; vi <= viMax; vi++)
            {
                double v = -hS + vi * shortSide;
                Point2D p0 = LocalToWorld(minU - hL, v, seedCenter, dirLongX, dirLongY, dirShortX, dirShortY);
                Point2D p1 = LocalToWorld(maxU + hL, v, seedCenter, dirLongX, dirLongY, dirShortX, dirShortY);

                if (hasBoundaryPline)
                {
                    Line fullLine = new Line(
                        new Point3d(p0.X, p0.Y, 0),
                        new Point3d(p1.X, p1.Y, 0));
                    Point3dCollection hits = new Point3dCollection();
                    boundaryPline.IntersectWith(fullLine, Intersect.OnBothOperands, hits, IntPtr.Zero, IntPtr.Zero);

                    if (hits.Count >= 2)
                    {
                        var sorted = new List<(double t, Point3d pt)>();
                        foreach (Point3d hp in hits) sorted.Add((fullLine.GetParameterAtPoint(hp), hp));
                        sorted.Sort((a, b) => a.t.CompareTo(b.t));
                        // 取最内段作为裁剪结果
                        hLines.Add((
                            new Point2D(sorted[0].pt.X, sorted[0].pt.Y),
                            new Point2D(sorted[sorted.Count - 1].pt.X, sorted[sorted.Count - 1].pt.Y)));
                        lineCount++;
                    }
                    fullLine.Dispose();
                }
                else
                {
                    // 退化为点检测
                    hLines.Add((p0, p1));
                    lineCount++;
                }
            }

            // 生成垂直网格线（沿短边方向）
            for (int ui = uiMin; ui <= uiMax; ui++)
            {
                double u = -hL + ui * longSide;
                Point2D p0 = LocalToWorld(u, minV - hS, seedCenter, dirLongX, dirLongY, dirShortX, dirShortY);
                Point2D p1 = LocalToWorld(u, maxV + hS, seedCenter, dirLongX, dirLongY, dirShortX, dirShortY);

                if (hasBoundaryPline)
                {
                    Line fullLine = new Line(
                        new Point3d(p0.X, p0.Y, 0),
                        new Point3d(p1.X, p1.Y, 0));
                    Point3dCollection hits = new Point3dCollection();
                    boundaryPline.IntersectWith(fullLine, Intersect.OnBothOperands, hits, IntPtr.Zero, IntPtr.Zero);

                    if (hits.Count >= 2)
                    {
                        var sorted = new List<(double t, Point3d pt)>();
                        foreach (Point3d hp in hits) sorted.Add((fullLine.GetParameterAtPoint(hp), hp));
                        sorted.Sort((a, b) => a.t.CompareTo(b.t));
                        vLines.Add((
                            new Point2D(sorted[0].pt.X, sorted[0].pt.Y),
                            new Point2D(sorted[sorted.Count - 1].pt.X, sorted[sorted.Count - 1].pt.Y)));
                        lineCount++;
                    }
                    fullLine.Dispose();
                }
                else
                {
                    vLines.Add((p0, p1));
                    lineCount++;
                }
            }

            // 检测内部完整网格单元格 → 生成闭合多段线
            for (int ui = uiMin; ui < uiMax; ui++)
            {
                for (int vi = viMin; vi < viMax; vi++)
                {
                    double u0 = -hL + ui * longSide;
                    double u1 = -hL + (ui + 1) * longSide;
                    double v0 = -hS + vi * shortSide;
                    double v1 = -hS + (vi + 1) * shortSide;

                    Point2D[] corners = new Point2D[4];
                    corners[0] = LocalToWorld(u0, v0, seedCenter, dirLongX, dirLongY, dirShortX, dirShortY);
                    corners[1] = LocalToWorld(u1, v0, seedCenter, dirLongX, dirLongY, dirShortX, dirShortY);
                    corners[2] = LocalToWorld(u1, v1, seedCenter, dirLongX, dirLongY, dirShortX, dirShortY);
                    corners[3] = LocalToWorld(u0, v1, seedCenter, dirLongX, dirLongY, dirShortX, dirShortY);

                    // 判断单元格是否完全在边界内
                    bool allInside = true;
                    if (hasBoundaryPline)
                    {
                        foreach (var c in corners)
                        {
                            if (!IsPointInClosedCurve(new Point3d(c.X, c.Y, 0), boundaryPline))
                            { allInside = false; break; }
                        }
                    }
                    else
                    {
                        foreach (var c in corners)
                        {
                            if (!PointInPolygon(c, boundary))
                            { allInside = false; break; }
                        }
                    }

                    if (allInside)
                    {
                        interiorCells.Add(new List<Point2D>(corners));
                    }
                }
            }

            // 写入图形
            int totalLines = WriteLinesToDrawing(hLines, vLines);
            int totalCells = WriteInteriorCellsToDrawing(interiorCells);

            ed.WriteMessage($"\nSeed2 完成！线段={totalLines} 条，内部单元格={totalCells} 个");

            return totalLines + totalCells;
        }

        /// <summary>
        /// 统计种子填充结果：闭合多段线=uncut，LINE=cut
        /// </summary>
        public SeedStatistics GetStatistics(ObjectId boundaryId)
        {
            SeedStatistics stats = new SeedStatistics();

            // 读取边界 Polygon 用于空间过滤
            Polygon boundary = PolylineToPolygon(boundaryId);
            if (boundary == null) return stats;
            boundary.EnsureCCW();

            using (Transaction tr = _db.TransactionManager.StartTransaction())
            {
                BlockTable bt = tr.GetObject(_db.BlockTableId, OpenMode.ForRead) as BlockTable;
                BlockTableRecord ms = tr.GetObject(bt[BlockTableRecord.ModelSpace], OpenMode.ForRead) as BlockTableRecord;

                foreach (ObjectId id in ms)
                {
                    DBObject obj = tr.GetObject(id, OpenMode.ForRead);
                    Entity e = obj as Entity;
                    if (e == null || e.Layer != ResultLayer) continue;

                    if (obj is Polyline pline && pline.Closed)
                    {
                        // 取中心点检查是否在边界内
                        double cx = 0, cy = 0;
                        for (int i = 0; i < pline.NumberOfVertices; i++)
                        {
                            var pt = pline.GetPoint3dAt(i);
                            cx += pt.X; cy += pt.Y;
                        }
                        cx /= pline.NumberOfVertices; cy /= pline.NumberOfVertices;
                        if (!PointInPolygon(new Point2D(cx, cy), boundary))
                            continue;

                        stats.UncutCount++;
                        stats.UncutArea += pline.Area;
                    }
                    else if (obj is Line line)
                    {
                        // 取中点检查是否在边界内
                        Point3d mid = new Point3d(
                            (line.StartPoint.X + line.EndPoint.X) / 2,
                            (line.StartPoint.Y + line.EndPoint.Y) / 2, 0);
                        if (!PointInPolygon(new Point2D(mid.X, mid.Y), boundary))
                            continue;

                        stats.CutCount++;
                        stats.CutArea += line.Length;
                    }
                }
                tr.Commit();
            }

            return stats;
        }

        // ==================== 辅助方法 ====================

        /// <summary>
        /// 将 AutoCAD Polyline 转换为模型 Polygon
        /// </summary>
        private Polygon PolylineToPolygon(ObjectId plineId)
        {
            using (Transaction tr = _db.TransactionManager.StartTransaction())
            {
                Polyline pline = tr.GetObject(plineId, OpenMode.ForRead) as Polyline;
                if (pline == null) return null;

                Polygon poly = new Polygon();
                for (int i = 0; i < pline.NumberOfVertices; i++)
                {
                    Point2d pt = pline.GetPoint2dAt(i);
                    poly.Vertices.Add(new Point2D(pt.X, pt.Y));
                }
                tr.Commit();
                return poly;
            }
        }

        /// <summary>
        /// 提取种子矩形的宽度、高度、旋转角
        /// </summary>
        private void ExtractSeedParams(Polygon seedPoly, out double width, out double height, out double angle)
        {
            if (seedPoly.Vertices.Count < 4)
            {
                width = height = angle = 0;
                return;
            }

            // 计算四条边的长度和方向
            double d01 = (seedPoly.Vertices[0] - seedPoly.Vertices[1]).Length;
            double d12 = (seedPoly.Vertices[1] - seedPoly.Vertices[2]).Length;
            double d23 = (seedPoly.Vertices[2] - seedPoly.Vertices[3]).Length;
            double d30 = (seedPoly.Vertices[3] - seedPoly.Vertices[0]).Length;

            // 两条长边和两条短边的平均
            double e1 = (d01 + d23) / 2;
            double e2 = (d12 + d30) / 2;

            width = Math.Max(e1, e2);
            height = Math.Min(e1, e2);

            // 取第一条边的方向角
            Point2D dir = seedPoly.Vertices[1] - seedPoly.Vertices[0];
            angle = Math.Atan2(dir.Y, dir.X);
        }

        /// <summary>
        /// 将局部坐标 (u,v) 转换为世界坐标
        /// </summary>
        private Point2D LocalToWorld(double u, double v, Point2D center,
            double dirLongX, double dirLongY, double dirShortX, double dirShortY)
        {
            double x = center.X + u * dirLongX + v * dirShortX;
            double y = center.Y + u * dirLongY + v * dirShortY;
            return new Point2D(x, y);
        }

        /// <summary>
        /// 使用 AutoCAD IntersectWith 检测点是否在闭合曲线内（射线法）
        /// </summary>
        private bool IsPointInClosedCurve(Point3d point, Polyline pline)
        {
            if (pline == null) return false;

            try
            {
                // 构造一条从点向右的射线
                Point3d rayEnd = new Point3d(point.X + 1e10, point.Y, 0);
                Line ray = new Line(point, rayEnd);
                Point3dCollection hits = new Point3dCollection();
                pline.IntersectWith(ray, Intersect.OnBothOperands, hits, IntPtr.Zero, IntPtr.Zero);
                ray.Dispose();

                // 奇数个交点=内部
                return (hits.Count % 2) == 1;
            }
            catch
            {
                return false;
            }
        }

        /// <summary>
        /// 射线法判断点是否在多边形内（纯算法，不依赖 AutoCAD）
        /// </summary>
        private bool PointInPolygon(Point2D point, Polygon polygon)
        {
            bool inside = false;
            int n = polygon.Vertices.Count;
            for (int i = 0, j = n - 1; i < n; j = i++)
            {
                Point2D vi = polygon.Vertices[i];
                Point2D vj = polygon.Vertices[j];
                if ((vi.Y > point.Y) != (vj.Y > point.Y) &&
                    point.X < (vj.X - vi.X) * (point.Y - vi.Y) / (vj.Y - vi.Y) + vi.X)
                {
                    inside = !inside;
                }
            }
            return inside;
        }

        /// <summary>
        /// 将内部 Polygon 列表写入图形（闭合多段线）
        /// </summary>
        private void ClearResultLayer()
        {
            using (Transaction tr = _db.TransactionManager.StartTransaction())
            {
                BlockTable bt = tr.GetObject(_db.BlockTableId, OpenMode.ForRead) as BlockTable;
                BlockTableRecord ms = tr.GetObject(bt[BlockTableRecord.ModelSpace], OpenMode.ForWrite) as BlockTableRecord;
                var ids = new System.Collections.Generic.List<ObjectId>();
                foreach (ObjectId id in ms)
                {
                    DBObject obj = tr.GetObject(id, OpenMode.ForRead);
                    Entity e = obj as Entity;
                    if (e != null && e.Layer == ResultLayer) ids.Add(id);
                }
                foreach (var id in ids)
                {
                    DBObject obj = tr.GetObject(id, OpenMode.ForWrite);
                    obj.Erase();
                }
                tr.Commit();
            }
        }

        private int WritePolygonsToDrawing(List<Polygon> polygons)
        {
            int count = 0;
            using (Transaction tr = _db.TransactionManager.StartTransaction())
            {
                BlockTable bt = tr.GetObject(_db.BlockTableId, OpenMode.ForRead) as BlockTable;
                BlockTableRecord ms = tr.GetObject(bt[BlockTableRecord.ModelSpace], OpenMode.ForWrite) as BlockTableRecord;

                // 确保图层存在
                LayerTable lt = tr.GetObject(_db.LayerTableId, OpenMode.ForRead) as LayerTable;
                if (!lt.Has(ResultLayer))
                {
                    lt.UpgradeOpen();
                    LayerTableRecord ltr = new LayerTableRecord();
                    ltr.Name = ResultLayer;
                    lt.Add(ltr);
                    tr.AddNewlyCreatedDBObject(ltr, true);
                }

                foreach (var poly in polygons)
                {
                    if (poly.Vertices.Count < 3) continue;

                    Polyline pline = new Polyline();
                    for (int i = 0; i < poly.Vertices.Count; i++)
                    {
                        pline.AddVertexAt(i, new Point2d(poly.Vertices[i].X, poly.Vertices[i].Y), 0, 0, 0);
                    }
                    pline.Closed = true;
                    pline.Layer = ResultLayer;

                    ms.AppendEntity(pline);
                    tr.AddNewlyCreatedDBObject(pline, true);
                    count++;
                }
                tr.Commit();
            }
            return count;
        }

        /// <summary>
        /// 将边界线段列表写入图形（LINE）
        /// </summary>
        private int WriteSegmentsToDrawing(List<(Point2D, Point2D)> segments)
        {
            int count = 0;
            using (Transaction tr = _db.TransactionManager.StartTransaction())
            {
                BlockTable bt = tr.GetObject(_db.BlockTableId, OpenMode.ForRead) as BlockTable;
                BlockTableRecord ms = tr.GetObject(bt[BlockTableRecord.ModelSpace], OpenMode.ForWrite) as BlockTableRecord;

                // 确保图层存在
                LayerTable lt = tr.GetObject(_db.LayerTableId, OpenMode.ForRead) as LayerTable;
                if (!lt.Has(ResultLayer))
                {
                    lt.UpgradeOpen();
                    LayerTableRecord ltr = new LayerTableRecord();
                    ltr.Name = ResultLayer;
                    lt.Add(ltr);
                    tr.AddNewlyCreatedDBObject(ltr, true);
                }

                foreach (var seg in segments)
                {
                    Line line = new Line(
                        new Point3d(seg.Item1.X, seg.Item1.Y, 0),
                        new Point3d(seg.Item2.X, seg.Item2.Y, 0));
                    line.Layer = ResultLayer;

                    ms.AppendEntity(line);
                    tr.AddNewlyCreatedDBObject(line, true);
                    count++;
                }
                tr.Commit();
            }
            return count;
        }

        /// <summary>
        /// 将水平/垂直网格线写入图形（LINE）
        /// </summary>
        private int WriteLinesToDrawing(
            List<(Point2D a, Point2D b)> hLines,
            List<(Point2D a, Point2D b)> vLines)
        {
            int count = 0;
            using (Transaction tr = _db.TransactionManager.StartTransaction())
            {
                BlockTable bt = tr.GetObject(_db.BlockTableId, OpenMode.ForRead) as BlockTable;
                BlockTableRecord ms = tr.GetObject(bt[BlockTableRecord.ModelSpace], OpenMode.ForWrite) as BlockTableRecord;

                LayerTable lt = tr.GetObject(_db.LayerTableId, OpenMode.ForRead) as LayerTable;
                if (!lt.Has(ResultLayer))
                {
                    lt.UpgradeOpen();
                    LayerTableRecord ltr = new LayerTableRecord();
                    ltr.Name = ResultLayer;
                    lt.Add(ltr);
                    tr.AddNewlyCreatedDBObject(ltr, true);
                }

                foreach (var seg in hLines)
                {
                    Line line = new Line(
                        new Point3d(seg.a.X, seg.a.Y, 0),
                        new Point3d(seg.b.X, seg.b.Y, 0));
                    line.Layer = ResultLayer;
                    ms.AppendEntity(line);
                    tr.AddNewlyCreatedDBObject(line, true);
                    count++;
                }
                foreach (var seg in vLines)
                {
                    Line line = new Line(
                        new Point3d(seg.a.X, seg.a.Y, 0),
                        new Point3d(seg.b.X, seg.b.Y, 0));
                    line.Layer = ResultLayer;
                    ms.AppendEntity(line);
                    tr.AddNewlyCreatedDBObject(line, true);
                    count++;
                }
                tr.Commit();
            }
            return count;
        }

        /// <summary>
        /// 将内部完整网格单元格写入图形（闭合多段线）
        /// </summary>
        private int WriteInteriorCellsToDrawing(List<List<Point2D>> cells)
        {
            int count = 0;
            using (Transaction tr = _db.TransactionManager.StartTransaction())
            {
                BlockTable bt = tr.GetObject(_db.BlockTableId, OpenMode.ForRead) as BlockTable;
                BlockTableRecord ms = tr.GetObject(bt[BlockTableRecord.ModelSpace], OpenMode.ForWrite) as BlockTableRecord;

                LayerTable lt = tr.GetObject(_db.LayerTableId, OpenMode.ForRead) as LayerTable;
                if (!lt.Has(ResultLayer))
                {
                    lt.UpgradeOpen();
                    LayerTableRecord ltr = new LayerTableRecord();
                    ltr.Name = ResultLayer;
                    lt.Add(ltr);
                    tr.AddNewlyCreatedDBObject(ltr, true);
                }

                foreach (var verts in cells)
                {
                    if (verts.Count < 3) continue;
                    Polyline pline = new Polyline();
                    for (int i = 0; i < verts.Count; i++)
                    {
                        pline.AddVertexAt(i, new Point2d(verts[i].X, verts[i].Y), 0, 0, 0);
                    }
                    pline.Closed = true;
                    pline.Layer = ResultLayer;
                    ms.AppendEntity(pline);
                    tr.AddNewlyCreatedDBObject(pline, true);
                    count++;
                }
                tr.Commit();
            }
            return count;
        }
    }
}
