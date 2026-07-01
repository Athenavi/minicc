using System.Collections.Generic;

namespace FloorPlanGenerator.Models
{
    /// <summary>
    /// 顶层 JSON 结构
    /// </summary>
    public class RoomJson
    {
        public string Level { get; set; }
        public List<RoomData> Spaces { get; set; }
    }
}
