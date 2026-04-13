# Argus Cloud — Plan 4: Dashboard (Login + Project Selector)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add authentication and multi-project support to the Next.js dashboard. Cloud users log in via GitHub/Google OAuth, select a project from a dropdown, and see per-project drift data.

**Architecture:**

1. After OAuth the Go server creates a one-time code (reusing `oauth_sessions`) and redirects to `${ARGUS_UI_URL}/auth/callback?code=<code>`.
2. The UI callback page exchanges the code for a JWT via `POST /api/v1/auth/token` (already exists).
3. The JWT is stored in `localStorage` and sent as `Authorization: Bearer <token>` on all API calls.
4. A project selector dropdown in the dashboard header stores the selected project ID in the URL search param `?project=<id>`.
5. `GET /api/v1/baselines` already reads `projectID` from context. The `ResolveProject` middleware is extended to also accept a JWT + `?project_id=<id>` query param (with ownership validation), in addition to the existing API-key path.
6. CORS headers are added so the browser can call the server from a different origin (port 3000 → 4000) in development.

**Tech Stack:** Go (server changes), Next.js 14 App Router, TypeScript, shadcn/ui.

---

## File Map

| File | Action | What changes |
|---|---|---|
| `server/cmd/main.go` | Modify | Add `ARGUS_UI_URL` env var; pass to `OAuthConfig` |
| `server/internal/auth/auth_handlers.go` | Modify | `OAuthConfig` gains `UIURL`; `issueJWTAndRedirect` creates code + redirects to UI |
| `server/internal/auth/middleware.go` | Modify | Add `corsMiddleware`; extend `ResolveProject` with JWT + `?project_id` path |
| `server/internal/store/db.go` | Modify | Add `OwnsProject(userID, projectID string) (bool, error)` |
| `ui/app/login/page.tsx` | Create | Login page: GitHub + Google OAuth buttons |
| `ui/app/auth/callback/page.tsx` | Create | Client component: exchanges code → stores JWT → redirects to `/` |
| `ui/lib/api.ts` | Modify | `fetchBaselines(projectID?)` sends `Authorization: Bearer`; add `fetchMe()` |
| `ui/app/page.tsx` | Modify | Auth check → redirect to `/login`; project selector; pass `project_id` to fetch |

---

## Task 1: Server — `ARGUS_UI_URL` + redirect to UI after OAuth

**Files:**
- Modify: `server/cmd/main.go`
- Modify: `server/internal/auth/auth_handlers.go`

**Context:**

Currently `issueJWTAndRedirect` sets an `argus_token` httpOnly cookie and redirects to `/` (the Go server root at port 4000). This doesn't reach the UI on port 3000. The fix: always redirect via a one-time code to the UI URL, the same way the CLI flow already works.

The `argus_token` cookie is kept so that `GET /auth/cli` can detect an existing session and skip re-auth.

- [ ] **Step 1: Add `UIURL` to `OAuthConfig` in `auth_handlers.go`**

In `server/internal/auth/auth_handlers.go`, add `UIURL string` to `OAuthConfig`:

```go
type OAuthConfig struct {
	BaseURL            string
	UIURL              string // e.g. "http://localhost:3000" — where to redirect after web OAuth
	GitHubClientID     string
	GitHubClientSecret string
	GoogleClientID     string
	GoogleClientSecret string
}
```

Then update `issueJWTAndRedirect` to redirect to the UI via code instead of setting a cookie redirect:

```go
func (h *authHandlers) issueJWTAndRedirect(w http.ResponseWriter, r *http.Request, userID string) {
	tok, err := IssueToken(userID)
	if err != nil {
		slog.Error("issue token", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	// Keep the cookie so /auth/cli can reuse an existing session.
	http.SetCookie(w, &http.Cookie{
		Name:     "argus_token",
		Value:    tok,
		Path:     "/",
		MaxAge:   30 * 24 * 3600,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	// CLI login flow — create a code and send it to the CLI's local callback.
	if c, err := r.Cookie("argus_cli_redirect"); err == nil {
		http.SetCookie(w, &http.Cookie{Name: "argus_cli_redirect", MaxAge: -1, Path: "/"})
		h.createSessionCodeAndRedirect(w, r, userID, c.Value)
		return
	}
	// Web login flow — create a one-time code and send the browser to the UI callback.
	uiURL := h.cfg.UIURL
	if uiURL == "" {
		uiURL = "http://localhost:3000"
	}
	h.createSessionCodeAndRedirect(w, r, userID, uiURL+"/auth/callback")
}
```

