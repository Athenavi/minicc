using System;
using System.Collections.Generic;
using System.Linq;
using Autodesk.Revit.DB;
using Autodesk.Revit.DB.Architecture;
using AxisGrid.Models;

namespace AxisGrid.Services
{
    /// <summary>
    /// 轴网同步服务：核心算法，将 Revit 中的 DetailLine 集合同步为轴网图
    /// 处理：打断、节点创建、T形/十字形连接
    /// </summary>
    public class GridSyncService
    {
        private readonly Document _doc;
        private readonly ElementId _viewPlanId;
        private const double Tolerance = 0.01; // 约 3mm
        private const string NodeFilledRegionName = "轴网节点";

        // 节点填充区域参数（直径约 8mm 的圆）
        private const double NodeRadiusFeet = 0.013; // ~4mm 半径

        public GridSyncService(Document doc, ViewPlan viewPlan)
        {
            _doc = doc;
            _viewPlanId = viewPlan?.Id;
        }

        public GridSyncService(Document doc)
        {
            _doc = doc;
        }

        /// <summary>
        /// 主入口：从 Revit 文档中读取所有 DetailLine，重建轴网图，更新节点
        /// </summary>
        public AxisGraph RebuildGraph(ViewPlan viewPlan)
        {
            // 1. 收集所有 DetailLine
            var detailLines = CollectDetailLines(viewPlan);
            if (detailLines.Count == 0) return new AxisGraph();

            // 2. 构建轴网图（内存中）
            AxisGraph graph = BuildGraph(detailLines);

            // 3. 在 Revit 中实际打断轴线：删除旧线，创建新线段
            SplitLines(graph, detailLines);

            // 4. 更新节点（FilledRegion）
            UpdateNodes(graph, viewPlan);

            return graph;
        }

        /// <summary>
        /// 在 Revit 文档中实际执行打断：
        /// 1. 删除所有原始 DetailLine
        /// 2. 为 AxisGraph 中的每条 Edge 创建新的 DetailLine
        /// 3. 返回更新后的图（新 ElementId）
        /// </summary>
        private void SplitLines(AxisGraph graph, List<CurveElement> originalLines)
        {
            if (graph.Edges.Count == 0) return;
            if (_viewPlanId == null) return;

            // 获取视图
            View view = _doc.GetElement(_viewPlanId) as View;
            if (view == null) return;

            // 获取默认线样式（通过 LineStyle 参数）
            GraphicsStyle defaultStyle = null;
            foreach (var dl in originalLines)
            {
                try
                {
                    // Revit 2026: CurveElement 没有 GraphicsStyleId 属性
                    // 通过 BuiltInParameter.BUILDING_CURVE_GSTYLE 参数获取
                    Parameter gsParam = dl.get_Parameter(BuiltInParameter.BUILDING_CURVE_GSTYLE);
                    if (gsParam != null)
                    {
                        ElementId gsId = gsParam.AsElementId();
                        if (gsId != null && gsId != ElementId.InvalidElementId)
                            defaultStyle = _doc.GetElement(gsId) as GraphicsStyle;
                    }
                }
                catch { }
                if (defaultStyle != null) break;
            }

            // 删除所有原始 DetailLine
            foreach (var dl in originalLines)
            {
                try { _doc.Delete(dl.Id); } catch { }
            }

            // 为每条 Edge 创建新的 DetailLine
            foreach (var edge in graph.Edges)
            {
                if (edge.IsDeleted) continue;
                if (edge.StartNodeIdx >= graph.Nodes.Count || edge.EndNodeIdx >= graph.Nodes.Count) continue;

                XYZ p1 = graph.Nodes[edge.StartNodeIdx];
                XYZ p2 = graph.Nodes[edge.EndNodeIdx];

                if (p1.DistanceTo(p2) < Tolerance * 2) continue;

                try
                {
                    Curve curve;
                    if (edge.IsArc && edge.OriginalCurve is Arc)
                    {
                        XYZ mid = edge.OriginalCurve.Evaluate(0.5, false);
                        curve = Arc.Create(p1, p2, mid);
                    }
                    else
                    {
                        curve = Line.CreateBound(p1, p2);
                    }

                    // 使用 Doc.Create.NewDetailCurve 创建详图线
                    CurveElement newLine = _doc.Create.NewDetailCurve(view, curve) as CurveElement;
                    if (newLine != null)
                    {
                        edge.DetailLineId = newLine.Id;
                        // 应用线样式
                        if (defaultStyle != null && newLine.get_Parameter(BuiltInParameter.BUILDING_CURVE_GSTYLE) != null)
                        {
                            newLine.get_Parameter(BuiltInParameter.BUILDING_CURVE_GSTYLE).Set(defaultStyle.Id);
                        }
                    }
                }
                catch { }
            }
        }

