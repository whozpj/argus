import logging
import threading

logger = logging.getLogger("argus_sdk")


def _post(endpoint: str, event: dict) -> None:
    try:
        import httpx
        with httpx.Client(timeout=3.0) as client:
            client.post(f"{endpoint}/api/v1/events", json=event)
    except Exception as exc:
        logger.debug("argus: failed to report event: %s", exc)


def report(endpoint: str, event: dict) -> None:
    threading.Thread(target=_post, args=(endpoint, event), daemon=True).start()
