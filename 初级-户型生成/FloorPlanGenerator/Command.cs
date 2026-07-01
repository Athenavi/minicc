using System;
using System.Linq;
using System.Windows.Forms;
using Autodesk.Revit.Attributes;
using Autodesk.Revit.DB;
using Autodesk.Revit.UI;
using FloorPlanGenerator.Models;
using FloorPlanGenerator.Services;

namespace FloorPlanGenerator
{
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
                // Step 1: 检查当前视图是否为楼层平面视图
                if (!(doc.ActiveView is ViewPlan viewPlan))
                {
                    TaskDialog.Show("提示",
                        "请先打开一个楼层平面视图，再执行此命令！\n" +
                        "当前视图不是楼层平面视图。");
                    return Result.Cancelled;
                }

                // Step 2: 弹出文件选择器选择 huxi.json
                OpenFileDialog openDialog = new OpenFileDialog
                {
                    Title = "请选择户型数据文件 (huxi.json)",
                    Filter = "JSON 文件 (*.json)|*.json|所有文件 (*.*)|*.*",
                    DefaultExt = ".json",
                    Multiselect = false
                };

                if (openDialog.ShowDialog() != DialogResult.OK)
                    return Result.Cancelled;

                string jsonPath = openDialog.FileName;

                // Step 3: 解析 JSON
                JsonReader reader = new JsonReader();
                RoomJson roomJson = reader.ReadFromFile(jsonPath);

                if (roomJson?.Spaces == null || roomJson.Spaces.Count == 0)
                {
                    TaskDialog.Show("错误", "JSON 文件中没有有效的房间数据！");
                    return Result.Failed;
                }

                // Step 4: 初始化户型生成服务并生成
                FloorPlanService floorPlanService = new FloorPlanService(doc);
                floorPlanService.GenerateFloorPlan(roomJson.Spaces, viewPlan);

                TaskDialog.Show("完成",
                    $"户型生成成功！\n" +
                    $"共创建 {roomJson.Spaces.Count} 个房间。\n" +
                    $"楼层平面: {viewPlan.Name}");

                return Result.Succeeded;
            }
            catch (Exception ex)
            {
                message = ex.Message;
                TaskDialog.Show("错误",
                    $"户型生成失败：\n{ex.Message}\n\n" +
                    $"堆栈：\n{ex.StackTrace}");
                return Result.Failed;
            }
        }
    }
}
