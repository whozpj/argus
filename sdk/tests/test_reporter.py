"""Tests for argus_sdk._reporter — background HTTP posting."""
import time
import threading
from unittest.mock import MagicMock, patch as mock_patch

import pytest

import argus_sdk._reporter as reporter_module
from argus_sdk._reporter import _post_with_retry, report, flush


_SAMPLE_EVENT = {
    "model": "claude-sonnet-4-6",
    "provider": "anthropic",
    "input_tokens": 100,
    "output_tokens": 50,
    "latency_ms": 200,
    "finish_reason": "stop",
    "timestamp_utc": "2026-04-07T14:22:01Z",
}


@pytest.fixture(autouse=True)
def reset_worker():
    """Ensure each test starts with a clean worker state."""
    import queue
    reporter_module._q = queue.Queue()
    reporter_module._worker_thread = None
    yield
    # Drain and stop the worker if one was started
    reporter_module._flush()
    reporter_module._q = queue.Queue()
    reporter_module._worker_thread = None


# ---------------------------------------------------------------------------
# _post_with_retry — low-level HTTP tests
# ---------------------------------------------------------------------------

def test_reporter_posts_to_correct_url():
    """`_post_with_retry` must call POST /api/v1/events on the given endpoint."""
    mock_response = MagicMock()
    mock_response.status_code = 200
    mock_client_instance = MagicMock()
    mock_client_instance.post.return_value = mock_response

    with mock_patch("httpx.Client") as mock_httpx_client:
        mock_httpx_client.return_value.__enter__.return_value = mock_client_instance
        _post_with_retry("http://localhost:4000", _SAMPLE_EVENT)

    mock_client_instance.post.assert_called_once_with(
        "http://localhost:4000/api/v1/events",
        json=_SAMPLE_EVENT,
        headers={},
    )


def test_reporter_swallows_connection_error():
    """Network failures must never propagate to the caller."""
    import httpx

    with mock_patch("httpx.Client") as mock_httpx_client:
        mock_httpx_client.return_value.__enter__.return_value.post.side_effect = (
            httpx.ConnectError("Connection refused")
        )
        _post_with_retry("http://localhost:4000", _SAMPLE_EVENT)  # must not raise


def test_reporter_swallows_timeout_error():
    import httpx

    with mock_patch("httpx.Client") as mock_httpx_client:
        mock_httpx_client.return_value.__enter__.return_value.post.side_effect = (
            httpx.TimeoutException("timed out")
        )
        _post_with_retry("http://localhost:4000", _SAMPLE_EVENT)  # must not raise


def test_reporter_swallows_unexpected_exception():
    with mock_patch("httpx.Client") as mock_httpx_client:
        mock_httpx_client.return_value.__enter__.return_value.post.side_effect = RuntimeError("boom")
        _post_with_retry("http://localhost:4000", _SAMPLE_EVENT)  # must not raise


def test_reporter_does_not_retry_on_4xx():
    """4xx responses are client errors; retrying won't help, should only call once."""
    mock_response = MagicMock()
    mock_response.status_code = 400
    mock_client_instance = MagicMock()
    mock_client_instance.post.return_value = mock_response

    with mock_patch("httpx.Client") as mock_httpx_client:
        mock_httpx_client.return_value.__enter__.return_value = mock_client_instance
        _post_with_retry("http://localhost:4000", _SAMPLE_EVENT)

    assert mock_client_instance.post.call_count == 1


def test_reporter_retries_on_5xx():
    """5xx responses are server errors; should retry up to max attempts."""
    mock_response = MagicMock()
    mock_response.status_code = 500
    mock_client_instance = MagicMock()
    mock_client_instance.post.return_value = mock_response

    with mock_patch("httpx.Client") as mock_httpx_client, \
         mock_patch("time.sleep"):  # skip actual delays
        mock_httpx_client.return_value.__enter__.return_value = mock_client_instance
        _post_with_retry("http://localhost:4000", _SAMPLE_EVENT)

    # 1 initial attempt + 3 retries = 4 total
    assert mock_client_instance.post.call_count == 4


# ---------------------------------------------------------------------------
# report() + worker thread tests
# ---------------------------------------------------------------------------

def test_report_does_not_block():
    """`report()` must return before the HTTP call completes."""
    call_started = threading.Event()
    call_done = threading.Event()

    def slow_post(endpoint, event, api_key=None):
        call_started.set()
        time.sleep(0.1)
        call_done.set()

    with mock_patch("argus_sdk._reporter._post_with_retry", side_effect=slow_post):
        t0 = time.monotonic()
        report("http://localhost:4000", _SAMPLE_EVENT)
        elapsed = time.monotonic() - t0

    assert elapsed < 0.05, f"report() blocked for {elapsed:.3f}s — expected < 0.05s"
    call_started.wait(timeout=1.0)


def test_report_uses_single_persistent_worker():
    """`report()` reuses the same worker thread across multiple calls."""
    posted = []

    def capture_post(endpoint, event, api_key=None):
        posted.append(event)

    with mock_patch("argus_sdk._reporter._post_with_retry", side_effect=capture_post):
        report("http://localhost:4000", _SAMPLE_EVENT)
        report("http://localhost:4000", _SAMPLE_EVENT)
        report("http://localhost:4000", _SAMPLE_EVENT)
        flush(timeout=2.0)

    assert len(posted) == 3
    # Only one worker thread should exist
    assert reporter_module._worker_thread is not None
    workers = [t for t in threading.enumerate() if t.name == "argus-worker"]
    assert len(workers) <= 1


def test_flush_waits_for_all_events():
    """`flush()` must block until all queued events have been sent."""
    posted = []

    def slow_post(endpoint, event, api_key=None):
        time.sleep(0.02)
        posted.append(event)

    with mock_patch("argus_sdk._reporter._post_with_retry", side_effect=slow_post):
        for _ in range(5):
            report("http://localhost:4000", _SAMPLE_EVENT)
        flush(timeout=5.0)

    assert len(posted) == 5


def test_flush_is_safe_with_no_events():
    """`flush()` must not hang when there is nothing queued."""
    flush(timeout=1.0)  # should return immediately


def test_worker_is_daemon():
    """The worker thread must be a daemon so it doesn't prevent process exit."""
    with mock_patch("argus_sdk._reporter._post_with_retry"):
        report("http://localhost:4000", _SAMPLE_EVENT)

    assert reporter_module._worker_thread is not None
    assert reporter_module._worker_thread.daemon is True
