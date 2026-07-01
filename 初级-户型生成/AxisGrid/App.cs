using System;
using System.Reflection;
using System.Windows.Media.Imaging;
using Autodesk.Revit.DB;
using Autodesk.Revit.UI;
using AxisGrid.Updaters;

namespace AxisGrid
{
    public class App : IExternalApplication
    {
        private static AxisGridUpdater _updater;

        public Result OnStartup(UIControlledApplication application)
        {
            try
            {
                string tabName = "轴网工具";
                string panelName = "轴网";

                try { application.CreateRibbonTab(tabName); } catch { }

                RibbonPanel panel = application.GetRibbonPanels(tabName)
                    .Find(p => p.Name == panelName);
                if (panel == null)
                    panel = application.CreateRibbonPanel(tabName, panelName);

                string assemblyPath = Assembly.GetExecutingAssembly().Location;

                PushButtonData syncBtn = new PushButtonData(
                    "SyncAxisGrid", "同步轴网", assemblyPath, "AxisGrid.Command");
                panel.AddItem(syncBtn);

                PushButtonData nodeBtn = new PushButtonData(
                    "EditAxisNode", "节点操作", assemblyPath, "AxisGrid.NodeEditCommand");
                nodeBtn.ToolTip = "在轴线上添加节点、移动节点、删除节点";
                panel.AddItem(nodeBtn);

                application.ControlledApplication.DocumentOpened += OnDocumentOpened;
                application.ControlledApplication.DocumentCreated += OnDocumentCreated;

                return Result.Succeeded;
            }
            catch (Exception ex)
            {
                TaskDialog.Show("轴网插件启动错误", ex.Message);
                return Result.Failed;
            }
        }

        private void OnDocumentOpened(object sender, Autodesk.Revit.DB.Events.DocumentOpenedEventArgs e)
        {
            RegisterUpdater(e.Document);
        }

        private void OnDocumentCreated(object sender, Autodesk.Revit.DB.Events.DocumentCreatedEventArgs e)
        {
            RegisterUpdater(e.Document);
        }

        private void RegisterUpdater(Document doc)
        {
            if (_updater == null)
                _updater = new AxisGridUpdater(doc.Application.ActiveAddInId);

            DocumentService.SetDocument(doc);
            _updater.Register();
        }

        public Result OnShutdown(UIControlledApplication application)
        {
            if (_updater != null)
            {
                _updater.Unregister();
                _updater = null;
            }
            return Result.Succeeded;
        }
    }
}