- [ ] **Step 2: Wire `ARGUS_UI_URL` in `main.go`**

In `server/cmd/main.go`, read the env var and populate `OAuthConfig`:

```go
oauthCfg := auth.OAuthConfig{
	BaseURL:            getenv("ARGUS_BASE_URL", "http://localhost:4000"),
	UIURL:              getenv("ARGUS_UI_URL", "http://localhost:3000"),
	GitHubClientID:     getenv("GITHUB_CLIENT_ID", ""),
	GitHubClientSecret: getenv("GITHUB_CLIENT_SECRET", ""),
	GoogleClientID:     getenv("GOOGLE_CLIENT_ID", ""),
	GoogleClientSecret: getenv("GOOGLE_CLIENT_SECRET", ""),
}
```

- [ ] **Step 3: Build the server to verify no compile errors**

```bash
cd /Users/prithviraj/Documents/CS/argus/server && /opt/homebrew/bin/go build ./...
```

Expected: exits 0, no output.

- [ ] **Step 4: Commit**

```bash
cd /Users/prithviraj/Documents/CS/argus
git add server/cmd/main.go server/internal/auth/auth_handlers.go
git commit -m "feat(auth): redirect to UI via one-time code after OAuth"
```

---

## Task 2: Server — CORS + JWT-scoped baselines

**Files:**
- Modify: `server/internal/auth/middleware.go`
- Modify: `server/internal/store/db.go`
- Modify: `server/cmd/main.go`

**Context:**

The browser on port 3000 calls the API on port 4000 — cross-origin. We need CORS headers so the browser doesn't block these requests.

For project-scoped baselines: currently `ResolveProject` only handles `argus_sk_` API keys. For the web UI, the user selects a project and sends `?project_id=<id>` with a JWT. We extend `ResolveProject` to handle this case.

- [ ] **Step 1: Add `OwnsProject` to the store**

In `server/internal/store/db.go`, add:

```go
// OwnsProject returns true if userID is the owner of projectID.
func (db *DB) OwnsProject(userID, projectID string) (bool, error) {
	var count int
	err := db.pool.QueryRow(
		`SELECT COUNT(*) FROM projects WHERE id = $1 AND user_id = $2`,
		projectID, userID,
	).Scan(&count)
	return count > 0, err
}
```

- [ ] **Step 2: Add CORS middleware and extend `ResolveProject` in `middleware.go`**

Add a `CORSMiddleware` function and update `ResolveProject` to handle JWT + `?project_id`:

