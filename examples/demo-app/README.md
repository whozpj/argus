# Argus Demo App

Two scripts that show Argus working end-to-end.

| Script | What it does | Needs API keys? |
|---|---|---|
| `app.py` | Interactive Q&A assistant — every real LLM call sends a signal to Argus | Yes |
| `simulate.py` | Sends synthetic events to the server to demo the dashboard and drift detection | No |

---

## Quick demo (no API keys needed)

**Terminal 1 — start the Argus API server (port 4000):**

```bash
cd ../../server
go run ./cmd/main.go
```

**Terminal 2 — start the dashboard (port 3000):**

```bash
cd ../../ui
npm run dev
```

**Terminal 3 — run the simulator:**

```bash
python simulate.py
```

This sends 200 normal events then 50 drifted events for two models
(`claude-sonnet-4-6` and `gpt-4o`). Output looks like:

```
Argus drift simulator
  server  : http://localhost:4000
  models  : claude-sonnet-4-6, gpt-4o
  baseline: 200 events per model
  drift   : 50 events per model
  delay   : 0.02s between events

Server reachable ✓

── claude-sonnet-4-6 ──
  Phase 1: 200 baseline events
  baseline  [████████████████████████████████████████] 100%  (200/200)
  ✓ Baseline complete — is_ready will flip at 200 events
  Phase 2: 50 drifted events
  drift     [████████████████████████████████████████] 100%  (50/50)
  ✓ Drift phase complete

──────────────────────────────────────────────────
  Sent 500 events across 2 model(s) in 12.3s

  Next steps:
  1. Open http://localhost:3000 — check the baselines table
  2. Wait up to 60 seconds for the drift detector to run
  3. Watch the server logs for: WARN DRIFT DETECTED
```

Open [http://localhost:3000](http://localhost:3000) to see the dashboard.

Within 60 seconds the server logs will show:

```
WARN DRIFT DETECTED model=claude-sonnet-4-6 score=1
WARN DRIFT DETECTED model=gpt-4o score=1
```

---

## Real LLM usage (requires API key)

**Setup:**

```bash
# Install deps (uses the local sdk from this repo)
python3 -m venv .venv
source .venv/bin/activate
pip install anthropic httpx
pip install -e ../../sdk   # local argus-sdk
```

**Run:**

```bash
# Anthropic
ANTHROPIC_API_KEY=sk-ant-... python app.py

# OpenAI
OPENAI_API_KEY=sk-... python app.py --provider openai

# Custom model
python app.py --model claude-haiku-4-5

# Custom Argus server
python app.py --argus http://my-server:4000
```

Every question you ask is answered normally — but in the background the
SDK captures token counts and latency and sends them to Argus. No prompt
text or response text ever leaves your app.

---

## Simulator flags

```
--argus URL        Argus server URL (default: http://localhost:4000)
--models LIST      Comma-separated model names (default: claude-sonnet-4-6,gpt-4o)
--baseline N       Baseline events per model (default: 200)
--drift N          Drifted events per model (default: 50)
--delay SECONDS    Sleep between events (default: 0.02)
--drift-only       Skip baseline phase — useful for repeat runs
```

**Example: run drift only after baseline already exists**

```bash
python simulate.py --drift-only --drift 100
```
