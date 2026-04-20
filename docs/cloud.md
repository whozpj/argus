# Argus Cloud — Developer Guide

This document covers the cloud version of Argus, which runs at [argus-sdk.com](https://argus-sdk.com). It uses PostgreSQL, multi-tenant auth, and deploys to AWS ECS Fargate via GitHub Actions.

---

## What changed from the original self-hosted version

| Area | Self-hosted v1 | Cloud (current) |
|---|---|---|
| Database | SQLite (single file) | PostgreSQL |
| DB connection | `ARGUS_DB_PATH` env var | `POSTGRES_URL` env var |
| Data isolation | Single global namespace | Per-project via `project_id` |
| Auth | None | JWT + GitHub/Google OAuth + API key middleware |
| Hosting | Local Docker container | AWS ECS Fargate at argus-sdk.com |
| Deploy | Manual | GitHub Actions on push to `main` |

---

## Running locally

You need a running Postgres instance. The easiest way:

```bash
docker run --rm -p 5432:5432 \
  -e POSTGRES_USER=argus \
  -e POSTGRES_PASSWORD=argus \
  -e POSTGRES_DB=argus \
  postgres:15-alpine
```

Then start the server:

```bash
cd server
POSTGRES_URL="postgres://argus:argus@localhost:5432/argus?sslmode=disable" go run ./cmd/main.go
```

The schema is applied automatically on startup. No migrations to run manually.

---

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `POSTGRES_URL` | `postgres://argus:argus@localhost:5432/argus?sslmode=disable` | Postgres connection string |
| `ARGUS_ADDR` | `:4000` | Server listen address |
| `ARGUS_SLACK_WEBHOOK` | _(empty)_ | Slack webhook URL for drift alerts |
| `JWT_SECRET` | `dev-secret-change-in-production` | HS256 signing key — **change in production** |
| `ARGUS_BASE_URL` | `http://localhost:4000` | Public base URL used to construct OAuth redirect URIs |
| `ARGUS_UI_URL` | `http://localhost:3000` | Dashboard URL — browser is redirected here after web OAuth |
| `GITHUB_CLIENT_ID` | _(empty)_ | GitHub OAuth app client ID |
| `GITHUB_CLIENT_SECRET` | _(empty)_ | GitHub OAuth app client secret |
| `GOOGLE_CLIENT_ID` | _(empty)_ | Google OAuth app client ID |
| `GOOGLE_CLIENT_SECRET` | _(empty)_ | Google OAuth app client secret |

`ARGUS_DB_PATH` is gone — the server no longer uses SQLite.

---

## Database schema

Three existing tables now have a `project_id` column:

- `events (project_id, model, provider, input_tokens, output_tokens, latency_ms, finish_reason, timestamp_utc, created_at)`
- `baselines (project_id, model, count, mean_output_tokens, m2_output_tokens, mean_latency_ms, m2_latency_ms, is_ready, updated_at)` — composite PK on `(project_id, model)`
- `drift_state (project_id, model, score, p_output_tokens, p_latency_ms, alerted, checked_at)` — composite PK on `(project_id, model)`

Four new tables for auth:

- `users (id, email, display_name, github_id, google_id, created_at)`
- `projects (id, user_id, name, created_at)`
- `api_keys (id, project_id, key_hash, key_prefix, name, created_at)`
- `oauth_sessions (code, user_id, expires_at)` — short-lived codes for CLI login

Requests with a valid `argus_sk_…` API key are scoped to that key's project. Unauthenticated requests fall back to the `"self-hosted"` project.

---

## Running tests

Tests use [testcontainers-go](https://testcontainers.com/) to spin up a throwaway `postgres:15-alpine` container automatically — no manual setup needed. **Docker must be running.**

```bash
cd server && go test ./... -timeout 180s
```

The first run pulls the `postgres:15-alpine` image (~80 MB). Subsequent runs are faster.

Expected output:
```
ok   github.com/whozpj/argus/server/internal/alerts
ok   github.com/whozpj/argus/server/internal/api
ok   github.com/whozpj/argus/server/internal/drift
ok   github.com/whozpj/argus/server/internal/store
```

---

## Completed plans

**Plan 2 — Auth & API Keys** ✅ Done
JWT middleware, GitHub + Google OAuth (/auth/github, /auth/google), API key generation + validation, /api/v1/me, /api/v1/projects, /api/v1/projects/:id/keys, /api/v1/auth/token (CLI code exchange).

**Plan 3 — SDK + CLI** ✅ Done
`api_key` parameter in `patch()`, `argus login` / `argus status` / `argus projects` CLI commands. 59 SDK tests passing.

**Plan 4 — Dashboard** ✅ Done
Login page (`/login`), OAuth callback (`/auth/callback`), project selector dropdown, per-project baselines via JWT + `?project_id`, CORS middleware, user settings page (`/settings`) with display name editing (`PATCH /api/v1/me`). Playwright e2e tests for login, callback, dashboard, and settings.

**Plan 5 — AWS Infrastructure** ✅ Done
ECS Fargate + RDS PostgreSQL 15 + ALB + Route 53/ACM + Secrets Manager. GitHub Actions deploys on push to `main` via OIDC (no long-lived AWS keys). All infrastructure managed by Terraform in `deploy/terraform/`. Live at [argus-sdk.com](https://argus-sdk.com).

---

## Design specs

- Cloud architecture: [docs/superpowers/specs/2026-04-08-argus-cloud-design.md](superpowers/specs/2026-04-08-argus-cloud-design.md)
- AWS infrastructure: [docs/superpowers/specs/2026-04-16-argus-cloud-plan-5-aws-infra.md](superpowers/specs/2026-04-16-argus-cloud-plan-5-aws-infra.md)
