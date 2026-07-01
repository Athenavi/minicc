using System;
using System.Collections.Generic;
using Autodesk.Revit.DB;

namespace AxisGrid.Models
{
    /// <summary>
    /// 表示一条轴线段（两个节点之间的连接）
    /// </summary>
    public class AxisEdge
    {
        /// <summary>所属 DetailLine 的 ElementId</summary>
        public ElementId DetailLineId { get; set; }

        /// <summary>起点节点索引</summary>
        public int StartNodeIdx { get; set; }

        /// <summary>终点节点索引</summary>
        public int EndNodeIdx { get; set; }

        /// <summary>是否为圆弧</summary>
        public bool IsArc { get; set; }

        /// <summary>圆弧中心（仅圆弧）</summary>
        public XYZ ArcCenter { get; set; }

        /// <summary>原始 DetailLine 的 Curve 引用</summary>
        public Curve OriginalCurve { get; set; }

        /// <summary>该边是否已删除（标记用）</summary>
        public bool IsDeleted { get; set; }

        public override string ToString()
        {
            return $"Edge({StartNodeIdx}→{EndNodeIdx})";
        }
    }

    /// <summary>
    /// 轴网图数据结构：维护所有轴线段和节点的拓扑关系
    /// </summary>
    public class AxisGraph
    {
        /// <summary>所有节点（位置）</summary>
        public List<XYZ> Nodes { get; set; } = new List<XYZ>();

        /// <summary>所有轴线段</summary>
        public List<AxisEdge> Edges { get; set; } = new List<AxisEdge>();

        /// <summary>节点对应的 FilledRegion ElementId（用于更新）</summary>
        public Dictionary<int, ElementId> NodeRegionIds { get; set; } = new Dictionary<int, ElementId>();

        /// <summary>DetailLine → 对应的 Edge 索引列表</summary>
        public Dictionary<ElementId, List<int>> DetailLineToEdges { get; set; } = new Dictionary<ElementId, List<int>>();

        /// <summary>添加节点（去重）</summary>
        public int AddNode(XYZ point, double tolerance = 0.01)
        {
            for (int i = 0; i < Nodes.Count; i++)
                if (Nodes[i].DistanceTo(point) < tolerance)
                    return i;
            Nodes.Add(point);
            return Nodes.Count - 1;
        }

        /// <summary>添加边</summary>
        public void AddEdge(AxisEdge edge)
        {
            Edges.Add(edge);
            int idx = Edges.Count - 1;
            if (!DetailLineToEdges.ContainsKey(edge.DetailLineId))
                DetailLineToEdges[edge.DetailLineId] = new List<int>();
            DetailLineToEdges[edge.DetailLineId].Add(idx);
        }

        /// <summary>删除指定 DetailLine 的所有边</summary>
        public List<int> RemoveEdgesForDetailLine(ElementId detailLineId)
        {
            var removed = new List<int>();
            if (DetailLineToEdges.TryGetValue(detailLineId, out var indices))
            {
                foreach (int idx in indices)
                {
                    if (idx < Edges.Count)
                    {
                        Edges[idx].IsDeleted = true;
                        removed.Add(idx);
                    }
                }
                DetailLineToEdges.Remove(detailLineId);
            }
            return removed;
        }

        /// <summary>清理已标记删除的边</summary>
        public void PurgeDeletedEdges()
        {
            Edges.RemoveAll(e => e.IsDeleted);
        }

        /// <summary>删除孤立节点（没有边连接的节点）</summary>
        public void RemoveOrphanNodes()
        {
            var used = new bool[Nodes.Count];
            foreach (var edge in Edges)
            {
                if (edge.StartNodeIdx < used.Length) used[edge.StartNodeIdx] = true;
                if (edge.EndNodeIdx < used.Length) used[edge.EndNodeIdx] = true;
            }
            // 重建节点和边索引
            var oldToNew = new int[Nodes.Count];
            var newNodes = new List<XYZ>();
            for (int i = 0; i < Nodes.Count; i++)
            {
                if (used[i])
                {
                    oldToNew[i] = newNodes.Count;
                    newNodes.Add(Nodes[i]);
                }
                else oldToNew[i] = -1;
            }
            // 更新边索引
            foreach (var edge in Edges)
            {
                if (edge.StartNodeIdx < oldToNew.Length)
                    edge.StartNodeIdx = oldToNew[edge.StartNodeIdx];
                if (edge.EndNodeIdx < oldToNew.Length)
                    edge.EndNodeIdx = oldToNew[edge.EndNodeIdx];
            }
            Nodes = newNodes;
        }
    }
}
