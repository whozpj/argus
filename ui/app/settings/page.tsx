"use client";

import { Suspense, useRef, useState } from "react";
import { Check, AlertCircle } from "lucide-react";

import Shell, { useShell } from "@/components/Shell";
import { updateDisplayName } from "@/lib/api";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

function SettingsInner() {
  const { me } = useShell();
  const [name, setName] = useState(me.display_name ?? "");
  const [status, setStatus] = useState<"idle" | "saving" | "saved" | "error">("idle");
  const [errorMsg, setErrorMsg] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    if (name.length > 50) return;
    setStatus("saving");
    setErrorMsg("");
    try {
      const updated = await updateDisplayName(name);
      setName(updated.display_name ?? "");
      setStatus("saved");
      setTimeout(() => setStatus("idle"), 2500);
    } catch (err: unknown) {
      setStatus("error");
      setErrorMsg(err instanceof Error ? err.message : "Failed to save");
    }
  };

  return (
    <div className="max-w-lg space-y-6" data-testid="settings-root">
      <div>
        <h1 className="text-[22px] font-medium text-[#202124]">Settings</h1>
        <p className="text-sm text-[#5f6368] mt-0.5">Manage your profile information.</p>
      </div>

      <Card className="bg-white border-[#dadce0]">
        <CardHeader>
          <CardTitle className="text-sm font-medium text-[#202124]">Display name</CardTitle>
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
                className="flex h-9 w-full rounded-md border border-[#dadce0] bg-white px-3 py-1 text-sm text-[#202124] placeholder:text-[#5f6368] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[#1a73e8]"
              />
              <p className="text-xs text-[#5f6368]">
                Shown in the dashboard topbar instead of your email. Max 50 characters.
              </p>
            </div>

            {status === "saved" && (
              <div className="flex items-center gap-1.5 text-sm text-[#137333]" data-testid="save-success">
                <Check className="h-3.5 w-3.5" />
                Saved
              </div>
            )}
            {status === "error" && (
              <div className="flex items-center gap-1.5 text-sm text-red-600" data-testid="save-error">
                <AlertCircle className="h-3.5 w-3.5" />
                {errorMsg}
              </div>
            )}

            <button
              type="submit"
              disabled={status === "saving"}
              data-testid="save-button"
              className="inline-flex items-center justify-center h-9 px-4 rounded-md bg-[#1a73e8] text-white text-sm font-medium hover:bg-[#1765cc] disabled:opacity-50"
            >
              {status === "saving" ? "Saving…" : "Save"}
            </button>
          </form>
        </CardContent>
      </Card>

      <Card className="bg-white border-[#dadce0]">
        <CardHeader>
          <CardTitle className="text-sm font-medium text-[#202124]">Account</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-between text-sm">
            <span className="text-[#5f6368]">Email</span>
            <span data-testid="account-email" className="font-mono text-xs text-[#202124]">{me.email}</span>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

export default function SettingsPage() {
  return (
    <Suspense>
      <Shell>
        <SettingsInner />
      </Shell>
    </Suspense>
  );
}
