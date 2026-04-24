"use client";

import { useEffect, useRef, Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { Activity } from "lucide-react";

const SERVER = process.env.NEXT_PUBLIC_ARGUS_SERVER ?? "http://localhost:4000";

function CallbackInner() {
  const router = useRouter();
  const params = useSearchParams();
  const ran = useRef(false);

  useEffect(() => {
    // Strict-mode guard — useEffect fires twice in dev; only run once.
    if (ran.current) return;
    ran.current = true;

    const code = params.get("code");
    if (!code) {
      router.replace("/login");
      return;
    }

    fetch(`${SERVER}/api/v1/auth/token`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ code }),
    })
      .then((res) => (res.ok ? res.json() : Promise.reject(res.status)))
      .then((data: { token: string }) => {
        localStorage.setItem("argus_token", data.token);
        router.replace("/dashboard");
      })
      .catch(() => {
        router.replace("/login");
      });
  }, [params, router]);

  return (
    <div className="min-h-screen bg-background flex flex-col items-center justify-center gap-4">
      <div className="inline-flex items-center justify-center w-12 h-12 rounded-xl bg-primary text-primary-foreground animate-pulse">
        <Activity className="h-6 w-6" />
      </div>
      <p className="text-sm text-muted-foreground">Signing you in…</p>
    </div>
  );
}

export default function AuthCallbackPage() {
  return (
    <Suspense
      fallback={
        <div className="min-h-screen bg-background flex items-center justify-center">
          <p className="text-sm text-muted-foreground">Loading…</p>
        </div>
      }
    >
      <CallbackInner />
    </Suspense>
  );
}
