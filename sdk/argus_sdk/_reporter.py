import atexit
import logging
import queue
import threading
import time

logger = logging.getLogger("argus_sdk")

_SENTINEL = object()
_SHUTDOWN_TIMEOUT = 5.0  # seconds to wait for flush on exit
_RETRY_DELAYS = [0.5, 1.0, 2.0]  # backoff between retries

_q: queue.Queue = queue.Queue()
_worker_thread: threading.Thread | None = None
_lock = threading.Lock()


def _post_with_retry(endpoint: str, event: dict) -> None:
    try:
        import httpx
    except ImportError:
        logger.debug("argus: httpx not installed, cannot report event")
        return

    url = f"{endpoint}/api/v1/events"
    last_exc: Exception | None = None

    for attempt, delay in enumerate([0] + _RETRY_DELAYS):
        if delay:
            time.sleep(delay)
        try:
            with httpx.Client(timeout=3.0) as client:
                resp = client.post(url, json=event)
            if resp.status_code < 500:
                # 2xx = success; 4xx = our fault, retrying won't help
                if resp.status_code >= 400:
                    logger.debug("argus: server rejected event (status %s)", resp.status_code)
                return
            last_exc = None
            logger.debug("argus: server error %s, attempt %d", resp.status_code, attempt + 1)
        except Exception as exc:
            last_exc = exc
            logger.debug("argus: request failed (attempt %d): %s", attempt + 1, exc)

    logger.debug("argus: gave up reporting event after %d attempts: %s", len(_RETRY_DELAYS) + 1, last_exc)


def _worker(endpoint: str) -> None:
    while True:
        item = _q.get()
        if item is _SENTINEL:
            _q.task_done()
            break
        event = item
        try:
            _post_with_retry(endpoint, event)
        finally:
            _q.task_done()


def _flush() -> None:
    """Drain the queue and stop the worker. Called automatically on process exit."""
    if _worker_thread is None or not _worker_thread.is_alive():
        return
    _q.put(_SENTINEL)
    _worker_thread.join(timeout=_SHUTDOWN_TIMEOUT)


def _ensure_worker(endpoint: str) -> None:
    global _worker_thread
    with _lock:
        if _worker_thread is not None and _worker_thread.is_alive():
            return
        t = threading.Thread(target=_worker, args=(endpoint,), daemon=True, name="argus-worker")
        t.start()
        _worker_thread = t
        atexit.register(_flush)


def report(endpoint: str, event: dict) -> None:
    _ensure_worker(endpoint)
    _q.put(event)


def flush(timeout: float = _SHUTDOWN_TIMEOUT) -> None:
    """Block until all queued events have been sent (or timeout expires).

    Useful in short scripts and CLI tools where you want to ensure events
    are delivered before the process exits.
    """
    _q.join()
