"""Tests for argus_sdk patch() and the Anthropic/OpenAI wrappers."""
import re
import time
from unittest.mock import MagicMock, call, patch as mock_patch

import pytest

from argus_sdk import patch, _wrap_class_init
from argus_sdk._anthropic import patch as anthropic_patch
from argus_sdk._openai import patch as openai_patch

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _anthropic_response(
    model="claude-sonnet-4-6",
    input_tokens=100,
    output_tokens=50,
    stop_reason="stop",
):
    resp = MagicMock()
    resp.model = model
    resp.usage.input_tokens = input_tokens
    resp.usage.output_tokens = output_tokens
    resp.stop_reason = stop_reason
    return resp


def _openai_response(
    model="gpt-4o",
    prompt_tokens=100,
    completion_tokens=50,
    finish_reason="stop",
):
    resp = MagicMock()
    resp.model = model
    resp.usage.prompt_tokens = prompt_tokens
    resp.usage.completion_tokens = completion_tokens
    resp.choices = [MagicMock(finish_reason=finish_reason)]
    return resp


# ---------------------------------------------------------------------------
# Anthropic — happy path
# ---------------------------------------------------------------------------

def test_anthropic_patch_captures_event():
    posted = []

    client = MagicMock()
    client.messages.create.return_value = _anthropic_response()

    with mock_patch("argus_sdk._anthropic.report", side_effect=lambda ep, ev, api_key=None: posted.append(ev)):
        anthropic_patch(client, "http://localhost:4000")
        client.messages.create(model="claude-sonnet-4-6", max_tokens=100, messages=[])

    assert len(posted) == 1
    e = posted[0]
    assert e["model"] == "claude-sonnet-4-6"
    assert e["provider"] == "anthropic"
    assert e["input_tokens"] == 100
    assert e["output_tokens"] == 50
    assert e["finish_reason"] == "stop"
    assert e["latency_ms"] >= 0
    assert e["timestamp_utc"].endswith("Z")


def test_anthropic_response_returned():
    """patch() must not swallow the response — user code depends on it."""
    client = MagicMock()
    response = _anthropic_response(output_tokens=77)
    client.messages.create.return_value = response

    with mock_patch("argus_sdk._anthropic.report"):
        anthropic_patch(client, "http://localhost:4000")
        result = client.messages.create(model="claude-sonnet-4-6", max_tokens=100, messages=[])

    assert result is response


# ---------------------------------------------------------------------------
# Anthropic — edge cases
# ---------------------------------------------------------------------------

def test_anthropic_null_stop_reason_becomes_empty_string():
    """stop_reason=None (e.g. mid-stream errors) must not produce null in the event."""
    posted = []

    client = MagicMock()
    client.messages.create.return_value = _anthropic_response(stop_reason=None)

    with mock_patch("argus_sdk._anthropic.report", side_effect=lambda ep, ev, api_key=None: posted.append(ev)):
        anthropic_patch(client, "http://localhost:4000")
        client.messages.create(model="claude-sonnet-4-6", max_tokens=100, messages=[])

    assert posted[0]["finish_reason"] == ""


def test_anthropic_event_has_all_required_keys():
    """The event payload must match the SDK integration contract exactly."""
    REQUIRED = {"model", "provider", "input_tokens", "output_tokens", "latency_ms", "finish_reason", "timestamp_utc"}
    posted = []

    client = MagicMock()
    client.messages.create.return_value = _anthropic_response()

    with mock_patch("argus_sdk._anthropic.report", side_effect=lambda ep, ev, api_key=None: posted.append(ev)):
        anthropic_patch(client, "http://localhost:4000")
        client.messages.create(model="claude-sonnet-4-6", max_tokens=100, messages=[])

    assert set(posted[0].keys()) == REQUIRED


