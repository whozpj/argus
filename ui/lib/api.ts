import type { BaselinesResponse, MeResponse } from "./types";

const SERVER = process.env.NEXT_PUBLIC_ARGUS_SERVER ?? "http://localhost:4000";

export function getToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem("argus_token");
}

export function logout(): void {
  if (typeof window !== "undefined") {
    localStorage.removeItem("argus_token");
  }
}

export async function fetchMe(): Promise<MeResponse> {
  const token = getToken();
  if (!token) throw new Error("not authenticated");

  const res = await fetch(`${SERVER}/api/v1/me`, {
    headers: { Authorization: `Bearer ${token}` },
  });

  if (res.status === 401) throw new Error("unauthorized");
  if (!res.ok) throw new Error(`/api/v1/me: ${res.status}`);
  return res.json();
}

export async function fetchBaselines(projectID?: string): Promise<BaselinesResponse> {
  const token = getToken();
  const headers: Record<string, string> = {};
  if (token) headers["Authorization"] = `Bearer ${token}`;

  const url = projectID
    ? `${SERVER}/api/v1/baselines?project_id=${encodeURIComponent(projectID)}`
    : `${SERVER}/api/v1/baselines`;

  const res = await fetch(url, { headers });
  if (!res.ok) throw new Error(`/api/v1/baselines: ${res.status}`);
  return res.json();
}
