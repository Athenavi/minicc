"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useState, useEffect } from "react";

export default function ClientLayout({
  children,
  navItems,
}: {
  children: React.ReactNode;
  navItems: { href: string; label: string; desc: string }[];
}) {
  const pathname = usePathname();
  const router = useRouter();
  const [token, setToken] = useState<string | null>(null);
  const [userName, setUserName] = useState("");

  useEffect(() => {
    setToken(localStorage.getItem("minicc_token"));
    const stored = localStorage.getItem("minicc_user");
    if (stored) {
      try { setUserName(JSON.parse(stored).name || ""); } catch {}
    }
  }, [pathname]);

  // Hide nav on auth pages
  const isAuthPage = pathname === "/login" || pathname === "/register";
  if (isAuthPage) return <>{children}</>;

  const handleLogout = () => {
    localStorage.removeItem("minicc_token");
    localStorage.removeItem("minicc_user");
    router.push("/login");
  };

  return (
    <>
      <nav className="bg-white dark:bg-gray-800 border-b dark:border-gray-700 shadow-sm sticky top-0 z-50">
        <div className="max-w-7xl mx-auto px-4">
          <div className="flex items-center gap-1 overflow-x-auto py-2">
            <Link href="/" className="text-sm font-bold text-blue-600 dark:text-blue-400 mr-3 whitespace-nowrap hover:opacity-80 transition-opacity">
              MiniCC
            </Link>
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
              </Link>
            ))}

            {/* Auth - right side */}
            <div className="ml-auto flex items-center gap-2 shrink-0">
              {token ? (
                <>
                  <Link href="/profile"
                    className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-medium transition-all ${
                      pathname === "/profile"
                        ? "bg-blue-600 text-white shadow"
                        : "text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
                    }`}>
                    <span className="w-5 h-5 rounded-full bg-blue-600 text-white flex items-center justify-center text-[10px] font-bold">
                      {userName?.charAt(0)?.toUpperCase() || "?"}
                    </span>
                    <span className="hidden sm:inline">{userName || "Profile"}</span>
                  </Link>
                  <button onClick={handleLogout}
                    className="px-2 py-1.5 text-xs text-gray-400 hover:text-red-500 transition-colors">
                    ✕
                  </button>
                </>
              ) : (
                <Link href="/login"
                  className="px-3 py-1.5 bg-blue-600 text-white text-xs font-medium rounded-md hover:bg-blue-700 transition-all whitespace-nowrap">
                  Sign in
                </Link>
              )}
            </div>
          </div>
        </div>
      </nav>
      <main>{children}</main>
    </>
  );
}
