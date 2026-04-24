# SDK Production Readiness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make argus-sdk fully production-ready: fix default endpoint, add User-Agent header, async client support, streaming support, and clean up deps.

**Architecture:** All changes in existing modules — no new files. `_anthropic.py` and `_openai.py` gain wrapper classes for streaming and `inspect.iscoroutinefunction` branching for async. `_reporter.py` gains a User-Agent header. `__init__.py` gains AsyncAnthropic/AsyncOpenAI patching.

**Tech Stack:** Python 3.12+, pytest, pytest-asyncio, httpx, importlib.metadata

---

### Task 1: Quick fixes (endpoint, User-Agent, pyproject.toml, cli.py)

**Files:**
- Modify: `sdk/argus_sdk/__init__.py`
- Modify: `sdk/argus_sdk/_reporter.py`
- Modify: `sdk/argus_sdk/cli.py`
- Modify: `sdk/pyproject.toml`
- Test: `sdk/tests/test_reporter.py`

- [ ] **Step 1: Update test_reporter_posts_to_correct_url to expect User-Agent header**

In `sdk/tests/test_reporter.py`, change the assertion from `headers={}` to include the User-Agent:

```python
def test_reporter_posts_to_correct_url():
    mock_response = MagicMock()
    mock_response.status_code = 200
    mock_client_instance = MagicMock()
    mock_client_instance.post.return_value = mock_response

    with mock_patch("httpx.Client") as mock_httpx_client:
        mock_httpx_client.return_value.__enter__.return_value = mock_client_instance
        _post_with_retry("http://localhost:4000", _SAMPLE_EVENT)

    call_args = mock_client_instance.post.call_args
    assert call_args[0][0] == "http://localhost:4000/api/v1/events"
    assert call_args[1]["json"] == _SAMPLE_EVENT
    assert "User-Agent" in call_args[1]["headers"]
    assert call_args[1]["headers"]["User-Agent"].startswith("argus-sdk/")
```

- [ ] **Step 2: Add User-Agent tests to test_reporter.py**

Append to `sdk/tests/test_reporter.py`:

```python
def test_user_agent_present_in_request():
    """Every request must carry a User-Agent header."""
    mock_response = MagicMock()
    mock_response.status_code = 200
    mock_client_instance = MagicMock()
    mock_client_instance.post.return_value = mock_response

    with mock_patch("httpx.Client") as mock_httpx_client:
        mock_httpx_client.return_value.__enter__.return_value = mock_client_instance
        _post_with_retry("http://localhost:4000", _SAMPLE_EVENT)

    headers = mock_client_instance.post.call_args[1]["headers"]
    assert "User-Agent" in headers


def test_user_agent_format():
    """User-Agent must match 'argus-sdk/{version} python/{major}.{minor}'."""
    import re
    mock_response = MagicMock()
    mock_response.status_code = 200
    mock_client_instance = MagicMock()
    mock_client_instance.post.return_value = mock_response

    with mock_patch("httpx.Client") as mock_httpx_client:
        mock_httpx_client.return_value.__enter__.return_value = mock_client_instance
        _post_with_retry("http://localhost:4000", _SAMPLE_EVENT)

    ua = mock_client_instance.post.call_args[1]["headers"]["User-Agent"]
    assert re.match(r"argus-sdk/[\d.]+ python/\d+\.\d+", ua), f"Bad User-Agent: {ua!r}"
```

- [ ] **Step 3: Run new tests to confirm they fail**

```bash
cd /Users/prithviraj/Documents/CS/argus && make sdk-test 2>&1 | tail -20
```

Expected: FAIL (User-Agent not yet added)

- [ ] **Step 4: Add User-Agent to _reporter.py**

Replace the top of `sdk/argus_sdk/_reporter.py`:

```python
import atexit
import logging
import queue
import sys
import threading
import time
from importlib.metadata import version as _pkg_version

logger = logging.getLogger("argus_sdk")

try:
    _SDK_VERSION = _pkg_version("argus-sdk")
except Exception:
    _SDK_VERSION = "unknown"
_USER_AGENT = f"argus-sdk/{_SDK_VERSION} python/{sys.version_info.major}.{sys.version_info.minor}"
```

And update `_post_with_retry` to use it:
```python
    headers = {"User-Agent": _USER_AGENT}
    if api_key:
        headers["Authorization"] = f"Bearer {api_key}"
```

