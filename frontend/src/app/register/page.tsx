"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { apiUrl } from "@/lib/api";

export default function RegisterPage() {
  const router = useRouter();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [step, setStep] = useState<"form" | "success">("form");

  const validate = (): boolean => {
    if (!name.trim()) { setError("Name is required"); return false; }
    if (!email.trim()) { setError("Email is required"); return false; }
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) { setError("Invalid email format"); return false; }
    if (!password) { setError("Password is required"); return false; }
    if (password.length < 6) { setError("Password must be at least 6 characters"); return false; }
    if (password !== confirmPassword) { setError("Passwords do not match"); return false; }
    return true;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    if (!validate()) return;

    setLoading(true);
    try {
      const res = await fetch(apiUrl("/v1/auth/register"), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: name.trim(), email: email.trim(), password }),
      });
      const data = await res.json();
      if (!res.ok || !data.success) {
        setError(data.error || "Registration failed");
        return;
      }
      localStorage.setItem("minicc_token", data.data.token);
      localStorage.setItem("minicc_user", JSON.stringify(data.data.user));
      setStep("success");
      setTimeout(() => router.push("/"), 1500);
    } catch (err: any) {
      setError(`Connection failed: ${err.message}`);
    } finally {
      setLoading(false);
    }
  };

  if (step === "success") {
    return (
      <div className="min-h-screen bg-gray-50 dark:bg-gray-900 flex items-center justify-center p-4">
        <div className="bg-white dark:bg-gray-800 rounded-xl p-8 shadow-sm border border-gray-200 dark:border-gray-700 text-center max-w-sm w-full">
          <div className="text-5xl mb-4">✅</div>
          <h2 className="text-lg font-bold text-gray-800 dark:text-gray-100">Account created!</h2>
          <p className="text-sm text-gray-500 dark:text-gray-400 mt-2">Redirecting to dashboard...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 flex items-center justify-center p-4">
      <div className="w-full max-w-sm">
        <div className="text-center mb-8">
          <div className="text-4xl mb-2">🚀</div>
          <h1 className="text-xl font-bold text-gray-800 dark:text-gray-100">Create account</h1>
          <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">Get started with MiniCC V2</p>
        </div>

        <form onSubmit={handleSubmit} className="bg-white dark:bg-gray-800 rounded-xl p-6 shadow-sm border border-gray-200 dark:border-gray-700">
          <div className="space-y-3.5">
            <div>
              <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">Name</label>
              <input type="text" value={name} onChange={(e) => setName(e.target.value)}
                className="w-full px-3 py-2 text-sm border rounded-lg dark:bg-gray-700 dark:border-gray-600 dark:text-gray-100 focus:ring-2 focus:ring-blue-500 outline-none transition-all"
                placeholder="Your name" autoFocus disabled={loading} />
            </div>
            <div>
              <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">Email</label>
              <input type="email" value={email} onChange={(e) => setEmail(e.target.value)}
                className="w-full px-3 py-2 text-sm border rounded-lg dark:bg-gray-700 dark:border-gray-600 dark:text-gray-100 focus:ring-2 focus:ring-blue-500 outline-none transition-all"
                placeholder="you@example.com" disabled={loading} />
            </div>
            <div>
              <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">Password</label>
              <input type="password" value={password} onChange={(e) => setPassword(e.target.value)}
                className="w-full px-3 py-2 text-sm border rounded-lg dark:bg-gray-700 dark:border-gray-600 dark:text-gray-100 focus:ring-2 focus:ring-blue-500 outline-none transition-all"
                placeholder="Min 6 characters" disabled={loading} />
            </div>
            <div>
              <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">Confirm password</label>
              <input type="password" value={confirmPassword} onChange={(e) => setConfirmPassword(e.target.value)}
                className="w-full px-3 py-2 text-sm border rounded-lg dark:bg-gray-700 dark:border-gray-600 dark:text-gray-100 focus:ring-2 focus:ring-blue-500 outline-none transition-all"
                placeholder="Repeat password" disabled={loading} />
            </div>
          </div>

          {/* Password strength indicator */}
          {password && (
            <div className="mt-3">
              <div className="flex gap-1 h-1">
                {[1, 2, 3].map((level) => (
                  <div key={level} className={`flex-1 rounded-full transition-all ${
                    password.length >= level * 3 ? (
                      password.length >= 9 ? "bg-green-500" : password.length >= 6 ? "bg-amber-500" : "bg-red-500"
                    ) : "bg-gray-200 dark:bg-gray-600"
                  }`} />
                ))}
              </div>
              <p className="text-[10px] text-gray-400 mt-1">
                {password.length < 6 ? "Weak" : password.length < 9 ? "Medium" : "Strong"}
              </p>
            </div>
          )}

          {error && (
            <div className="mt-3 p-2.5 bg-red-50 dark:bg-red-900/30 border border-red-200 dark:border-red-800 rounded-lg text-xs text-red-600 dark:text-red-400">
              {error}
            </div>
          )}

          <button type="submit" disabled={loading}
            className="w-full mt-4 px-4 py-2.5 bg-blue-600 text-white text-sm font-medium rounded-lg hover:bg-blue-700 disabled:opacity-50 transition-all flex items-center justify-center gap-2">
            {loading ? (
              <><span className="inline-block w-3.5 h-3.5 border-2 border-white/30 border-t-white rounded-full animate-spin" /> Creating...</>
            ) : "Create account"}
          </button>
        </form>

        <p className="text-center text-xs text-gray-500 dark:text-gray-400 mt-4">
          Already have an account?{" "}
          <Link href="/login" className="text-blue-600 dark:text-blue-400 hover:underline font-medium">
            Sign in
          </Link>
        </p>
      </div>
    </div>
  );
}
