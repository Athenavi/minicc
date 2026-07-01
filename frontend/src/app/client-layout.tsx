"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

export default function ClientLayout({
  children,
  navItems,
}: {
  children: React.ReactNode;
  navItems: { href: string; label: string; desc: string }[];
}) {
  const pathname = usePathname();

  return (
    <>
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
      <main>{children}</main>
    </>
  );
}
