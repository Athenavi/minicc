"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import { api } from "@/lib/api";

type Step = "checking" | "ready" | "done" | "error";

export default function InstallPage() {
  const router = useRouter();
  const [step, setStep] = useState<Step>("checking");
  const [statusMsg, setStatusMsg] = useState("");
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    checkStatus();
  }, []);

  const checkStatus = async () => {
    try {
      const data = await api("/v1/install/status", { skipAuth: true });
      if (data.data?.needed) {
        setStep("ready");
        setStatusMsg(data.data.reason || "System needs initialization");
      } else {
        // Already initialized, redirect to login
        router.push("/login");
      }
    } catch {
      setStep("error");
      setStatusMsg("Cannot connect to backend");
    }
  };

  const validate = (): boolean => {
    if (!name.trim()) { setError("Name is required"); return false; }
    if (!email.trim()) { setError("Email is required"); return false; }
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) { setError("Invalid email"); return false; }
    if (!password) { setError("Password is required"); return false; }
    if (password.length < 8) { setError("Password must be at least 8 characters"); return false; }
    if (password !== confirmPassword) { setError("Passwords do not match"); return false; }
    return true;
  };

  const handleSetup = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    if (!validate()) return;

    setLoading(true);
    try {
      const data = await api("/v1/install/setup", {
        method: "POST",
        body: JSON.stringify({ name: name.trim(), email: email.trim(), password }),
      });
      setStep("done");
      setStatusMsg(`Admin user "${data.data?.user?.email}" created`);
      setTimeout(() => router.push("/"), 2000);
    } catch (err: any) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  // Checking state
  if (step === "checking") {
    return (
      <div className="min-h-screen bg-gray-50 dark:bg-gray-900 flex items-center justify-center p-4">
        <div className="text-center">
          <div className="w-8 h-8 border-4 border-blue-600 border-t-transparent rounded-full animate-spin mx-auto mb-4" />
          <p className="text-sm text-gray-500 dark:text-gray-400">Checking system status...</p>
        </div>
      </div>
    );
  }

  // Error state
  if (step === "error") {
    return (
      <div className="min-h-screen bg-gray-50 dark:bg-gray-900 flex items-center justify-center p-4">
        <div className="bg-white dark:bg-gray-800 rounded-xl p-8 shadow-sm border border-gray-200 dark:border-gray-700 text-center max-w-sm w-full">
          <div className="text-4xl mb-3">⚠️</div>
          <h2 className="text-lg font-bold text-gray-800 dark:text-gray-100">Connection error</h2>
          <p className="text-sm text-gray-500 dark:text-gray-400 mt-2">Cannot connect to the backend. Make sure the server is running.</p>
          <button onClick={checkStatus}
            className="mt-4 px-4 py-2 bg-blue-600 text-white text-sm font-medium rounded-lg hover:bg-blue-700 transition-colors">
            Retry
          </button>
        </div>
      </div>
    );
  }

  // Done state
  if (step === "done") {
    return (
      <div className="min-h-screen bg-gray-50 dark:bg-gray-900 flex items-center justify-center p-4">
        <div className="bg-white dark:bg-gray-800 rounded-xl p-8 shadow-sm border border-gray-200 dark:border-gray-700 text-center max-w-sm w-full">
          <div className="text-5xl mb-3">✅</div>
          <h2 className="text-lg font-bold text-gray-800 dark:text-gray-100">System initialized</h2>
          <p className="text-sm text-gray-500 dark:text-gray-400 mt-2">{statusMsg}</p>
          <p className="text-xs text-gray-400 mt-2">Redirecting to dashboard...</p>
        </div>
      </div>
    );
  }

  // Setup form
  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 flex items-center justify-center p-4">
      <div className="w-full max-w-sm">
        <div className="text-center mb-8">
          <div className="text-4xl mb-2">⚡</div>
          <h1 className="text-xl font-bold text-gray-800 dark:text-gray-100">Initialize MiniCC</h1>
          <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">Create the first administrator account</p>
        </div>

        <form onSubmit={handleSetup} className="bg-white dark:bg-gray-800 rounded-xl p-6 shadow-sm border border-gray-200 dark:border-gray-700">
          <div className="space-y-3.5">
            <div>
              <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">Name</label>
              <input type="text" value={name} onChange={(e) => setName(e.target.value)}
                className="w-full px-3 py-2 text-sm border rounded-lg dark:bg-gray-700 dark:border-gray-600 dark:text-gray-100 focus:ring-2 focus:ring-blue-500 outline-none"
                placeholder="Admin name" autoFocus disabled={loading} />
            </div>
            <div>
              <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">Email</label>
              <input type="email" value={email} onChange={(e) => setEmail(e.target.value)}
                className="w-full px-3 py-2 text-sm border rounded-lg dark:bg-gray-700 dark:border-gray-600 dark:text-gray-100 focus:ring-2 focus:ring-blue-500 outline-none"
                placeholder="admin@example.com" disabled={loading} />
            </div>
            <div>
              <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">Password</label>
              <input type="password" value={password} onChange={(e) => setPassword(e.target.value)}
                className="w-full px-3 py-2 text-sm border rounded-lg dark:bg-gray-700 dark:border-gray-600 dark:text-gray-100 focus:ring-2 focus:ring-blue-500 outline-none"
                placeholder="Min 8 characters" disabled={loading} />
            </div>
            <div>
              <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">Confirm password</label>
              <input type="password" value={confirmPassword} onChange={(e) => setConfirmPassword(e.target.value)}
                className="w-full px-3 py-2 text-sm border rounded-lg dark:bg-gray-700 dark:border-gray-600 dark:text-gray-100 focus:ring-2 focus:ring-blue-500 outline-none"
                placeholder="Repeat password" disabled={loading} />
            </div>
          </div>

          {password && (
            <div className="mt-3 flex gap-1 h-1">
              {[1, 2, 3].map((l) => (
                <div key={l} className={`flex-1 rounded-full transition-all ${
                  password.length >= l * 3
                    ? password.length >= 9 ? "bg-green-500" : password.length >= 6 ? "bg-amber-500" : "bg-red-500"
                    : "bg-gray-200 dark:bg-gray-600"
                }`} />
              ))}
            </div>
          )}

          {error && (
            <div className="mt-3 p-2.5 bg-red-50 dark:bg-red-900/30 border border-red-200 dark:border-red-800 rounded-lg text-xs text-red-600 dark:text-red-400">
              {error}
            </div>
          )}

          {statusMsg && statusMsg !== "System needs initialization" && (
            <div className="mt-3 p-2.5 bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-lg text-xs text-amber-600 dark:text-amber-400">
              {statusMsg}
            </div>
          )}

          <button type="submit" disabled={loading}
            className="w-full mt-4 px-4 py-2.5 bg-blue-600 text-white text-sm font-medium rounded-lg hover:bg-blue-700 disabled:opacity-50 transition-all flex items-center justify-center gap-2">
            {loading ? (
              <><span className="w-3.5 h-3.5 border-2 border-white/30 border-t-white rounded-full animate-spin" /> Initializing...</>
            ) : "Initialize system"}
          </button>
        </form>
      </div>
    </div>
  );
}
