"""Tests for api_key support — Authorization header threading through the full stack."""
import json
import queue
import threading
from http.server import BaseHTTPRequestHandler, HTTPServer
from unittest.mock import MagicMock, patch as mock_patch

import pytest

import argus_sdk._reporter as reporter
from argus_sdk import patch as sdk_patch
from argus_sdk._anthropic import patch as anthropic_patch
from argus_sdk._openai import patch as openai_patch


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------

@pytest.fixture(autouse=True)
def reset_worker():
    """Each test starts with a clean reporter state."""
    reporter._q = queue.Queue()
    reporter._worker_thread = None
    yield
    reporter._flush()
    reporter._q = queue.Queue()
    reporter._worker_thread = None


def _capture_server():
    """Return (server, port, captured_dict). captured_dict fills after each POST."""
    captured: dict = {}

    class Handler(BaseHTTPRequestHandler):
        def do_POST(self):
            length = int(self.headers.get("Content-Length", 0))
            body = self.rfile.read(length)
            captured["auth"] = self.headers.get("Authorization", "")
            captured["body"] = body
            captured["content_type"] = self.headers.get("Content-Type", "")
            self.send_response(202)
            self.end_headers()

        def log_message(self, *args):
            pass

    server = HTTPServer(("localhost", 0), Handler)
    t = threading.Thread(target=server.serve_forever, daemon=True)
    t.start()
    return server, server.server_address[1], captured


# ---------------------------------------------------------------------------
# reporter.report() — low-level header tests
# ---------------------------------------------------------------------------

def test_report_sends_bearer_header_when_api_key_set():
    """api_key kwarg → Authorization: Bearer <key> header on the ingest request."""
    server, port, captured = _capture_server()
    reporter.report(f"http://localhost:{port}", {"model": "gpt-4o", "provider": "openai"}, api_key="argus_sk_abc123")
    reporter.flush()
    server.shutdown()
    assert captured["auth"] == "Bearer argus_sk_abc123"


def test_report_omits_authorization_header_when_no_api_key():
    """No api_key → no Authorization header at all."""
    server, port, captured = _capture_server()
    reporter.report(f"http://localhost:{port}", {"model": "gpt-4o", "provider": "openai"})
    reporter.flush()
    server.shutdown()
    assert captured.get("auth", "") == ""


def test_report_body_is_valid_json():
    """The request body must be parseable JSON."""
    server, port, captured = _capture_server()
    reporter.report(f"http://localhost:{port}", {"model": "gpt-4o", "provider": "openai", "input_tokens": 10})
    reporter.flush()
    server.shutdown()
    body = json.loads(captured["body"])
    assert body["model"] == "gpt-4o"


def test_report_body_has_required_contract_fields():
    """Every ingest POST must contain all 7 fields defined in the SDK contract."""
    REQUIRED = {"model", "provider", "input_tokens", "output_tokens", "latency_ms", "finish_reason", "timestamp_utc"}
    server, port, captured = _capture_server()
    event = {k: "x" if isinstance(k, str) else 0 for k in REQUIRED}
    event.update({"model": "gpt-4o", "provider": "openai", "input_tokens": 1,
                   "output_tokens": 1, "latency_ms": 10, "finish_reason": "stop",
                   "timestamp_utc": "2026-04-13T00:00:00Z"})
    reporter.report(f"http://localhost:{port}", event, api_key="key")
    reporter.flush()
    server.shutdown()
    body = json.loads(captured["body"])
    assert REQUIRED.issubset(body.keys()), f"Missing fields: {REQUIRED - body.keys()}"


def test_multiple_events_all_carry_api_key():
    """All events from a single report() call share the same api_key."""
    received_auths = []

    class MultiHandler(BaseHTTPRequestHandler):
        def do_POST(self):
            length = int(self.headers.get("Content-Length", 0))
            self.rfile.read(length)
            received_auths.append(self.headers.get("Authorization", ""))
            self.send_response(202)
            self.end_headers()

        def log_message(self, *args):
            pass

    srv = HTTPServer(("localhost", 0), MultiHandler)
    t = threading.Thread(target=srv.serve_forever, daemon=True)
    t.start()
    port = srv.server_address[1]

    for _ in range(5):
        reporter.report(f"http://localhost:{port}", {"model": "gpt-4o", "provider": "openai"}, api_key="argus_sk_multi")
    reporter.flush()
    srv.shutdown()

    assert len(received_auths) == 5
    assert all(h == "Bearer argus_sk_multi" for h in received_auths)


