"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import { api, apiUrl } from "@/lib/api";

interface UserInfo {
  user_id: string;
  email: string;
  role: string;
  perms?: string[];
}

export default function ProfilePage() {
  const router = useRouter();
  const [user, setUser] = useState<UserInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    fetchProfile();
  }, [router]);

  const fetchProfile = async () => {
    try {
      const data = await api("/v1/profile");
      setUser(data.data);
    } catch {
      router.push("/login");
    } finally {
      setLoading(false);
    }
  };

  const handleLogout = async () => {
    try {
      await fetch(apiUrl("/v1/auth/logout"), {
        method: "POST",
        credentials: "include",
      });
    } catch {}
    router.push("/login");
  };

  if (loading) {
    return (
      <div className="min-h-screen bg-gray-50 dark:bg-gray-900 p-4">
        <div className="max-w-2xl mx-auto animate-pulse space-y-4">
          <div className="h-8 bg-gray-200 dark:bg-gray-700 rounded w-48" />
          <div className="h-32 bg-gray-200 dark:bg-gray-700 rounded-xl" />
        </div>
      </div>
    );
  }

  if (!user) {
    return (
      <div className="min-h-screen bg-gray-50 dark:bg-gray-900 flex items-center justify-center">
        <p className="text-gray-400">Not authenticated</p>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 p-4">
      <div className="max-w-2xl mx-auto">
        <h1 className="text-xl font-bold text-gray-800 dark:text-gray-100 mb-6">Profile</h1>

        {/* User card */}
        <div className="bg-white dark:bg-gray-800 rounded-xl p-6 shadow-sm border border-gray-200 dark:border-gray-700 mb-4">
          <div className="flex items-center gap-4 mb-4">
            <div className="w-12 h-12 rounded-full bg-blue-600 flex items-center justify-center text-white font-bold text-lg">
              {user.email?.charAt(0)?.toUpperCase() || "?"}
            </div>
            <div>
              <h2 className="font-semibold text-gray-800 dark:text-gray-100">{user.email}</h2>
              <p className="text-sm text-gray-500 dark:text-gray-400">{user.user_id}</p>
            </div>
            <span className={`ml-auto px-2.5 py-0.5 rounded-full text-[10px] font-medium ${
              user.role === "owner" ? "bg-purple-100 dark:bg-purple-900/30 text-purple-600" :
              user.role === "admin" ? "bg-blue-100 dark:bg-blue-900/30 text-blue-600" :
              "bg-gray-100 dark:bg-gray-700 text-gray-600"
            }`}>
              {user.role}
            </span>
          </div>
        </div>

        {/* Session card */}
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
