"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import { api, apiUrl } from "@/lib/api";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Toaster, toast } from "sonner";

export default function ProfilePage() {
  const router = useRouter();
  const [user, setUser] = useState<any>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api("/v1/profile", { skipAuth: true })
      .then((d) => { setUser(d.data); })
      .catch(() => router.push("/login"))
      .finally(() => setLoading(false));
  }, [router]);

  const handleLogout = async () => {
    await fetch(apiUrl("/v1/auth/logout"), { method: "POST", credentials: "include" });
    router.push("/login");
  };

  if (loading) {
    return (
      <div className="min-h-screen bg-gray-50 dark:bg-gray-900 p-4">
        <div className="max-w-lg mx-auto space-y-4">
          <Skeleton className="h-8 w-48" />
          <Skeleton className="h-40 rounded-xl" />
          <Skeleton className="h-16 rounded-xl" />
        </div>
      </div>
    );
  }

  if (!user) return null;

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 p-4">
      <Toaster richColors />
      <div className="max-w-lg mx-auto space-y-4">
        <h1 className="text-xl font-bold text-gray-800 dark:text-gray-100">Profile</h1>

        <Card>
          <CardHeader>
            <div className="flex items-center gap-4">
              <Avatar className="h-12 w-12">
                <AvatarFallback className="bg-blue-600 text-white text-lg">
                  {user.email?.charAt(0)?.toUpperCase() || "?"}
                </AvatarFallback>
              </Avatar>
              <div className="flex-1">
                <CardTitle className="text-base">{user.email}</CardTitle>
                <p className="text-sm text-gray-500 dark:text-gray-400">{user.user_id}</p>
              </div>
              <Badge variant={user.role === "owner" ? "default" : "secondary"}>
                {user.role}
              </Badge>
            </div>
          </CardHeader>
        </Card>

        <Card>
          <CardContent className="flex items-center justify-between p-4">
            <div>
              <p className="text-sm font-medium">Session</p>
              <p className="text-xs text-gray-500">Sign out of your account</p>
            </div>
            <Button variant="destructive" size="sm" onClick={handleLogout}>
              Sign out
            </Button>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
