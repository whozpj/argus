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

    _wrap_class_init(FakeClient, "http://localhost:4000", provider="anthropic", api_key=None)
    _wrap_class_init(FakeClient, "http://localhost:4000", provider="anthropic", api_key=None)  # second call

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
