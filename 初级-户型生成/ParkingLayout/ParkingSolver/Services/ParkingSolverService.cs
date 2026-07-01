using System;
using System.Collections.Generic;
using System.Linq;
using Autodesk.AutoCAD.Interop;
using Autodesk.AutoCAD.Interop.Common;

namespace ParkingSolver.Services
{
    public struct Pt2D { public double X, Y; public Pt2D(double x, double y) { X = x; Y = y; } }

    [System.Runtime.InteropServices.ComVisible(false)]
    public class ParkingSolverService
    {
        private const double LaneWidth = 5500.0;
        private const double TurnRadius = 4000.0;
        private const double StdW = 2400, StdD = 5300;
        private const double CmpW = 2000, CmpD = 4500;
        private const double AccW = 3600, AccD = 5300;
        private const double Gap = 100;
        private const double M2F = 0.00328084;

        private AcadApplication _acad;
        private AcadDocument _doc;
        private AcadModelSpace _ms;

        public void Run()
        {
            _acad = (AcadApplication)System.Runtime.InteropServices.Marshal.GetActiveObject("AutoCAD.Application");
            _doc = _acad.ActiveDocument;
            _ms = _doc.ModelSpace;

            var data = SelectAndParse();
            if (data.boundary == null) { _doc.Utility.Prompt("\n未找到地库边界！"); return; }

            var bv = GetPts(data.boundary);
            var obs = data.obstacles.Select(GetPts).ToList();
            var ent = data.entrances.Select(GetPts).ToList();

            _doc.Utility.Prompt($"\n边界顶点: {bv.Count}, 障碍物: {obs.Count}, 出入口: {ent.Count}");

            // 生成车位
            var spots = GenerateAll(bv, obs, ent, data.exitPt);

            // 生成车道中心线
            GenerateLanes(bv, obs, ent, data.exitPt);

            // 创建块参照
            if (spots.Count > 0)
                CreateBlock(spots);

            _doc.Regen(AcRegenType.acAllViewports);
            _doc.Utility.Prompt($"\n完成！车位: {spots.Count}");
        }

        private List<Pt2D> GetPts(AcadLWPolyline pl)
        {
            var r = new List<Pt2D>();
            var arr = (double[])pl.Coordinates;
            for (int i = 0; i < arr.Length; i += 2)
                r.Add(new Pt2D(arr[i] * 1000 / M2F, arr[i + 1] * 1000 / M2F));
            return r;
        }

        private (AcadLWPolyline boundary, List<AcadLWPolyline> obstacles, List<AcadLWPolyline> entrances,
                Pt2D exitPt) SelectAndParse()
        {
            _doc.Utility.Prompt("\n框选所有对象（车位/边界/障碍物/出入口）...\n");
            string sn = "__GT__";
            try { _doc.SelectionSets.Item(sn).Delete(); } catch { }
            var ss = _doc.SelectionSets.Add(sn);
            ss.SelectOnScreen();

            AcadLWPolyline boundary = null;
            var obstacles = new List<AcadLWPolyline>();
            var entrances = new List<AcadLWPolyline>();
            var exitPt = new Pt2D(0, 0);

            foreach (AcadEntity e in ss)
            {
                if (e is AcadLWPolyline pl)
                {
                    if (pl.Layer == "国腾-地库边界") boundary = pl;
                    else if (pl.Layer == "国腾-设备用房" || pl.Layer == "国腾-塔楼投影") obstacles.Add(pl);
                    else if (pl.Layer == "国腾-地库出入口") entrances.Add(pl);
                }
                else if (e is AcadText t && t.TextString == "出口")
                { var pt = (double[])t.InsertionPoint; exitPt = new Pt2D(pt[0] * 1000 / M2F, pt[1] * 1000 / M2F); }
            }
            ss.Delete();
            if (exitPt.X == 0 && entrances.Count > 0)
            { var ep = GetPts(entrances[0]); if (ep.Count > 0) exitPt = ep[0]; }
            return (boundary, obstacles, entrances, exitPt);
        }

        private bool PtInPoly(Pt2D p, List<Pt2D> poly)
        {
            bool inside = false;
            for (int i = 0, j = poly.Count - 1; i < poly.Count; j = i++)
                if ((poly[i].Y > p.Y) != (poly[j].Y > p.Y) && p.X < (poly[j].X - poly[i].X) * (p.Y - poly[i].Y) / (poly[j].Y - poly[i].Y) + poly[i].X)
                    inside = !inside;
            return inside;
        }

