using System;
using System.Reflection;
using System.Runtime.InteropServices;
using Autodesk.AutoCAD.Runtime;

[assembly: AssemblyTitle("SeedFill")]
[assembly: AssemblyDescription("AutoCAD Plugin - 种子填充")]
[assembly: AssemblyCompany("")]
[assembly: AssemblyProduct("SeedFill")]
[assembly: AssemblyCopyright("Copyright 2025")]
[assembly: ComVisible(false)]
[assembly: Guid("B2C3D4E5-F6A7-8901-BCDE-F23456789012")]
[assembly: AssemblyVersion("1.0.0.0")]
[assembly: AssemblyFileVersion("1.0.0.0")]

namespace SeedFill
{
    /// <summary>
    /// AutoCAD 插件入口
    /// </summary>
    public class SeedFillApp : IExtensionApplication
    {
        public void Initialize()
        {
        }

        public void Terminate()
        {
        }
    }
}
