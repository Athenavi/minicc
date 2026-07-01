using Autodesk.Revit.DB;

namespace FloorPlanGenerator.Services
{
    /// <summary>
    /// 单位转换扩展方法
    /// </summary>
    public static class UnitExtensions
    {
        /// <summary>毫米 → Revit 内部单位（英尺）</summary>
        public static double MmToFeet(this double mm) => mm / 304.8;
    }
}