- [ ] **Step 5: Update default endpoint in __init__.py**

Change line 8 of `sdk/argus_sdk/__init__.py`:
```python
def patch(endpoint: str = "https://argus-sdk.com", client: Any = None, api_key: str | None = None) -> None:
```

- [ ] **Step 6: Update default --server in cli.py**

Change line 88 of `sdk/argus_sdk/cli.py`:
```python
@click.option("--server", default="https://argus-sdk.com", help="Argus server URL")
```

- [ ] **Step 7: Update pyproject.toml**

Add openai optional dep:
```toml
[project.optional-dependencies]
openai = ["openai>=1.0"]
dev = [
    "pytest>=8.0",
    "pytest-asyncio>=0.23",
    "ruff>=0.4",
]
```

- [ ] **Step 8: Run all tests**

```bash
cd /Users/prithviraj/Documents/CS/argus && make sdk-test 2>&1 | tail -20
```

Expected: all pass

- [ ] **Step 9: Commit**

```bash
git add sdk/argus_sdk/_reporter.py sdk/argus_sdk/__init__.py sdk/argus_sdk/cli.py sdk/pyproject.toml sdk/tests/test_reporter.py
git commit -m "feat(sdk): add User-Agent header, fix default endpoint to argus-sdk.com, add openai optional dep"
```

---

### Task 2: Async Anthropic non-streaming

**Files:**
- Modify: `sdk/argus_sdk/_anthropic.py`
- Test: `sdk/tests/test_patch.py`

- [ ] **Step 1: Write failing async test**

Append to `sdk/tests/test_patch.py`:

```python
# ---------------------------------------------------------------------------
# Async Anthropic — non-streaming
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_async_anthropic_captures_event():
    posted = []

    client = MagicMock()
    async def async_create(*args, **kwargs):
        return _anthropic_response()
    client.messages.create = async_create

    with mock_patch("argus_sdk._anthropic.report", side_effect=lambda ep, ev, api_key=None: posted.append(ev)):
        anthropic_patch(client, "http://localhost:4000")
        await client.messages.create(model="claude-sonnet-4-6", max_tokens=100, messages=[])

    assert len(posted) == 1
    e = posted[0]
    assert e["model"] == "claude-sonnet-4-6"
    assert e["provider"] == "anthropic"
    assert e["input_tokens"] == 100
    assert e["output_tokens"] == 50


@pytest.mark.asyncio
async def test_async_anthropic_response_passthrough():
    client = MagicMock()
    response = _anthropic_response(output_tokens=77)
    async def async_create(*args, **kwargs):
        return response
    client.messages.create = async_create

    with mock_patch("argus_sdk._anthropic.report"):
        anthropic_patch(client, "http://localhost:4000")
        result = await client.messages.create(model="claude-sonnet-4-6", max_tokens=100, messages=[])

    assert result is response
```

- [ ] **Step 2: Run to verify fail**

```bash
cd /Users/prithviraj/Documents/CS/argus && make sdk-test 2>&1 | grep -E "FAILED|PASSED|ERROR" | tail -10
```

- [ ] **Step 3: Rewrite _anthropic.py with async support**