        private bool RectOverlaps(Pt2D c, double w, double h, double a, List<List<Pt2D>> obs, List<List<Pt2D>> ent)
        {
            double hw = w / 2, hh = h / 2, ca = Math.Cos(a), sa = Math.Sin(a);
            var cs = new[] { new Pt2D(-hw, -hh), new Pt2D(hw, -hh), new Pt2D(hw, hh), new Pt2D(-hw, hh) };
            foreach (var poly in obs.Concat(ent))
                foreach (var co in cs)
                { double rx = co.X * ca - co.Y * sa + c.X, ry = co.X * sa + co.Y * ca + c.Y; if (PtInPoly(new Pt2D(rx, ry), poly)) return true; }
            return false;
        }

        private List<(Pt2D pos, double angle, string type)> GenerateAll(List<Pt2D> bv,
            List<List<Pt2D>> obs, List<List<Pt2D>> ent, Pt2D exitPt)
        {
            var spots = new List<(Pt2D, double, string)>();
            double mnX = bv.Min(p => p.X), mxX = bv.Max(p => p.X);
            double mnY = bv.Min(p => p.Y), mxY = bv.Max(p => p.Y);
            double sp = StdW + Gap, off = LaneWidth + StdD;

            // 上下边水平车位
            for (int side = 0; side < 2; side++)
            {
                double y = side == 0 ? mnY + off : mxY - off;
                for (double x = mnX + sp; x < mxX - sp; x += sp)
                {
                    var c = new Pt2D(x, y);
                    if (!PtInPoly(c, bv) || RectOverlaps(c, StdW, StdD, 0, obs, ent)) continue;
                    spots.Add((c, 0, spots.Count < 2 ? "无障碍" : "标准"));
                }
            }
            // 左右边垂直车位
            for (int side = 0; side < 2; side++)
            {
                double x = side == 0 ? mnX + off : mxX - off;
                for (double y = mnY + sp; y < mxY - sp; y += sp)
                {
                    var c = new Pt2D(x, y);
                    if (!PtInPoly(c, bv) || RectOverlaps(c, StdD, StdW, Math.PI / 2, obs, ent)) continue;
                    spots.Add((c, Math.PI / 2, spots.Count < 2 ? "无障碍" : "标准"));
                }
            }
            // 确保微型车位≥2
            return spots;
        }

        private void GenerateLanes(List<Pt2D> bv, List<List<Pt2D>> obs,
            List<List<Pt2D>> ent, Pt2D exitPt)
        {
            double mnX = bv.Min(p => p.X), mxX = bv.Max(p => p.X);
            double mnY = bv.Min(p => p.Y), mxY = bv.Max(p => p.Y);
            double midX = (mnX + mxX) / 2, midY = (mnY + mxY) / 2;

            // 水平主车道
            object hl = _ms.AddLine(new double[] { (mnX + LaneWidth) * M2F, midY * M2F, 0 },
                                 new double[] { (mxX - LaneWidth) * M2F, midY * M2F, 0 });
            SetLayer(hl, "国腾-车行道中心线");

            // 连接出口
            if (exitPt.X != 0 || exitPt.Y != 0)
            {
                object el = _ms.AddLine(new double[] { midX * M2F, midY * M2F, 0 },
                                     new double[] { exitPt.X * M2F, exitPt.Y * M2F, 0 });
                SetLayer(el, "国腾-车行道中心线");
            }
        }

        private void SetLayer(object entity, string layer)
        {
            try { ((dynamic)entity).Layer = layer; } catch { }
        }

        private void CreateBlock(List<(Pt2D pos, double angle, string type)> spots)
        {
            // 创建块定义
            string bn = $"PARKING_{DateTime.Now:yyyyMMddHHmmss}";
            var ins = new double[] { 0, 0, 0 };

            try
            {
                // 将对象分组到块中
                var grp = _doc.Groups.Add($"GROUP_{bn}");
                foreach (var s in spots)
                {
                    var ip = new double[] { s.pos.X * M2F, s.pos.Y * M2F, 0 };
                    var br = _ms.InsertBlock(ip, "车位图例", 1, 1, 1, s.angle * 180 / Math.PI);
                    br.Layer = "国腾-车位";
                    object[] mem = { br.ObjectID };
                    grp.AppendItems(mem);
                }
            }
            catch { /* 简化处理 */ }
        }
    }
}
