using System;
using System.Collections.Generic;
using System.Linq;
using Autodesk.Revit.Attributes;
using Autodesk.Revit.DB;
using Autodesk.Revit.UI;
using Autodesk.Revit.UI.Selection;
using ClosedRegion.Models;
using ClosedRegion.Services;

namespace ClosedRegion.Commands
{
    /// <summary>
    /// 详图线选择过滤器
    /// </summary>
    public class DetailLineFilter : ISelectionFilter
    {
        public bool AllowElement(Element elem) => elem is DetailLine;
        public bool AllowReference(Reference refer, XYZ point) => false;
    }

    [Transaction(TransactionMode.Manual)]
    public class MaxRegionCommand : IExternalCommand
    {
        public Result Execute(ExternalCommandData cd, ref string message, ElementSet elements)
        {
            var uiapp = cd.Application;
            var uidoc = uiapp.ActiveUIDocument;
            var doc = uidoc.Document;
            var view = doc.ActiveView as ViewPlan;

            if (view == null)
            {
                TaskDialog.Show("提示", "请先打开楼层平面视图！");
                return Result.Cancelled;
            }

            try
            {
                // 选择详图线
                var refs = uidoc.Selection.PickObjects(
                    ObjectType.Element, new DetailLineFilter(), "请选择白色曲线（详图线）");
                if (refs == null || refs.Count == 0) return Result.Cancelled;

                var curves = new List<Curve>();
                foreach (var r in refs)
                {
                    if (doc.GetElement(r.ElementId) is DetailLine dl)
                    {
                        var c = dl.GeometryCurve;
                        if (c != null) curves.Add(c);
                    }
                }
                if (curves.Count == 0)
                { TaskDialog.Show("提示", "未选中有效曲线"); return Result.Failed; }

                // 处理曲线
                var processor = new CurveProcessor(curves, doc);
                var segments = processor.Process();

                // 检测封闭区域
                var detector = new FaceDetector();
                var (minFaces, maxFaces) = detector.Detect(segments);

                if (maxFaces.Count == 0)
                { TaskDialog.Show("诊断", $"未找到最大封闭区域\n曲线段: {segments.Count}\n最小面: {minFaces.Count}"); return Result.Failed; }

                // 生成填充区域
                var frService = new FilledRegionService(doc);
                using (var t = new Transaction(doc, "生成最大封闭区域"))
                {
                    t.Start();
                    frService.DeleteExisting(view);
                    frService.CreateMaxRegions(maxFaces, view);
                    t.Commit();
                }

                string diag = $"曲线段: {segments.Count}\n最小面: {minFaces.Count}\n最大面: {maxFaces.Count}";
                for (int i = 0; i < maxFaces.Count; i++)
                    diag += $"\n区域{i+1}: {maxFaces[i].Edges.Count}边, 面积{maxFaces[i].Area:F2}";
                TaskDialog.Show("完成", diag);
                return Result.Succeeded;
            }
            catch (Exception ex)
            {
                message = ex.Message;
                TaskDialog.Show("错误", $"生成失败：{ex.Message}");
                return Result.Failed;
            }
        }
    }

    [Transaction(TransactionMode.Manual)]
    public class MinRegionCommand : IExternalCommand
    {
        public Result Execute(ExternalCommandData cd, ref string message, ElementSet elements)
        {
            var uiapp = cd.Application;
            var uidoc = uiapp.ActiveUIDocument;
            var doc = uidoc.Document;
            var view = doc.ActiveView as ViewPlan;

            if (view == null)
            {
                TaskDialog.Show("提示", "请先打开楼层平面视图！");
                return Result.Cancelled;
            }

            try
            {
                var refs = uidoc.Selection.PickObjects(
                    ObjectType.Element, new DetailLineFilter(), "请选择白色曲线（详图线）");
                if (refs == null || refs.Count == 0) return Result.Cancelled;

                var curves = new List<Curve>();
                foreach (var r in refs)
                {
                    if (doc.GetElement(r.ElementId) is DetailLine dl)
                    {
                        var c = dl.GeometryCurve;
                        if (c != null) curves.Add(c);
                    }
                }
                if (curves.Count == 0)
                { TaskDialog.Show("提示", "未选中有效曲线"); return Result.Failed; }

                var processor = new CurveProcessor(curves, doc);
                var segments = processor.Process();

                var detector = new FaceDetector();
                var (minFaces, maxFaces) = detector.Detect(segments);

                if (minFaces.Count == 0)
                { TaskDialog.Show("提示", "未找到最小封闭区域"); return Result.Failed; }

                var frService = new FilledRegionService(doc);
                using (var t = new Transaction(doc, "生成最小封闭区域"))
                {
                    t.Start();
                    frService.DeleteExisting(view);
                    frService.CreateMinRegions(minFaces, view);
                    t.Commit();
                }

                TaskDialog.Show("完成",
                    $"最小封闭区域已生成！共 {minFaces.Count} 个\n" +
                    $"总面积: {minFaces.Sum(f => f.Area):F2}");
                return Result.Succeeded;
            }
            catch (Exception ex)
            {
                message = ex.Message;
                TaskDialog.Show("错误", $"生成失败：{ex.Message}");
                return Result.Failed;
            }
        }
    }
}
