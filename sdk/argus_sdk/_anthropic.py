import datetime
import time

from ._reporter import report


def patch(client: object, endpoint: str, api_key: str | None = None) -> None:
    """Wrap client.messages.create to capture signals after each response."""
    messages = client.messages  # type: ignore[attr-defined]
    original_create = messages.create

    def _create(*args, **kwargs):
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
