using System;
using System.Collections.Generic;
using System.Linq;
using Autodesk.Revit.Attributes;
using Autodesk.Revit.DB;
using Autodesk.Revit.UI;
using AxisGrid.Models;
using AxisGrid.Services;

namespace AxisGrid
{
    /// <summary>
    /// 节点编辑命令：在轴线上添加节点、移动节点、删除节点
    /// 节点操作后自动触发轴网同步（通过 IUpdater 或手动）
    /// </summary>
    [Transaction(TransactionMode.Manual)]
    [Regeneration(RegenerationOption.Manual)]
    public class NodeEditCommand : IExternalCommand
    {
        public Result Execute(
            ExternalCommandData commandData,
            ref string message,
            ElementSet elements)
        {
            UIApplication uiapp = commandData.Application;
            UIDocument uidoc = uiapp.ActiveUIDocument;
            Document doc = uidoc.Document;

            try
            {
                ViewPlan viewPlan = uidoc.ActiveGraphicalView as ViewPlan;
                if (viewPlan == null)
                {
                    TaskDialog.Show("提示", "请先打开一个楼层平面视图！");
                    return Result.Cancelled;
                }

                // 选择操作类型
                string[] choices = new string[] { "添加节点", "移动节点", "删除节点" };
                int choice = 0;
                if (!TryGetChoice("节点操作", "请选择操作类型：", choices, out choice))
                    return Result.Cancelled;

                using (Transaction t = new Transaction(doc, "节点操作"))
                {
                    t.Start();

                    if (choice == 0) AddNode(doc, viewPlan);
                    else if (choice == 1) MoveNode(doc, viewPlan);
                    else if (choice == 2) DeleteNode(doc, viewPlan);

                    t.Commit();
                }

                return Result.Succeeded;
            }
            catch (Exception ex)
            {
                message = ex.Message;
                TaskDialog.Show("错误", $"节点操作失败：\n{ex.Message}");
                return Result.Failed;
            }
        }

        private bool TryGetChoice(string title, string prompt, string[] choices, out int choice)
        {
            choice = 0;
            return false; // 基类不支持选择对话框，由用户调用不同命令
        }

        /// <summary>
        /// 添加节点：在选中的详图线上插入一个节点
        /// 该线在此点被打断成两段
        /// </summary>
        private void AddNode(Document doc, ViewPlan viewPlan)
        {
            UIDocument uidoc = new UIDocument(doc);
            Reference pickedRef = uidoc.Selection.PickObject(
                Autodesk.Revit.UI.Selection.ObjectType.Element,
                "请选择要添加节点的详图线");

            if (pickedRef == null) return;
            CurveElement ce = doc.GetElement(pickedRef) as CurveElement;
            if (ce == null) return;

            XYZ pickPoint = pickedRef.GlobalPoint;
            Curve curve = ce.GeometryCurve;

            // 将拾取点投影到曲线上
            XYZ proj = curve.Project(pickPoint).XYZPoint;

            // 在投影点打断该线
            Curve segment1 = null, segment2 = null;
            double t = curve.Project(proj).Parameter;

            if (curve is Line)
            {
                segment1 = Line.CreateBound(curve.GetEndPoint(0), proj);
                segment2 = Line.CreateBound(proj, curve.GetEndPoint(1));
            }
            else if (curve is Arc arc)
            {
                XYZ mid1 = arc.Evaluate(t / 2, false);
                XYZ mid2 = arc.Evaluate((t + 1) / 2, false);
                segment1 = Arc.Create(curve.GetEndPoint(0), proj, mid1);
                segment2 = Arc.Create(proj, curve.GetEndPoint(1), mid2);
            }

            if (segment1 == null || segment2 == null) return;

            // 获取线样式
            Parameter gsParam = ce.get_Parameter(BuiltInParameter.BUILDING_CURVE_GSTYLE);
            ElementId gsId = gsParam?.AsElementId() ?? ElementId.InvalidElementId;

            // 删除原线，创建两条新线
            doc.Delete(ce.Id);

            CurveElement newLine1 = doc.Create.NewDetailCurve(viewPlan, segment1) as CurveElement;
            CurveElement newLine2 = doc.Create.NewDetailCurve(viewPlan, segment2) as CurveElement;

            if (gsId != ElementId.InvalidElementId)
            {
                if (newLine1 != null)
                    newLine1.get_Parameter(BuiltInParameter.BUILDING_CURVE_GSTYLE)?.Set(gsId);
                if (newLine2 != null)
                    newLine2.get_Parameter(BuiltInParameter.BUILDING_CURVE_GSTYLE)?.Set(gsId);
            }
        }