```python
import datetime
import inspect
import time

from ._reporter import report


class _SyncAnthropicStreamWrapper:
    def __init__(self, stream, t0, endpoint, api_key):
        self._stream = stream
        self._t0 = t0
        self._endpoint = endpoint
        self._api_key = api_key
        self._captured = {}

    def __iter__(self):
        for event in self._stream:
            if event.type == "message_start":
                self._captured["model"] = event.message.model
                self._captured["input_tokens"] = event.message.usage.input_tokens
            elif event.type == "message_delta":
                self._captured["output_tokens"] = event.usage.output_tokens
                self._captured["finish_reason"] = event.delta.stop_reason or ""
            yield event
        latency_ms = int((time.monotonic() - self._t0) * 1000)
        report(self._endpoint, {
            **self._captured,
            "provider": "anthropic",
            "latency_ms": latency_ms,
            "timestamp_utc": _now(),
        }, api_key=self._api_key)

    def __getattr__(self, name):
        return getattr(self._stream, name)


class _AsyncAnthropicStreamWrapper:
    def __init__(self, stream, t0, endpoint, api_key):
        self._stream = stream
        self._t0 = t0
        self._endpoint = endpoint
        self._api_key = api_key
        self._captured = {}

    async def __aiter__(self):
        async for event in self._stream:
            if event.type == "message_start":
                self._captured["model"] = event.message.model
                self._captured["input_tokens"] = event.message.usage.input_tokens
            elif event.type == "message_delta":
                self._captured["output_tokens"] = event.usage.output_tokens
                self._captured["finish_reason"] = event.delta.stop_reason or ""
            yield event
        latency_ms = int((time.monotonic() - self._t0) * 1000)
        report(self._endpoint, {
            **self._captured,
            "provider": "anthropic",
            "latency_ms": latency_ms,
            "timestamp_utc": _now(),
        }, api_key=self._api_key)

    def __getattr__(self, name):
        return getattr(self._stream, name)


def patch(client: object, endpoint: str, api_key: str | None = None) -> None:
    """Wrap client.messages.create to capture signals after each response."""
    messages = client.messages  # type: ignore[attr-defined]
    original_create = messages.create

    if inspect.iscoroutinefunction(original_create):
        async def _create(*args, **kwargs):
            if kwargs.get("stream"):
                t0 = time.monotonic()
                stream = await original_create(*args, **kwargs)
                return _AsyncAnthropicStreamWrapper(stream, t0, endpoint, api_key)
            t0 = time.monotonic()
            response = await original_create(*args, **kwargs)
            latency_ms = int((time.monotonic() - t0) * 1000)
            report(endpoint, {
                "model": response.model,
                "provider": "anthropic",
                "input_tokens": response.usage.input_tokens,
                "output_tokens": response.usage.output_tokens,
                "latency_ms": latency_ms,
                "finish_reason": response.stop_reason or "",
                "timestamp_utc": _now(),
            }, api_key=api_key)
            return response
    else:
        def _create(*args, **kwargs):
            if kwargs.get("stream"):
                t0 = time.monotonic()
                stream = original_create(*args, **kwargs)
                return _SyncAnthropicStreamWrapper(stream, t0, endpoint, api_key)
            t0 = time.monotonic()
            response = original_create(*args, **kwargs)
            latency_ms = int((time.monotonic() - t0) * 1000)
            report(endpoint, {
                "model": response.model,
                "provider": "anthropic",
                "input_tokens": response.usage.input_tokens,
                "output_tokens": response.usage.output_tokens,
                "latency_ms": latency_ms,
                "finish_reason": response.stop_reason or "",
                "timestamp_utc": _now(),
            }, api_key=api_key)
            return response

    messages.create = _create


def _now() -> str:
    return datetime.datetime.now(datetime.UTC).strftime("%Y-%m-%dT%H:%M:%SZ")
```

- [ ] **Step 4: Run tests**

```bash
cd /Users/prithviraj/Documents/CS/argus && make sdk-test 2>&1 | tail -20
```

- [ ] **Step 5: Commit**

```bash
git add sdk/argus_sdk/_anthropic.py sdk/tests/test_patch.py
git commit -m "feat(sdk): async Anthropic support + streaming wrapper classes"
```

---

### Task 3: Async OpenAI + streaming (all variants)

**Files:**
- Modify: `sdk/argus_sdk/_openai.py`
- Test: `sdk/tests/test_patch.py`

- [ ] **Step 1: Write failing tests**

Append to `sdk/tests/test_patch.py`:

```python
# ---------------------------------------------------------------------------
# Async OpenAI — non-streaming
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_async_openai_captures_event():
    posted = []

    client = MagicMock()
    async def async_create(*args, **kwargs):
        return _openai_response()
    client.chat.completions.create = async_create

    with mock_patch("argus_sdk._openai.report", side_effect=lambda ep, ev, api_key=None: posted.append(ev)):
        openai_patch(client, "http://localhost:4000")
        await client.chat.completions.create(model="gpt-4o", messages=[])

    assert len(posted) == 1
    e = posted[0]
    assert e["model"] == "gpt-4o"
    assert e["provider"] == "openai"
    assert e["input_tokens"] == 100
    assert e["output_tokens"] == 50


@pytest.mark.asyncio
async def test_async_openai_response_passthrough():
    client = MagicMock()
    response = _openai_response(completion_tokens=33)
    async def async_create(*args, **kwargs):
        return response
    client.chat.completions.create = async_create

    with mock_patch("argus_sdk._openai.report"):
        openai_patch(client, "http://localhost:4000")
        result = await client.chat.completions.create(model="gpt-4o", messages=[])

    assert result is response


# ---------------------------------------------------------------------------
# Anthropic sync streaming
# ---------------------------------------------------------------------------

def _make_anthropic_stream_events(model="claude-sonnet-4-6", input_tokens=100, output_tokens=50):
    start = MagicMock()
    start.type = "message_start"
    start.message.model = model
    start.message.usage.input_tokens = input_tokens

    delta = MagicMock()
    delta.type = "message_delta"
    delta.usage.output_tokens = output_tokens
    delta.delta.stop_reason = "end_turn"

    return [start, delta]


def test_anthropic_sync_streaming_yields_events():
    """Sync streaming wrapper must yield all events through."""
    from argus_sdk._anthropic import _SyncAnthropicStreamWrapper

    events = _make_anthropic_stream_events()
    stream = MagicMock()
    stream.__iter__ = MagicMock(return_value=iter(events))

    wrapper = _SyncAnthropicStreamWrapper(stream, time.monotonic(), "http://localhost:4000", None)

    with mock_patch("argus_sdk._anthropic.report"):
        yielded = list(wrapper)

    assert yielded == events


def test_anthropic_sync_streaming_reports_after_exhaustion():
    """Signal reported once, after the stream is fully consumed."""
    from argus_sdk._anthropic import _SyncAnthropicStreamWrapper

    posted = []
    events = _make_anthropic_stream_events(input_tokens=200, output_tokens=75)
    stream = MagicMock()
    stream.__iter__ = MagicMock(return_value=iter(events))

    wrapper = _SyncAnthropicStreamWrapper(stream, time.monotonic(), "http://localhost:4000", None)

    with mock_patch("argus_sdk._anthropic.report", side_effect=lambda ep, ev, api_key=None: posted.append(ev)):
        list(wrapper)

    assert len(posted) == 1
    e = posted[0]
    assert e["model"] == "claude-sonnet-4-6"
    assert e["input_tokens"] == 200
    assert e["output_tokens"] == 75
    assert e["finish_reason"] == "end_turn"
    assert e["provider"] == "anthropic"


def test_anthropic_sync_streaming_via_patch():
    """patch() with stream=True returns a wrapper transparently."""
    posted = []
    client = MagicMock()
    events = _make_anthropic_stream_events()
    stream = MagicMock()
    stream.__iter__ = MagicMock(return_value=iter(events))
    client.messages.create.return_value = stream

    with mock_patch("argus_sdk._anthropic.report", side_effect=lambda ep, ev, api_key=None: posted.append(ev)):
        anthropic_patch(client, "http://localhost:4000")
        result = client.messages.create(model="claude-sonnet-4-6", max_tokens=100, messages=[], stream=True)
        list(result)

    assert len(posted) == 1


# ---------------------------------------------------------------------------
# Anthropic async streaming
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_async_anthropic_streaming_yields_events():
    from argus_sdk._anthropic import _AsyncAnthropicStreamWrapper

    events = _make_anthropic_stream_events()

    async def async_iter():
        for e in events:
            yield e

    stream = MagicMock()
    stream.__aiter__ = async_iter

    wrapper = _AsyncAnthropicStreamWrapper(stream, time.monotonic(), "http://localhost:4000", None)

    with mock_patch("argus_sdk._anthropic.report"):
        yielded = [e async for e in wrapper]

    assert yielded == events


@pytest.mark.asyncio
async def test_async_anthropic_streaming_reports_after_exhaustion():
    from argus_sdk._anthropic import _AsyncAnthropicStreamWrapper

    posted = []
    events = _make_anthropic_stream_events(input_tokens=300, output_tokens=60)

    async def async_iter():
        for e in events:
            yield e

    stream = MagicMock()
    stream.__aiter__ = async_iter

    wrapper = _AsyncAnthropicStreamWrapper(stream, time.monotonic(), "http://localhost:4000", None)

    with mock_patch("argus_sdk._anthropic.report", side_effect=lambda ep, ev, api_key=None: posted.append(ev)):
        async for _ in wrapper:
            pass

    assert len(posted) == 1
    e = posted[0]
    assert e["input_tokens"] == 300
    assert e["output_tokens"] == 60


# ---------------------------------------------------------------------------
# OpenAI sync streaming
# ---------------------------------------------------------------------------

def _make_openai_stream_chunks(model="gpt-4o", prompt_tokens=100, completion_tokens=50):
    chunk1 = MagicMock()
    chunk1.model = model
    chunk1.choices = [MagicMock(finish_reason=None)]
    chunk1.usage = None

    chunk2 = MagicMock()
    chunk2.model = model
    chunk2.choices = [MagicMock(finish_reason="stop")]
    chunk2.usage = None

    chunk_usage = MagicMock()
    chunk_usage.model = model
    chunk_usage.choices = []
    chunk_usage.usage = MagicMock(prompt_tokens=prompt_tokens, completion_tokens=completion_tokens)

    return [chunk1, chunk2, chunk_usage]


def test_openai_sync_streaming_yields_chunks():
    from argus_sdk._openai import _SyncOpenAIStreamWrapper

    chunks = _make_openai_stream_chunks()
    stream = MagicMock()
    stream.__iter__ = MagicMock(return_value=iter(chunks))

    wrapper = _SyncOpenAIStreamWrapper(stream, time.monotonic(), "http://localhost:4000", None)

    with mock_patch("argus_sdk._openai.report"):
        yielded = list(wrapper)

    assert yielded == chunks


def test_openai_sync_streaming_reports_after_exhaustion():
    from argus_sdk._openai import _SyncOpenAIStreamWrapper

    posted = []
    chunks = _make_openai_stream_chunks(prompt_tokens=200, completion_tokens=80)
    stream = MagicMock()
    stream.__iter__ = MagicMock(return_value=iter(chunks))

    wrapper = _SyncOpenAIStreamWrapper(stream, time.monotonic(), "http://localhost:4000", None)

    with mock_patch("argus_sdk._openai.report", side_effect=lambda ep, ev, api_key=None: posted.append(ev)):
        list(wrapper)

    assert len(posted) == 1
    e = posted[0]
    assert e["model"] == "gpt-4o"
    assert e["input_tokens"] == 200
    assert e["output_tokens"] == 80
    assert e["finish_reason"] == "stop"
    assert e["provider"] == "openai"


def test_openai_stream_options_injected():
    """patch() must inject stream_options include_usage=True for stream=True calls."""
    client = MagicMock()
    chunks = _make_openai_stream_chunks()
    stream = MagicMock()
    stream.__iter__ = MagicMock(return_value=iter(chunks))
    client.chat.completions.create.return_value = stream

    with mock_patch("argus_sdk._openai.report"):
        openai_patch(client, "http://localhost:4000")
        result = client.chat.completions.create(model="gpt-4o", messages=[], stream=True)
        list(result)

    call_kwargs = client.chat.completions.create.call_args[1]
    assert call_kwargs.get("stream_options", {}).get("include_usage") is True


# ---------------------------------------------------------------------------
# OpenAI async streaming
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_async_openai_streaming_reports_after_exhaustion():
    from argus_sdk._openai import _AsyncOpenAIStreamWrapper

    posted = []
    chunks = _make_openai_stream_chunks(prompt_tokens=150, completion_tokens=40)

    async def async_iter():
        for c in chunks:
            yield c

    stream = MagicMock()
    stream.__aiter__ = async_iter

    wrapper = _AsyncOpenAIStreamWrapper(stream, time.monotonic(), "http://localhost:4000", None)

    with mock_patch("argus_sdk._openai.report", side_effect=lambda ep, ev, api_key=None: posted.append(ev)):
        async for _ in wrapper:
            pass

    assert len(posted) == 1
    e = posted[0]
    assert e["input_tokens"] == 150
    assert e["output_tokens"] == 40
```

