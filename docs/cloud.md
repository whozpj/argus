# Argus Cloud — Developer Guide

This document covers the `cloud` branch, which migrates Argus from SQLite to PostgreSQL and lays the foundation for a multi-tenant hosted platform.

---

## What changed from `main`

| Area | `main` | `cloud` |
|---|---|---|
| Database | SQLite (single file) | PostgreSQL |
| DB connection | `ARGUS_DB_PATH` env var | `POSTGRES_URL` env var |
| Data isolation | Single global namespace | Per-project via `project_id` |
| Auth | None | Tables ready; hardcoded `"self-hosted"` for now |

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

`ARGUS_DB_PATH` is gone — the server no longer uses SQLite.

---

## Database schema

Three existing tables now have a `project_id` column:

- `events (project_id, model, provider, input_tokens, output_tokens, latency_ms, finish_reason, timestamp_utc, created_at)`
- `baselines (project_id, model, count, mean_output_tokens, m2_output_tokens, mean_latency_ms, m2_latency_ms, is_ready, updated_at)` — composite PK on `(project_id, model)`
- `drift_state (project_id, model, score, p_output_tokens, p_latency_ms, alerted, checked_at)` — composite PK on `(project_id, model)`

Four new tables for the upcoming auth system:

- `users (id, email, github_id, google_id, created_at)`
- `projects (id, user_id, name, created_at)`
- `api_keys (id, project_id, key_hash, key_prefix, name, created_at)`
- `oauth_sessions (code, user_id, expires_at)` — short-lived codes for CLI login

All data is currently scoped to a hardcoded project ID `"self-hosted"`. Real API key resolution comes in Plan 2.

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

## What's next (upcoming plans)

**Plan 2 — Auth & API Keys**
JWT middleware, GitHub + Google OAuth endpoints (`/auth/github`, `/auth/google`), API key generation and validation, `/api/v1/me`, `/api/v1/projects`, `/api/v1/projects/:id/keys`.

**Plan 3 — SDK + CLI**
`api_key` parameter in `patch()`, `argus login` / `argus status` / `argus projects` CLI commands.

**Plan 4 — Dashboard**
Login page, project selector, per-project dashboard URL.

**Plan 5 — AWS Infrastructure**
Terraform for ECS Fargate, RDS, ALB, S3, CloudFront, Secrets Manager. GitHub Actions CI/CD.

---

## Design spec

Full design decisions and architecture rationale: [docs/superpowers/specs/2026-04-08-argus-cloud-design.md](superpowers/specs/2026-04-08-argus-cloud-design.md)
