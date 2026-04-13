# Argus Cloud — Design Spec
**Date:** 2026-04-08  
**Status:** Approved

---

## Context

Argus v1 is self-hosted: one Docker container, SQLite, single-tenant. There is no auth, no project isolation, and the endpoint is hardcoded to `localhost`. This works for local use but means users must run their own infrastructure and can't share or view drift data from anywhere.

This spec covers the next version: a managed cloud platform hosted on AWS where developers sign up, create projects, get API keys, and view live drift dashboards from anywhere — including the terminal.

---

## AWS Infrastructure

**Approach: ECS Fargate + RDS PostgreSQL + S3/CloudFront**

| Component | Service | Notes |
|---|---|---|
| Go server | ECS Fargate (Docker) | Same container as today; no Lambda restructuring needed |
| Load balancer | Application Load Balancer | HTTPS termination, routes to ECS tasks |
| Database | RDS PostgreSQL | Replaces SQLite; same schema + 4 new tables |
| Dashboard | S3 + CloudFront | Next.js static export; `NEXT_PUBLIC_ARGUS_SERVER` → ALB URL |
| Secrets | AWS Secrets Manager | DB credentials, OAuth client secrets, JWT signing key |

The drift detection background goroutine runs inside the ECS task exactly as it does today — no restructuring required.

**Self-hosted mode remains supported.** Users who want to run their own instance can still use the Docker container pointed at their own Postgres (or SQLite for small deployments).

---

## Database Schema Changes

Three existing tables gain a `project_id` column:

```sql
ALTER TABLE events      ADD COLUMN project_id TEXT NOT NULL;
ALTER TABLE baselines   ADD COLUMN project_id TEXT NOT NULL;
ALTER TABLE drift_state ADD COLUMN project_id TEXT NOT NULL;
```

Four new tables:

```sql
CREATE TABLE users (
    id         TEXT PRIMARY KEY,   -- UUID
    email      TEXT UNIQUE NOT NULL,
    github_id  TEXT UNIQUE,
    google_id  TEXT UNIQUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE projects (
    id         TEXT PRIMARY KEY,   -- UUID
    user_id    TEXT NOT NULL REFERENCES users(id),
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE api_keys (
    id          TEXT PRIMARY KEY,  -- UUID
    project_id  TEXT NOT NULL REFERENCES projects(id),
    key_hash    TEXT UNIQUE NOT NULL,  -- bcrypt hash of the raw key
    key_prefix  TEXT NOT NULL,         -- first 8 chars, for display (argus_sk_XXXXXXXX****)
    name        TEXT NOT NULL,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE oauth_sessions (
    code        TEXT PRIMARY KEY,  -- short-lived code issued to CLI
    user_id     TEXT NOT NULL REFERENCES users(id),
    expires_at  TIMESTAMPTZ NOT NULL
);
```

All existing queries in `server/internal/store/` gain a `WHERE project_id = $1` clause.

---

## Auth Flow

### Web OAuth (GitHub + Google)

1. User visits `https://argus.app/login`
2. Picks GitHub or Google
3. Standard OAuth 2.0 redirect → callback → look up or create `users` row → issue JWT (signed with secret from Secrets Manager, 30-day expiry)
4. JWT stored in an httpOnly cookie for web sessions

### CLI Login (`argus login`)

1. CLI starts a local HTTP server on a random available port (e.g. `localhost:52341`)
2. CLI opens browser: `https://argus.app/auth/cli?redirect=http://localhost:52341/callback`
3. User completes GitHub or Google OAuth on the web as normal
4. Server issues a short-lived one-time code (10 min expiry), stores in `oauth_sessions`
5. Redirects browser to `http://localhost:52341/callback?code=<code>`
6. CLI's local server receives the code, POSTs to `https://argus.app/api/v1/auth/token` to exchange for a long-lived JWT
7. JWT saved to `~/.argus/credentials` (JSON: `{"token": "...", "endpoint": "https://argus.app"}`)
8. CLI prints: `Logged in as user@example.com`

### API Key Auth (SDK → Ingest)

- SDK sends `Authorization: Bearer argus_sk_<key>` on every `POST /api/v1/events`
- Server middleware hashes the key (bcrypt), looks up in `api_keys`, resolves `project_id`
- Injects `project_id` into the request context for downstream handlers
- Returns `401` if key missing or invalid, `403` if key is revoked

---

## Server Changes (`server/`)

### New endpoints

