from __future__ import annotations

import json
import pathlib


def _path() -> pathlib.Path:
    return pathlib.Path.home() / ".config" / "argus" / "credentials.json"


def save(server: str, token: str, email: str) -> None:
    """Write credentials to ~/.config/argus/credentials.json."""
    p = _path()
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(json.dumps({"server": server, "token": token, "email": email}, indent=2))


def load() -> dict | None:
    """Load credentials from ~/.config/argus/credentials.json. Returns None if not found."""
    p = _path()
    if not p.exists():
        return None
    try:
        return json.loads(p.read_text())
    except Exception:
        return None
