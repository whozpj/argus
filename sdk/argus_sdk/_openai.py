import datetime
import inspect
import time

from ._reporter import report


def _inject_stream_options(kwargs: dict) -> dict:
    so = dict(kwargs.get("stream_options") or {})
    so["include_usage"] = True
    return {**kwargs, "stream_options": so}


class _SyncOpenAIStreamWrapper:
    def __init__(self, stream, t0, endpoint, api_key):
        self._stream = stream
        self._t0 = t0
        self._endpoint = endpoint
        self._api_key = api_key
        self._captured = {"model": "", "input_tokens": 0, "output_tokens": 0, "finish_reason": ""}

    def __iter__(self):
        for chunk in self._stream:
            if chunk.model and not self._captured["model"]:
                self._captured["model"] = chunk.model
            if chunk.choices and chunk.choices[0].finish_reason:
                self._captured["finish_reason"] = chunk.choices[0].finish_reason
            if chunk.usage is not None:
                self._captured["input_tokens"] = chunk.usage.prompt_tokens
                self._captured["output_tokens"] = chunk.usage.completion_tokens
            yield chunk
        latency_ms = int((time.monotonic() - self._t0) * 1000)
        report(self._endpoint, {
            **self._captured,
            "provider": "openai",
            "latency_ms": latency_ms,
            "timestamp_utc": _now(),
        }, api_key=self._api_key)

    def __getattr__(self, name):
        return getattr(self._stream, name)


class _AsyncOpenAIStreamWrapper:
    def __init__(self, stream, t0, endpoint, api_key):
        self._stream = stream
        self._t0 = t0
        self._endpoint = endpoint
        self._api_key = api_key
        self._captured = {"model": "", "input_tokens": 0, "output_tokens": 0, "finish_reason": ""}

    async def __aiter__(self):
        async for chunk in self._stream:
            if chunk.model and not self._captured["model"]:
                self._captured["model"] = chunk.model
            if chunk.choices and chunk.choices[0].finish_reason:
                self._captured["finish_reason"] = chunk.choices[0].finish_reason
            if chunk.usage is not None:
                self._captured["input_tokens"] = chunk.usage.prompt_tokens
                self._captured["output_tokens"] = chunk.usage.completion_tokens
            yield chunk
        latency_ms = int((time.monotonic() - self._t0) * 1000)
        report(self._endpoint, {
            **self._captured,
            "provider": "openai",
            "latency_ms": latency_ms,
            "timestamp_utc": _now(),
        }, api_key=self._api_key)

    def __getattr__(self, name):
        return getattr(self._stream, name)


def patch(client: object, endpoint: str, api_key: str | None = None) -> None:
    """Wrap client.chat.completions.create to capture signals after each response."""
    completions = client.chat.completions  # type: ignore[attr-defined]
    original_create = completions.create

    if inspect.iscoroutinefunction(original_create):
        async def _create(*args, **kwargs):
            if kwargs.get("stream"):
                t0 = time.monotonic()
                stream = await original_create(*args, **_inject_stream_options(kwargs))
                return _AsyncOpenAIStreamWrapper(stream, t0, endpoint, api_key)
            t0 = time.monotonic()
            response = await original_create(*args, **kwargs)
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
            }, api_key=api_key)
            return response
    else:
        def _create(*args, **kwargs):
            if kwargs.get("stream"):
                t0 = time.monotonic()
                stream = original_create(*args, **_inject_stream_options(kwargs))
                return _SyncOpenAIStreamWrapper(stream, t0, endpoint, api_key)
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
            }, api_key=api_key)
            return response

    completions.create = _create


def _now() -> str:
    return datetime.datetime.now(datetime.UTC).strftime("%Y-%m-%dT%H:%M:%SZ")