def test_anthropic_timestamp_format():
    """timestamp_utc must be ISO 8601 UTC: YYYY-MM-DDTHH:MM:SSZ."""
    posted = []

    client = MagicMock()
    client.messages.create.return_value = _anthropic_response()

    with mock_patch("argus_sdk._anthropic.report", side_effect=lambda ep, ev, api_key=None: posted.append(ev)):
        anthropic_patch(client, "http://localhost:4000")
        client.messages.create(model="claude-sonnet-4-6", max_tokens=100, messages=[])

    ts = posted[0]["timestamp_utc"]
    assert re.fullmatch(r"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z", ts), f"Bad timestamp: {ts!r}"


def test_anthropic_latency_is_measured():
    """latency_ms must reflect actual wall-clock time of the underlying call."""
    posted = []

    client = MagicMock()

    def slow_create(*args, **kwargs):
        time.sleep(0.05)
        return _anthropic_response()

    client.messages.create.side_effect = slow_create

    with mock_patch("argus_sdk._anthropic.report", side_effect=lambda ep, ev, api_key=None: posted.append(ev)):
        anthropic_patch(client, "http://localhost:4000")
        client.messages.create(model="claude-sonnet-4-6", max_tokens=100, messages=[])

    assert posted[0]["latency_ms"] >= 40, "Expected at least 40 ms for a 50 ms sleep"


# ---------------------------------------------------------------------------
# OpenAI — happy path
# ---------------------------------------------------------------------------

def test_openai_patch_captures_event():
    posted = []

    client = MagicMock()
    client.chat.completions.create.return_value = _openai_response()

    with mock_patch("argus_sdk._openai.report", side_effect=lambda ep, ev, api_key=None: posted.append(ev)):
        openai_patch(client, "http://localhost:4000")
        client.chat.completions.create(model="gpt-4o", messages=[])

    assert len(posted) == 1
    e = posted[0]
    assert e["model"] == "gpt-4o"
    assert e["provider"] == "openai"
    assert e["input_tokens"] == 100
    assert e["output_tokens"] == 50
    assert e["finish_reason"] == "stop"
    assert e["latency_ms"] >= 0
    assert e["timestamp_utc"].endswith("Z")


def test_openai_response_returned():
    """OpenAI wrapper must not swallow the response."""
    client = MagicMock()
    response = _openai_response(completion_tokens=33)
    client.chat.completions.create.return_value = response

    with mock_patch("argus_sdk._openai.report"):
        openai_patch(client, "http://localhost:4000")
        result = client.chat.completions.create(model="gpt-4o", messages=[])

    assert result is response


# ---------------------------------------------------------------------------
# OpenAI — edge cases
# ---------------------------------------------------------------------------

def test_openai_no_choices_gives_empty_finish_reason():
    """If choices is empty (shouldn't happen, but guard it), finish_reason is ''."""
    posted = []

    client = MagicMock()
    resp = MagicMock()
    resp.model = "gpt-4o"
    resp.choices = []
    resp.usage.prompt_tokens = 10
    resp.usage.completion_tokens = 5
    client.chat.completions.create.return_value = resp

    with mock_patch("argus_sdk._openai.report", side_effect=lambda ep, ev, api_key=None: posted.append(ev)):
        openai_patch(client, "http://localhost:4000")
        client.chat.completions.create(model="gpt-4o", messages=[])

    assert posted[0]["finish_reason"] == ""


def test_openai_null_usage_gives_zero_tokens():
    """Some streaming/mock responses omit usage; must not crash."""
    posted = []

    client = MagicMock()
    resp = MagicMock()
    resp.model = "gpt-4o"
    resp.usage = None
    resp.choices = [MagicMock(finish_reason="stop")]
    client.chat.completions.create.return_value = resp

    with mock_patch("argus_sdk._openai.report", side_effect=lambda ep, ev, api_key=None: posted.append(ev)):
        openai_patch(client, "http://localhost:4000")
        client.chat.completions.create(model="gpt-4o", messages=[])

    assert posted[0]["input_tokens"] == 0
    assert posted[0]["output_tokens"] == 0