        /// <summary>
        /// 收集视图中所有 DetailLine（详图线）
        /// Revit 2026 中详图线类别 ID=-2000051，名称为"线"
        /// </summary>
        private List<CurveElement> CollectDetailLines(ViewPlan viewPlan)
        {
            var results = new List<CurveElement>();
            try
            {
                // 方法1：直接收集视图中所有 CurveElement（不依赖类别过滤）
                var collector = new FilteredElementCollector(_doc, viewPlan.Id);
                foreach (CurveElement ce in collector.OfClass(typeof(CurveElement)))
                {
                    // 所有 CurveElement 都接受，由用户自行管理画了什么线
                    results.Add(ce);
                }
            }
            catch { }

            return results;
        }

        /// <summary>
        /// 核心：从 CurveElement 集合构建轴网图
        /// 1. 提取所有端点 + 交点作为节点
        /// 2. 在交点处打断轴线
        /// 3. 每条打断后的线段 = 一条 Edge
        /// </summary>
        private AxisGraph BuildGraph(List<CurveElement> detailLines)
        {
            var graph = new AxisGraph();

            if (detailLines.Count == 0) return graph;

            // Step 1: 计算所有交点
            // 交点类型：
            //   a) 端点（每条线的起点/终点）
            //   b) 线与线的交叉点
            //   c) T 形连接点（端点落在另一条线上）
            var allPoints = new List<(XYZ point, int lineIdx, string role)>();

            for (int i = 0; i < detailLines.Count; i++)
            {
                Curve ci = detailLines[i].GeometryCurve;
                allPoints.Add((ci.GetEndPoint(0), i, "end"));
                allPoints.Add((ci.GetEndPoint(1), i, "end"));

                for (int j = i + 1; j < detailLines.Count; j++)
                {
                    Curve cj = detailLines[j].GeometryCurve;
                    var inters = IntersectionService.FindIntersections(ci, cj);
                    foreach (var pt in inters)
                    {
                        allPoints.Add((pt, i, "cross"));
                        allPoints.Add((pt, j, "cross"));
                    }

                    // 检查端点落在另一条线上（T 形连接）
                    CheckEndpointOnCurve(ci, cj, i, j, allPoints);
                    CheckEndpointOnCurve(cj, ci, j, i, allPoints);
                }
            }

            // Step 2: 按线分组，对每条线按参数位置排序划分
            // 每条线在交点处被打断，形成若干段
            for (int i = 0; i < detailLines.Count; i++)
            {
                Curve curve = detailLines[i].GeometryCurve;
                ElementId lineId = detailLines[i].Id;

                // 收集该线上的所有断点
                var breakPoints = new List<(XYZ point, double param)>();
                foreach (var item in allPoints)
                {
                    if (item.lineIdx == i)
                    {
                        double t = IntersectionService.GetParameter(curve, item.point);
                        if (t < -0.001 || t > 1.001)
                        {
                            if (curve is Line)
                            {
                                XYZ e0 = curve.GetEndPoint(0), e1 = curve.GetEndPoint(1);
                                double dx = e1.X - e0.X, dy = e1.Y - e0.Y;
                                double len2 = dx * dx + dy * dy;
                                if (len2 > 1e-12)
                                {
                                    double px = item.point.X - e0.X, py = item.point.Y - e0.Y;
                                    t = (px * dx + py * dy) / len2;
                                }
                            }
                        }
                        if (t >= -0.001 && t <= 1.001)
                        {
                            // 去重
                            bool dup = false;
                            foreach (var bp in breakPoints)
                                if (bp.point.DistanceTo(item.point) < Tolerance)
                                { dup = true; break; }
                            if (!dup)
                                breakPoints.Add((item.point, Math.Max(0, Math.Min(1, t))));
                        }
                    }
                }

                // 按参数排序
                breakPoints.Sort((a, b) => a.param.CompareTo(b.param));

                if (breakPoints.Count < 2) continue; // 需要至少 2 个点（起点+终点）

                // 每相邻两个断点构成一条 Edge
                for (int k = 0; k < breakPoints.Count - 1; k++)
                {
                    double t1 = breakPoints[k].param;
                    double t2 = breakPoints[k + 1].param;

                    // 跳过长度太小的段
                    if (t2 - t1 < 0.001) continue;

                    try
                    {
                        // 计算该段的起终点
                        XYZ p1 = curve.Evaluate(t1, false);
                        XYZ p2 = curve.Evaluate(t2, false);

                        if (p1.DistanceTo(p2) < Tolerance * 2) continue;

                        // 添加节点
                        int n1 = graph.AddNode(p1);
                        int n2 = graph.AddNode(p2);

                        // 创建子曲线段
                        Curve segment = null;
                        if (curve is Line line)
                        {
                            segment = Line.CreateBound(p1, p2);
                        }
                        else if (curve is Arc arc)
                        {
                            XYZ mid = curve.Evaluate((t1 + t2) / 2, false);
                            segment = Arc.Create(p1, p2, mid);
                        }

                        if (segment != null)
                        {
                            graph.AddEdge(new AxisEdge
                            {
                                DetailLineId = lineId,
                                StartNodeIdx = n1,
                                EndNodeIdx = n2,
                                IsArc = curve is Arc,
                                OriginalCurve = segment
                            });
                        }
                    }
                    catch { }
                }
            }

            // 处理共线情况：如果两条边端点重合且方向相同，合并
            MergeCollinearEdges(graph);

            return graph;
        }

