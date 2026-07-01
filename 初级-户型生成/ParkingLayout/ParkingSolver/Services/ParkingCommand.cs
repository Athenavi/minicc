using System;
using System.Runtime.InteropServices;

namespace ParkingSolver.Services
{
    /// <summary>COM 接口</summary>
    [ComVisible(true)]
    [Guid("F6A7B8C9-D0E1-2345-6789-012345678901")]
    public interface IParkingCommand
    {
        string Execute();
    }

    /// <summary>车位排布命令入口</summary>
    [ComVisible(true)]
    [Guid("E5F6A7B8-C9D0-1234-EF56-789012345678")]
    [ProgId("ParkingSolver.Command")]
    [ClassInterface(ClassInterfaceType.AutoDual)]
    public class ParkingCommand : IParkingCommand
    {
        public string Execute()
        {
            try
            {
                var solver = new ParkingSolverService();
                solver.Run();
                return "车位排布完成";
            }
            catch (Exception ex)
            {
                return $"错误: {ex.Message}";
            }
        }
    }
}
