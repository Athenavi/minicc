using System;
using System.Collections.Generic;
using System.Linq;
using Autodesk.Revit.DB;
using ClosedRegion.Models;

namespace ClosedRegion.Services
{
    /// <summary>
    /// 曲线预处理：求交、分割、去重、弧线/样条处理
    /// </summary>
    public class CurveProcessor
    {
        /// <summary>浮点容差（英尺），~5mm</summary>
        public const double Tolerance = 0.0164;

        private List<Curve> _curves;
        private Document _doc;

        public CurveProcessor(List<Curve> curves, Document doc)
        {
            _curves = curves;
            _doc = doc;
        }

        /// <summary>
        /// 主入口：处理所有曲线 → 分割后的线段列表
        /// </summary>
        public List<Curve> Process()
        {
            // 1. 去重
            var deduped = RemoveDuplicates(_curves);
            // 2. 查找交点
            var splitPts = FindAllIntersections(deduped);
            // 3. 分割
            var segments = SplitAtPoints(deduped, splitPts);
            // 4. 过滤极小段
            segments = segments.Where(s => s.Length > Tolerance).ToList();
            return segments;
        }

        private List<Curve> RemoveDuplicates(List<Curve> curves)
        {
            var result = new List<Curve>();
            foreach (var c in curves)
            {
                bool dup = false;
                foreach (var r in result)
                {
                    if (CurvesOverlap(c, r)) { dup = true; break; }
                }
                if (!dup) result.Add(c);
            }
            return result;
        }

        private bool CurvesOverlap(Curve a, Curve b)
        {
            // 端点匹配数
            int match = 0;
            if (a.GetEndPoint(0).DistanceTo(b.GetEndPoint(0)) < Tolerance) match++;
            if (a.GetEndPoint(0).DistanceTo(b.GetEndPoint(1)) < Tolerance) match++;
            if (a.GetEndPoint(1).DistanceTo(b.GetEndPoint(0)) < Tolerance) match++;
            if (a.GetEndPoint(1).DistanceTo(b.GetEndPoint(1)) < Tolerance) match++;
            if (match >= 2) return true;
            return false;
        }

        private Dictionary<int, List<XYZ>> FindAllIntersections(List<Curve> curves)
        {
            var result = new Dictionary<int, List<XYZ>>();
            for (int i = 0; i < curves.Count; i++) result[i] = new List<XYZ>();

            for (int i = 0; i < curves.Count; i++)
            {
                for (int j = i + 1; j < curves.Count; j++)
                {
                    // 使用 Revit 2026 API 求交
                    var inter = curves[i].Intersect(curves[j], out IntersectionResultArray pts);
                    if (inter == SetComparisonResult.Overlap && pts != null && pts.Size > 0)
                    {
                        foreach (IntersectionResult ir in pts)
                        {
                            XYZ p = ir.XYZPoint;
                            if (!IsEndpoint(curves[i], p)) result[i].Add(p);
                            if (!IsEndpoint(curves[j], p)) result[j].Add(p);
                        }
                    }
                }
            }
            return result;
        }

        private bool IsEndpoint(Curve c, XYZ p)
        {
            return p.DistanceTo(c.GetEndPoint(0)) < Tolerance ||
                   p.DistanceTo(c.GetEndPoint(1)) < Tolerance;
        }

        /// <summary>
        /// 在交点处分割曲线（保留原始曲线类型：直线/弧线）
        /// </summary>
        private List<Curve> SplitAtPoints(List<Curve> curves, Dictionary<int, List<XYZ>> splitPts)
        {
            var segments = new List<Curve>();

            for (int i = 0; i < curves.Count; i++)
            {
                var pts = splitPts[i]
                    .OrderBy(p => p.DistanceTo(curves[i].GetEndPoint(0)))
                    .ToList();

                if (pts.Count == 0)
                {
                    segments.Add(curves[i]);
                    continue;
                }

                XYZ prev = curves[i].GetEndPoint(0);
                foreach (var pt in pts)
                {
                    if (pt.DistanceTo(prev) > Tolerance)
                        segments.Add(CreateSubCurve(curves[i], prev, pt));
                    prev = pt;
                }
                XYZ end = curves[i].GetEndPoint(1);
                if (prev.DistanceTo(end) > Tolerance)
                    segments.Add(CreateSubCurve(curves[i], prev, end));
            }

            return segments;
        }

        /// <summary>
        /// 创建子曲线（保留原始曲线类型）
        /// </summary>
        private Curve CreateSubCurve(Curve original, XYZ from, XYZ to)
        {
            if (original is Arc)
            {
                // 在弧线上取中点来构建子弧
                double p1 = original.Project(from).Parameter;
                double p2 = original.Project(to).Parameter;
                double pmid = (p1 + p2) / 2.0;
                XYZ mid = original.Evaluate(pmid, true);
                return Arc.Create(from, to, mid);
            }
            if (original is Line)
            {
                return Line.CreateBound(from, to);
            }
            // 样条曲线/其他类型：用直线近似
            return Line.CreateBound(from, to);
        }
    }
}