- [ ] **Step 2: Run to verify fail**

```bash
cd /Users/prithviraj/Documents/CS/argus && make sdk-test 2>&1 | grep -E "FAILED|ERROR" | head -20
```

- [ ] **Step 3: Rewrite _openai.py with async + streaming support**

```python
import datetime
import inspect
import time

from ._reporter import report


def _inject_stream_options(kwargs: dict) -> dict:
    so = dict(kwargs.get("stream_options") or {})
    so["include_usage"] = True
    return {**kwargs, "stream_options": so}


class _SyncOpenAIStreamWrapper:
    def __init__(self, stream, t0, endpoint, api_key):
        self._stream = stream
        self._t0 = t0
        self._endpoint = endpoint
        self._api_key = api_key
        self._captured = {"model": "", "input_tokens": 0, "output_tokens": 0, "finish_reason": ""}

    def __iter__(self):
        for chunk in self._stream:
            if chunk.model and not self._captured["model"]:
                self._captured["model"] = chunk.model
            if chunk.choices and chunk.choices[0].finish_reason:
                self._captured["finish_reason"] = chunk.choices[0].finish_reason
            if chunk.usage is not None:
                self._captured["input_tokens"] = chunk.usage.prompt_tokens
                self._captured["output_tokens"] = chunk.usage.completion_tokens
            yield chunk
        latency_ms = int((time.monotonic() - self._t0) * 1000)
        report(self._endpoint, {
            **self._captured,
            "provider": "openai",
            "latency_ms": latency_ms,
            "timestamp_utc": _now(),
        }, api_key=self._api_key)

    def __getattr__(self, name):
        return getattr(self._stream, name)


class _AsyncOpenAIStreamWrapper:
    def __init__(self, stream, t0, endpoint, api_key):
        self._stream = stream
        self._t0 = t0
        self._endpoint = endpoint
        self._api_key = api_key
        self._captured = {"model": "", "input_tokens": 0, "output_tokens": 0, "finish_reason": ""}

    async def __aiter__(self):
        async for chunk in self._stream:
            if chunk.model and not self._captured["model"]:
                self._captured["model"] = chunk.model
            if chunk.choices and chunk.choices[0].finish_reason:
                self._captured["finish_reason"] = chunk.choices[0].finish_reason
            if chunk.usage is not None:
                self._captured["input_tokens"] = chunk.usage.prompt_tokens
                self._captured["output_tokens"] = chunk.usage.completion_tokens
            yield chunk
        latency_ms = int((time.monotonic() - self._t0) * 1000)
        report(self._endpoint, {
            **self._captured,
            "provider": "openai",
            "latency_ms": latency_ms,
            "timestamp_utc": _now(),
        }, api_key=self._api_key)

    def __getattr__(self, name):
        return getattr(self._stream, name)


def patch(client: object, endpoint: str, api_key: str | None = None) -> None:
    """Wrap client.chat.completions.create to capture signals after each response."""
    completions = client.chat.completions  # type: ignore[attr-defined]
    original_create = completions.create

    if inspect.iscoroutinefunction(original_create):
        async def _create(*args, **kwargs):
            if kwargs.get("stream"):
                t0 = time.monotonic()
                stream = await original_create(*args, **_inject_stream_options(kwargs))
                return _AsyncOpenAIStreamWrapper(stream, t0, endpoint, api_key)
            t0 = time.monotonic()
            response = await original_create(*args, **kwargs)
            latency_ms = int((time.monotonic() - t0) * 1000)
            finish_reason = ""
            if response.choices:
                finish_reason = response.choices[0].finish_reason or ""
            report(endpoint, {
                "model": response.model,
                "provider": "openai",
                "input_tokens": response.usage.prompt_tokens if response.usage else 0,
                "output_tokens": response.usage.completion_tokens if response.usage else 0,
                "latency_ms": latency_ms,
                "finish_reason": finish_reason,
                "timestamp_utc": _now(),
            }, api_key=api_key)
            return response
    else:
        def _create(*args, **kwargs):
            if kwargs.get("stream"):
                t0 = time.monotonic()
                stream = original_create(*args, **_inject_stream_options(kwargs))
                return _SyncOpenAIStreamWrapper(stream, t0, endpoint, api_key)
            t0 = time.monotonic()
            response = original_create(*args, **kwargs)
            latency_ms = int((time.monotonic() - t0) * 1000)
            finish_reason = ""
            if response.choices:
                finish_reason = response.choices[0].finish_reason or ""
            report(endpoint, {
                "model": response.model,
                "provider": "openai",
                "input_tokens": response.usage.prompt_tokens if response.usage else 0,
                "output_tokens": response.usage.completion_tokens if response.usage else 0,
                "latency_ms": latency_ms,
                "finish_reason": finish_reason,
                "timestamp_utc": _now(),
            }, api_key=api_key)
            return response

    completions.create = _create


def _now() -> str:
    return datetime.datetime.now(datetime.UTC).strftime("%Y-%m-%dT%H:%M:%SZ")
```