| Method | Path | Auth | Purpose |
|---|---|---|---|
| GET | `/api/v1/me` | JWT | Return user info + list of projects |
| POST | `/api/v1/projects` | JWT | Create a new project |
| GET | `/api/v1/projects` | JWT | List user's projects |
| POST | `/api/v1/projects/:id/keys` | JWT | Create API key for project |
| GET | `/api/v1/baselines` | JWT or API key | Return baselines scoped to project |
| POST | `/api/v1/events` | API key | Ingest event (unchanged shape, adds project_id) |
| GET | `/auth/github` | — | Start GitHub OAuth |
| GET | `/auth/github/callback` | — | GitHub OAuth callback |
| GET | `/auth/google` | — | Start Google OAuth |
| GET | `/auth/google/callback` | — | Google OAuth callback |
| GET | `/auth/cli` | — | CLI login entry point |
| POST | `/api/v1/auth/token` | — | Exchange CLI code for JWT |

### New internal packages

- `server/internal/auth/` — JWT issue/validate, OAuth flows (GitHub + Google), middleware
- `server/internal/store/users.go` — CRUD for users, projects, api_keys, oauth_sessions

### Existing packages touched

- `server/internal/store/` — all queries gain `project_id` scoping
- `server/internal/ingest/` — add API key middleware
- `server/internal/api/` — baselines handler scoped by project from JWT or API key
- `server/cmd/main.go` — wire new routes, switch DB to Postgres (`pgx` driver)

---

## SDK Changes (`sdk/`)

### `patch()` signature update

```python
# Before
patch(endpoint="http://localhost:4000")

# After — api_key points to cloud; backward-compatible
patch(endpoint="https://argus.app", api_key="argus_sk_...")
# OR still works for self-hosted
patch(endpoint="http://localhost:4000")
```

The `api_key` is sent as `Authorization: Bearer <api_key>` on every event POST. If omitted, no auth header is sent (self-hosted mode).

### New CLI entry point

Added to `pyproject.toml`:
```toml
[project.scripts]
argus = "argus_sdk.cli:main"
```

New file: `sdk/argus_sdk/cli.py` — uses `click` for command parsing.

### CLI commands

**`argus login`**
- Starts local callback server (random port)
- Opens browser to `https://argus.app/auth/cli?redirect=http://localhost:<port>/callback`
- Waits for code, exchanges for JWT
- Saves to `~/.argus/credentials`

**`argus status`**
- Reads `~/.argus/credentials`
- Calls `GET /api/v1/me` → prints user email + projects
- For each project, calls `GET /api/v1/baselines` → prints drift summary table

**`argus projects`**
- Lists all projects with masked API key prefix

**`argus projects create <name>`**
- Creates project, prints full API key once (not stored server-side in plaintext)

### New dependencies added to `pyproject.toml`

```toml
dependencies = [
    "anthropic>=0.25",
    "httpx>=0.27",
    "click>=8.0",          # CLI parsing
    "rich>=13.0",          # pretty terminal tables
    "keyring>=24.0",       # secure credential storage (falls back to ~/.argus/credentials)
]
```

---

## Dashboard Changes (`ui/`)

- Login page at `/login` — GitHub and Google OAuth buttons
- Project selector dropdown in the nav — switches active project, scopes all API calls
- `fetchBaselines()` sends JWT cookie (web) or — not needed for web; cookie is automatic
- Per-project dashboard URL: `https://argus.app/projects/<id>`
- Authenticated API calls use the httpOnly JWT cookie automatically (same-origin)

---

## Deploy Changes (`deploy/`)

- `Dockerfile` — unchanged structure; add `POSTGRES_URL` env var, remove SQLite volume
- New: `deploy/terraform/` — ECS cluster, Fargate task definition, ALB, RDS instance, S3 bucket, CloudFront distribution, Secrets Manager entries
- New: `deploy/github-actions/` — CI pipeline: build Docker image → push to ECR → update ECS task definition

---

## Verification

1. **Unit tests** — new `server/internal/auth/` package gets tests for JWT issue/validate, API key hashing, OAuth session lifecycle
2. **Integration tests** — extend existing Go tests to use Postgres (via `testcontainers-go`) instead of SQLite
3. **SDK tests** — extend `sdk/tests/` to cover `cli.py` commands (mocked HTTP), `api_key` header on events
4. **End-to-end** — run `argus login` locally against a staging environment, create a project, run `simulate.py` pointed at the staging ingest URL, verify drift shows up in the hosted dashboard
5. **Self-hosted regression** — run the existing Docker image with `POSTGRES_URL` pointing at a local Postgres; confirm all existing tests pass

---

## Out of Scope (v2 only)

- Team/org accounts (sharing projects across multiple users)
- Billing / usage limits
- Prompt versioning
- Traffic splitting
- Kafka / ClickHouse