def test_openai_event_has_all_required_keys():
    REQUIRED = {"model", "provider", "input_tokens", "output_tokens", "latency_ms", "finish_reason", "timestamp_utc"}
    posted = []

    client = MagicMock()
    client.chat.completions.create.return_value = _openai_response()

    with mock_patch("argus_sdk._openai.report", side_effect=lambda ep, ev, api_key=None: posted.append(ev)):
        openai_patch(client, "http://localhost:4000")
        client.chat.completions.create(model="gpt-4o", messages=[])

    assert set(posted[0].keys()) == REQUIRED


# ---------------------------------------------------------------------------
# patch() — top-level API
# ---------------------------------------------------------------------------

def test_patch_noop_when_no_llm_library():
    """patch() must not raise even if neither anthropic nor openai is installed."""
    with mock_patch.dict("sys.modules", {"anthropic": None, "openai": None}):
        patch(endpoint="http://localhost:4000")


def test_patch_with_explicit_anthropic_client():
    """patch(client=...) should instrument a passed-in anthropic client directly."""
    posted = []

    class FakeAnthropicClient:
        pass

    FakeAnthropicClient.__module__ = "anthropic"
    client = FakeAnthropicClient()
    client.messages = MagicMock()
    client.messages.create.return_value = _anthropic_response()

    with mock_patch("argus_sdk._anthropic.report", side_effect=lambda ep, ev, api_key=None: posted.append(ev)):
        patch(endpoint="http://localhost:4000", client=client)
        client.messages.create(model="claude-sonnet-4-6", max_tokens=100, messages=[])

    assert len(posted) == 1
    assert posted[0]["provider"] == "anthropic"


def test_patch_with_explicit_openai_client():
    """patch(client=...) should instrument a passed-in openai client directly."""
    posted = []

    class FakeOpenAIClient:
        pass

    FakeOpenAIClient.__module__ = "openai"
    client = FakeOpenAIClient()
    client.chat = MagicMock()
    client.chat.completions.create.return_value = _openai_response()

    with mock_patch("argus_sdk._openai.report", side_effect=lambda ep, ev, api_key=None: posted.append(ev)):
        patch(endpoint="http://localhost:4000", client=client)
        client.chat.completions.create(model="gpt-4o", messages=[])

    assert len(posted) == 1
    assert posted[0]["provider"] == "openai"


def test_class_level_patched_only_once():
    """_wrap_class_init must be idempotent — calling it twice should not double-wrap."""
    class FakeClient:
        def __init__(self):
            self.messages = MagicMock()
            self.messages.create.return_value = _anthropic_response()

    _wrap_class_init(FakeClient, "http://localhost:4000", provider="anthropic", api_key=None, name=None)
    _wrap_class_init(FakeClient, "http://localhost:4000", provider="anthropic", api_key=None, name=None)  # second call

    assert FakeClient._argus_patched is True

    posted = []
    with mock_patch("argus_sdk._anthropic.report", side_effect=lambda ep, ev, api_key=None: posted.append(ev)):
        with mock_patch("argus_sdk._anthropic.patch") as mock_ap:
            FakeClient()
            assert mock_ap.call_count == 1  # wrapped once, not twice


def test_endpoint_passed_to_reporter():
    """The endpoint given to patch() must be forwarded to report()."""
    posted_endpoints = []

    client = MagicMock()
    client.messages.create.return_value = _anthropic_response()

    with mock_patch("argus_sdk._anthropic.report", side_effect=lambda ep, ev, api_key=None: posted_endpoints.append(ep)):
        anthropic_patch(client, "http://my-argus-server:4000")
        client.messages.create(model="claude-sonnet-4-6", max_tokens=100, messages=[])

    assert posted_endpoints[0] == "http://my-argus-server:4000"


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
    from argus_sdk._anthropic import _SyncAnthropicStreamWrapper

    events = _make_anthropic_stream_events()
    stream = MagicMock()
    stream.__iter__ = MagicMock(return_value=iter(events))

    wrapper = _SyncAnthropicStreamWrapper(stream, time.monotonic(), "http://localhost:4000", None, None)

    with mock_patch("argus_sdk._anthropic.report"):
        yielded = list(wrapper)

    assert yielded == events