- [ ] **Step 4: Run tests**

```bash
cd /Users/prithviraj/Documents/CS/argus && make sdk-test 2>&1 | tail -20
```

- [ ] **Step 5: Commit**

```bash
git add sdk/argus_sdk/_openai.py sdk/tests/test_patch.py
git commit -m "feat(sdk): async OpenAI support + sync/async streaming for both providers"
```

---

### Task 4: Auto-patch AsyncAnthropic + AsyncOpenAI in __init__.py

**Files:**
- Modify: `sdk/argus_sdk/__init__.py`
- Test: `sdk/tests/test_patch.py`

- [ ] **Step 1: Write failing test**

Append to `sdk/tests/test_patch.py`:

```python
# ---------------------------------------------------------------------------
# Auto-patch async classes
# ---------------------------------------------------------------------------

def test_auto_patch_wraps_async_anthropic_class():
    """patch() in auto-mode must also wrap AsyncAnthropic.__init__."""
    class FakeAsyncAnthropic:
        pass

    FakeAsyncAnthropic.__module__ = "anthropic"

    fake_anthropic = MagicMock()
    fake_anthropic.Anthropic = MagicMock
    fake_anthropic.AsyncAnthropic = FakeAsyncAnthropic

    with mock_patch.dict("sys.modules", {"anthropic": fake_anthropic}):
        with mock_patch("argus_sdk._wrap_class_init") as mock_wrap:
            patch(endpoint="http://localhost:4000")

    calls = [c.args[0] for c in mock_wrap.call_args_list]
    assert FakeAsyncAnthropic in calls


def test_auto_patch_wraps_async_openai_class():
    """patch() in auto-mode must also wrap AsyncOpenAI.__init__."""
    class FakeAsyncOpenAI:
        pass

    FakeAsyncOpenAI.__module__ = "openai"

    fake_openai = MagicMock()
    fake_openai.OpenAI = MagicMock
    fake_openai.AsyncOpenAI = FakeAsyncOpenAI

    with mock_patch.dict("sys.modules", {"openai": fake_openai}):
        with mock_patch("argus_sdk._wrap_class_init") as mock_wrap:
            patch(endpoint="http://localhost:4000")

    calls = [c.args[0] for c in mock_wrap.call_args_list]
    assert FakeAsyncOpenAI in calls
```

