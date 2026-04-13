import queue
import threading

import pytest

import argus_sdk._reporter as reporter


@pytest.fixture(autouse=True)
def reset_worker():
    """Ensure each test starts and ends with a clean worker state."""
    reporter._q = queue.Queue()
    reporter._worker_thread = None
    yield
    reporter._flush()
    reporter._q = queue.Queue()
    reporter._worker_thread = None


def _start_capture_server():
    """Start a minimal HTTP server that captures the last request headers."""
    from http.server import BaseHTTPRequestHandler, HTTPServer

    captured = {}

    class Handler(BaseHTTPRequestHandler):
        def do_POST(self):
            length = int(self.headers.get("Content-Length", 0))
            self.rfile.read(length)
            captured["auth"] = self.headers.get("Authorization", "")
            self.send_response(202)
            self.end_headers()

        def log_message(self, *args):
            pass

    server = HTTPServer(("localhost", 0), Handler)
    port = server.server_address[1]
    t = threading.Thread(target=server.serve_forever, daemon=True)
    t.start()
    return server, port, captured


def test_report_sends_authorization_header_when_api_key_set():
    server, port, captured = _start_capture_server()
    endpoint = f"http://localhost:{port}"

    reporter.report(endpoint, {"model": "gpt-4o", "provider": "openai"}, api_key="argus_sk_testkey123")
    reporter.flush()

    server.shutdown()
    assert captured.get("auth") == "Bearer argus_sk_testkey123"


def test_report_no_authorization_header_without_api_key():
    server, port, captured = _start_capture_server()
    endpoint = f"http://localhost:{port}"

    reporter.report(endpoint, {"model": "gpt-4o", "provider": "openai"})
    reporter.flush()

    server.shutdown()
    assert captured.get("auth", "") == ""
