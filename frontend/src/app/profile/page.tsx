"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";

interface UserInfo {
  id: string;
  email: string;
  name: string;
  role: string;
}

export default function ProfilePage() {
  const router = useRouter();
  const [user, setUser] = useState<UserInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [copied, setCopied] = useState(false);
  const [showToken, setShowToken] = useState(false);

  useEffect(() => {
    const stored = localStorage.getItem("minicc_user");
    const token = localStorage.getItem("minicc_token");
    if (!token) {
      router.push("/login");
      return;
    }
    if (stored) {
      setUser(JSON.parse(stored));
    }
    setLoading(false);
  }, [router]);

  const handleLogout = () => {
    localStorage.removeItem("minicc_token");
    localStorage.removeItem("minicc_user");
    router.push("/login");
  };

  const copyToken = () => {
    const token = localStorage.getItem("minicc_token") || "";
    navigator.clipboard.writeText(token);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  if (loading) {
    return (
      <div className="min-h-screen bg-gray-50 dark:bg-gray-900 p-4">
        <div className="max-w-2xl mx-auto">
          <div className="animate-pulse space-y-4">
            <div className="h-8 bg-gray-200 dark:bg-gray-700 rounded w-48" />
            <div className="h-32 bg-gray-200 dark:bg-gray-700 rounded-xl" />
            <div className="h-24 bg-gray-200 dark:bg-gray-700 rounded-xl" />
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 p-4">
      <div className="max-w-2xl mx-auto">
        <h1 className="text-xl font-bold text-gray-800 dark:text-gray-100 mb-6">Profile</h1>

        {/* User info card */}
        <div className="bg-white dark:bg-gray-800 rounded-xl p-6 shadow-sm border border-gray-200 dark:border-gray-700 mb-4">
          <div className="flex items-center gap-4 mb-4">
            <div className="w-12 h-12 rounded-full bg-blue-600 flex items-center justify-center text-white font-bold text-lg">
              {user?.name?.charAt(0)?.toUpperCase() || "?"}
            </div>
            <div>
              <h2 className="font-semibold text-gray-800 dark:text-gray-100">{user?.name || "User"}</h2>
              <p className="text-sm text-gray-500 dark:text-gray-400">{user?.email || "—"}</p>
            </div>
            <span className={`ml-auto px-2.5 py-0.5 rounded-full text-[10px] font-medium ${
              user?.role === "owner" ? "bg-purple-100 dark:bg-purple-900/30 text-purple-600 dark:text-purple-400" :
              user?.role === "admin" ? "bg-blue-100 dark:bg-blue-900/30 text-blue-600 dark:text-blue-400" :
              "bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-400"
            }`}>
              {user?.role || "user"}
            </span>
          </div>

          <div className="grid grid-cols-2 gap-3 text-sm">
            <div className="p-3 bg-gray-50 dark:bg-gray-700/50 rounded-lg">
              <p className="text-[10px] text-gray-400 uppercase tracking-wider">User ID</p>
              <p className="font-mono text-xs text-gray-700 dark:text-gray-300 mt-0.5">{user?.id || "—"}</p>
            </div>
            <div className="p-3 bg-gray-50 dark:bg-gray-700/50 rounded-lg">
              <p className="text-[10px] text-gray-400 uppercase tracking-wider">Role</p>
              <p className="text-xs text-gray-700 dark:text-gray-300 mt-0.5 capitalize">{user?.role || "—"}</p>
            </div>
          </div>
        </div>

        {/* Token card */}
        <div className="bg-white dark:bg-gray-800 rounded-xl p-6 shadow-sm border border-gray-200 dark:border-gray-700 mb-4">
          <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-3">🔑 Access Token</h3>
          <div className="relative">
            <input
              type={showToken ? "text" : "password"}
              value={localStorage.getItem("minicc_token") || ""}
              readOnly
              className="w-full px-3 py-2 text-xs font-mono border rounded-lg dark:bg-gray-700 dark:border-gray-600 dark:text-gray-100 pr-20"
            />
            <div className="absolute right-1 top-1 flex gap-1">
              <button onClick={() => setShowToken(!showToken)}
                className="px-2 py-1 text-[10px] bg-gray-100 dark:bg-gray-600 rounded hover:bg-gray-200 dark:hover:bg-gray-500 text-gray-600 dark:text-gray-300">
                {showToken ? "Hide" : "Show"}
              </button>
              <button onClick={copyToken}
                className="px-2 py-1 text-[10px] bg-blue-100 dark:bg-blue-900/30 text-blue-600 dark:text-blue-400 rounded hover:bg-blue-200 dark:hover:bg-blue-900/50">
                {copied ? "Copied!" : "Copy"}
              </button>
            </div>
          </div>
          <p className="text-[10px] text-gray-400 mt-2">Use this token in the Authorization header for API requests.</p>
        </div>

        {/* Actions */}
        <div className="flex items-center justify-between bg-white dark:bg-gray-800 rounded-xl p-4 shadow-sm border border-gray-200 dark:border-gray-700">
          <div>
            <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-300">Session</h3>
            <p className="text-xs text-gray-400 mt-0.5">Sign out of your account</p>
          </div>
          <button onClick={handleLogout}
            className="px-4 py-2 bg-red-600 text-white text-sm font-medium rounded-lg hover:bg-red-700 transition-colors">
            Sign out
          </button>
        </div>
      </div>
    </div>
  );
}
