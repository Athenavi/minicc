using System;
using System.Collections.Generic;
using Autodesk.Revit.DB;

namespace AxisGrid.Services
{
    /// <summary>
    /// 求交服务：计算详图线之间的交点
    /// 支持 Line-Line, Line-Arc, Arc-Arc
    /// </summary>
    public static class IntersectionService
    {
        private const double Tolerance = 0.01; // 约 3mm

        /// <summary>
        /// 计算两条曲线的所有交点
        /// 支持 Line-Line 手动计算作为备选（防止 Curve.Intersect 在 Revit 2026 中失效）
        /// </summary>
        public static List<XYZ> FindIntersections(Curve curve1, Curve curve2)
        {
            var results = new List<XYZ>();

            // 方法1：Revit 原生求交
            try
            {
                IntersectionResultArray arr = null;
                SetComparisonResult result = curve1.Intersect(curve2, out arr);
                if (result == SetComparisonResult.Overlap && arr != null && arr.Size > 0)
                {
                    foreach (IntersectionResult ir in arr)
                        results.Add(ir.XYZPoint);
                    if (results.Count > 0) return results;
                }
            }
            catch { }

            // 方法2：手动 Line-Line 求交（备选）
            try
            {
                if (curve1 is Line line1 && curve2 is Line line2)
                {
                    XYZ p1 = line1.GetEndPoint(0), p2 = line1.GetEndPoint(1);
                    XYZ p3 = line2.GetEndPoint(0), p4 = line2.GetEndPoint(1);

                    XYZ result = LineLineIntersect(p1, p2, p3, p4);
                    if (result != null)
                    {
                        // 验证交点在两条线段上
                        if (IsPointOnSegment(result, p1, p2) && IsPointOnSegment(result, p3, p4))
                            results.Add(result);
                    }
                }
            }
            catch { }

            return results;
        }

        /// <summary>
        /// 手动计算两条直线的交点（延伸无限长）
        /// </summary>
        private static XYZ LineLineIntersect(XYZ p1, XYZ p2, XYZ p3, XYZ p4)
        {
            double d1x = p2.X - p1.X, d1y = p2.Y - p1.Y;
            double d2x = p4.X - p3.X, d2y = p4.Y - p3.Y;
            double d3x = p3.X - p1.X, d3y = p3.Y - p1.Y;

            double det = d1x * d2y - d1y * d2x;
            if (Math.Abs(det) < 1e-10) return null;

            double t = (d3x * d2y - d3y * d2x) / det;
            return new XYZ(p1.X + t * d1x, p1.Y + t * d1y, 0);
        }

        /// <summary>
        /// 判断点是否在线段上（含端点容差）
        /// </summary>
        private static bool IsPointOnSegment(XYZ pt, XYZ segStart, XYZ segEnd, double tolerance = 0.01)
        {
            double len = segStart.DistanceTo(segEnd);
            if (len < 1e-8) return pt.DistanceTo(segStart) < tolerance;
            double d1 = pt.DistanceTo(segStart);
            double d2 = pt.DistanceTo(segEnd);
            return Math.Abs(d1 + d2 - len) < tolerance;
        }

        /// <summary>
        /// 计算起点到终点的方向向量（替代 - 运算符）
        /// </summary>
        private static XYZ Subtract(XYZ a, XYZ b)
        {
            return new XYZ(a.X - b.X, a.Y - b.Y, a.Z - b.Z);
        }

        /// <summary>
        /// 计算曲线与点的关系：点是否在曲线上（含端点）
        /// </summary>
        public static bool IsPointOnCurve(Curve curve, XYZ point, double tolerance = Tolerance)
        {
            try
            {
                XYZ proj = curve.Project(point).XYZPoint;
                return proj.DistanceTo(point) < tolerance;
            }
            catch
            {
                return false;
            }
        }

        /// <summary>
        /// 判断点是否是曲线的端点
        /// </summary>
        public static bool IsEndpoint(Curve curve, XYZ point, double tolerance = Tolerance)
        {
            return point.DistanceTo(curve.GetEndPoint(0)) < tolerance
                || point.DistanceTo(curve.GetEndPoint(1)) < tolerance;
        }

        /// <summary>
        /// 获取曲线的参数化位置 t（0=起点, 1=终点）
        /// 使用多种方法确保精度
        /// </summary>
        public static double GetParameter(Curve curve, XYZ point)
        {
            try
            {
                // 方法1：Project
                IntersectionResult ir = curve.Project(point);
                if (ir != null) return ir.Parameter;
            }
            catch { }

            try
            {
                // 方法2：对直线直接用端点计算
                if (curve is Line)
                {
                    XYZ e0 = curve.GetEndPoint(0);
                    XYZ e1 = curve.GetEndPoint(1);
                    double len = e0.DistanceTo(e1);
                    if (len < 1e-8) return 0;
                    double dx = e1.X - e0.X, dy = e1.Y - e0.Y;
                    double px = point.X - e0.X, py = point.Y - e0.Y;
                    double t = (px * dx + py * dy) / (dx * dx + dy * dy);
                    return Math.Max(0, Math.Min(1, t));
                }
                // 方法3：对圆弧用角度比
                if (curve is Arc)
                {
                    // 点在弧上的参数从 Project 获取，这里用距离近似
                    return -1; // 让调用方使用 Project 的结果
                }
            }
            catch { }
            return -1;
        }
    }
}
