#!/usr/bin/env python3
"""
Argus drift simulator — populates the Argus server with synthetic events
so the dashboard shows real data without needing LLM API keys.

What it does
------------
Phase 1 — Baseline (200 events per model)
  Normal output_tokens ~60±15, latency_ms ~350±80.
  After 200 events, Argus marks the baseline "ready" and drift detection
  will start firing every 60 seconds.

Phase 2 — Drift (50 events per model)
  output_tokens spikes to ~450±60, latency_ms spikes to ~1800±200.
  This is a 7× shift — well above the Mann-Whitney threshold.
  Argus will fire a WARN log (and a Slack alert if configured) on the
  next 60-second detector tick.

Usage
-----
    # Make sure the Argus server is running first:
    cd ../../server && go run ./cmd/main.go

    # Then in another terminal:
    python simulate.py

    # Optional flags:
    python simulate.py --argus http://localhost:4000 --models claude-sonnet-4-6,gpt-4o
    python simulate.py --baseline 200 --drift 50 --delay 0.05
"""

import argparse
import datetime
import json
import random
import sys
import time
import urllib.request
import urllib.error


def parse_args():
    p = argparse.ArgumentParser(description="Populate Argus with synthetic drift events")
    p.add_argument("--argus", default="http://localhost:4000", metavar="URL")
    p.add_argument("--models", default="claude-sonnet-4-6,gpt-4o",
                   help="Comma-separated model names to simulate")
    p.add_argument("--baseline", type=int, default=200,
                   help="Number of baseline events per model (default: 200)")
    p.add_argument("--drift", type=int, default=50,
                   help="Number of drifted events per model (default: 50)")
    p.add_argument("--delay", type=float, default=0.02,
                   help="Seconds between events (default: 0.02)")
    p.add_argument("--drift-only", action="store_true",
                   help="Skip baseline phase — useful if you already have 200+ events")
    return p.parse_args()


def now_utc() -> str:
    return datetime.datetime.now(datetime.UTC).strftime("%Y-%m-%dT%H:%M:%SZ")


def post_event(endpoint: str, event: dict) -> bool:
    body = json.dumps(event).encode()
    req = urllib.request.Request(
        f"{endpoint}/api/v1/events",
        data=body,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=5) as resp:
            return resp.status == 202
    except urllib.error.URLError as e:
        print(f"\n[error] Cannot reach Argus server at {endpoint}: {e.reason}")
        print("        Is the server running? Try: cd ../../server && go run ./cmd/main.go")
        sys.exit(1)


def normal_event(model: str) -> dict:
    """Baseline-range event: short outputs, normal latency."""
    provider = "anthropic" if "claude" in model else "openai"
    return {
        "model": model,
        "provider": provider,
        "input_tokens": int(random.gauss(80, 20)),
        "output_tokens": max(10, int(random.gauss(60, 15))),
        "latency_ms": max(50, int(random.gauss(350, 80))),
        "finish_reason": "stop",
        "timestamp_utc": now_utc(),
    }


def drifted_event(model: str) -> dict:
    """Drifted event: long outputs, high latency — a 7× shift."""
    provider = "anthropic" if "claude" in model else "openai"
    return {
        "model": model,
        "provider": provider,
        "input_tokens": int(random.gauss(80, 20)),
        "output_tokens": max(200, int(random.gauss(450, 60))),
        "latency_ms": max(500, int(random.gauss(1800, 200))),
        "finish_reason": "stop",
        "timestamp_utc": now_utc(),
    }


def progress(label: str, i: int, total: int, width: int = 40) -> None:
    filled = int(width * i / total)
    bar = "█" * filled + "░" * (width - filled)
    pct = int(100 * i / total)
    print(f"\r  {label}  [{bar}] {pct:3d}%  ({i}/{total})", end="", flush=True)


def simulate_model(endpoint: str, model: str, n_baseline: int, n_drift: int,
                   delay: float, skip_baseline: bool) -> None:
    print(f"\n── {model} ──")

    if not skip_baseline:
        print(f"  Phase 1: {n_baseline} baseline events")
        for i in range(1, n_baseline + 1):
            post_event(endpoint, normal_event(model))
            progress("baseline", i, n_baseline)
            time.sleep(delay)
        print(f"\n  ✓ Baseline complete — is_ready will flip at 200 events")

    print(f"  Phase 2: {n_drift} drifted events")
    for i in range(1, n_drift + 1):
        post_event(endpoint, drifted_event(model))
        progress("drift   ", i, n_drift)
        time.sleep(delay)
    print(f"\n  ✓ Drift phase complete")


def check_server(endpoint: str) -> None:
    req = urllib.request.Request(f"{endpoint}/healthz", method="GET")
    try:
        with urllib.request.urlopen(req, timeout=3):
            pass
    except urllib.error.URLError as e:
        print(f"[error] Cannot reach Argus server at {endpoint}: {e.reason}")
        print("        Start it with: cd ../../server && go run ./cmd/main.go")
        sys.exit(1)


def main():
    args = parse_args()
    models = [m.strip() for m in args.models.split(",") if m.strip()]

    print("Argus drift simulator")
    print(f"  server  : {args.argus}")
    print(f"  models  : {', '.join(models)}")
    if not args.drift_only:
        print(f"  baseline: {args.baseline} events per model")
    print(f"  drift   : {args.drift} events per model")
    print(f"  delay   : {args.delay}s between events")
    print()

    check_server(args.argus)
    print("Server reachable ✓\n")

    t0 = time.monotonic()
    for model in models:
        simulate_model(
            endpoint=args.argus,
            model=model,
            n_baseline=args.baseline,
            n_drift=args.drift,
            delay=args.delay,
            skip_baseline=args.drift_only,
        )

    elapsed = time.monotonic() - t0
    total = len(models) * (args.baseline + args.drift)

    print(f"\n{'─'*50}")
    print(f"  Sent {total} events across {len(models)} model(s) in {elapsed:.1f}s")
    print()
    print("  Next steps:")
    print(f"  1. Open http://localhost:3000 — check the baselines table")
    print(f"  2. Wait up to 60 seconds for the drift detector to run")
    print(f"  3. Watch the server logs for: WARN DRIFT DETECTED")
    if len(models) > 0:
        first = models[0]
        print(f"\n  Or query the API directly:")
        print(f"  curl {args.argus}/api/v1/baselines | python -m json.tool")


if __name__ == "__main__":
    main()
