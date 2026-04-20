# Frontend Redesign — Design Spec

**Date:** 2026-04-20  
**Status:** Approved

---

## Goal

Replace the current vibe-coded Next.js UI with a GCP Console-style design. Add a public landing page with animated demos. Add a multi-page docs section inside the dashboard sidebar.

---

## Routing changes

| Route | Before | After |
|---|---|---|
| `/` | Dashboard (redirects to `/login` if not authed) | Landing page (public, no auth) |
| `/dashboard` | — | Dashboard (redirects to `/login` if not authed) |
| `/login` | Login page | Login page (restyled) |
| `/auth/callback` | OAuth callback | Unchanged |
| `/settings` | Settings page | Settings page (restyled, inside shell) |
| `/docs/[slug]` | — | Docs viewer (inside shell, 6 pages) |

Auth callbacks (server-side at `/auth/github/callback`, `/auth/google/callback`) already redirect to `ARGUS_UI_URL/auth/callback` which then stores the JWT and redirects to `/`. That redirect must change to `/dashboard`.

---

## Design system

**GCP Console palette:**
- Topbar background: `#202124`
- Page background: `#f1f3f4`
- Sidebar background: `#ffffff`
- Sidebar border: `#e0e0e0`
- Active nav item: background `#e8f0fe`, text `#1a73e8`
- Primary blue: `#1a73e8`
- Body text: `#202124`
- Secondary text: `#5f6368`
- Alert amber: `#f59e0b` / `#fef3c7`
- Success green: `#34a853`

**Typography:** `system-ui` (already available). No new font imports needed.

**No new shadcn components.** Use existing: `badge`, `button`, `card`, `dropdown-menu`, `select`, `table`. The GCP look comes from colors and layout, not new component libraries.

---

## Shared shell (dashboard + settings + docs)

A new `ui/components/Shell.tsx` client component wraps all authenticated pages. It renders:

- **Topbar** (fixed, `h-14`, `bg-[#202124]`):
  - Left: ⚡ logo + "Argus" wordmark (white)
  - Center-left: project selector `<Select>` (shows current project name, lists all projects)
  - Right: user email + logout button
- **Sidebar** (fixed left, `w-40`, `bg-white`, `border-r`):
  - Nav items: Overview → `/dashboard`, Models → `/dashboard?tab=models`, Alerts → `/dashboard?tab=alerts`, Settings → `/settings`, Docs → `/docs/quickstart`
  - Active item highlighted in blue
- **Content area** (fills remaining space, `bg-[#f1f3f4]`, scrollable)

Shell handles its own auth check: calls `fetchMe()` on mount, redirects to `/login` on 401. Removes the per-page auth boilerplate in dashboard and settings.

The project selector in the topbar replaces the current inline dropdown in `page.tsx`. Selecting a project updates the URL `?project=<id>` and the baselines data.

---

## Landing page (`/`)

Public — no auth, no shell. Standalone page.

### Sections (top to bottom):

**Nav** (sticky, white, `border-b`):
- Logo + "Argus"
- Links: Docs → `/docs/quickstart`, Pricing (anchor `#pricing`), GitHub → external
- Right: Sign in → `/login`, Get started free → `/login` (primary button)

**Hero** (two-column):
- Left: badge "Now in public beta", headline "Know when your LLM **behavior changes** before users do", subtext, two CTAs (Start for free → `/login`, Read the docs → `/docs/quickstart`)
- Right: animated terminal showing `pip install argus-sdk` → `patch(api_key=...)` → `⚠ DRIFT — output_tokens +42%`

**Stats bar** (four columns, `border-y`):
- 1 line to instrument / 60s drift check interval / 98% detection rate at +20% shift / 0% prompt data stored

**Dashboard preview section** (`bg-[#f8f9fa]`):
- Label: "SEE IT IN ACTION — LIVE DASHBOARD"
- Animated dashboard mockup (not the real app — a static HTML+SVG replica):
  - GCP shell (topbar + sidebar + content)
  - Summary cards (99k events / 3 models / 1 active alert / 42s last checked)
  - Chart: blue line draws in steadily → spikes amber at drift marker → "drift detected" label appears → amber fill reveals → Slack toast slides in from bottom-right
  - Slack toast: "#alerts · Argus · just now — ⚠ Drift detected on claude-sonnet-4-6, output_tokens +42%, score: 0.84"
  - Replay button below

**Features grid** (6 cards, 3-column):
1. Statistical drift detection — Mann-Whitney U + Bonferroni
2. Zero prompt exposure — only derived signals collected
3. Non-blocking instrumentation — background thread, zero latency added
4. Slack alerts — hysteresis, fires once, clears cleanly
5. Multi-model support — Anthropic, OpenAI, any compatible provider
6. Self-host or cloud — Docker container or Argus Cloud

**CTA section** (centered):
- "Start monitoring your LLMs today" / "Free to start. No credit card required." / "Create free account" → `/login`

**Footer**: logo, links (Docs, GitHub, Privacy)

### Animation implementation

The dashboard preview is a self-contained `<DashboardDemo>` client component. It uses CSS animations triggered on mount (IntersectionObserver fires once when section enters viewport):

1. `0.3s` delay: baseline band fades in
2. `0.3s–1.7s`: blue `<polyline>` draws via `stroke-dashoffset` animation
3. `1.9s`: vertical drift marker line fades in
4. `1.9s–2.8s`: amber `<polyline>` draws via `stroke-dashoffset`
5. `2.1s`: "drift detected" text label fades in
6. `2.4s`: drift badge (`output_tokens +42% above baseline`) pops in with scale bounce
7. `2.8s`: Slack toast slides in from right with `translateX` + `opacity`

A "↺ Replay" button resets all animations by toggling a CSS class.