        /// <summary>
        /// 删除节点：连接两条共线轴线的节点被删除时，两条轴线合并为一条
        /// 只连接一条轴线的节点被删除时，仅删除该轴线
        /// </summary>
        private void DeleteNode(Document doc, ViewPlan viewPlan)
        {
            UIDocument uidoc = new UIDocument(doc);
            Reference pickedRef = uidoc.Selection.PickObject(
                Autodesk.Revit.UI.Selection.ObjectType.Element,
                "请选择节点 (FilledRegion)");

            if (pickedRef == null) return;
            FilledRegion region = doc.GetElement(pickedRef) as FilledRegion;
            if (region == null)
            {
                TaskDialog.Show("提示", "请选择一个 FilledRegion（填充区域）作为节点");
                return;
            }

            XYZ center = GetRegionCenter(region);

            // 查找以该点为中心的所有轴线
            var nearbyLines = FindLinesAtPoint(doc, viewPlan, center, 0.05);

            if (nearbyLines.Count == 0)
            {
                // 孤立节点，直接删除
                doc.Delete(region.Id);
                return;
            }

            if (nearbyLines.Count == 1)
            {
                // 只连一条轴线 → 删除该轴线
                doc.Delete(nearbyLines[0].Id);
                doc.Delete(region.Id);
                return;
            }

            if (nearbyLines.Count == 2)
            {
                // 连两条轴线 → 检查是否共线
                Curve c1 = nearbyLines[0].GeometryCurve;
                Curve c2 = nearbyLines[1].GeometryCurve;

                // 取方向
                XYZ dir1 = (c1.GetEndPoint(1) - c1.GetEndPoint(0)).Normalize();
                XYZ dir2 = (c2.GetEndPoint(1) - c2.GetEndPoint(0)).Normalize();

                // 检查方向是否相反（共线）
                if (Math.Abs(Math.Abs(dir1.DotProduct(dir2)) - 1.0) < 0.001)
                {
                    // 共线 → 合并为一条轴线
                    XYZ farEnd1 = GetFarEnd(c1, center);
                    XYZ farEnd2 = GetFarEnd(c2, center);

                    // 获取线样式
                    Parameter gsParam = nearbyLines[0].get_Parameter(BuiltInParameter.BUILDING_CURVE_GSTYLE);
                    ElementId gsId = gsParam?.AsElementId() ?? ElementId.InvalidElementId;

                    // 删除两条旧线和节点
                    doc.Delete(nearbyLines[0].Id);
                    doc.Delete(nearbyLines[1].Id);
                    doc.Delete(region.Id);

                    // 创建合并后的线
                    Curve mergedCurve = Line.CreateBound(farEnd1, farEnd2);
                    CurveElement newLine = doc.Create.NewDetailCurve(viewPlan, mergedCurve) as CurveElement;
                    if (gsId != ElementId.InvalidElementId && newLine != null)
                        newLine.get_Parameter(BuiltInParameter.BUILDING_CURVE_GSTYLE)?.Set(gsId);
                }
                else
                {
                    // 不共线（十字形）→ 只删除其中一条
                    doc.Delete(nearbyLines[0].Id);
                }
                return;
            }

            // 连三条以上 → 只删除节点本身
            doc.Delete(region.Id);
        }