```go
// CORSMiddleware adds permissive CORS headers for the Argus dashboard.
// In production this should be restricted to the UI origin.
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

Update `ResolveProject` to add a third resolution path — JWT + `?project_id`:

```go
func ResolveProject(db *store.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			projectID := "self-hosted"

			authHeader := r.Header.Get("Authorization")

			// Path 1: API key — argus_sk_* prefix.
			if strings.HasPrefix(authHeader, "Bearer argus_sk_") {
				rawKey := strings.TrimPrefix(authHeader, "Bearer ")
				hash := HashAPIKey(rawKey)
				pid, ok, err := db.ResolveAPIKey(hash)
				if err != nil {
					http.Error(w, "internal error", http.StatusInternalServerError)
					return
				}
				if !ok {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				projectID = pid

			// Path 2: JWT + ?project_id — web dashboard users.
			} else if strings.HasPrefix(authHeader, "Bearer ") && r.URL.Query().Get("project_id") != "" {
				tok := strings.TrimPrefix(authHeader, "Bearer ")
				userID, err := ValidateToken(tok)
				if err == nil {
					pid := r.URL.Query().Get("project_id")
					owns, err := db.OwnsProject(userID, pid)
					if err != nil {
						http.Error(w, "internal error", http.StatusInternalServerError)
						return
					}
					if !owns {
						http.Error(w, "forbidden", http.StatusForbidden)
						return
					}
					projectID = pid
				}
			}

			ctx := context.WithValue(r.Context(), contextKeyProjectID, projectID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
```

- [ ] **Step 3: Wire CORS in `main.go`**

Wrap the mux with `CORSMiddleware`:

```go
handler := auth.CORSMiddleware(mux)

slog.Info("argus server starting", "addr", addr, "dsn", dsn)
if err := http.ListenAndServe(addr, handler); err != nil {
    slog.Error("server error", "err", err)
    os.Exit(1)
}
```

Also remove the inline `Access-Control-Allow-Origin: *` from `server/internal/api/baselines.go` (line 86) — CORS is now handled by the middleware.

- [ ] **Step 4: Build and verify**

```bash
cd /Users/prithviraj/Documents/CS/argus/server && /opt/homebrew/bin/go build ./...
```

- [ ] **Step 5: Commit**

```bash
cd /Users/prithviraj/Documents/CS/argus
git add server/internal/auth/middleware.go server/internal/store/db.go server/cmd/main.go server/internal/api/baselines.go
git commit -m "feat(server): add CORS middleware and JWT-scoped baselines"
```

---

## Task 3: UI — Login page

**Files:**
- Create: `ui/app/login/page.tsx`

**Context:**

A simple page with two buttons: "Continue with GitHub" and "Continue with Google". Each links directly to the server's OAuth endpoint. The server URL comes from `NEXT_PUBLIC_ARGUS_SERVER`.

- [ ] **Step 1: Create `ui/app/login/page.tsx`**

```tsx
export default function LoginPage() {
  const server = process.env.NEXT_PUBLIC_ARGUS_SERVER ?? "http://localhost:4000";

  return (
    <div className="min-h-screen bg-background flex items-center justify-center">
      <div className="w-full max-w-sm space-y-6 p-8">
        <div className="text-center space-y-2">
          <h1 className="text-2xl font-semibold tracking-tight">Argus</h1>
          <p className="text-sm text-muted-foreground">LLM Drift Monitor</p>
        </div>

        <div className="space-y-3">
          <a
            href={`${server}/auth/github`}
            className="flex w-full items-center justify-center gap-3 rounded-md border bg-background px-4 py-2 text-sm font-medium shadow-sm hover:bg-muted transition-colors"
          >
            <GitHubIcon />
            Continue with GitHub
          </a>
          <a
            href={`${server}/auth/google`}
            className="flex w-full items-center justify-center gap-3 rounded-md border bg-background px-4 py-2 text-sm font-medium shadow-sm hover:bg-muted transition-colors"
          >
            <GoogleIcon />
            Continue with Google
          </a>
        </div>
      </div>
    </div>
  );
}

function GitHubIcon() {
  return (
    <svg viewBox="0 0 16 16" className="h-4 w-4 fill-current" aria-hidden="true">
      <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z" />
    </svg>
  );
}

function GoogleIcon() {
  return (
    <svg viewBox="0 0 24 24" className="h-4 w-4" aria-hidden="true">
      <path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z" />
      <path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" />
      <path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z" />
      <path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" />
    </svg>
  );
}
```

- [ ] **Step 2: TypeCheck**

```bash
cd /Users/prithviraj/Documents/CS/argus/ui && npm run typecheck 2>&1 | tail -5
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd /Users/prithviraj/Documents/CS/argus
git add ui/app/login/page.tsx
git commit -m "feat(ui): add login page with GitHub and Google OAuth buttons"
```

---

## Task 4: UI — Auth callback page

**Files:**
- Create: `ui/app/auth/callback/page.tsx`

**Context:**

After OAuth the server redirects to `/auth/callback?code=<code>`. This client component exchanges the code for a JWT via `POST /api/v1/auth/token`, stores the token in `localStorage`, and redirects to `/`.

- [ ] **Step 1: Create `ui/app/auth/callback/page.tsx`**

```tsx
"use client";

import { useEffect, useRef } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense } from "react";

function CallbackInner() {
  const router = useRouter();
  const params = useSearchParams();
  const ran = useRef(false);

  useEffect(() => {
    if (ran.current) return;
    ran.current = true;

    const code = params.get("code");
    if (!code) {
      router.replace("/login");
      return;
    }

    const server = process.env.NEXT_PUBLIC_ARGUS_SERVER ?? "http://localhost:4000";

    fetch(`${server}/api/v1/auth/token`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ code }),
    })
      .then((r) => (r.ok ? r.json() : Promise.reject(r.status)))
      .then((data: { token: string }) => {
        localStorage.setItem("argus_token", data.token);
        router.replace("/");
      })
      .catch(() => {
        router.replace("/login");
      });
  }, [params, router]);

  return (
    <div className="min-h-screen flex items-center justify-center text-sm text-muted-foreground">
      Signing in…
    </div>
  );
}

