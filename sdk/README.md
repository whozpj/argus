# argus-sdk

Detect when your LLM's behavior has statistically shifted — one line of code to instrument.

---

## What it does

LLMs change. Model providers silently update weights, swap infrastructure, or adjust safety filters. Your evals pass, but production quietly drifts. Argus catches this.

`argus-sdk` wraps your existing Anthropic or OpenAI client and captures derived signals from every LLM call — output tokens, latency, finish reason. It ships those signals in the background to Argus, which runs statistical tests (Mann-Whitney U + Bonferroni correction) to detect distribution shifts and alerts you via Slack when drift is confirmed.

**No prompt text. No completion text. Derived signals only.**

---

## Install

```bash
pip install argus-sdk
```

For OpenAI support:

```bash
pip install "argus-sdk[openai]"
```

---

## Quick start

### Cloud (argus-sdk.com)

```python
from argus_sdk import patch
patch(api_key="argus_sk_...")  # defaults to https://argus-sdk.com

import anthropic
client = anthropic.Anthropic()  # automatically instrumented

response = client.messages.create(
    model="claude-sonnet-4-6",
    max_tokens=256,
    messages=[{"role": "user", "content": "Hello"}],
)
```

### Self-hosted

```bash
docker run -p 4000:4000 -p 3000:3000 -v argus-data:/data ghcr.io/whozpj/argus:latest
```

```python
from argus_sdk import patch
patch(endpoint="http://localhost:4000")

import anthropic
client = anthropic.Anthropic()
```

---

## Supported clients

| Client | Sync | Async | Streaming |
|---|---|---|---|
| `anthropic.Anthropic` | ✓ | — | ✓ |
| `anthropic.AsyncAnthropic` | — | ✓ | ✓ |
| `openai.OpenAI` | ✓ | — | ✓ |
| `openai.AsyncOpenAI` | — | ✓ | ✓ |

Streaming calls (`stream=True`) are transparently intercepted — the signal is reported after the stream is exhausted. User code is unchanged.

**Note:** `client.messages.stream()` (Anthropic context manager API) is not intercepted.

---

## Explicit mode

Instrument a specific instance instead of all future clients:

```python
import anthropic
from argus_sdk import patch

client = anthropic.Anthropic()
patch(endpoint="https://argus-sdk.com", client=client, api_key="argus_sk_...")
```

---

## Flush on exit (short scripts & CLIs)

Signals are sent in a background worker thread. For short-lived scripts:

```python
from argus_sdk import patch, flush

patch(api_key="argus_sk_...")

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

## Self-hosted environment variables

| Variable | Default | Description |
|---|---|---|
| `ARGUS_ADDR` | `:4000` | Server listen address |
| `ARGUS_DB_PATH` | `argus.db` | SQLite file path (self-hosted) |
| `ARGUS_SLACK_WEBHOOK` | _(empty)_ | Slack webhook URL for alerts |

---

## License

MIT
