using System;
using System.Collections.Generic;
using Autodesk.AutoCAD.DatabaseServices;
using Autodesk.AutoCAD.Geometry;
using SeedFill.Models;

namespace SeedFill.Services
{
    /// <summary>
    /// 多边形裁剪 — 使用 AutoCAD Region.IntersectWith
    /// 支持任意凹多边形、圆弧段边界
    /// </summary>
    public static class ClippingService
    {
        /// <summary>
        /// 用边界裁剪种子，返回裁剪后的多边形（null=完全在外）
        /// </summary>
        public static Polygon Clip(Database db, ObjectId boundaryId, Polygon seedPoly)
        {
            using (Transaction tr = db.TransactionManager.StartTransaction())
            {
                // 读取边界多段线
                Polyline boundaryPline = tr.GetObject(boundaryId, OpenMode.ForRead) as Polyline;
                if (boundaryPline == null || !boundaryPline.Closed) return null;

                // 创建种子多段线（从 Polygon 转为 AutoCAD Polyline）
                Polyline seedPline = PolygonToPolyline(seedPoly);
                if (seedPline == null) return null;

                try
                {
                    // 创建 Region
                    DBObjectCollection seedCurves = new DBObjectCollection { seedPline };
                    DBObjectCollection seedRegions = Region.CreateFromCurves(seedCurves);
                    if (seedRegions == null || seedRegions.Count == 0) return null;

                    DBObjectCollection bndCurves = new DBObjectCollection();
                    // 复制边界（避免在原对象上操作）
                    bndCurves.Add(boundaryPline.Clone() as Polyline);
                    DBObjectCollection bndRegions = Region.CreateFromCurves(bndCurves);
                    if (bndRegions == null || bndRegions.Count == 0) return null;

                    Region seedRegion = seedRegions[0] as Region;
                    Region bndRegion = bndRegions[0] as Region;

                    // 执行求交
                    seedRegion.BooleanOperation(BooleanOperationType.BoolIntersect, bndRegion);

                    // 检查结果是否为空
                    if (seedRegion.Area < 1e-12) return null;

                    // 提取结果边界 → 多段线
                    DBObjectCollection exploded = new DBObjectCollection();
                    seedRegion.Explode(exploded);

                    Polygon result = ExplodedToPolygon(exploded);
                    if (result != null) result.IsClipped = true;

                    return result;
                }
                finally
                {
                    // 清理临时对象
                    seedPline.Dispose();
                }
            }
        }

        /// <summary>
        /// 将内部 Polygon 转为 AutoCAD Polyline
        /// </summary>
        private static Polyline PolygonToPolyline(Polygon poly)
        {
            if (poly.Vertices.Count < 3) return null;
            Polyline pline = new Polyline();
            for (int i = 0; i < poly.Vertices.Count; i++)
            {
                pline.AddVertexAt(i, new Point2d(poly.Vertices[i].X, poly.Vertices[i].Y), 0, 0, 0);
            }
            pline.Closed = true;
            return pline;
        }

        /// <summary>
        /// 将 Region.Explode 得到的曲线集合转为 Polygon
        /// </summary>
        private static Polygon ExplodedToPolygon(DBObjectCollection curves)
        {
            // 收集所有端点，按连接顺序排列
            var points = new List<Point2D>();
            if (curves.Count == 0) return null;

            // 取第一条曲线的起点
            Entity first = curves[0] as Entity;
            Point3d startPt = GetStartPoint(first);
            Point3d endPt = GetEndPoint(first);
            points.Add(new Point2D(startPt.X, startPt.Y));
            points.Add(new Point2D(endPt.X, endPt.Y));

            // 依次连接后续曲线
            for (int i = 1; i < curves.Count; i++)
            {
                Entity curve = curves[i] as Entity;
                Point3d cs = GetStartPoint(curve);
                Point3d ce = GetEndPoint(curve);

                Point2D last = points[points.Count - 1];
                double dx = cs.X - last.X;
                double dy = cs.Y - last.Y;
                if (Math.Sqrt(dx * dx + dy * dy) < 1e-8)
                    points.Add(new Point2D(ce.X, ce.Y));
                else
                    points.Add(new Point2D(cs.X, cs.Y));
            }

            // 清理重复的尾点
            while (points.Count > 3)
            {
                double dx = points[0].X - points[points.Count - 1].X;
                double dy = points[0].Y - points[points.Count - 1].Y;
                if (Math.Sqrt(dx * dx + dy * dy) < 1e-8)
                    points.RemoveAt(points.Count - 1);
                else break;
            }

            if (points.Count < 3) return null;

            Polygon poly = new Polygon { Vertices = points };
            poly.EnsureCCW();
            return poly;
        }

        private static Point3d GetStartPoint(Entity ent)
        {
            if (ent is Line line) return line.StartPoint;
            if (ent is Arc arc) return arc.StartPoint;
            if (ent is Polyline pline) return pline.GetPoint3dAt(0);
            return Point3d.Origin;
        }

        private static Point3d GetEndPoint(Entity ent)
        {
            if (ent is Line line) return line.EndPoint;
            if (ent is Arc arc) return arc.EndPoint;
            if (ent is Polyline pline) return pline.GetPoint3dAt(pline.NumberOfVertices - 1);
            return Point3d.Origin;
        }
    }
}