# ---------------------------------------------------------------------------
# anthropic_patch — api_key flows through wrapper → report()
# ---------------------------------------------------------------------------

def test_api_key_flows_through_anthropic_patch():
    """anthropic patch(api_key=...) passes the key to report()."""
    calls = []
    client = MagicMock()
    client.messages.create.return_value = _anthropic_response()

    with mock_patch("argus_sdk._anthropic.report",
                    side_effect=lambda ep, ev, api_key=None: calls.append(api_key)):
        anthropic_patch(client, "http://localhost:4000", api_key="argus_sk_ant")
        client.messages.create(model="claude-sonnet-4-6", max_tokens=100, messages=[])

    assert calls == ["argus_sk_ant"]


def test_api_key_none_anthropic_passes_none_to_report():
    """anthropic patch with no api_key passes api_key=None to report()."""
    calls = []
    client = MagicMock()
    client.messages.create.return_value = _anthropic_response()

    with mock_patch("argus_sdk._anthropic.report",
                    side_effect=lambda ep, ev, api_key=None: calls.append(api_key)):
        anthropic_patch(client, "http://localhost:4000")
        client.messages.create(model="claude-sonnet-4-6", max_tokens=100, messages=[])

    assert calls == [None]


# ---------------------------------------------------------------------------
# openai_patch — api_key flows through wrapper → report()
# ---------------------------------------------------------------------------

def test_api_key_flows_through_openai_patch():
    """openai patch(api_key=...) passes the key to report()."""
    calls = []
    client = MagicMock()
    client.chat.completions.create.return_value = _openai_response()

    with mock_patch("argus_sdk._openai.report",
                    side_effect=lambda ep, ev, api_key=None: calls.append(api_key)):
        openai_patch(client, "http://localhost:4000", api_key="argus_sk_oai")
        client.chat.completions.create(model="gpt-4o", messages=[])

    assert calls == ["argus_sk_oai"]


def test_api_key_none_openai_passes_none_to_report():
    """openai patch with no api_key passes api_key=None to report()."""
    calls = []
    client = MagicMock()
    client.chat.completions.create.return_value = _openai_response()

    with mock_patch("argus_sdk._openai.report",
                    side_effect=lambda ep, ev, api_key=None: calls.append(api_key)):
        openai_patch(client, "http://localhost:4000")
        client.chat.completions.create(model="gpt-4o", messages=[])

    assert calls == [None]


# ---------------------------------------------------------------------------
# sdk_patch() top-level — api_key wires through explicit and auto modes
# ---------------------------------------------------------------------------

def test_api_key_flows_through_explicit_anthropic_instance():
    """patch(client=..., api_key=...) wires api_key when given an explicit instance."""
    calls = []

    class FakeAnthropicClient:
        pass

    FakeAnthropicClient.__module__ = "anthropic"
    client = FakeAnthropicClient()
    client.messages = MagicMock()
    client.messages.create.return_value = _anthropic_response()

    with mock_patch("argus_sdk._anthropic.report",
                    side_effect=lambda ep, ev, api_key=None: calls.append(api_key)):
        sdk_patch(endpoint="http://localhost:4000", client=client, api_key="argus_sk_explicit")
        client.messages.create(model="claude-sonnet-4-6", max_tokens=100, messages=[])

    assert calls == ["argus_sk_explicit"]


def test_api_key_flows_through_explicit_openai_instance():
    """patch(client=..., api_key=...) wires api_key for openai instances."""
    calls = []

    class FakeOpenAIClient:
        pass

    FakeOpenAIClient.__module__ = "openai"
    client = FakeOpenAIClient()
    client.chat = MagicMock()
    client.chat.completions.create.return_value = _openai_response()

    with mock_patch("argus_sdk._openai.report",
                    side_effect=lambda ep, ev, api_key=None: calls.append(api_key)):
        sdk_patch(endpoint="http://localhost:4000", client=client, api_key="argus_sk_oai_explicit")
        client.chat.completions.create(model="gpt-4o", messages=[])

    assert calls == ["argus_sk_oai_explicit"]


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

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
