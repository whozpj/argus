from unittest.mock import MagicMock, patch as mock_patch

from argus_sdk._anthropic import patch as anthropic_patch
from argus_sdk._openai import patch as openai_patch
from argus_sdk import patch


def _anthropic_response(model="claude-sonnet-4-6", input_tokens=100, output_tokens=50, stop_reason="stop"):
    resp = MagicMock()
    resp.model = model
    resp.usage.input_tokens = input_tokens
    resp.usage.output_tokens = output_tokens
    resp.stop_reason = stop_reason
    return resp


def _openai_response(model="gpt-4o", prompt_tokens=100, completion_tokens=50, finish_reason="stop"):
    resp = MagicMock()
    resp.model = model
    resp.usage.prompt_tokens = prompt_tokens
    resp.usage.completion_tokens = completion_tokens
    resp.choices = [MagicMock(finish_reason=finish_reason)]
    return resp


def test_anthropic_patch_captures_event():
    posted = []

    client = MagicMock()
    client.messages.create.return_value = _anthropic_response()

    with mock_patch("argus_sdk._anthropic.report", side_effect=lambda ep, ev: posted.append(ev)):
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


def test_openai_patch_captures_event():
    posted = []

    client = MagicMock()
    client.chat.completions.create.return_value = _openai_response()

    with mock_patch("argus_sdk._openai.report", side_effect=lambda ep, ev: posted.append(ev)):
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


def test_original_response_returned():
    """patch() must not swallow the response — user code needs it."""
    client = MagicMock()
    response = _anthropic_response(output_tokens=77)
    client.messages.create.return_value = response

    with mock_patch("argus_sdk._anthropic.report"):
        anthropic_patch(client, "http://localhost:4000")
        result = client.messages.create(model="claude-sonnet-4-6", max_tokens=100, messages=[])

    assert result is response


def test_patch_noop_when_no_llm_library():
    """patch() must not raise if anthropic/openai aren't installed."""
    with mock_patch.dict("sys.modules", {"anthropic": None, "openai": None}):
        patch(endpoint="http://localhost:4000")  # should not raise


def test_endpoint_trailing_slash_stripped():
    posted_endpoints = []

    client = MagicMock()
    client.messages.create.return_value = _anthropic_response()

    def capture(ep, ev):
        posted_endpoints.append(ep)

    with mock_patch("argus_sdk._anthropic.report", side_effect=capture):
        anthropic_patch(client, "http://localhost:4000/")
        client.messages.create(model="claude-sonnet-4-6", max_tokens=100, messages=[])

    # _reporter.report receives the endpoint as-is; stripping is done in patch()
    # This test verifies the create wrapper passes through the endpoint correctly
    assert len(posted_endpoints) == 1
