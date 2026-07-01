using System;
using System.Collections.Generic;

namespace ClosedRegion.Models
{
    /// <summary>2D 点</summary>
    public struct Point2D
    {
        public double X, Y;
        public Point2D(double x, double y) { X = x; Y = y; }
        public override string ToString() => $"({X:F3},{Y:F3})";
        public static Point2D operator +(Point2D a, Point2D b) => new Point2D(a.X + b.X, a.Y + b.Y);
        public static Point2D operator -(Point2D a, Point2D b) => new Point2D(a.X - b.X, a.Y - b.Y);
        public static Point2D operator *(Point2D a, double s) => new Point2D(a.X * s, a.Y * s);
    }

    /// <summary>图顶点</summary>
    public class GraphVertex
    {
        public Point2D Point { get; set; }
        public List<GraphEdge> Edges { get; set; } = new List<GraphEdge>();

        public override bool Equals(object obj)
        {
            if (obj is GraphVertex other)
                return Point.X == other.Point.X && Point.Y == other.Point.Y;
            return false;
        }

        public override int GetHashCode()
        {
            return Point.X.GetHashCode() ^ Point.Y.GetHashCode();
        }
    }

    /// <summary>图边（对应一条曲线段）</summary>
    public class GraphEdge
    {
        public GraphVertex Start { get; set; }
        public GraphVertex End { get; set; }
        public bool Used { get; set; }
        public double Length => Math.Sqrt(
            (End.Point.X - Start.Point.X) * (End.Point.X - Start.Point.X) +
            (End.Point.Y - Start.Point.Y) * (End.Point.Y - Start.Point.Y));

        public override bool Equals(object obj)
        {
            if (obj is GraphEdge other)
                return (Start.Equals(other.Start) && End.Equals(other.End)) ||
                       (Start.Equals(other.End) && End.Equals(other.Start)); // 无向边
            return false;
        }

        public override int GetHashCode()
        {
            return Start.GetHashCode() ^ End.GetHashCode();
        }
    }

    /// <summary>封闭区域（面）</summary>
    public class ClosedFace
    {
        public List<GraphEdge> Edges { get; set; } = new List<GraphEdge>();
        public double Area { get; set; }
        public bool IsClockwise { get; set; }

        /// <summary>计算多边形面积（鞋带公式）</summary>
        public static double CalcArea(List<Point2D> polygon)
        {
            if (polygon.Count < 3) return 0;
            double sum = 0;
            for (int i = 0; i < polygon.Count; i++)
            {
                int j = (i + 1) % polygon.Count;
                sum += polygon[i].X * polygon[j].Y;
                sum -= polygon[j].X * polygon[i].Y;
            }
            return Math.Abs(sum) / 2.0;
        }
    }
}
