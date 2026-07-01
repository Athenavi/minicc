"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

const navItems = [
  { href: "/", label: "💬 Chat", desc: "AI Chat" },
  { href: "/editor", label: "✏️ Editor", desc: "AI Code Editor" },
  { href: "/workflow", label: "🔀 Workflow", desc: "Visual Graph" },
  { href: "/rpa", label: "🤖 RPA", desc: "Automation" },
  { href: "/enterprise", label: "🏢 Enterprise", desc: "CRM/ERP" },
  { href: "/devops", label: "🚀 DevOps", desc: "CI/CD" },
  { href: "/system", label: "🔬 System", desc: "Self-Monitor" },
];

export default function RootLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900">
      {/* Top Navigation */}
      <nav className="bg-white dark:bg-gray-800 border-b dark:border-gray-700 shadow-sm sticky top-0 z-50">
        <div className="max-w-7xl mx-auto px-4">
          <div className="flex items-center gap-1 overflow-x-auto py-2">
            <span className="text-sm font-bold text-blue-600 dark:text-blue-400 mr-3 whitespace-nowrap">MiniCC</span>
            {navItems.map((item) => (
              <Link
                key={item.href}
                href={item.href}
                className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-medium whitespace-nowrap transition-all ${
                  pathname === item.href
                    ? "bg-blue-600 text-white shadow"
                    : "text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
                }`}
              >
                <span>{item.label}</span>
                <span className="hidden sm:inline opacity-70">· {item.desc}</span>
              </Link>
            ))}
          </div>
        </div>
      </nav>

      {/* Page Content */}
      <main>{children}</main>
    </div>
  );
}
