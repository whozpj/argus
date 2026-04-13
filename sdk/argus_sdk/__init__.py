from __future__ import annotations

from typing import Any

from ._reporter import flush as flush  # noqa: F401 — re-exported for public API


def patch(endpoint: str = "http://localhost:4000", client: Any = None, api_key: str | None = None) -> None:
    """Instrument LLM clients to send signal events to the Argus server.

    Usage (auto — instruments all future clients):
        from argus_sdk import patch
        patch(endpoint="https://argus.example.com", api_key="argus_sk_...")

        import anthropic
        client = anthropic.Anthropic()  # automatically instrumented

    Usage (explicit — instrument a specific instance):
        patch(endpoint="https://argus.example.com", client=my_client, api_key="argus_sk_...")
    """
    _endpoint = endpoint.rstrip("/")

    if client is not None:
        _patch_instance(client, _endpoint, api_key)
        return

    _try_patch_anthropic_class(_endpoint, api_key)
    _try_patch_openai_class(_endpoint, api_key)


def _patch_instance(client: Any, endpoint: str, api_key: str | None) -> None:
    module = type(client).__module__ or ""
    if "anthropic" in module:
        from ._anthropic import patch as _ap
        _ap(client, endpoint, api_key)
    elif "openai" in module:
        from ._openai import patch as _op
        _op(client, endpoint, api_key)


def _try_patch_anthropic_class(endpoint: str, api_key: str | None) -> None:
    try:
        import anthropic
        _wrap_class_init(anthropic.Anthropic, endpoint, provider="anthropic", api_key=api_key)
    except ImportError:
        pass


def _try_patch_openai_class(endpoint: str, api_key: str | None) -> None:
    try:
        import openai
        _wrap_class_init(openai.OpenAI, endpoint, provider="openai", api_key=api_key)
    except ImportError:
        pass


def _wrap_class_init(cls: type, endpoint: str, provider: str, api_key: str | None) -> None:
    if getattr(cls, "_argus_patched", False):
        return

    original_init = cls.__init__

    def __init__(self, *args, **kwargs):
        original_init(self, *args, **kwargs)
        if provider == "anthropic":
            from ._anthropic import patch as _ap
            _ap(self, endpoint, api_key)
        else:
            from ._openai import patch as _op
            _op(self, endpoint, api_key)

    cls.__init__ = __init__
    cls._argus_patched = True
