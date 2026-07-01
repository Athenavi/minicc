using System;
using Autodesk.Revit.Attributes;
using Autodesk.Revit.DB;
using Autodesk.Revit.UI;
using AxisGrid.Services;

namespace AxisGrid
{
    /// <summary>
    /// 手动同步命令 — 将视图中所有 DetailLine 重建为拓扑干净的轴网
    /// </summary>
    [Transaction(TransactionMode.Manual)]
    [Regeneration(RegenerationOption.Manual)]
    public class Command : IExternalCommand
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

                using (Transaction t = new Transaction(doc, "同步轴网"))
                {
                    t.Start();

                    GridSyncService service = new GridSyncService(doc, viewPlan);
                    var graph = service.RebuildGraph(viewPlan);

                    t.Commit();

                    // 详细调试信息
                    string msg = $"轴网同步完成！\n节点数: {graph.Nodes.Count}\n轴线数: {graph.Edges.Count}";

                    if (graph.Nodes.Count == 0 && graph.Edges.Count == 0)
                    {
                        // 显示视图中所有 CurveElement 的详细信息
                        var collector = new FilteredElementCollector(doc, viewPlan.Id);
                        int total = 0;
                        string cats = "";
                        foreach (CurveElement ce in collector.OfClass(typeof(CurveElement)))
                        {
                            total++;
                            string curveInfo = "?";
                            try
                            {
                                Curve c = ce.GeometryCurve;
                                if (c != null)
                                {
                                    XYZ s = c.GetEndPoint(0);
                                    XYZ e = c.GetEndPoint(1);
                                    curveInfo = $"{c.GetType().Name} ({s.X:F1},{s.Y:F1})-({e.X:F1},{e.Y:F1})";
                                }
                            }
                            catch { }
                            if (ce.Category != null)
                                cats += $"\n  [{ce.Category.Id.Value}] {curveInfo}";
                        }
                        msg += $"\n\n视图中 CurveElement 总数: {total}{cats}";
                    }

                    TaskDialog.Show("同步结果", msg);
                }

                return Result.Succeeded;
            }
            catch (Exception ex)
            {
                message = ex.Message;
                TaskDialog.Show("错误", $"轴网同步失败：\n{ex.Message}");
                return Result.Failed;
            }
        }
    }
}