export default function AuthCallbackPage() {
  return (
    <Suspense>
      <CallbackInner />
    </Suspense>
  );
}
```

- [ ] **Step 2: TypeCheck**

```bash
cd /Users/prithviraj/Documents/CS/argus/ui && npm run typecheck 2>&1 | tail -5
```

- [ ] **Step 3: Commit**

```bash
cd /Users/prithviraj/Documents/CS/argus
git add ui/app/auth/callback/page.tsx
git commit -m "feat(ui): add auth callback page for OAuth code exchange"
```

---

## Task 5: UI — Auth-aware API + project selector in dashboard

**Files:**
- Modify: `ui/lib/api.ts`
- Modify: `ui/app/page.tsx`

**Context:**

`lib/api.ts` needs two changes:
1. A `fetchMe()` function (calls `GET /api/v1/me` with JWT, returns user + projects).
2. `fetchBaselines(projectID?)` — passes `Authorization: Bearer <token>` and `?project_id=<id>` when a project is selected.

`page.tsx` needs to become a client component to:
1. Read the JWT from `localStorage` on mount.
2. If no token → redirect to `/login`.
3. Call `fetchMe()` to get user + projects.
4. Show a project selector in the header.
5. Read selected project from URL `?project=<id>` search param.
6. Fetch baselines for the selected project.

- [ ] **Step 1: Update `ui/lib/api.ts`**

Replace the full content:

```typescript
import type { BaselinesResponse, MeResponse } from "./types";

const SERVER = process.env.NEXT_PUBLIC_ARGUS_SERVER ?? "http://localhost:4000";

function getToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem("argus_token");
}

export async function fetchMe(): Promise<MeResponse> {
  const token = getToken();
  if (!token) throw new Error("not authenticated");
  const res = await fetch(`${SERVER}/api/v1/me`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (res.status === 401) throw new Error("unauthorized");
  if (!res.ok) throw new Error(`me: ${res.status}`);
  return res.json();
}

export async function fetchBaselines(projectID?: string): Promise<BaselinesResponse> {
  const token = getToken();
  const headers: Record<string, string> = {};
  if (token) headers["Authorization"] = `Bearer ${token}`;

  let url = `${SERVER}/api/v1/baselines`;
  if (projectID) url += `?project_id=${encodeURIComponent(projectID)}`;

  const res = await fetch(url, { headers });
  if (!res.ok) throw new Error(`baselines: ${res.status}`);
  return res.json();
}

export function logout(): void {
  localStorage.removeItem("argus_token");
}
```

- [ ] **Step 2: Add `MeResponse` to `ui/lib/types.ts`**

Open `ui/lib/types.ts` and add:

```typescript
export type Project = {
  id: string;
  name: string;
  created_at: string;
};

export type MeResponse = {
  id: string;
  email: string;
  projects: Project[];
};
```

- [ ] **Step 3: Rewrite `ui/app/page.tsx` as a client component**

Replace the full content of `ui/app/page.tsx`:

```tsx
"use client";

import { useEffect, useState, useCallback } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense } from "react";
import { fetchMe, fetchBaselines, logout } from "@/lib/api";
import type { BaselinesResponse, MeResponse } from "@/lib/types";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select";
import { Activity, AlertTriangle, CheckCircle, Database, RefreshCw } from "lucide-react";
import type { BaselineModel } from "@/lib/types";

