using System;
using System.Collections.Generic;
using System.IO;
using System.Text.RegularExpressions;
using FloorPlanGenerator.Models;

namespace FloorPlanGenerator.Services
{
    /// <summary>
    /// JSON 文件读取与解析服务（手动解析，不依赖 System.Web.Extensions）
    /// </summary>
    public class JsonReader
    {
        public RoomJson ReadFromFile(string filePath)
        {
            string jsonContent = File.ReadAllText(filePath);
            return Parse(jsonContent);
        }

        public RoomJson Parse(string jsonContent)
        {
            var result = new RoomJson { Spaces = new List<RoomData>() };

            // 提取 Level 值
            Match levelMatch = Regex.Match(jsonContent, @"""Level""\s*:\s*""([^""]*)""");
            if (levelMatch.Success)
                result.Level = levelMatch.Value.Split(':')[1].Trim().Trim('"');

            // 提取每个 Spaces 对象
            string spacesContent = ExtractArrayContent(jsonContent, "Spaces");
            if (string.IsNullOrEmpty(spacesContent))
                return result;

            // 分割每个房间对象
            var roomBlocks = SplitObjectBlocks(spacesContent);

            foreach (var block in roomBlocks)
            {
                var room = new RoomData();
                room.CentroidX = ExtractDouble(block, "CentroidX");
                room.CentroidY = ExtractDouble(block, "CentroidY");
                room.Bay = ExtractDouble(block, "Bay");
                room.Depth = ExtractDouble(block, "Depth");
                room.Name = ExtractString(block, "Name");
                result.Spaces.Add(room);
            }

            return result;
        }

        private string ExtractArrayContent(string json, string arrayName)
        {
            Match match = Regex.Match(json, @"""" + arrayName + @"""\s*:\s*\[");
            if (!match.Success) return null;

            int start = match.Index + match.Length;
            int depth = 1;
            int i = start;
            while (i < json.Length && depth > 0)
            {
                if (json[i] == '[') depth++;
                else if (json[i] == ']') depth--;
                i++;
            }
            return json.Substring(start, i - start - 1);
        }

        private List<string> SplitObjectBlocks(string content)
        {
            var blocks = new List<string>();
            int i = 0;
            while (i < content.Length)
            {
                // 跳过空白和逗号
                while (i < content.Length && (content[i] == ' ' || content[i] == '\r' || content[i] == '\n' || content[i] == '\t' || content[i] == ','))
                    i++;

                if (i >= content.Length || content[i] != '{') break;

                int start = i;
                int depth = 0;
                while (i < content.Length)
                {
                    if (content[i] == '{') depth++;
                    else if (content[i] == '}') { depth--; if (depth == 0) { i++; break; } }
                    i++;
                }
                blocks.Add(content.Substring(start, i - start));
            }
            return blocks;
        }

        private double ExtractDouble(string block, string key)
        {
            Match match = Regex.Match(block, @"""" + key + @"""\s*:\s*(-?[\d.]+)");
            if (match.Success && double.TryParse(match.Groups[1].Value, out double val))
                return val;
            return 0;
        }

        private string ExtractString(string block, string key)
        {
            Match match = Regex.Match(block, @"""" + key + @"""\s*:\s*""([^""]*)""");
            return match.Success ? match.Groups[1].Value : "";
        }
    }
}
