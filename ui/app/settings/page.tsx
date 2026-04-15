"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Activity, ArrowLeft, Check, AlertCircle } from "lucide-react";
import { fetchMe, updateDisplayName } from "@/lib/api";
import type { MeResponse } from "@/lib/types";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

export default function SettingsPage() {
  const router = useRouter();
  const [me, setMe] = useState<MeResponse | null>(null);
  const [name, setName] = useState("");
  const [status, setStatus] = useState<"idle" | "saving" | "saved" | "error">("idle");
  const [errorMsg, setErrorMsg] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    fetchMe()
      .then((data) => {
        setMe(data);
        setName(data.display_name ?? "");
      })
      .catch(() => router.replace("/login"));
  }, [router]);

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    if (name.length > 50) return; // client-side guard
    setStatus("saving");
    setErrorMsg("");
    try {
      const updated = await updateDisplayName(name);
      setMe(updated);
      setName(updated.display_name ?? "");
      setStatus("saved");
      setTimeout(() => setStatus("idle"), 2500);
    } catch (err: unknown) {
      setStatus("error");
      setErrorMsg(err instanceof Error ? err.message : "Failed to save");
    }
  };

  if (!me) {
    return (
      <div className="min-h-screen bg-background flex items-center justify-center">
        <p className="text-sm text-muted-foreground">Loading…</p>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-background">
      {/* Header */}
      <header className="border-b">
        <div className="max-w-2xl mx-auto px-6 py-4 flex items-center gap-3">
          <button
            onClick={() => router.back()}
            className="inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors"
            aria-label="Go back"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <Activity className="h-5 w-5 text-primary" />
          <span className="font-semibold text-sm tracking-tight">Argus</span>
          <span className="text-muted-foreground text-sm">/</span>
          <span className="text-sm">Settings</span>
        </div>
      </header>

      {/* Content */}
      <main className="max-w-2xl mx-auto px-6 py-8 space-y-6">
        <div>
          <h1 className="text-lg font-semibold">Account</h1>
          <p className="text-sm text-muted-foreground mt-1">
            Manage your profile information.
          </p>
        </div>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Display name</CardTitle>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleSave} className="space-y-4" data-testid="settings-form">
              <div className="space-y-1.5">
                <input
                  ref={inputRef}
                  type="text"
                  value={name}
                  onChange={(e) => {
                    setName(e.target.value);
                    if (status !== "idle") setStatus("idle");
                  }}
                  placeholder="Your name"
                  maxLength={50}
                  data-testid="display-name-input"
                  className="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-sm transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                />
                <p className="text-xs text-muted-foreground">
                  Shown in the dashboard header instead of your email. Max 50 characters.
                </p>
              </div>

              {/* Feedback */}
              {status === "saved" && (
                <div className="flex items-center gap-1.5 text-sm text-green-600" data-testid="save-success">
                  <Check className="h-3.5 w-3.5" />
                  Saved
                </div>
              )}
              {status === "error" && (
                <div className="flex items-center gap-1.5 text-sm text-destructive" data-testid="save-error">
                  <AlertCircle className="h-3.5 w-3.5" />
                  {errorMsg}
                </div>
              )}

              <button
                type="submit"
                disabled={status === "saving"}
                data-testid="save-button"
                className="inline-flex items-center justify-center rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground shadow transition-colors hover:bg-primary/90 disabled:opacity-50"
              >
                {status === "saving" ? "Saving…" : "Save"}
              </button>
            </form>
          </CardContent>
        </Card>

        {/* Account info */}
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Account</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            <div className="flex items-center justify-between text-sm">
              <span className="text-muted-foreground">Email</span>
              <span data-testid="account-email" className="font-mono text-xs">{me.email}</span>
            </div>
          </CardContent>
        </Card>
      </main>
    </div>
  );
}
