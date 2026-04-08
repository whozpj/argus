# argus-sdk

Detect when your LLM's behavior has statistically shifted — one line of code to instrument, one Docker container to run.

---

## What it does

LLMs change. Model providers silently update weights, swap infrastructure, or adjust safety filters. Your evals pass, but production quietly drifts. Argus catches this.

`argus-sdk` wraps your existing Anthropic or OpenAI client and captures derived signals from every LLM call — output tokens, latency, finish reason. It ships those signals in the background to a self-hosted Argus server, which runs statistical tests (Mann-Whitney U + Bonferroni correction) to detect distribution shifts and alerts you via Slack when drift is confirmed.

**No prompt text. No completion text. Derived signals only.**

---

## Install

```bash
pip install argus-sdk
```

---

## Quick start

### 1. Run the Argus server

```bash
docker run -p 4000:4000 -p 3000:3000 -v argus-data:/data ghcr.io/whozpj/argus:latest
```

Dashboard: `http://localhost:3000`  
Ingest API: `http://localhost:4000`

### 2. Instrument your client

**Auto-mode** — instruments all clients created after `patch()`:

```python
from argus_sdk import patch
patch(endpoint="http://localhost:4000")

import anthropic
client = anthropic.Anthropic()  # automatically instrumented

response = client.messages.create(
    model="claude-sonnet-4-6",
    max_tokens=256,
    messages=[{"role": "user", "content": "Hello"}],
)
```

**Explicit mode** — instrument a specific instance:

```python
import anthropic
from argus_sdk import patch

client = anthropic.Anthropic()
patch(endpoint="http://localhost:4000", client=client)
```

Both OpenAI and Anthropic sync clients are supported.

---

## Flush on exit (short scripts & CLIs)

By default, signals are sent in a background worker thread and flushed automatically when your process exits. For short-lived scripts where you want to guarantee delivery before exit:

```python
from argus_sdk import patch, flush

patch(endpoint="http://localhost:4000")

# ... your LLM calls ...

flush()  # blocks until all queued events are sent
```

---

## What gets captured

| Field | Example |
|---|---|
| `model` | `claude-sonnet-4-6` |
| `provider` | `anthropic` |
| `input_tokens` | `312` |
| `output_tokens` | `87` |
| `latency_ms` | `843` |
| `finish_reason` | `stop` |
| `timestamp_utc` | `2026-04-07T14:22:01Z` |

No prompt text. No completion text.

---

## How drift detection works

The Argus server builds a baseline per model using Welford's online algorithm (ready after 200 events). Every 60 seconds it runs a Mann-Whitney U test on output tokens and latency against the current window. Bonferroni correction controls false positives across multiple signals. A hysteresis state machine fires alerts when drift score exceeds 0.7 and clears when it drops below 0.4 for three consecutive windows.

---

## Environment

| Variable | Default | Description |
|---|---|---|
| `ARGUS_ADDR` | `:4000` | Server listen address |
| `ARGUS_DB_PATH` | `argus.db` | SQLite file path |
| `ARGUS_SLACK_WEBHOOK` | _(empty)_ | Slack webhook URL for alerts |

---

## Self-hosted

Argus is fully self-hosted. No data leaves your infrastructure. The server is a single Go binary backed by SQLite, bundled with the Next.js dashboard into one Docker image.

---

## License

MIT
