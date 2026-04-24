# argus-sdk Production Readiness — Design Spec

**Date:** 2026-04-23  
**Scope:** Make argus-sdk fully production-ready against argus-sdk.com: fix default endpoint, add async client support, add transparent streaming support, clean up dependencies, add User-Agent header, update README.

---

## Goals

1. Default endpoint points to `https://argus-sdk.com` — no config required for cloud users.
2. `AsyncAnthropic` and `AsyncOpenAI` clients are instrumented correctly (async wrapper, await semantics).
3. Streaming calls (`stream=True`) are transparently intercepted — user code unchanged, signals reported after stream exhausts.
4. `openai` declared as an optional dependency; `pytest-asyncio` re-added (actually used now for async tests).
5. Every HTTP request carries `User-Agent: argus-sdk/{version} python/{python_version}`.
6. README examples use `https://argus-sdk.com`.
7. Version bumped to `0.2.0`.

---

## Architecture

No new files. All changes are in the existing modules:

| File | Change |
|---|---|
| `argus_sdk/__init__.py` | Patch `AsyncAnthropic` + `AsyncOpenAI` in auto-mode; update default endpoint |
| `argus_sdk/_anthropic.py` | Detect async via `inspect.iscoroutinefunction`; wrap streaming via `__iter__`/`__aiter__` |
| `argus_sdk/_openai.py` | Same as above; inject `stream_options={"include_usage": True}` for streaming |
| `argus_sdk/_reporter.py` | Add `User-Agent` header using `importlib.metadata` |
| `argus_sdk/cli.py` | Update default `--server` to `https://argus-sdk.com` |
| `pyproject.toml` | Version `0.2.0`; add `openai` optional dep; re-add `pytest-asyncio` to dev |
| `sdk/README.md` | Update all examples to `https://argus-sdk.com` |
| `sdk/tests/` | ~20 new tests for async + streaming |

---

## Detailed Design

### Default endpoint

`patch()` signature changes from:
```python
def patch(endpoint: str = "http://localhost:4000", ...)
```
to:
```python
def patch(endpoint: str = "https://argus-sdk.com", ...)
```

Same change in `cli.py` for the `--server` default.

Self-hosted users explicitly pass their endpoint — no behavior change for them.

---

### Async client support (`_anthropic.py`, `_openai.py`)

After grabbing `original_create`, check:
```python
import inspect
if inspect.iscoroutinefunction(original_create):
    async def _create(*args, **kwargs): ...
else:
    def _create(*args, **kwargs): ...
```

The async variant `await`s the original call and `await`s the stream wrapper. `report()` is called synchronously from async context — safe because it only enqueues to the background thread queue.

In `__init__.py`, `_try_patch_anthropic_class` patches both `anthropic.Anthropic` and `anthropic.AsyncAnthropic`. `_try_patch_openai_class` patches both `openai.OpenAI` and `openai.AsyncOpenAI`. `_patch_instance` routing already works via module name inspection — no change needed.

---

### Streaming support

**Detection:** `kwargs.get("stream")` is truthy.

**OpenAI injection:** Before calling original, inject:
```python
if kwargs.get("stream"):
    so = dict(kwargs.get("stream_options") or {})
    so["include_usage"] = True
    kwargs = {**kwargs, "stream_options": so}
```
This gives us `usage` on the final chunk.

**Sync stream wrapping (Anthropic):**

Record `t0` before calling original. After getting the stream back, patch its `__iter__` in-place:
```python
original_iter = stream.__iter__

def __iter__():
    for event in original_iter():
        if event.type == "message_start":
            captured["model"] = event.message.model
            captured["input_tokens"] = event.message.usage.input_tokens
        elif event.type == "message_delta":
            captured["output_tokens"] = event.usage.output_tokens
            captured["finish_reason"] = event.delta.stop_reason or ""
        yield event
    # stream exhausted — report now
    latency_ms = int((time.monotonic() - t0) * 1000)
    report(endpoint, {**captured, "latency_ms": latency_ms, ...}, api_key)

stream.__iter__ = __iter__
return stream  # same object — all methods preserved
```

**Async stream wrapping:** Same pattern but patch `__aiter__` with an `async def __aiter__` that `async for`-loops the original.

**OpenAI sync stream wrapping:** Iterate chunks, accumulate `content`, find the final chunk where `usage` is not None (appears when `include_usage=True`), capture `prompt_tokens` and `completion_tokens`.

**Out of scope:** `client.messages.stream()` (Anthropic context manager API). Documented in README.

---

### User-Agent header (`_reporter.py`)

```python
import sys
from importlib.metadata import version as _pkg_version

_SDK_VERSION = _pkg_version("argus-sdk")
_USER_AGENT = f"argus-sdk/{_SDK_VERSION} python/{sys.version_info.major}.{sys.version_info.minor}"
```

Added to `headers` dict in `_post_with_retry` alongside the Authorization header.

---

### Dependencies (`pyproject.toml`)

```toml
[project.optional-dependencies]
openai = ["openai>=1.0"]
dev = [
    "pytest>=8.0",
    "pytest-asyncio>=0.23",
    "ruff>=0.4",
]
```

`anthropic` stays as a required dependency (SDK is primarily for Anthropic). `openai` is optional — users install `argus-sdk[openai]`.

---

### Tests (~20 new)

**`test_patch.py` additions:**
- Async Anthropic: event capture, response passthrough, latency measurement
- Async OpenAI: same
- Sync Anthropic streaming: events yielded correctly, signal reported after exhaustion
- Async Anthropic streaming: same with `async for`
- Sync OpenAI streaming: `stream_options` injected, usage captured from final chunk
- Async OpenAI streaming: same

**`test_reporter.py` addition:**
- `User-Agent` header present in every request
- `User-Agent` format matches `argus-sdk/{version} python/3.x`

All new async tests use `pytest-asyncio` with `asyncio_mode = "auto"` (already configured).

---

## Version

`0.2.0` — minor version bump because the default endpoint changes (technically breaking for anyone relying on the localhost default, though that's only local dev usage).

---

## Out of Scope

- `client.messages.stream()` context manager API (Anthropic)
- Rate limiting / backpressure on the event queue
- Configurable retry delays
- Configurable log levels
