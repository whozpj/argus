# Argus — Project Context for Claude

## What This Is

Argus v1 is a self-hosted, open source LLM behavioral drift detection tool.
Developers wrap their existing LLM client with one line and run one Docker container.
They get a dashboard showing when their model's outputs have statistically shifted.

This is **v1 only** — no prompt versioning, no traffic splitting, no Kafka, no ClickHouse.
Those are future versions. Keep scope tight.

## Stack

| Layer | Tech |
|---|---|
| SDK | Python 3.12+, `argus-sdk` (pip) |
| Server | Go 1.23 — HTTP ingest, SQLite, drift detection |
| Dashboard | Next.js 14, TypeScript, shadcn/ui |
| Storage | SQLite (single file, ships inside the Docker container) |
| Container | Single Docker image — server + UI bundled |

## Directory Structure

```
sdk/          Python package — pip install argus-sdk
server/       Go server — ingest API, baseline, drift detection, alerts
ui/           Next.js dashboard
deploy/       Dockerfile
docs/         Documentation
```

## Key Algorithms (in server/)

- **Welford's online algorithm** — running mean/variance without storing raw events
- **Mann-Whitney U test** — detects distribution shifts in continuous signals
- **Bonferroni correction** — controls false positives when testing multiple signals
- **Hysteresis state machine** — alert fires at drift_score > 0.7, clears below 0.4 for 3 consecutive windows

## SDK Integration Contract

The SDK intercepts LLM responses and POSTs a signal event to the server:

```json
{
  "model":         "claude-sonnet-4-6",
  "provider":      "anthropic",
  "input_tokens":  312,
  "output_tokens": 87,
  "latency_ms":    843,
  "finish_reason": "stop",
  "timestamp_utc": "2026-04-07T14:22:01Z"
}
```

No prompt text. No completion text. Derived signals only.

## Workflow Preferences

- **Commit after each major completed step** — not batched at the end
- **Explain concepts simply** — owner is new to AI infra and distributed systems
- **Don't over-engineer** — this is v1, resist adding features beyond the scope above
- **No Kafka, no ClickHouse** — SQLite only for v1

## Build Commands

```bash
make sdk-install    # create sdk/.venv and install deps
make sdk-test       # run pytest
make server-build   # go build -> server/bin/argus
make ui-install     # npm install in ui/
```
