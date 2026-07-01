using System;
using System.Runtime.InteropServices;
using Autodesk.AutoCAD.ApplicationServices;
using Autodesk.AutoCAD.DatabaseServices;
using Autodesk.AutoCAD.EditorInput;
using Autodesk.AutoCAD.Runtime;
using SeedFill.Models;
using SeedFill.Services;

namespace SeedFill.Commands
{
    /// <summary>
    /// COM 可见类 — 供 LISP 通过 vlax-create-object 调用
    /// </summary>
    [ComVisible(true)]
    [Guid("A1B2C3D4-E5F6-7890-ABCD-EF1234567890")]
    [ProgId("SeedFill.Command")]
    public class SeedCommandCom
    {
        /// <summary>
        /// 执行种子填充（由 LISP 调用）
        /// </summary>
        public string ExecuteSeed()
        {
            Document doc = Application.DocumentManager.MdiActiveDocument;
            Editor ed = doc.Editor;

            try
            {
                SeedFillService service = new SeedFillService(doc.Database);
                SeedStatistics stats = service.Execute();

                if (stats == null) return "已取消";

                return $"未裁剪: {stats.UncutCount}个({stats.UncutArea:F2})  裁切: {stats.CutCount}个({stats.CutArea:F2})  总计: {stats.TotalCount}个({stats.TotalArea:F2})";
            }
            catch (System.Exception ex)
            {
                return $"错误: {ex.Message}";
            }
        }

        /// <summary>
        /// 执行种子统计（由 LISP 调用）
        /// </summary>
        public string ExecuteStatistics()
        {
            Document doc = Application.DocumentManager.MdiActiveDocument;
            Editor ed = doc.Editor;

            try
            {
                PromptEntityOptions peo = new PromptEntityOptions("\n请选择边界多段线");
                peo.SetRejectMessage("请选择闭合多段线");
                peo.AddAllowedClass(typeof(Polyline), true);
                PromptEntityResult per = ed.GetEntity(peo);
                if (per.Status != PromptStatus.OK) return "已取消";

                SeedFillService service = new SeedFillService(doc.Database);
                SeedStatistics stats = service.GetStatistics(per.ObjectId);

                return $"未裁剪: {stats.UncutCount}个({stats.UncutArea:F2})  裁切: {stats.CutCount}个({stats.CutArea:F2})  总计: {stats.TotalCount}个({stats.TotalArea:F2})";
            }
            catch (System.Exception ex)
            {
                return $"错误: {ex.Message}";
            }
        }
    }

    /// <summary>
    /// 命令（保留，供手动 NETLOAD 使用）
    /// </summary>
    public class SeedCommands
    {
        [CommandMethod("Seed")]
        public void SeedCommand()
        {
            Document doc = Application.DocumentManager.MdiActiveDocument;
            Editor ed = doc.Editor;
            try
            {
                SeedFillService service = new SeedFillService(doc.Database);
                SeedStatistics stats = service.Execute();
                if (stats != null)
                {
                    ed.WriteMessage($"\n未裁剪: {stats.UncutCount}个({stats.UncutArea:F2})  裁切: {stats.CutCount}个({stats.CutArea:F2})  总计: {stats.TotalCount}个({stats.TotalArea:F2})");
                }
            }
            catch (System.Exception ex)
            {
                ed.WriteMessage($"\n错误: {ex.Message}");
            }
        }

        [CommandMethod("Statistics")]
        public void StatisticsCommand()
        {
            Document doc = Application.DocumentManager.MdiActiveDocument;
            Editor ed = doc.Editor;
            try
            {
                PromptEntityOptions peo = new PromptEntityOptions("\n请选择边界多段线");
                peo.SetRejectMessage("请选择闭合多段线");
                peo.AddAllowedClass(typeof(Polyline), true);
                PromptEntityResult per = ed.GetEntity(peo);
                if (per.Status != PromptStatus.OK) return;

                SeedFillService service = new SeedFillService(doc.Database);
                SeedStatistics stats = service.GetStatistics(per.ObjectId);
                ed.WriteMessage($"\n未裁剪种子: {stats.UncutCount} 个, 面积: {stats.UncutArea:F2}");
                // diag layer
                using (Transaction tr = doc.Database.TransactionManager.StartTransaction()) {
                    BlockTable bt = tr.GetObject(doc.Database.BlockTableId, OpenMode.ForRead) as BlockTable;
                    BlockTableRecord ms = tr.GetObject(bt[BlockTableRecord.ModelSpace], OpenMode.ForRead) as BlockTableRecord;
                    int pc=0,po=0,nl=0;
                    foreach (ObjectId id in ms) {
                        DBObject obj = tr.GetObject(id, OpenMode.ForRead);
                        Entity e = obj as Entity; if (e == null || e.Layer != "种子填充结果") continue;
                        if (obj is Polyline pp) { if(pp.Closed) pc++; else po++; }
                        else if (obj is Line) nl++;
                    }
                    tr.Commit();
                    ed.WriteMessage($"\n[diag] layer 种子填充结果: closedPline={pc} openPline={po} lines={nl}");
                }
                ed.WriteMessage($"\n裁切种子:   {stats.CutCount} 个, 面积: {stats.CutArea:F2}");
                ed.WriteMessage($"\n总计:        {stats.TotalCount} 个, 面积: {stats.TotalArea:F2}");
            }
            catch (System.Exception ex)
            {
                ed.WriteMessage($"\n错误: {ex.Message}");
            }
        }

        [CommandMethod("Seed2")]
        public void Seed2Command()
        {
            Document doc = Application.DocumentManager.MdiActiveDocument;
            Editor ed = doc.Editor;
            try
            {
                SeedFillService service = new SeedFillService(doc.Database);
                int lines = service.Execute2();
                ed.WriteMessage($"\nSeed2 完成！共 {lines} 条线段");
            }
            catch (System.Exception ex)
            {
                ed.WriteMessage($"\n错误: {ex.Message}");
            }
        }
    }
}
