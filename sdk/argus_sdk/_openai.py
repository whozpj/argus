import datetime
import time

from ._reporter import report


def patch(client: object, endpoint: str) -> None:
    """Wrap client.chat.completions.create to capture signals after each response."""
    completions = client.chat.completions  # type: ignore[attr-defined]
    original_create = completions.create

    def _create(*args, **kwargs):
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
        })
        return response

    completions.create = _create


def _now() -> str:
    return datetime.datetime.now(datetime.UTC).strftime("%Y-%m-%dT%H:%M:%SZ")
