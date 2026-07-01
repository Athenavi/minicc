using System;
using System.Collections.Generic;
using System.Linq;
using Autodesk.Revit.DB;
using ClosedRegion.Models;

namespace ClosedRegion.Services
{
    /// <summary>
    /// 封闭区域检测算法
    /// 核心：半边结构最小角遍历找最小面，Union-Find 合并相邻面得到最大面
    /// </summary>
    public class FaceDetector
    {
        private const double Tolerance = 0.0164; // ~5mm
        private List<GraphEdge> _edges;
        private Dictionary<string, GraphVertex> _vertices;

        public (List<ClosedFace> minimalFaces, List<ClosedFace> maximalFaces) Detect(List<Curve> segments)
        {
            BuildGraph(segments);
            var allFaces = FindAllFaces();

            // 过滤外部面（顺时针=外环，逆时针=内部面）
            var interiorFaces = allFaces.Where(f => !f.IsClockwise).ToList();

            // 最小封闭区域 = 每个内部面
            var minimalFaces = interiorFaces;

            // 最大封闭区域 = 合并相邻面
            var maximalFaces = MergeAdjacent(minimalFaces);

            return (minimalFaces, maximalFaces);
        }

        private void BuildGraph(List<Curve> segments)
        {
            _vertices = new Dictionary<string, GraphVertex>();
            _edges = new List<GraphEdge>();

            foreach (var curve in segments)
            {
                XYZ s = curve.GetEndPoint(0), e = curve.GetEndPoint(1);
                if (s.DistanceTo(e) < Tolerance) continue;
                var sv = GetOrCreateVertex(s);
                var ev = GetOrCreateVertex(e);
                var edge = new GraphEdge { Start = sv, End = ev };
                _edges.Add(edge);
                sv.Edges.Add(edge);
                ev.Edges.Add(edge);
            }
        }

        private GraphVertex GetOrCreateVertex(XYZ pt)
        {
            string key = $"{pt.X:F6},{pt.Y:F6}";
            if (!_vertices.ContainsKey(key))
                _vertices[key] = new GraphVertex { Point = new Point2D(pt.X, pt.Y) };
            return _vertices[key];
        }

        private static XYZ P2X(Point2D p) => new XYZ(p.X, p.Y, 0);

        /// <summary>
        /// 半边遍历查找所有面
        /// </summary>
        private List<ClosedFace> FindAllFaces()
        {
            var faces = new List<ClosedFace>();
            var visited = new HashSet<string>();

            foreach (var edge in _edges)
            {
                foreach (bool dir in new[] { true, false })
                {
                    string startKey = $"{edge.GetHashCode()}_{dir}";
                    if (visited.Contains(startKey)) continue;

                    // 跟踪路径
                    var pathEdges = new List<GraphEdge>();
                    var pathVerts = new List<Point2D>();

                    var ce = edge;
                    bool cd = dir;
                    bool complete = true;

                    for (int step = 0; step < 10000; step++)
                    {
                        string key = $"{ce.GetHashCode()}_{cd}";
                        if (visited.Contains(key)) { complete = false; break; }
                        visited.Add(key);

                        pathEdges.Add(ce);
                        var cv = cd ? ce.End : ce.Start;
                        pathVerts.Add(cv.Point);

                        // 找下一跳：最小逆时针转角
                        var next = FindNext(ce, cd);
                        if (next.edge == null) { complete = false; break; }

                        ce = next.edge;
                        cd = next.direction;

                        if (ce == edge && cd == dir) break; // 回到起点
                    }

                    if (complete && pathEdges.Count >= 3)
                    {
                        double area = ClosedFace.CalcArea(pathVerts);
                        bool cw = SignedArea(pathVerts) < 0;

                        if (area > Tolerance * Tolerance)
                        {
                            faces.Add(new ClosedFace
                            {
                                Edges = pathEdges,
                                Area = area,
                                IsClockwise = cw
                            });
                        }
                    }
                }
            }
            return faces;
        }

        /// <summary>
        /// 鞋带公式计算有符号面积
        /// </summary>
        private static double SignedArea(List<Point2D> pts)
        {
            double sum = 0;
            for (int i = 0; i < pts.Count; i++)
            {
                int j = (i + 1) % pts.Count;
                sum += pts[i].X * pts[j].Y - pts[j].X * pts[i].Y;
            }
            return sum;
        }

