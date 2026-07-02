const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

interface FetchOptions extends RequestInit {
  skipAuth?: boolean;
}

export async function api(path: string, options: FetchOptions = {}): Promise<any> {
  const { skipAuth, ...fetchOpts } = options;
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(fetchOpts.headers as Record<string, string>),
  };

  if (!skipAuth) {
    const token = localStorage.getItem("minicc_token");
    if (token) {
      headers["Authorization"] = `Bearer ${token}`;
    }
  }

  const res = await fetch(`${API_BASE}${path}`, {
    ...fetchOpts,
    headers,
  });

  if (res.status === 401 && !skipAuth) {
    localStorage.removeItem("minicc_token");
    localStorage.removeItem("minicc_user");
    if (typeof window !== "undefined") {
      window.location.href = "/login";
    }
    throw new Error("Unauthorized");
  }

  // SSE responses return plain text
  const contentType = res.headers.get("content-type") || "";
  if (contentType.includes("text/event-stream")) {
    return res;
  }

  const data = await res.json();

  if (!res.ok) {
    throw new Error(data.error || `HTTP ${res.status}`);
  }

  return data;
}

export function apiUrl(path: string): string {
  return `${API_BASE}${path}`;
}