        /// <summary>
        /// 检查曲线 A 的端点是否落在曲线 B 上（T 形连接检测）
        /// </summary>
        private void CheckEndpointOnCurve(Curve curveA, Curve curveB,
            int idxA, int idxB, List<(XYZ, int, string)> points)
        {
            for (int e = 0; e < 2; e++)
            {
                XYZ ep = curveA.GetEndPoint(e);
                // 跳过 B 的端点
                if (ep.DistanceTo(curveB.GetEndPoint(0)) < Tolerance) continue;
                if (ep.DistanceTo(curveB.GetEndPoint(1)) < Tolerance) continue;

                if (IntersectionService.IsPointOnCurve(curveB, ep))
                {
                    points.Add((ep, idxB, "cross"));
                }
            }
        }

        /// <summary>
        /// 合并共线的相邻边：只要两条边共享一个节点且方向相同（共线），就合并
        /// 不检查 DetailLineId（因为 SplitLines 后旧 ID 已失效）
        /// 规则 C：端到端碰在一起形成节点，各自保持独立（不合并）
        /// </summary>
        private void MergeCollinearEdges(AxisGraph graph)
        {
            bool merged = true;
            while (merged)
            {
                merged = false;
                for (int i = 0; i < graph.Edges.Count && !merged; i++)
                {
                    for (int j = i + 1; j < graph.Edges.Count && !merged; j++)
                    {
                        var ei = graph.Edges[i];
                        var ej = graph.Edges[j];
                        if (ei.IsDeleted || ej.IsDeleted) continue;
                        if (ei.IsArc || ej.IsArc) continue;

                        // 检查是否共享一个节点且方向相同
                        if (TryMergeEdges(graph, ei, ej, i, j))
                            merged = true;
                    }
                }
            }
        }

