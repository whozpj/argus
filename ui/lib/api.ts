import type { BaselinesResponse } from "./types";

const SERVER = process.env.NEXT_PUBLIC_ARGUS_SERVER ?? "http://localhost:4000";

export async function fetchBaselines(): Promise<BaselinesResponse> {
  const res = await fetch(`${SERVER}/api/v1/baselines`, {
    next: { revalidate: 10 }, // refresh every 10 seconds (Next.js cache)
  });
  if (!res.ok) throw new Error(`baselines: ${res.status}`);
  return res.json();
}
