using System.Collections.Generic;

namespace SeedFill.Models
{
    /// <summary>种子填充统计结果</summary>
    public class SeedStatistics
    {
        public int UncutCount { get; set; }  // 未裁剪种子数
        public double UncutArea { get; set; } // 未裁剪种子总面积
        public int CutCount { get; set; }    // 裁切种子数
        public double CutArea { get; set; }  // 裁切种子总面积
        public int TotalCount => UncutCount + CutCount;
        public double TotalArea => UncutArea + CutArea;
    }

    /// <summary>二维点</summary>
    public struct Point2D
    {
        public double X { get; set; }
        public double Y { get; set; }
        public Point2D(double x, double y) { X = x; Y = y; }

        public static Point2D operator +(Point2D a, Point2D b) => new Point2D(a.X + b.X, a.Y + b.Y);
        public static Point2D operator -(Point2D a, Point2D b) => new Point2D(a.X - b.X, a.Y - b.Y);
        public static Point2D operator *(Point2D a, double s) => new Point2D(a.X * s, a.Y * s);

        public double Length => System.Math.Sqrt(X * X + Y * Y);
    }

    /// <summary>多边形（闭合线段集合）</summary>
    public class Polygon
    {
        public List<Point2D> Vertices { get; set; } = new List<Point2D>();
        public bool IsClipped { get; set; } // true=被裁剪过的种子

        public double Area
        {
            get
            {
                if (Vertices.Count < 3) return 0;
                return System.Math.Abs(SignedArea);
            }
        }

        /// <summary>有符号面积（正=逆时针，负=顺时针）</summary>
        public double SignedArea
        {
            get
            {
                if (Vertices.Count < 3) return 0;
                double sum = 0;
                for (int i = 0; i < Vertices.Count; i++)
                {
                    int j = (i + 1) % Vertices.Count;
                    sum += Vertices[i].X * Vertices[j].Y;
                    sum -= Vertices[j].X * Vertices[i].Y;
                }
                return sum / 2.0;
            }
        }

        /// <summary>确保多边形顶点为逆时针顺序</summary>
        public void EnsureCCW()
        {
            if (SignedArea < 0)
                Vertices.Reverse();
        }
    }
}
