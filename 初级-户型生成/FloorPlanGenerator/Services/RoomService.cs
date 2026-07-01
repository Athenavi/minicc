using System.Collections.Generic;
using System.Linq;
using Autodesk.Revit.DB;
using Autodesk.Revit.DB.Architecture;
using FloorPlanGenerator.Models;

namespace FloorPlanGenerator.Services
{
    /// <summary>
    /// 房间与房间标记生成服务
    /// </summary>
    public class RoomService
    {
        private readonly Document _doc;

        public RoomService(Document doc)
        {
            _doc = doc;
        }

        /// <summary>
        /// 在指定的楼层平面视图中创建房间和房间标记
        /// </summary>
        public List<Element> CreateRoomsAndTags(
            List<RoomData> roomsData,
            Level level,
            ViewPlan viewPlan)
        {
            var results = new List<Element>();

            using (Transaction t = new Transaction(_doc, "创建房间与标记"))
            {
                t.Start();

                foreach (var roomData in roomsData)
                {
                    // 在房间中心点放置房间
                    XYZ point = new XYZ(
                        roomData.CentroidX.MmToFeet(),
                        roomData.CentroidY.MmToFeet(),
                        0);

                    Room room = _doc.Create.NewRoom(level, new UV(point.X, point.Y));
                    if (room != null)
                    {
                        // 通过参数设置房间名称（支持中文）
                        room.get_Parameter(BuiltInParameter.ROOM_NAME).Set(roomData.Name);
                        results.Add(room);

                    // 创建房间标记
                        RoomTag tag = _doc.Create.NewRoomTag(
                            new LinkElementId(room.Id),
                            new UV(point.X, point.Y),
                            viewPlan.Id);
                        if (tag != null)
                            results.Add(tag);
                    }
                }

                t.Commit();
            }

            return results;
        }

        /// <summary>
        /// 删除视图中现有的房间和标记
        /// </summary>
        public void DeleteExistingRooms(ViewPlan viewPlan)
        {
            FilteredElementCollector collector = new FilteredElementCollector(_doc, viewPlan.Id);

            var roomTags = collector
                .OfClass(typeof(SpatialElementTag))
                .Cast<SpatialElementTag>()
                .ToList();

            var rooms = new FilteredElementCollector(_doc)
                .OfClass(typeof(SpatialElement))
                .Cast<SpatialElement>()
                .Where(r => r.LevelId == viewPlan.GenLevel.Id && r is Room)
                .ToList();

            List<ElementId> idsToDelete = new List<ElementId>();
            idsToDelete.AddRange(roomTags.Select(r => r.Id));
            idsToDelete.AddRange(rooms.Select(r => r.Id));

            if (idsToDelete.Count > 0)
            {
                _doc.Delete(idsToDelete);
            }
        }
    }
}
