using System;
using System.Reflection;
using Autodesk.Revit.UI;

namespace ClosedRegion
{
    public class App : IExternalApplication
    {
        public Result OnStartup(UIControlledApplication application)
        {
            string tabName = "封闭区域";
            string panelName = "区域工具";

            try { application.CreateRibbonTab(tabName); }
            catch { }

            RibbonPanel panel = application.GetRibbonPanels(tabName)
                .Find(p => p.Name == panelName);
            if (panel == null)
                panel = application.CreateRibbonPanel(tabName, panelName);

            string assemblyPath = Assembly.GetExecutingAssembly().Location;

            PushButtonData btnMax = new PushButtonData(
                "MaxRegionCommand", "最大区域", assemblyPath,
                "ClosedRegion.Commands.MaxRegionCommand");

            PushButtonData btnMin = new PushButtonData(
                "MinRegionCommand", "最小区域", assemblyPath,
                "ClosedRegion.Commands.MinRegionCommand");

            panel.AddItem(btnMax);
            panel.AddItem(btnMin);

            return Result.Succeeded;
        }

        public Result OnShutdown(UIControlledApplication application)
        {
            return Result.Succeeded;
        }
    }
}
