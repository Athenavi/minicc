using System.Collections.Generic;
using Autodesk.Revit.DB;
using FloorPlanGenerator.Services;

namespace FloorPlanGenerator.Models
{
    /// <summary>
    /// 单个房间的输入数据
    /// </summary>
    public class RoomData
    {
        /// <summary>房间中心点 X (mm)</summary>
        public double CentroidX { get; set; }

        /// <summary>房间中心点 Y (mm)</summary>
        public double CentroidY { get; set; }

        /// <summary>房间名称</summary>
        public string Name { get; set; }

        /// <summary>开间 — 水平方向长度 (mm)</summary>
        public double Bay { get; set; }

        /// <summary>进深 — 垂直方向长度 (mm)</summary>
        public double Depth { get; set; }

        /// <summary>
        /// 获取房间的四个边界（墙中心线），返回 [左, 右, 下, 上]
        /// 左/右为垂直方向线，下/上为水平方向线
        /// 坐标单位转换为 Revit 内部单位（英尺）
        /// </summary>
        public List<BoundaryLine> GetBoundaryLines()
        {
            double cx = CentroidX.MmToFeet();
            double cy = CentroidY.MmToFeet();
            double halfBay = (Bay / 2.0).MmToFeet();
            double halfDepth = (Depth / 2.0).MmToFeet();

            double leftX = cx - halfBay;   // 左边界 X
            double rightX = cx + halfBay;  // 右边界 X
            double bottomY = cy - halfDepth; // 下边界 Y
            double topY = cy + halfDepth;    // 上边界 Y

            return new List<BoundaryLine>
            {
                // 左墙（垂直，从下到上）
                new BoundaryLine
                {
                    Start = new XYZ(leftX, bottomY, 0),
                    End = new XYZ(leftX, topY, 0),
                    Orientation = Orientation.Vertical,
                    IsLeft = true
                },
                // 右墙（垂直，从下到上）
                new BoundaryLine
                {
                    Start = new XYZ(rightX, bottomY, 0),
                    End = new XYZ(rightX, topY, 0),
                    Orientation = Orientation.Vertical,
                    IsLeft = false
                },
                // 下墙（水平，从左到右）
                new BoundaryLine
                {
                    Start = new XYZ(leftX, bottomY, 0),
                    End = new XYZ(rightX, bottomY, 0),
                    Orientation = Orientation.Horizontal,
                    IsLeft = false // IsBottom
                },
                // 上墙（水平，从左到右）
                new BoundaryLine
                {
                    Start = new XYZ(leftX, topY, 0),
                    End = new XYZ(rightX, topY, 0),
                    Orientation = Orientation.Horizontal,
                    IsLeft = true // IsTop
                }
            };
        }
    }

    /// <summary>墙体的朝向</summary>
    public enum Orientation
    {
        Horizontal, // 水平方向（上下墙）
        Vertical    // 垂直方向（左右墙）
    }

    /// <summary>墙体边界线（中心线）</summary>
    public class BoundaryLine
    {
        public XYZ Start { get; set; }
        public XYZ End { get; set; }
        public Orientation Orientation { get; set; }

        /// <summary>
        /// 对于 Vertical 线：true=左墙, false=右墙
        /// 对于 Horizontal 线：true=上墙, false=下墙
        /// </summary>
        public bool IsLeft { get; set; }

        /// <summary>所属的房间索引</summary>
        public int RoomIndex { get; set; }

        /// <summary>所属的房间名称</summary>
        public string RoomName { get; set; }

        /// <summary>是否为外墙（仅一个房间独有）</summary>
        public bool IsExterior { get; set; } = true;

        /// <summary>墙厚度 (mm)</summary>
        public double ThicknessMm => IsExterior ? 200.0 : 100.0;

        /// <summary>在 Revit 中的墙厚度 (英尺)</summary>
        public double ThicknessFeet => ThicknessMm.MmToFeet();

        /// <summary>获取 Revit Line</summary>
        public Line GetLine()
        {
            return Line.CreateBound(Start, End);
        }
    }
}