        /// <summary>
        /// 最小角选下一跳
        /// </summary>
        private (GraphEdge edge, bool direction) FindNext(GraphEdge incoming, bool incomingDir)
        {
            var vertex = incomingDir ? incoming.End : incoming.Start;
            var fromPt = incomingDir ? incoming.Start.Point : incoming.End.Point;
            var toPt = vertex.Point;
            double inAngle = Math.Atan2(toPt.Y - fromPt.Y, toPt.X - fromPt.X);

            double bestAngle = double.MaxValue;
            GraphEdge bestEdge = null;
            bool bestDir = false;

            foreach (var cand in vertex.Edges)
            {
                if (cand == incoming) continue;
                bool outward = cand.Start == vertex;
                var outPt = outward ? cand.End.Point : cand.Start.Point;
                double outAngle = Math.Atan2(outPt.Y - toPt.Y, outPt.X - toPt.X);
                double rel = outAngle - inAngle;
                if (rel < 0) rel += 2 * Math.PI;
                if (rel <= bestAngle && rel >= -1e-10)
                {
                    bestAngle = rel;
                    bestEdge = cand;
                    bestDir = outward;
                }
            }
            return (bestEdge, bestDir);
        }

        /// <summary>
        /// 更稳健的合并：基于顶点坐标共享判断相邻
        /// </summary>
        private List<ClosedFace> MergeAdjacent(List<ClosedFace> faces)
        {
            if (faces.Count == 0) return new List<ClosedFace>();

            int n = faces.Count;
            int[] parent = new int[n];
            for (int i = 0; i < n; i++) parent[i] = i;

            int Find(int x) { while (parent[x] != x) { parent[x] = parent[parent[x]]; x = parent[x]; } return x; }
            void Union(int a, int b) { int ra = Find(a), rb = Find(b); if (ra != rb) parent[ra] = rb; }

            // 通过顶点坐标比较判断面是否相邻（共享边 = 共享两个顶点）
            var vertToFaces = new Dictionary<string, List<int>>();
            for (int i = 0; i < n; i++)
            {
                var seen = new HashSet<string>();
                foreach (var e in faces[i].Edges)
                {
                    // 每条边的两个端点在面中只计一次
                    string k1 = $"{e.Start.Point.X:F4},{e.Start.Point.Y:F4}";
                    string k2 = $"{e.End.Point.X:F4},{e.End.Point.Y:F4}";
                    if (seen.Add(k1))
                    {
                        if (!vertToFaces.ContainsKey(k1)) vertToFaces[k1] = new List<int>();
                        vertToFaces[k1].Add(i);
                    }
                    if (seen.Add(k2))
                    {
                        if (!vertToFaces.ContainsKey(k2)) vertToFaces[k2] = new List<int>();
                        vertToFaces[k2].Add(i);
                    }
                }
            }

            // 共享2个或更多顶点的面视为相邻（即有公共边）
            for (int i = 0; i < n; i++)
            {
                for (int j = i + 1; j < n; j++)
                {
                    if (Find(i) == Find(j)) continue;

                    // 计算面 i 和面 j 共享的顶点数
                    var vertsI = new HashSet<string>();
                    var vertsJ = new HashSet<string>();
                    foreach (var e in faces[i].Edges)
                    {
                        vertsI.Add($"{e.Start.Point.X:F4},{e.Start.Point.Y:F4}");
                        vertsI.Add($"{e.End.Point.X:F4},{e.End.Point.Y:F4}");
                    }
                    foreach (var e in faces[j].Edges)
                    {
                        vertsJ.Add($"{e.Start.Point.X:F4},{e.Start.Point.Y:F4}");
                        vertsJ.Add($"{e.End.Point.X:F4},{e.End.Point.Y:F4}");
                    }

                    int shared = 0;
                    foreach (var v in vertsI)
                        if (vertsJ.Contains(v)) shared++;

                    if (shared >= 2) // 共享2个顶点 = 共享一条边
                        Union(i, j);
                }
            }

            // 按根节点分组
            var groups = new Dictionary<int, List<int>>();
            for (int i = 0; i < n; i++)
            {
                int root = Find(i);
                if (!groups.ContainsKey(root)) groups[root] = new List<int>();
                groups[root].Add(i);
            }

            // 每组生成合并面：收集边界边
            var merged = new List<ClosedFace>();
            foreach (var kv in groups)
            {
                var idxs = kv.Value;
                if (idxs.Count == 0) continue;

                // 收集组内所有边
                var edgeSet = new HashSet<GraphEdge>();
                double totalArea = 0;
                foreach (int idx in idxs)
                {
                    foreach (var e in faces[idx].Edges)
                        edgeSet.Add(e);
                    totalArea += faces[idx].Area;
                }

                // 边界边 = 不被组内其他面共享的边
                var boundary = new List<GraphEdge>();
                foreach (var e in edgeSet)
                {
                    int cnt = 0;
                    foreach (int idx in idxs)
                        if (faces[idx].Edges.Contains(e)) cnt++;
                    if (cnt == 1) boundary.Add(e);
                }

                if (boundary.Count >= 3)
                {
                    merged.Add(new ClosedFace
                    {
                        Edges = boundary,
                        Area = totalArea,
                        IsClockwise = false
                    });
                }
            }

            return merged;
        }
    }
}