        /// <summary>
        /// 移动节点：复制节点到新位置
        /// </summary>
        private void MoveNode(Document doc, ViewPlan viewPlan)
        {
            // 移动节点 = 删除旧节点 + 在新位置添加节点
            DeleteNode(doc, viewPlan);

            // 在新位置画一条极短的辅助线再删除，触发 IUpdater 重建
            UIDocument uidoc = new UIDocument(doc);
            XYZ newPos = uidoc.Selection.PickPoint("请选择新节点位置");
            if (newPos != null)
            {
                Curve tinyCurve = Line.CreateBound(newPos, newPos + new XYZ(0.001, 0, 0));
                CurveElement temp = doc.Create.NewDetailCurve(viewPlan, tinyCurve) as CurveElement;
                if (temp != null)
                {
                    // 在新位置创建节点 FilledRegion
                    FilledRegionType nodeType = GetOrCreateNodeType(doc);
                    if (nodeType != null)
                    {
                        var curveLoop = new CurveLoop();
                        double r = 0.013;
                        int segs = 12;
                        for (int s = 0; s < segs; s++)
                        {
                            double a1 = (double)s / segs * 2 * Math.PI;
                            double a2 = (double)(s + 1) / segs * 2 * Math.PI;
                            XYZ p1 = new XYZ(newPos.X + r * Math.Cos(a1), newPos.Y + r * Math.Sin(a1), 0);
                            XYZ p2 = new XYZ(newPos.X + r * Math.Cos(a2), newPos.Y + r * Math.Sin(a2), 0);
                            double ma = (a1 + a2) / 2;
                            XYZ mid = new XYZ(newPos.X + r * Math.Cos(ma), newPos.Y + r * Math.Sin(ma), 0);
                            curveLoop.Append(Arc.Create(p1, p2, mid));
                        }
                        FilledRegion.Create(doc, nodeType.Id, viewPlan.Id, new List<CurveLoop> { curveLoop });
                    }
                    doc.Delete(temp.Id);
                }
            }
        }

        private XYZ GetRegionCenter(FilledRegion region)
        {
            // 用 BoundingBox 中心近似
            BoundingBoxXYZ bb = region.get_BoundingBox(null);
            if (bb != null) return (bb.Min + bb.Max) / 2;
            return XYZ.Zero;
        }

        private List<CurveElement> FindLinesAtPoint(Document doc, View view, XYZ point, double tolerance)
        {
            var result = new List<CurveElement>();
            var collector = new FilteredElementCollector(doc, view.Id);
            foreach (CurveElement ce in collector.OfClass(typeof(CurveElement)))
            {
                try
                {
                    Curve c = ce.GeometryCurve;
                    XYZ proj = c.Project(point).XYZPoint;
                    if (proj.DistanceTo(point) < tolerance)
                        result.Add(ce);
                }
                catch { }
            }
            return result;
        }

        private XYZ GetFarEnd(Curve curve, XYZ near)
        {
            XYZ e0 = curve.GetEndPoint(0);
            XYZ e1 = curve.GetEndPoint(1);
            return (e0.DistanceTo(near) < e1.DistanceTo(near)) ? e1 : e0;
        }

        private FilledRegionType GetOrCreateNodeType(Document doc)
        {
            var collector = new FilteredElementCollector(doc);
            var existing = collector
                .OfClass(typeof(FilledRegionType))
                .Cast<FilledRegionType>()
                .FirstOrDefault(fr => fr.Name == "轴网节点");
            if (existing != null) return existing;

            var defaultType = collector
                .OfClass(typeof(FilledRegionType))
                .Cast<FilledRegionType>()
                .FirstOrDefault();
            return defaultType?.Duplicate("轴网节点") as FilledRegionType;
        }
    }
}
