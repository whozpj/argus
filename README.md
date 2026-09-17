# Argus

[![PyPI](https://img.shields.io/pypi/v/argus-sdk)](https://pypi.org/project/argus-sdk/)
[![CI](https://github.com/whozpj/argus/actions/workflows/deploy.yml/badge.svg)](https://github.com/whozpj/argus/actions/workflows/deploy.yml)
![Python](https://img.shields.io/badge/python-3.12+-3776ab)
![Go](https://img.shields.io/badge/go-1.26-00add8)
![Next.js](https://img.shields.io/badge/next.js-14-000000)

**Argus tells you when your LLM's behavior has changed — before your users do.**

LLM providers ship silent model updates. A prompt that returned 60 tokens last week starts
returning 90. Refusals creep up. Latency doubles. Nothing errors, nothing logs — your users
just notice your product got worse. Argus watches the *statistical distribution* of your model's
behavior and fires an alert the moment it drifts.

Wrap your existing client with **one line of code**. Argus captures derived signals — token
counts, latency, finish reason — and never sees your prompts or completions. It runs a
non-parametric drift test every 60 seconds and alerts Slack when a model starts behaving
differently.

Runs two ways: **[Argus Cloud](https://argus-sdk.com)** (managed, nothing to host) or
**self-hosted** (one Docker container, no data leaves your machine).

![The Argus dashboard — live drift detection across models, with baseline stats and Slack-backed alerts](docs/assets/argus-dashboard.png)

---

## Engineering highlights

The parts of this project worth a closer look:

- **Statistical drift detection, not thresholds.** A [Mann-Whitney U test](server/internal/drift/)
  (non-parametric, no normality assumption) with Bonferroni correction across multiple signals,
  wrapped in a hysteresis state machine so alerts fire once and clear cleanly instead of flapping.
  Baselines are built with [Welford's online algorithm](server/internal/store/baseline.go) — running
  mean/variance with no need to store raw events.
- **Validated accuracy.** A [Monte Carlo test suite](server/internal/drift/accuracy_test.go)
  (3,500 trials) measures the detector empirically: **98% detection at a +20% shift, 0.2% false
  positive rate.** Reproducible with one `go test` command — see [Detection accuracy](#detection-accuracy).
- **Zero added latency.** The SDK intercepts responses on a background thread; your request path is
  untouched. Streaming (`stream=True`) is transparently wrapped and measured after the stream
  exhausts — including auto-injecting `stream_options` on OpenAI to recover token counts.
- **Multi-tenant cloud.** GitHub/Google OAuth, JWT sessions, hashed API keys, and a project-scoped
  Postgres schema — the same codebase falls back to an unauthenticated single-tenant mode when
  self-hosted.
- **Real infrastructure.** Ships to AWS via [Terraform](deploy/terraform/) (ECS Fargate, RDS,
  ALB, Route 53, Secrets Manager, IAM) with a [GitHub Actions](.github/workflows/deploy.yml)
  pipeline that runs the Go + Python test suites and deploys to ECS on every push to `main`.

## Tech stack

| Layer | Technology |
|---|---|
| **SDK** | Python 3.12+ (`pip install argus-sdk`), sync + async, Anthropic & OpenAI |
| **Server** | Go 1.26 — HTTP ingest, drift detection, auth, Slack alerts |
| **Dashboard** | Next.js 14, TypeScript, Tailwind, shadcn/ui |
| **Storage** | PostgreSQL (cloud) · SQLite (self-hosted) |
| **Infra** | Docker, Terraform, AWS ECS Fargate / RDS / ALB, GitHub Actions |

---

## Architecture

```mermaid
flowchart LR
    subgraph YOUR_APP["Your Application"]
        CODE["Your code\n(unchanged)"]
        SDK["argus-sdk\npatch()"]
    end

    subgraph PROVIDERS["LLM Providers"]
        ANTHROPIC["Anthropic"]
        OPENAI["OpenAI / compatible"]
    end

    subgraph ARGUS["Argus Server"]
        INGEST["Ingest API\n/api/v1/events"]
        BASELINE["Baseline Builder\nWelford algorithm"]
        DETECTOR["Drift Detector\nMann-Whitney U\nBonferroni correction"]
        ALERTS["Alert Dispatcher\nSlack webhook"]
        DB[("PostgreSQL")]
        UI["Dashboard"]
    end

    EXTERNAL["Slack"]

    CODE -->|"normal API call"| SDK
    SDK -->|"forward request"| ANTHROPIC
    SDK -->|"forward request"| OPENAI
    ANTHROPIC -->|"stream response"| SDK
    OPENAI -->|"stream response"| SDK
    SDK -->|"response"| CODE
    SDK -->|"signal event\n{tokens, latency,\nfinish_reason}\nasync, non-blocking"| INGEST

    INGEST --> DB
    DB --> BASELINE
    BASELINE -->|"every 60s"| DETECTOR
    DETECTOR -->|"drift_score > 0.7"| ALERTS
    ALERTS -->|"webhook"| EXTERNAL
    DB --> UI
```

1. `patch()` wraps your existing LLM client — requests and responses flow through unchanged.
2. After each response, the SDK posts a signal event to Argus in the background (non-blocking).
3. The server builds a statistical baseline from the first 200 requests per model.
4. Every 60 seconds it runs a Mann-Whitney U test comparing recent requests against the baseline.
5. If the drift score crosses 0.7, a Slack alert fires and the dashboard updates.

**No prompt text or completion text ever leaves your app** — only derived signals (token counts,
latency, finish reason).

---

## Quick start

### Option A — Argus Cloud (nothing to host)

Sign up at **[argus-sdk.com](https://argus-sdk.com)**, create a project, and grab an API key.

```bash
pip install argus-sdk
```

```python
from argus_sdk import patch
patch(endpoint="https://argus-sdk.com", api_key="argus_sk_...")

import anthropic
client = anthropic.Anthropic()
client.messages.create(...)   # signals sent to Argus in the background
```

Manage everything from the CLI:

```bash
argus login      # authenticate via GitHub or Google OAuth
argus status     # drift summary for all your projects
argus projects   # list projects and API key prefixes
```

### Option B — Self-host (one Docker container)

```bash
# 1. Start Postgres
docker run -d --name argus-db -p 5432:5432 \
  -e POSTGRES_USER=argus -e POSTGRES_PASSWORD=argus -e POSTGRES_DB=argus \
  postgres:15-alpine

# 2. Start Argus (macOS/Linux — host.docker.internal reaches the DB)
docker run -p 4000:4000 -p 3000:3000 \
  -e POSTGRES_URL="postgres://argus:argus@host.docker.internal:5432/argus?sslmode=disable" \
  argus/argus
```

Then point the SDK at your own container — omit the `api_key`:

```python
from argus_sdk import patch
patch(endpoint="http://localhost:4000")
```

Open [localhost:3000](http://localhost:3000) for the dashboard. Add `-e ARGUS_SLACK_WEBHOOK=...`
to the `docker run` command to enable Slack alerts.

> Prefer to instrument a single client instead of all of them?
> `patch(endpoint=..., client=my_client)` wraps just that instance.

---

## Detection accuracy

The drift detector is validated with a Monte Carlo suite
([`server/internal/drift/accuracy_test.go`](server/internal/drift/accuracy_test.go)) running 500
independent trials per condition (3,500 total). Each trial draws fresh samples from known
distributions, runs the full Mann-Whitney + Bonferroni pipeline, and records whether the detector
fired.

```bash
cd server && go test -v -run TestAccuracy ./internal/drift/
```

**False positive rate** — baseline and recent window drawn from the *same* distribution (mean 60
tokens, σ 15), 500 trials:

| Condition | FPR |
|---|---|
| Same distribution, 500 trials | **0.2%** |
| 5 different random seeds, 200 trials each | **< 5% on all seeds** |

**Detection power** — baseline N(60, 15²), recent window mean shifted by the stated multiple, 500
trials per row:

| Drift magnitude | Recent mean | TPR |
|---|---|---|
| 1.0× — no drift | 60 | 0.8% |
| 1.2× — +20% | 72 | **98.2%** |
| 1.5× — +50% | 90 | **100%** |
| 2.0× — +100% | 120 | **100%** |
| 3.0×+ | 180+ | **100%** |

The detector is intentionally sensitive: a +20% shift in output tokens — e.g. a model going from
averaging 60 tokens to 72 — is flagged 98% of the time, while keeping the false positive rate near
zero. The alert threshold (`score > 0.7`) corresponds to a Bonferroni-corrected p-value < 0.015,
well below α = 0.05.

<details>
<summary>Methodology &amp; tuning details</summary>

- **Algorithm**: Mann-Whitney U test (non-parametric) with Bonferroni correction across two signals
  (`output_tokens`, `latency_ms`)
- **Baseline**: 200 events required before detection starts
- **Recent window**: last 50 events (TPR is 100% at 3× drift for windows as small as 10)
- **Score formula**: `score = max(0, 1 - corrected_p / α)` where `α = 0.05`
- **Alert threshold**: `score > 0.7` → corrected p < 0.015
- **Clear threshold**: `score < 0.4` for 3 consecutive 60-second windows (hysteresis)
- **PRNG**: deterministic LCG (seed = 42) — results are reproducible
- Latency signal (5× shift: 350 ms → 1800 ms): **100% TPR** over 500 trials

</details>

---

## Project structure

```
sdk/          Python package — pip install argus-sdk
  argus_sdk/  patch(), Anthropic/OpenAI wrappers, credentials, CLI
server/       Go server
  cmd/        main.go entrypoint
  internal/
    ingest/   POST /api/v1/events handler
    store/    PostgreSQL DAL + Welford baseline builder
    drift/    Mann-Whitney U, Bonferroni, hysteresis detector (+ accuracy suite)
    alerts/   Slack webhook notifier
    api/      GET /api/v1/baselines handler
    auth/     JWT, OAuth (GitHub + Google), API-key middleware
ui/           Next.js 14 dashboard (TypeScript, Tailwind, shadcn/ui)
deploy/
  Dockerfile  Single-image build: server + UI
  terraform/  AWS infra — ECS Fargate, RDS, ALB, Route 53, Secrets Manager, IAM
.github/
  workflows/  GitHub Actions — test, build, deploy to ECS on push to main
examples/
  demo-app/   Simulator that drives the dashboard with no API keys required
docs/         Cloud developer guide + dashboard screenshot
```

## Development

Requirements: Python 3.12+, Go 1.26+, Node 20+, Docker.

```bash
make sdk-install   # create sdk/.venv and install deps
make sdk-test      # pytest (76 tests)
make server-build  # go build → server/bin/argus
make server-test   # go test ./...
make ui-install    # npm install in ui/

# Run locally (needs a running Postgres — see the self-host command above)
cd server && POSTGRES_URL="postgres://argus:argus@localhost:5432/argus?sslmode=disable" \
  go run ./cmd/main.go              # API on :4000
cd ui && npm run dev                 # dashboard on :3000
```

### Try it without API keys

`examples/demo-app/` sends synthetic events so you can watch drift detection work end-to-end with
no LLM keys. In three terminals (plus Postgres):

```bash
cd server && POSTGRES_URL="postgres://argus:argus@localhost:5432/argus?sslmode=disable" go run ./cmd/main.go
cd ui && npm run dev
cd examples/demo-app && python simulate.py
```

Open [localhost:3000](http://localhost:3000) and wait up to 60 seconds for `DRIFT DETECTED` to
appear. See [examples/demo-app/README.md](examples/demo-app/README.md) for details.

<details>
<summary>Publishing the SDK to PyPI (maintainers)</summary>

```bash
# Bump version in sdk/pyproject.toml, then:
cd sdk && source .venv/bin/activate
pip install build twine
python -m build                          # dist/argus_sdk-*.whl + .tar.gz
twine upload --repository testpypi dist/* # test first
twine upload dist/*                       # production
```

Authenticate with a PyPI API token (`__token__` / `pypi-...`) via `~/.pypirc` or the
`TWINE_PASSWORD` env var in CI.

</details>

---

## License

Released under the [MIT License](LICENSE).
