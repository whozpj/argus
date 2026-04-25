from __future__ import annotations

from typing import Any

from ._reporter import flush as flush  # noqa: F401 — re-exported for public API


def patch(
    endpoint: str = "https://argus-sdk.com",
    client: Any = None,
    api_key: str | None = None,
    name: str | None = None,
) -> None:
    """Instrument LLM clients to send signal events to the Argus server.

    Args:
        endpoint: Argus server URL.
        client:   Specific client instance to instrument. If omitted, all future
                  instances of Anthropic/OpenAI clients are instrumented automatically.
        api_key:  Argus project API key (argus_sk_…).
        name:     Optional pipeline label. When set, events are reported under
                  "<model>:<name>" so multiple patches of the same model (e.g.
                  "summarizer" vs "chatbot") appear as separate rows in the dashboard.

    Usage (auto — instruments all future clients):
        from argus_sdk import patch
        patch(endpoint="https://argus.example.com", api_key="argus_sk_...", name="chatbot")

        import anthropic
        client = anthropic.Anthropic()  # automatically instrumented

    Usage (explicit — instrument a specific instance):
        patch(endpoint="https://argus.example.com", client=my_client, api_key="argus_sk_...", name="summarizer")
    """
    _endpoint = endpoint.rstrip("/")

    if client is not None:
        _patch_instance(client, _endpoint, api_key, name)
        return

    _try_patch_anthropic_class(_endpoint, api_key, name)
    _try_patch_openai_class(_endpoint, api_key, name)


def _patch_instance(client: Any, endpoint: str, api_key: str | None, name: str | None) -> None:
    module = type(client).__module__ or ""
    if "anthropic" in module:
        from ._anthropic import patch as _ap
        _ap(client, endpoint, api_key, name)
    elif "openai" in module:
        from ._openai import patch as _op
        _op(client, endpoint, api_key, name)


def _try_patch_anthropic_class(endpoint: str, api_key: str | None, name: str | None) -> None:
    try:
        import anthropic
        _wrap_class_init(anthropic.Anthropic, endpoint, provider="anthropic", api_key=api_key, name=name)
        _wrap_class_init(anthropic.AsyncAnthropic, endpoint, provider="anthropic", api_key=api_key, name=name)
    except ImportError:
        pass


def _try_patch_openai_class(endpoint: str, api_key: str | None, name: str | None) -> None:
    try:
        import openai
        _wrap_class_init(openai.OpenAI, endpoint, provider="openai", api_key=api_key, name=name)
        _wrap_class_init(openai.AsyncOpenAI, endpoint, provider="openai", api_key=api_key, name=name)
    except ImportError:
        pass


def _wrap_class_init(cls: type, endpoint: str, provider: str, api_key: str | None, name: str | None) -> None:
    if getattr(cls, "_argus_patched", False):
        return

    original_init = cls.__init__

    def __init__(self, *args, **kwargs):
        original_init(self, *args, **kwargs)
        if provider == "anthropic":
            from ._anthropic import patch as _ap
            _ap(self, endpoint, api_key, name)
        else:
            from ._openai import patch as _op
            _op(self, endpoint, api_key, name)

    cls.__init__ = __init__
    cls._argus_patched = True
