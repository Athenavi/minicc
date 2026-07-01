using System;
using System.Collections.Generic;
using Autodesk.Revit.DB;
using Autodesk.Revit.UI;
using AxisGrid.Services;

namespace AxisGrid.Updaters
{
    /// <summary>
    /// IUpdater 实现：监听 DetailLine 的增删改，自动同步轴网
    /// 注意：Revit 2026 中 IUpdater 需通过 RegisterUpdater 注册
    /// </summary>
    public class AxisGridUpdater : IUpdater
    {
        private readonly UpdaterId _updaterId;
        private const string UpdaterName = "AxisGridUpdater";
        private const string UpdaterGuid = "A1B2C3D4-E5F6-7890-ABCD-1234567890AB";

        // 防止循环变更
        private static bool _isProcessing = false;
        private static readonly HashSet<ElementId> _modifiedByUpdater = new HashSet<ElementId>();

        public AxisGridUpdater(AddInId addInId)
        {
            _updaterId = new UpdaterId(addInId, new Guid(UpdaterGuid));
        }

        public UpdaterId GetUpdaterId() => _updaterId;
        public string GetUpdaterName() => UpdaterName;
        public string GetAdditionalInformation() => "监听 DetailLine 变更，自动打断轴线并生成节点";

        public ChangePriority GetChangePriority() => ChangePriority.Annotations;

        /// <summary>
        /// 注册 Updater（可多次调用以刷新不同文档的触发器）
        /// </summary>
        public void Register()
        {
            Document doc = DocumentService.GetDocument();
            if (doc == null) return;

            try
            {
                // RegisterUpdater 只能在每个会话中调用一次
                if (!UpdaterRegistry.IsUpdaterRegistered(_updaterId, doc))
                {
                    UpdaterRegistry.RegisterUpdater(this, doc);
                }

                // 添加触发器（对当前文档生效）
                var filter = new ElementCategoryFilter(new ElementId(-2000051));
                UpdaterRegistry.AddTrigger(
                    _updaterId,
                    filter,
                    Element.GetChangeTypeAny());
            }
            catch { }
        }

        /// <summary>
        /// 取消注册
        /// </summary>
        public void Unregister()
        {
            try { UpdaterRegistry.UnregisterUpdater(_updaterId); }
            catch { }
        }

        public void Execute(UpdaterData data)
        {
            // 防止循环变更
            if (_isProcessing) return;

            Document doc = data.GetDocument();

            // 收集所有变更的 DetailLine
            var changedIds = new HashSet<ElementId>();
            foreach (ElementId id in data.GetAddedElementIds())
                if (!_modifiedByUpdater.Contains(id)) changedIds.Add(id);
            foreach (ElementId id in data.GetModifiedElementIds())
                if (!_modifiedByUpdater.Contains(id)) changedIds.Add(id);
            foreach (ElementId id in data.GetDeletedElementIds())
                changedIds.Add(id);

            if (changedIds.Count == 0) return;

            try
            {
                _isProcessing = true;

                // 获取当前视图（IUpdater 中可能没有 UI 上下文，直接用 Document.ActiveView）
                View activeView = doc.ActiveView;
                ViewPlan viewPlan = activeView as ViewPlan;
                if (viewPlan == null) return;

                // 重建轴网（IUpdater 已在事务中执行，不能另开事务）
                var service = new GridSyncService(doc, viewPlan);
                service.RebuildGraph(viewPlan);
            }
            catch (Exception ex)
            {
                // IUpdater 中不能弹 TaskDialog(也会启动事务)，静默记录错误
                // 可通过手动同步按钮查看错误
            }
            finally
            {
                _isProcessing = false;
                _modifiedByUpdater.Clear();
            }
        }
    }

    /// <summary>
    /// 辅助服务：获取活动的 Document
    /// </summary>
    internal static class DocumentService
    {
        private static Document _doc;

        public static void SetDocument(Document doc) => _doc = doc;

        public static Document GetDocument() => _doc;
    }
}
