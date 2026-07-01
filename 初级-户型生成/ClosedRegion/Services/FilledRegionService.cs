using System;
using System.Collections.Generic;
using System.Linq;
using Autodesk.Revit.DB;
using ClosedRegion.Models;

namespace ClosedRegion.Services
{
    /// <summary>
    /// 填充区域生成服务
    /// </summary>
    public class FilledRegionService
    {
        private readonly Document _doc;
        private const string MaxTypeName = "最大区域";
        private const string MinTypeName = "最小区域";

        public FilledRegionService(Document doc) { _doc = doc; }

        public void DeleteExisting(ViewPlan view)
        {
            var collector = new FilteredElementCollector(_doc, view.Id);
            var regions = collector.OfClass(typeof(FilledRegion)).Cast<FilledRegion>().ToList();
            if (regions.Count > 0)
                _doc.Delete(regions.Select(r => r.Id).ToList());
        }

        public List<FilledRegion> CreateMaxRegions(List<ClosedFace> faces, ViewPlan view)
        {
            var type = GetOrCreateType(MaxTypeName);
            var results = new List<FilledRegion>();
            foreach (var f in faces)
            {
                var fr = CreateRegion(f, type, view);
                if (fr != null) results.Add(fr);
            }
            return results;
        }

        public List<FilledRegion> CreateMinRegions(List<ClosedFace> faces, ViewPlan view)
        {
            var type = GetOrCreateType(MinTypeName);
            var results = new List<FilledRegion>();
            foreach (var f in faces)
            {
                var fr = CreateRegion(f, type, view);
                if (fr != null) results.Add(fr);
            }
            return results;
        }

        private FilledRegion CreateRegion(ClosedFace face, FilledRegionType type, ViewPlan view)
        {
            int n = 0;
            foreach (var _ in face.Edges) n++;
            if (face.Edges.Count < 3) return null;

            // 将边排序为连续链
            var sortedEdges = new List<GraphEdge>();
            var remaining = new HashSet<GraphEdge>(face.Edges);
            var current = face.Edges[0];
            sortedEdges.Add(current);
            remaining.Remove(current);
            var currentEnd = current.End;

            while (remaining.Count > 0)
            {
                GraphEdge found = null;
                foreach (var e in remaining)
                {
                    if (e.Start == currentEnd) { found = e; break; }
                    if (e.End == currentEnd)
                    {
                        // 边方向反了，交换 start/end
                        var temp = e.Start;
                        e.Start = e.End;
                        e.End = temp;
                        found = e;
                        break;
                    }
                }
                if (found == null) break;
                sortedEdges.Add(found);
                remaining.Remove(found);
                currentEnd = found.End;
            }

            var curveLoop = new CurveLoop();
            foreach (var e in sortedEdges)
            {
                var start = new XYZ(e.Start.Point.X, e.Start.Point.Y, 0);
                var end = new XYZ(e.End.Point.X, e.End.Point.Y, 0);
                if (start.DistanceTo(end) > 0.001)
                    curveLoop.Append(Line.CreateBound(start, end));
            }

            return FilledRegion.Create(_doc, type.Id, view.Id, new List<CurveLoop> { curveLoop });
        }

        private FilledRegionType GetOrCreateType(string name)
        {
            var collector = new FilteredElementCollector(_doc);
            var existing = collector.OfClass(typeof(FilledRegionType))
                .Cast<FilledRegionType>().FirstOrDefault(t => t.Name == name);
            if (existing != null) return existing;

            var def = collector.OfClass(typeof(FilledRegionType))
                .Cast<FilledRegionType>().FirstOrDefault();
            if (def == null)
                throw new InvalidOperationException("项目中没有填充区域类型！");

            var newType = def.Duplicate(name) as FilledRegionType;
            if (newType == null)
                throw new InvalidOperationException($"无法创建类型：{name}");

            return newType;
        }
    }
}
