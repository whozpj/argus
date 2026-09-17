# Argus Dashboard

The web dashboard for [Argus](../README.md) — a Next.js 14 app (TypeScript, Tailwind,
shadcn/ui) that visualizes per-model baselines and drift alerts served by the Go API.

- `app/page.tsx` — public landing page
- `app/dashboard/` — auth-gated dashboard (baselines, drift badges, project selector)
- `app/login/`, `app/auth/callback/` — GitHub/Google OAuth flow
- `app/docs/[slug]/` — docs pages
- `components/Shell.tsx` — shared topbar + sidebar layout
- `lib/api.ts` — API client for the Argus server

## Development

```bash
npm install
npm run dev   # http://localhost:3000
```

The dashboard talks to the Argus API at `http://localhost:4000` by default; override with
`NEXT_PUBLIC_ARGUS_SERVER`. Start the API first — see the [root README](../README.md) for the
full local setup.

```bash
npm run build          # production build
npm run lint           # eslint
npx playwright test    # e2e tests (ui/e2e/)
```