        private bool TryMergeEdges(AxisGraph graph, AxisEdge ei, AxisEdge ej, int i, int j)
        {
            // 共享节点检查
            int sharedNode = -1;
            int eiOther = -1, ejOther = -1;

            if (ei.StartNodeIdx == ej.StartNodeIdx || ei.StartNodeIdx == ej.EndNodeIdx)
            {
                sharedNode = ei.StartNodeIdx;
                eiOther = ei.EndNodeIdx;
            }
            else if (ei.EndNodeIdx == ej.StartNodeIdx || ei.EndNodeIdx == ej.EndNodeIdx)
            {
                sharedNode = ei.EndNodeIdx;
                eiOther = ei.StartNodeIdx;
            }
            if (sharedNode < 0) return false;

            // 确定 ej 的另一端
            ejOther = (ej.StartNodeIdx == sharedNode) ? ej.EndNodeIdx : ej.StartNodeIdx;

            // 检查方向是否相同（共线）
            XYZ dir1 = (graph.Nodes[eiOther] - graph.Nodes[sharedNode]).Normalize();
            XYZ dir2 = (graph.Nodes[ejOther] - graph.Nodes[sharedNode]).Normalize();
            if (dir1.DotProduct(dir2) < 0.999) return false; // 方向不同

            // 合并
            var newEdge = new AxisEdge
            {
                DetailLineId = ei.DetailLineId,
                StartNodeIdx = eiOther,
                EndNodeIdx = ejOther,
                IsArc = false,
                OriginalCurve = Line.CreateBound(graph.Nodes[eiOther], graph.Nodes[ejOther])
            };

            ei.IsDeleted = true;
            ej.IsDeleted = true;
            graph.AddEdge(newEdge);
            return true;
        }

        /// <summary>
        /// 更新节点（创建/更新 FilledRegion）
        /// </summary>
        private void UpdateNodes(AxisGraph graph, ViewPlan viewPlan)
        {
            // 获取或创建节点填充区域类型
            FilledRegionType nodeType = GetOrCreateNodeType();

            // 删除多余的节点
            var toRemove = new List<int>();
            foreach (var kvp in graph.NodeRegionIds)
            {
                if (kvp.Key >= graph.Nodes.Count)
                {
                    toRemove.Add(kvp.Key);
                }
                else
                {
                    // 更新位置
                    try
                    {
                        Element nodeRegion = _doc.GetElement(kvp.Value);
                        if (nodeRegion != null)
                        {
                            // 移动 FilledRegion 到新位置（重新创建更简单）
                            _doc.Delete(kvp.Value);
                        }
                    }
                    catch { }
                    toRemove.Add(kvp.Key);
                }
            }
            foreach (var key in toRemove)
                graph.NodeRegionIds.Remove(key);

            // 为每个节点创建 FilledRegion
            for (int i = 0; i < graph.Nodes.Count; i++)
            {
                if (graph.NodeRegionIds.ContainsKey(i)) continue;

                try
                {
                    // 创建圆形填充区域
                    XYZ center = graph.Nodes[i];
                    double r = NodeRadiusFeet;

                    // 用一段弧近似圆（Revit FilledRegion 需要闭合轮廓）
                    var curveLoop = new CurveLoop();
                    int segments = 12;
                    for (int s = 0; s < segments; s++)
                    {
                        double ang1 = (double)s / segments * 2 * Math.PI;
                        double ang2 = (double)(s + 1) / segments * 2 * Math.PI;
                        XYZ p1 = new XYZ(center.X + r * Math.Cos(ang1),
                                         center.Y + r * Math.Sin(ang1), 0);
                        XYZ p2 = new XYZ(center.X + r * Math.Cos(ang2),
                                         center.Y + r * Math.Sin(ang2), 0);
                        // 计算弧中点
                        double midAng = (ang1 + ang2) / 2;
                        XYZ mid = new XYZ(center.X + r * Math.Cos(midAng),
                                          center.Y + r * Math.Sin(midAng), 0);
                        curveLoop.Append(Arc.Create(p1, p2, mid));
                    }

                    FilledRegion region = FilledRegion.Create(_doc, nodeType.Id, viewPlan.Id, new List<CurveLoop> { curveLoop });
                    graph.NodeRegionIds[i] = region.Id;
                }
                catch { }
            }
        }

        /// <summary>
        /// 获取或创建节点填充区域类型
        /// </summary>
        private FilledRegionType GetOrCreateNodeType()
        {
            // 查找已有的
            var collector = new FilteredElementCollector(_doc);
            var existing = collector
                .OfClass(typeof(FilledRegionType))
                .Cast<FilledRegionType>()
                .FirstOrDefault(fr => fr.Name == NodeFilledRegionName);

            if (existing != null) return existing;

            // 复制默认类型
            var defaultType = collector
                .OfClass(typeof(FilledRegionType))
                .Cast<FilledRegionType>()
                .FirstOrDefault();

            if (defaultType == null)
                throw new InvalidOperationException("没有可用的填充区域类型");

            return defaultType.Duplicate(NodeFilledRegionName) as FilledRegionType;
        }
    }
}