function DashboardInner() {
  const router = useRouter();
  const params = useSearchParams();
  const selectedProject = params.get("project") ?? "";

  const [me, setMe] = useState<MeResponse | null>(null);
  const [data, setData] = useState<BaselinesResponse | null>(null);
  const [fetchError, setFetchError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  // Auth check on mount
  useEffect(() => {
    fetchMe()
      .then((m) => {
        setMe(m);
        // If no project selected yet and user has projects, default to first
        if (!params.get("project") && m.projects.length > 0) {
          const url = new URL(window.location.href);
          url.searchParams.set("project", m.projects[0].id);
          router.replace(url.pathname + url.search);
        }
      })
      .catch(() => {
        router.replace("/login");
      });
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // Fetch baselines when project changes
  const loadBaselines = useCallback(() => {
    setFetchError(null);
    fetchBaselines(selectedProject || undefined)
      .then(setData)
      .catch((e: Error) => setFetchError(e.message))
      .finally(() => setLoading(false));
  }, [selectedProject]);

  useEffect(() => {
    if (me) loadBaselines();
  }, [me, loadBaselines]);

  const handleProjectChange = (id: string) => {
    const url = new URL(window.location.href);
    url.searchParams.set("project", id);
    router.push(url.pathname + url.search);
  };

  const handleLogout = () => {
    logout();
    router.replace("/login");
  };

  const alertedModels = data?.baselines.filter((b) => b.drift_alerted) ?? [];
  const checkedModels = data?.baselines.filter((b) => b.drift_score > 0 || b.drift_alerted) ?? [];
  const currentProjectName = me?.projects.find((p) => p.id === selectedProject)?.name;

  return (
    <div className="min-h-screen bg-background">
      <header className="border-b">
        <div className="max-w-6xl mx-auto px-6 py-4 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <Activity className="h-6 w-6 text-primary" />
            <h1 className="text-xl font-semibold tracking-tight">Argus</h1>
            <span className="text-sm text-muted-foreground">LLM Drift Monitor</span>
          </div>
          <div className="flex items-center gap-3">
            {me && me.projects.length > 0 && (
              <Select value={selectedProject} onValueChange={handleProjectChange}>
                <SelectTrigger className="w-48">
                  <SelectValue placeholder="Select project" />
                </SelectTrigger>
                <SelectContent>
                  {me.projects.map((p) => (
                    <SelectItem key={p.id} value={p.id}>
                      {p.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
            {me && (
              <button
                onClick={handleLogout}
                className="text-xs text-muted-foreground hover:text-foreground transition-colors"
              >
                Sign out
              </button>
            )}
          </div>
        </div>
      </header>

      <main className="max-w-6xl mx-auto px-6 py-8 space-y-6">
        {loading ? (
          <div className="text-sm text-muted-foreground">Loading…</div>
        ) : fetchError ? (
          <ErrorBanner message={fetchError} />
        ) : data ? (
          <>
            {currentProjectName && (
              <h2 className="text-sm font-medium text-muted-foreground">
                Project: <span className="text-foreground">{currentProjectName}</span>
              </h2>
            )}

            {alertedModels.length > 0 && (
              <div className="rounded-lg border border-yellow-400/60 bg-yellow-50 dark:bg-yellow-950/20 p-4 space-y-2">
                <div className="flex items-center gap-2 text-yellow-700 dark:text-yellow-400 font-semibold">
                  <AlertTriangle className="h-5 w-5" />
                  Drift detected on {alertedModels.length} model{alertedModels.length > 1 ? "s" : ""}
                </div>
                <ul className="space-y-1">
                  {alertedModels.map((b) => (
                    <li key={b.model} className="text-sm text-yellow-800 dark:text-yellow-300 font-mono">
                      {b.model} — score {b.drift_score.toFixed(2)}
                      {" · "}p(tokens) {b.p_output_tokens.toFixed(4)}
                      {" · "}p(latency) {b.p_latency_ms.toFixed(4)}
                    </li>
                  ))}
                </ul>
              </div>
            )}

            {checkedModels.length > 0 && alertedModels.length === 0 && (
              <div className="rounded-lg border border-green-400/60 bg-green-50 dark:bg-green-950/20 p-3 flex items-center gap-2 text-green-700 dark:text-green-400 text-sm">
                <CheckCircle className="h-4 w-4" />
                All models within baseline — no drift detected
              </div>
            )}

            <div className="grid grid-cols-1 sm:grid-cols-4 gap-4">
              <SummaryCard title="Total Events" value={data.total_events.toLocaleString()} icon={<Database className="h-4 w-4 text-muted-foreground" />} />
              <SummaryCard title="Models Tracked" value={data.baselines.length} icon={<Activity className="h-4 w-4 text-muted-foreground" />} />
              <SummaryCard title="Baselines Ready" value={data.baselines.filter((b) => b.is_ready).length} icon={<RefreshCw className="h-4 w-4 text-muted-foreground" />} />
              <SummaryCard title="Alerts Active" value={alertedModels.length} icon={<AlertTriangle className={`h-4 w-4 ${alertedModels.length > 0 ? "text-yellow-500" : "text-muted-foreground"}`} />} />
            </div>

            <Card>
              <CardHeader>
                <CardTitle className="text-base">Models</CardTitle>
              </CardHeader>
              <CardContent className="p-0">
                {data.baselines.length === 0 ? <EmptyState /> : <BaselineTable baselines={data.baselines} />}
              </CardContent>
            </Card>
          </>
        ) : null}
      </main>
    </div>
  );
}

export default function DashboardPage() {
  return (
    <Suspense>
      <DashboardInner />
    </Suspense>
  );
}

function SummaryCard({ title, value, icon }: { title: string; value: number | string; icon: React.ReactNode }) {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">{title}</CardTitle>
        {icon}
      </CardHeader>
      <CardContent>
        <p className="text-2xl font-bold">{value}</p>
      </CardContent>
    </Card>
  );
}

function DriftScoreBar({ score }: { score: number }) {
  const pct = Math.round(score * 100);
  const color = score >= 0.7 ? "bg-yellow-500" : score >= 0.4 ? "bg-orange-400" : "bg-green-500";
  return (
    <div className="flex items-center gap-2">
      <div className="w-20 h-2 rounded-full bg-muted overflow-hidden">
        <div className={`h-full rounded-full ${color}`} style={{ width: `${pct}%` }} />
      </div>
      <span className="text-xs tabular-nums">{score.toFixed(2)}</span>
    </div>
  );
}

function BaselineTable({ baselines }: { baselines: BaselineModel[] }) {
  const hasAnyDriftData = baselines.some((b) => b.drift_score > 0 || b.drift_alerted);
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Model</TableHead>
          <TableHead className="text-right">Events</TableHead>
          <TableHead className="text-right">Mean Tokens</TableHead>
          <TableHead className="text-right">Mean Latency</TableHead>
          <TableHead className="text-center">Baseline</TableHead>
          {hasAnyDriftData && <TableHead className="text-center">Drift Score</TableHead>}
          <TableHead className="text-center">Status</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {baselines.map((b) => (
          <TableRow key={b.model} className={b.drift_alerted ? "bg-yellow-50/50 dark:bg-yellow-950/10" : ""}>
            <TableCell className="font-mono text-sm">{b.model}</TableCell>
            <TableCell className="text-right">{b.count.toLocaleString()}</TableCell>
            <TableCell className="text-right">
              {b.mean_output_tokens}<span className="text-muted-foreground text-xs"> ±{b.stddev_output_tokens}</span>
            </TableCell>
            <TableCell className="text-right">
              {b.mean_latency_ms} ms<span className="text-muted-foreground text-xs"> ±{b.stddev_latency_ms}</span>
            </TableCell>
            <TableCell className="text-center">
              {b.is_ready ? <Badge variant="default">Ready</Badge> : <Badge variant="secondary">{b.count}/200</Badge>}
            </TableCell>
            {hasAnyDriftData && (
              <TableCell className="text-center">
                {b.drift_score > 0 || b.drift_alerted ? <DriftScoreBar score={b.drift_score} /> : <span className="text-xs text-muted-foreground">—</span>}
              </TableCell>
            )}
            <TableCell className="text-center">
              {b.drift_alerted ? (
                <Badge variant="destructive" className="bg-yellow-500 hover:bg-yellow-500 text-white">⚠ Drift</Badge>
              ) : b.is_ready && b.drift_score > 0 ? (
                <Badge variant="outline" className="text-green-600 border-green-400">OK</Badge>
              ) : b.is_ready ? (
                <Badge variant="outline">Monitoring</Badge>
              ) : (
                <Badge variant="secondary">Warming up</Badge>
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

function EmptyState() {
  return (
    <div className="py-16 text-center text-muted-foreground space-y-2">
      <p className="text-sm">No events received yet.</p>
      <p className="text-xs">Send events using the SDK with your project's API key.</p>
    </div>
  );
}

function ErrorBanner({ message }: { message: string }) {
  return (
    <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-sm text-destructive">
      <strong>Error:</strong> {message}
    </div>
  );
}
```

> **Note on Select component:** `shadcn/ui` `Select` must be installed. Check if it exists in `ui/components/ui/select.tsx`. If not, run:
> ```bash
> cd /Users/prithviraj/Documents/CS/argus/ui && npx shadcn@latest add select
> ```

- [ ] **Step 4: Check if shadcn Select is installed; add it if not**

```bash
ls /Users/prithviraj/Documents/CS/argus/ui/components/ui/select.tsx 2>/dev/null && echo "exists" || (cd /Users/prithviraj/Documents/CS/argus/ui && npx shadcn@latest add select --yes)
```

- [ ] **Step 5: TypeCheck**

```bash
cd /Users/prithviraj/Documents/CS/argus/ui && npm run typecheck 2>&1
```

Fix any type errors before continuing.

- [ ] **Step 6: Commit**

```bash
cd /Users/prithviraj/Documents/CS/argus
git add ui/lib/api.ts ui/lib/types.ts ui/app/page.tsx ui/components/ui/select.tsx
git commit -m "feat(ui): auth-aware dashboard with project selector"
```

---

## Task 6: End-to-end verification

> This task requires a running Postgres instance and GitHub/Google OAuth app credentials.

- [ ] **Step 1: Start Postgres**

```bash
docker run --rm -p 5432:5432 \
  -e POSTGRES_USER=argus -e POSTGRES_PASSWORD=argus -e POSTGRES_DB=argus \
  postgres:15-alpine
```

- [ ] **Step 2: Start the server (second terminal)**

```bash
cd /Users/prithviraj/Documents/CS/argus/server
POSTGRES_URL="postgres://argus:argus@localhost:5432/argus?sslmode=disable" \
GITHUB_CLIENT_ID=<id> GITHUB_CLIENT_SECRET=<secret> \
ARGUS_BASE_URL=http://localhost:4000 \
ARGUS_UI_URL=http://localhost:3000 \
/opt/homebrew/bin/go run ./cmd/main.go
```

- [ ] **Step 3: Start the UI (third terminal)**

```bash
cd /Users/prithviraj/Documents/CS/argus/ui && npm run dev
```

- [ ] **Step 4: Open `http://localhost:3000`**

Expected: redirected to `/login`. Two buttons visible: Continue with GitHub, Continue with Google.

- [ ] **Step 5: Click Continue with GitHub**

Expected: redirected to GitHub → authorize → back to `/auth/callback?code=<code>` → "Signing in…" → redirected to `/` with project dropdown in header.

- [ ] **Step 6: Create a project via curl**

```bash
TOKEN=$(cat ~/.config/argus/credentials.json | python3 -c "import json,sys; print(json.load(sys.stdin)['token'])")
curl -s -X POST http://localhost:4000/api/v1/projects \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"test-project"}' | python3 -m json.tool
```

Then refresh the dashboard — new project should appear in the dropdown.

- [ ] **Step 7: Run `argus status`**

```bash
argus status
```

Expected: prints your email and server URL (CLI login still works via existing code exchange flow).

- [ ] **Step 8: Update `docs/cloud.md`**

Mark Plan 3 done (already done), mark Plan 4 done, add Plan 5 stub.

```bash
cd /Users/prithviraj/Documents/CS/argus
git add docs/cloud.md
git commit -m "docs: mark Plan 4 done, stub Plan 5"
```