---

## Dashboard (`/dashboard`)

Wraps in `<Shell>`. Content area has two tabs driven by `?tab=` URL param (no visible tab UI — sidebar nav items link directly):

**Overview tab** (default):
- 4 summary cards: Total Events / Models Monitored / Active Alerts (amber if >0) / Last Checked
- Alert banner (amber, full-width) if any model is in drift — "⚠ Drift detected on [model list]. [Dismiss]"
- Per-model table: Model | Provider | Events | Baseline Status | Output Tokens (mean) | Latency (mean ms) | Drift Score (bar) | Status badge
- Drift score bar: green `<0.4`, amber `0.4–0.7`, red `>0.7`
- Status badge: "ok" (green) / "drift" (amber) / "no baseline" (gray)

All data comes from existing `GET /api/v1/baselines` — no new API endpoints needed.

Auto-refresh every 60s (same as today).

---

## Login page (`/login`)

No shell. Centered card on `#f1f3f4` background.

- Top: ⚡ icon in blue square, "Sign in to Argus", "LLM behavioral drift detection"
- Two OAuth buttons: Continue with GitHub / Continue with Google
- Links to `${NEXT_PUBLIC_ARGUS_SERVER}/auth/github` and `/auth/google` (unchanged)

After OAuth completes, `auth/callback` redirects to `/dashboard` instead of `/`.

---

## Settings page (`/settings`)

Inside shell. Content area (white card, `max-w-lg`):
- "Settings" heading
- Display name field + Save button
- Account section showing email (read-only)

No functional changes — same `PATCH /api/v1/me` call.

---

## Docs section (`/docs/[slug]`)

Inside shell. Content area renders a docs viewer.

**Six pages** (slug → title):
- `quickstart` — Quick Start
- `sdk` — SDK Reference
- `cli` — CLI Reference
- `self-hosting` — Self-hosting
- `how-it-works` — How drift detection works
- `alerts` — Alerts & Slack

**Implementation:** Each page is a `.tsx` file in `ui/app/docs/[slug]/` exporting a static content component. No MDX — plain TSX with prose styling. A `DOCS_PAGES` constant in `ui/lib/docs.ts` maps slugs to titles and file imports, used by the sidebar to highlight the active page.

**Docs sidebar:** The main Shell sidebar item "Docs" links to `/docs/quickstart`. When on any `/docs/*` route, the sidebar shows a sub-nav (indented, smaller font) listing all 6 pages. Active page highlighted.

**Content pages** (actual content, not placeholder):

`quickstart`:
```
## Quick Start

### 1. Install
pip install argus-sdk

### 2. Add one line to your app
from argus_sdk import patch
patch(endpoint="https://argus-sdk.com", api_key="argus_sk_...")

### 3. Your LLM code is unchanged
import anthropic
client = anthropic.Anthropic()
response = client.messages.create(model="claude-sonnet-4-6", messages=[...])
# Signals are sent to Argus in the background

### 4. Watch the dashboard
Log in at argus-sdk.com. Once 100+ events are collected, drift detection activates.
```

`sdk`:
- `patch(endpoint, api_key, client=None)` — parameters, what it wraps
- Anthropic example, OpenAI example
- Streaming: supported (wrapper intercepts stream_complete)
- Signal shape: model, provider, input_tokens, output_tokens, latency_ms, finish_reason, timestamp_utc

`cli`:
- `argus login` — opens browser, receives code, saves JWT to `~/.config/argus/credentials.json`
- `argus status` — shows email + drift summary per project
- `argus projects` — lists projects with masked API key prefix

`self-hosting`:
- Docker run command: `docker run -e POSTGRES_URL=... -p 4000:4000 -p 3000:3000 argus`
- All env vars table (from CLAUDE.md)
- Unauthenticated SDK requests fall back to `"self-hosted"` project

`how-it-works`:
- Welford's online algorithm for running baseline
- Mann-Whitney U test on output_tokens and latency_ms
- Bonferroni correction for multiple comparisons
- Hysteresis: alert fires at score > 0.7, clears below 0.4 for 3 consecutive windows
- Minimum 100 events before baseline is ready

`alerts`:
- Slack: set `ARGUS_SLACK_WEBHOOK` env var, Block Kit message format
- What triggers: score > 0.7 for first time (hysteresis — won't re-fire until clear + re-trigger)
- What clears: 3 consecutive windows below 0.4

---

## File structure

**New files:**
```
ui/
  app/
    page.tsx                         — landing page (replaces dashboard)
    dashboard/
      page.tsx                       — dashboard (moved from app/page.tsx)
    docs/
      [slug]/
        page.tsx                     — docs viewer (reads slug, renders content)
  components/
    Shell.tsx                        — shared authenticated shell (topbar + sidebar)
    DashboardDemo.tsx                — animated dashboard mockup for landing page
  lib/
    docs.ts                          — DOCS_PAGES map (slug → title + content)
```

**Modified files:**
```
ui/app/auth/callback/page.tsx        — change redirect from "/" to "/dashboard"
ui/app/login/page.tsx                — restyle to GCP aesthetic
ui/app/settings/page.tsx             — use Shell, restyle
ui/app/globals.css                   — add GCP color variables
```

---

## Testing

- Existing Playwright e2e tests cover login → callback → dashboard → settings flows. After the routing change (`/dashboard`), update test URLs from `/` to `/dashboard`.
- No new e2e tests required — landing page has no interactive state, docs are static.
- Visual correctness verified manually by running `npm run dev` and checking each page.

---

## Out of scope

- Pricing page (landing has a Pricing nav link — it anchors to a placeholder section or scrolls to CTA)
- Real-time WebSocket updates
- Dark mode
- Mobile responsiveness (desktop-first, same as GCP Console)
