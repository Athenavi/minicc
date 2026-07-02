"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import { api } from "@/lib/api";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import { Skeleton } from "@/components/ui/skeleton";
import { Toaster, toast } from "sonner";

type Step = "checking" | "ready" | "done" | "error";

export default function InstallPage() {
  const router = useRouter();
  const [step, setStep] = useState<Step>("checking");
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    api("/v1/install/status", { skipAuth: true })
      .then((d) => d.data?.needed ? setStep("ready") : router.push("/login"))
      .catch(() => setStep("error"));
  }, [router]);

  const strength = password.length < 6 ? 25 : password.length < 9 ? 50 : password.length < 12 ? 75 : 100;
  const strengthColor = password.length < 6 ? "bg-red-500" : password.length < 9 ? "bg-amber-500" : password.length < 12 ? "bg-blue-500" : "bg-green-500";

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name || !email || !password || password.length < 8) {
      toast.error("All fields required, password min 8 characters");
      return;
    }
    if (password !== confirmPassword) { toast.error("Passwords do not match"); return; }
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) { toast.error("Invalid email"); return; }

    setLoading(true);
    try {
      await api("/v1/install/setup", {
        method: "POST",
        body: JSON.stringify({ name, email, password }),
      });
      setStep("done");
      toast.success("System initialized!");
      setTimeout(() => router.push("/"), 1500);
    } catch (e: any) {
      toast.error(e.message);
    } finally {
      setLoading(false);
    }
  };

  if (step === "checking") return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 flex items-center justify-center p-4">
      <Skeleton className="h-80 w-full max-w-sm rounded-xl" />
    </div>
  );

  if (step === "error") return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 flex items-center justify-center p-4">
      <Card className="w-full max-w-sm text-center">
        <CardContent className="pt-6 space-y-4">
          <div className="text-4xl">⚠️</div>
          <p className="text-sm text-gray-500">Cannot connect to the backend.</p>
          <Button onClick={() => { setStep("checking"); }}>Retry</Button>
        </CardContent>
      </Card>
    </div>
  );

  if (step === "done") return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 flex items-center justify-center p-4">
      <Card className="w-full max-w-sm text-center">
        <CardContent className="pt-6 space-y-4">
          <div className="text-5xl">✅</div>
          <CardTitle>System initialized</CardTitle>
          <p className="text-sm text-gray-500">Redirecting to dashboard...</p>
          <Progress value={100} />
        </CardContent>
      </Card>
    </div>
  );

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 flex items-center justify-center p-4">
      <Toaster richColors />
      <Card className="w-full max-w-sm">
        <CardHeader className="text-center">
          <div className="text-3xl mb-2">⚡</div>
          <CardTitle>Initialize MiniCC</CardTitle>
          <CardDescription>Create the first administrator account</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="name">Name</Label>
              <Input id="name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Admin name" disabled={loading} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="email">Email</Label>
              <Input id="email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="admin@example.com" disabled={loading} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">Password</Label>
              <Input id="password" type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="Min 8 characters" disabled={loading} />
              {password && <Progress value={strength} className={strengthColor} />}
            </div>
            <div className="space-y-2">
              <Label htmlFor="confirm">Confirm password</Label>
              <Input id="confirm" type="password" value={confirmPassword} onChange={(e) => setConfirmPassword(e.target.value)} placeholder="Repeat password" disabled={loading} />
            </div>
            <Button type="submit" disabled={loading} className="w-full">
              {loading ? "Initializing..." : "Initialize system"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
