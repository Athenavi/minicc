using System;
using System.Linq;
using System.Reflection;

class Test {
    static void Main() {
        try {
            var asm = Assembly.LoadFrom(@"D:\cad\AutoCAD 2027\acmgd.dll");
            Console.WriteLine("Loaded: " + asm.FullName);
            Console.WriteLine("Types with CommandMethod:");
            foreach (var t in asm.GetExportedTypes()) {
                if (t.FullName.IndexOf("CommandMethod", StringComparison.OrdinalIgnoreCase) >= 0) {
                    Console.WriteLine("  " + t.FullName);
                }
            }
        } catch (Exception ex) {
            Console.WriteLine("Error: " + ex.Message);
            if (ex.InnerException != null) Console.WriteLine("  Inner: " + ex.InnerException.Message);
        }
    }
}