- [ ] **Step 2: Update _try_patch_anthropic_class and _try_patch_openai_class in __init__.py**

```python
def _try_patch_anthropic_class(endpoint: str, api_key: str | None) -> None:
    try:
        import anthropic
        _wrap_class_init(anthropic.Anthropic, endpoint, provider="anthropic", api_key=api_key)
        _wrap_class_init(anthropic.AsyncAnthropic, endpoint, provider="anthropic", api_key=api_key)
    except ImportError:
        pass


def _try_patch_openai_class(endpoint: str, api_key: str | None) -> None:
    try:
        import openai
        _wrap_class_init(openai.OpenAI, endpoint, provider="openai", api_key=api_key)
        _wrap_class_init(openai.AsyncOpenAI, endpoint, provider="openai", api_key=api_key)
    except ImportError:
        pass
```

- [ ] **Step 3: Run tests**

```bash
cd /Users/prithviraj/Documents/CS/argus && make sdk-test 2>&1 | tail -20
```

- [ ] **Step 4: Commit**

```bash
git add sdk/argus_sdk/__init__.py sdk/tests/test_patch.py
git commit -m "feat(sdk): auto-patch AsyncAnthropic and AsyncOpenAI classes"
```

---

### Task 5: Version bump + README

**Files:**
- Modify: `sdk/pyproject.toml`
- Modify: `sdk/README.md`

- [ ] **Step 1: Bump version to 0.2.0**

In `sdk/pyproject.toml`, change `version = "0.1.3"` to `version = "0.2.0"`.

- [ ] **Step 2: Update README**

Replace all `http://localhost:4000` with `https://argus-sdk.com` in `sdk/README.md`.

- [ ] **Step 3: Run full test suite**

```bash
cd /Users/prithviraj/Documents/CS/argus && make sdk-test 2>&1 | tail -5
```

Expected: all pass

- [ ] **Step 4: Commit**

```bash
git add sdk/pyproject.toml sdk/README.md
git commit -m "chore(sdk): bump version to 0.2.0, update README examples to argus-sdk.com"
```
