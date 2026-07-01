using System;
using System.Reflection;
using System.Windows.Media.Imaging;
using Autodesk.Revit.UI;

namespace FloorPlanGenerator
{
    /// <summary>
    /// Revit 2026 插件入口 — 注册 Ribbon 按钮
    /// </summary>
    public class App : IExternalApplication
    {
        public Result OnStartup(UIControlledApplication application)
        {
            // 创建 Ribbon 面板
            string tabName = "户型生成";
            string panelName = "户型工具";

            try
            {
                // 如果标签页不存在则创建
                application.CreateRibbonTab(tabName);
            }
            catch
            {
                // 标签可能已存在，忽略
            }

            // 获取或创建面板
            RibbonPanel panel = application.GetRibbonPanels(tabName)
                .Find(p => p.Name == panelName);

            if (panel == null)
            {
                panel = application.CreateRibbonPanel(tabName, panelName);
            }

            // 创建按钮
            string assemblyPath = Assembly.GetExecutingAssembly().Location;

            PushButtonData buttonData = new PushButtonData(
                "GenerateFloorPlan",
                "生成户型",
                assemblyPath,
                "FloorPlanGenerator.Command");

            // 尝试加载图标
            try
            {
                string iconPath = System.IO.Path.Combine(
                    System.IO.Path.GetDirectoryName(assemblyPath),
                    "Resources",
                    "icon.png");
                if (System.IO.File.Exists(iconPath))
                {
                    buttonData.LargeImage = new BitmapImage(new Uri(iconPath));
                }
            }
            catch
            {
                // 图标加载失败不影响功能
            }

            panel.AddItem(buttonData);

            return Result.Succeeded;
        }

        public Result OnShutdown(UIControlledApplication application)
        {
            return Result.Succeeded;
        }
    }
}
