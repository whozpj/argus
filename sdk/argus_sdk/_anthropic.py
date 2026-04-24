import datetime
import inspect
import time

from ._reporter import report


class _SyncAnthropicStreamWrapper:
    def __init__(self, stream, t0, endpoint, api_key):
        self._stream = stream
        self._t0 = t0
        self._endpoint = endpoint
        self._api_key = api_key
        self._captured = {}

    def __iter__(self):
        for event in self._stream:
            if event.type == "message_start":
                self._captured["model"] = event.message.model
                self._captured["input_tokens"] = event.message.usage.input_tokens
            elif event.type == "message_delta":
                self._captured["output_tokens"] = event.usage.output_tokens
                self._captured["finish_reason"] = event.delta.stop_reason or ""
            yield event
        latency_ms = int((time.monotonic() - self._t0) * 1000)
        report(self._endpoint, {
            **self._captured,
            "provider": "anthropic",
            "latency_ms": latency_ms,
            "timestamp_utc": _now(),
        }, api_key=self._api_key)

    def __getattr__(self, name):
        return getattr(self._stream, name)


class _AsyncAnthropicStreamWrapper:
    def __init__(self, stream, t0, endpoint, api_key):
        self._stream = stream
        self._t0 = t0
        self._endpoint = endpoint
        self._api_key = api_key
        self._captured = {}

    async def __aiter__(self):
        async for event in self._stream:
            if event.type == "message_start":
                self._captured["model"] = event.message.model
                self._captured["input_tokens"] = event.message.usage.input_tokens
            elif event.type == "message_delta":
                self._captured["output_tokens"] = event.usage.output_tokens
                self._captured["finish_reason"] = event.delta.stop_reason or ""
            yield event
        latency_ms = int((time.monotonic() - self._t0) * 1000)
        report(self._endpoint, {
            **self._captured,
            "provider": "anthropic",
            "latency_ms": latency_ms,
            "timestamp_utc": _now(),
        }, api_key=self._api_key)

    def __getattr__(self, name):
        return getattr(self._stream, name)


def patch(client: object, endpoint: str, api_key: str | None = None) -> None:
    """Wrap client.messages.create to capture signals after each response."""
    messages = client.messages  # type: ignore[attr-defined]
    original_create = messages.create

    if inspect.iscoroutinefunction(original_create):
        async def _create(*args, **kwargs):
            if kwargs.get("stream"):
                t0 = time.monotonic()
                stream = await original_create(*args, **kwargs)
                return _AsyncAnthropicStreamWrapper(stream, t0, endpoint, api_key)
            t0 = time.monotonic()
            response = await original_create(*args, **kwargs)
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
    else:
        def _create(*args, **kwargs):
            if kwargs.get("stream"):
                t0 = time.monotonic()
                stream = original_create(*args, **kwargs)
                return _SyncAnthropicStreamWrapper(stream, t0, endpoint, api_key)
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