def test_anthropic_sync_streaming_reports_after_exhaustion():
    from argus_sdk._anthropic import _SyncAnthropicStreamWrapper

    posted = []
    events = _make_anthropic_stream_events(input_tokens=200, output_tokens=75)
    stream = MagicMock()
    stream.__iter__ = MagicMock(return_value=iter(events))

    wrapper = _SyncAnthropicStreamWrapper(stream, time.monotonic(), "http://localhost:4000", None, None)

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

    class FakeAsyncStream:
        async def __aiter__(self):
            for e in events:
                yield e

    wrapper = _AsyncAnthropicStreamWrapper(FakeAsyncStream(), time.monotonic(), "http://localhost:4000", None, None)

    with mock_patch("argus_sdk._anthropic.report"):
        yielded = [e async for e in wrapper]

    assert yielded == events


@pytest.mark.asyncio
async def test_async_anthropic_streaming_reports_after_exhaustion():
    from argus_sdk._anthropic import _AsyncAnthropicStreamWrapper

    posted = []
    events = _make_anthropic_stream_events(input_tokens=300, output_tokens=60)

    class FakeAsyncStream:
        async def __aiter__(self):
            for e in events:
                yield e

    wrapper = _AsyncAnthropicStreamWrapper(FakeAsyncStream(), time.monotonic(), "http://localhost:4000", None, None)

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

    wrapper = _SyncOpenAIStreamWrapper(stream, time.monotonic(), "http://localhost:4000", None, None)

    with mock_patch("argus_sdk._openai.report"):
        yielded = list(wrapper)

    assert yielded == chunks


def test_openai_sync_streaming_reports_after_exhaustion():
    from argus_sdk._openai import _SyncOpenAIStreamWrapper

    posted = []
    chunks = _make_openai_stream_chunks(prompt_tokens=200, completion_tokens=80)
    stream = MagicMock()
    stream.__iter__ = MagicMock(return_value=iter(chunks))

    wrapper = _SyncOpenAIStreamWrapper(stream, time.monotonic(), "http://localhost:4000", None, None)

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
    chunks = _make_openai_stream_chunks()
    stream_mock = MagicMock()
    stream_mock.__iter__ = MagicMock(return_value=iter(chunks))

    underlying_create = MagicMock(return_value=stream_mock)
    client = MagicMock()
    client.chat.completions.create = underlying_create

    with mock_patch("argus_sdk._openai.report"):
        openai_patch(client, "http://localhost:4000")
        result = client.chat.completions.create(model="gpt-4o", messages=[], stream=True)
        list(result)

    # underlying_create (captured before patching) receives the injected kwargs
    call_kwargs = underlying_create.call_args[1]
    assert call_kwargs.get("stream_options", {}).get("include_usage") is True


# ---------------------------------------------------------------------------
# OpenAI async streaming
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_async_openai_streaming_reports_after_exhaustion():
    from argus_sdk._openai import _AsyncOpenAIStreamWrapper

    posted = []
    chunks = _make_openai_stream_chunks(prompt_tokens=150, completion_tokens=40)

    class FakeAsyncStream:
        async def __aiter__(self):
            for c in chunks:
                yield c

    wrapper = _AsyncOpenAIStreamWrapper(FakeAsyncStream(), time.monotonic(), "http://localhost:4000", None, None)

    with mock_patch("argus_sdk._openai.report", side_effect=lambda ep, ev, api_key=None: posted.append(ev)):
        async for _ in wrapper:
            pass

    assert len(posted) == 1
    e = posted[0]
    assert e["input_tokens"] == 150
    assert e["output_tokens"] == 40


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
