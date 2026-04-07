"""Tests for argus_sdk._reporter — background HTTP posting."""
import threading
from unittest.mock import MagicMock, patch as mock_patch

from argus_sdk._reporter import _post, report


_SAMPLE_EVENT = {
    "model": "claude-sonnet-4-6",
    "provider": "anthropic",
    "input_tokens": 100,
    "output_tokens": 50,
    "latency_ms": 200,
    "finish_reason": "stop",
    "timestamp_utc": "2026-04-07T14:22:01Z",
}


def test_reporter_posts_to_correct_url():
    """`_post` must call POST /api/v1/events on the given endpoint."""
    mock_response = MagicMock()
    mock_client_instance = MagicMock()
    mock_client_instance.post.return_value = mock_response

    with mock_patch("httpx.Client") as mock_httpx_client:
        mock_httpx_client.return_value.__enter__.return_value = mock_client_instance
        _post("http://localhost:4000", _SAMPLE_EVENT)

    mock_client_instance.post.assert_called_once_with(
        "http://localhost:4000/api/v1/events",
        json=_SAMPLE_EVENT,
    )


def test_reporter_swallows_connection_error():
    """Network failures must never propagate to the caller."""
    import httpx

    with mock_patch("httpx.Client") as mock_httpx_client:
        mock_httpx_client.return_value.__enter__.return_value.post.side_effect = (
            httpx.ConnectError("Connection refused")
        )
        _post("http://localhost:4000", _SAMPLE_EVENT)  # must not raise


def test_reporter_swallows_timeout_error():
    import httpx

    with mock_patch("httpx.Client") as mock_httpx_client:
        mock_httpx_client.return_value.__enter__.return_value.post.side_effect = (
            httpx.TimeoutException("timed out")
        )
        _post("http://localhost:4000", _SAMPLE_EVENT)  # must not raise


def test_reporter_swallows_unexpected_exception():
    with mock_patch("httpx.Client") as mock_httpx_client:
        mock_httpx_client.return_value.__enter__.return_value.post.side_effect = RuntimeError("boom")
        _post("http://localhost:4000", _SAMPLE_EVENT)  # must not raise


def test_report_fires_daemon_thread():
    """`report()` must start a daemon thread so it never blocks process exit."""
    threads_started = []

    real_thread_init = threading.Thread.__init__

    def capture_thread(self, *args, **kwargs):
        real_thread_init(self, *args, **kwargs)
        threads_started.append(self)

    with mock_patch.object(threading.Thread, "__init__", capture_thread):
        with mock_patch("argus_sdk._reporter._post"):  # don't actually POST
            report("http://localhost:4000", _SAMPLE_EVENT)

    assert len(threads_started) == 1
    assert threads_started[0].daemon is True


def test_report_does_not_block():
    """`report()` must return before the HTTP call completes."""
    import time

    call_started = threading.Event()
    call_done = threading.Event()

    def slow_post(endpoint, event):
        call_started.set()
        time.sleep(0.1)
        call_done.set()

    with mock_patch("argus_sdk._reporter._post", side_effect=slow_post):
        t0 = time.monotonic()
        report("http://localhost:4000", _SAMPLE_EVENT)
        elapsed = time.monotonic() - t0

    # report() returns almost instantly; 100 ms sleep is in the background thread
    assert elapsed < 0.05, f"report() blocked for {elapsed:.3f}s — expected < 0.05s"
    call_started.wait(timeout=1.0)  # background thread did fire
