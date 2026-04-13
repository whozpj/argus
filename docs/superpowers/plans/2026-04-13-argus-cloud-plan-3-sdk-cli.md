# Argus Cloud — Plan 3: SDK + CLI

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `api_key` support to the Python SDK so cloud users authenticate ingest calls, and ship an `argus` CLI with `login`, `status`, and `projects` commands.

**Architecture:** The SDK's `patch()` gains an `api_key` parameter; when set, every ingest request includes `Authorization: Bearer <api_key>`. The CLI is a `click` app shipped as a console script inside the existing `argus-sdk` package. Credentials (server URL + JWT) are stored in `~/.config/argus/credentials.json`. `argus login` opens the browser to the server's `/auth/cli` endpoint, receives a one-time code via a local HTTP callback server, exchanges it for a JWT, and saves it. `argus status` and `argus projects` call the server API with the stored JWT.

**Tech Stack:** Python 3.12+, `click>=8.0`, `httpx>=0.27`, stdlib `http.server`, `webbrowser`, `json`.

---

## File Map

| File | Action | What changes |
|---|---|---|
| `sdk/argus_sdk/_reporter.py` | Modify | `report()` and worker accept `api_key`; POST includes `Authorization` header |
| `sdk/argus_sdk/__init__.py` | Modify | `patch()` gains `api_key=None` parameter; passes it through |
| `sdk/argus_sdk/_anthropic.py` | Modify | `patch()` gains `api_key=None`; passes to `report()` |
| `sdk/argus_sdk/_openai.py` | Modify | `patch()` gains `api_key=None`; passes to `report()` |
| `sdk/argus_sdk/_credentials.py` | Create | `load()` / `save()` for `~/.config/argus/credentials.json` |
| `sdk/argus_sdk/cli.py` | Create | `argus` click group; `login`, `status`, `projects` commands |
| `sdk/pyproject.toml` | Modify | Add `click` dep; add `[project.scripts] argus = "argus_sdk.cli:cli"` |
| `sdk/tests/test_api_key.py` | Create | Tests: api_key flows through reporter, Authorization header is set |
| `sdk/tests/test_cli.py` | Create | Tests: credentials load/save, status + projects with mocked server |

---

## Task 1: Add `api_key` to reporter

**Files:**
- Modify: `sdk/argus_sdk/_reporter.py`
- Create: `sdk/tests/test_api_key.py`

- [ ] **Step 1: Write the failing tests**

Create `sdk/tests/test_api_key.py`:

```python
import threading
import time
from http.server import BaseHTTPRequestHandler, HTTPServer

import pytest

import argus_sdk._reporter as reporter


def _start_capture_server():
    """Start a minimal HTTP server that captures the last request headers."""
    captured = {}

    class Handler(BaseHTTPRequestHandler):
        def do_POST(self):
            length = int(self.headers.get("Content-Length", 0))
            self.rfile.read(length)
            captured["auth"] = self.headers.get("Authorization", "")
            self.send_response(202)
            self.end_headers()

        def log_message(self, *args):
            pass  # silence server logs in tests

    server = HTTPServer(("localhost", 0), Handler)
    port = server.server_address[1]
    t = threading.Thread(target=server.serve_forever, daemon=True)
    t.start()
    return server, port, captured


def test_report_sends_authorization_header_when_api_key_set():
    server, port, captured = _start_capture_server()
    endpoint = f"http://localhost:{port}"

    reporter.report(endpoint, {"model": "gpt-4o", "provider": "openai"}, api_key="argus_sk_testkey123")
    reporter.flush()

    server.shutdown()
    assert captured.get("auth") == "Bearer argus_sk_testkey123"


def test_report_no_authorization_header_without_api_key():
    server, port, captured = _start_capture_server()
    endpoint = f"http://localhost:{port}"

    reporter.report(endpoint, {"model": "gpt-4o", "provider": "openai"})
    reporter.flush()

    server.shutdown()
    assert captured.get("auth", "") == ""
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/prithviraj/Documents/CS/argus/sdk && python -m pytest tests/test_api_key.py -v 2>&1 | head -20
```

Expected: `TypeError` — `report()` got unexpected keyword argument `api_key`.

- [ ] **Step 3: Update `_reporter.py`**

Replace the full content of `sdk/argus_sdk/_reporter.py`:

```python
import atexit
import logging
import queue
import threading
import time

logger = logging.getLogger("argus_sdk")

_SENTINEL = object()
_SHUTDOWN_TIMEOUT = 5.0
_RETRY_DELAYS = [0.5, 1.0, 2.0]

_q: queue.Queue = queue.Queue()
_worker_thread: threading.Thread | None = None
_lock = threading.Lock()


def _post_with_retry(endpoint: str, event: dict, api_key: str | None = None) -> None:
    try:
        import httpx
    except ImportError:
        logger.debug("argus: httpx not installed, cannot report event")
        return

    url = f"{endpoint}/api/v1/events"
    headers = {}
    if api_key:
        headers["Authorization"] = f"Bearer {api_key}"

    last_exc: Exception | None = None

    for attempt, delay in enumerate([0] + _RETRY_DELAYS):
        if delay:
            time.sleep(delay)
        try:
            with httpx.Client(timeout=3.0) as client:
                resp = client.post(url, json=event, headers=headers)
            if resp.status_code < 500:
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
        event, api_key = item
        try:
            _post_with_retry(endpoint, event, api_key)
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


def report(endpoint: str, event: dict, api_key: str | None = None) -> None:
    _ensure_worker(endpoint)
    _q.put((event, api_key))


def flush(timeout: float = _SHUTDOWN_TIMEOUT) -> None:
    """Block until all queued events have been sent (or timeout expires)."""
    _q.join()
```

- [ ] **Step 4: Run tests and verify they pass**

```bash
cd /Users/prithviraj/Documents/CS/argus/sdk && python -m pytest tests/test_api_key.py -v
```

Expected: both tests PASS.

- [ ] **Step 5: Run full SDK test suite to check no regressions**

```bash
cd /Users/prithviraj/Documents/CS/argus/sdk && python -m pytest tests/ -v
```

Expected: all existing tests still pass.

- [ ] **Step 6: Commit**

```bash
cd /Users/prithviraj/Documents/CS/argus && git add sdk/argus_sdk/_reporter.py sdk/tests/test_api_key.py
git commit -m "feat(sdk): add api_key support to reporter"
```

---

## Task 2: Thread `api_key` through `patch()` and LLM wrappers

**Files:**
- Modify: `sdk/argus_sdk/__init__.py`
- Modify: `sdk/argus_sdk/_anthropic.py`
- Modify: `sdk/argus_sdk/_openai.py`

- [ ] **Step 1: Update `_anthropic.py`**

Replace the full content of `sdk/argus_sdk/_anthropic.py`:

```python
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
```

- [ ] **Step 2: Update `_openai.py`**

Replace the full content of `sdk/argus_sdk/_openai.py`:

```python
import datetime
import time

from ._reporter import report


def patch(client: object, endpoint: str, api_key: str | None = None) -> None:
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
        }, api_key=api_key)
        return response

    completions.create = _create


def _now() -> str:
    return datetime.datetime.now(datetime.UTC).strftime("%Y-%m-%dT%H:%M:%SZ")
```

- [ ] **Step 3: Update `__init__.py`**

Replace the full content of `sdk/argus_sdk/__init__.py`:

```python
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
```

- [ ] **Step 4: Run the full test suite**

```bash
cd /Users/prithviraj/Documents/CS/argus/sdk && python -m pytest tests/ -v
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
cd /Users/prithviraj/Documents/CS/argus && git add sdk/argus_sdk/__init__.py sdk/argus_sdk/_anthropic.py sdk/argus_sdk/_openai.py
git commit -m "feat(sdk): thread api_key through patch() and LLM wrappers"
```

---

## Task 3: Credentials helper

**Files:**
- Create: `sdk/argus_sdk/_credentials.py`

- [ ] **Step 1: Write the failing tests**

Add to `sdk/tests/test_cli.py` (create the file):

```python
import json
import os
import pathlib
import tempfile

import pytest

import argus_sdk._credentials as creds


def test_save_and_load(tmp_path, monkeypatch):
    monkeypatch.setenv("HOME", str(tmp_path))
    creds.save("http://localhost:4000", "test-token", "user@example.com")

    data = creds.load()
    assert data["server"] == "http://localhost:4000"
    assert data["token"] == "test-token"
    assert data["email"] == "user@example.com"


def test_load_returns_none_when_missing(tmp_path, monkeypatch):
    monkeypatch.setenv("HOME", str(tmp_path))
    assert creds.load() is None


def test_credentials_file_path(tmp_path, monkeypatch):
    monkeypatch.setenv("HOME", str(tmp_path))
    creds.save("http://localhost:4000", "tok", "u@example.com")
    expected = tmp_path / ".config" / "argus" / "credentials.json"
    assert expected.exists()
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/prithviraj/Documents/CS/argus/sdk && python -m pytest tests/test_cli.py -v 2>&1 | head -20
```

Expected: `ModuleNotFoundError` — `argus_sdk._credentials` not found.

- [ ] **Step 3: Implement `_credentials.py`**

Create `sdk/argus_sdk/_credentials.py`:

```python
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
```

- [ ] **Step 4: Run tests and verify they pass**

```bash
cd /Users/prithviraj/Documents/CS/argus/sdk && python -m pytest tests/test_cli.py -v
```

Expected: all 3 credential tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/prithviraj/Documents/CS/argus && git add sdk/argus_sdk/_credentials.py sdk/tests/test_cli.py
git commit -m "feat(cli): add credentials load/save helper"
```

---

## Task 4: CLI commands — `status` and `projects`

**Files:**
- Create: `sdk/argus_sdk/cli.py`
- Modify: `sdk/tests/test_cli.py`
- Modify: `sdk/pyproject.toml`

- [ ] **Step 1: Add `click` to `pyproject.toml`**

In `sdk/pyproject.toml`, add `"click>=8.0"` to the `dependencies` list and add the scripts entry:

```toml
[project]
name = "argus-sdk"
version = "0.1.3"
description = "Detect when your LLM's behavior has statistically shifted — one line of code to instrument, one Docker container to run."
readme = "README.md"
requires-python = ">=3.12"
dependencies = [
    "anthropic>=0.25",
    "httpx>=0.27",
    "click>=8.0",
]

[project.scripts]
argus = "argus_sdk.cli:cli"

[project.optional-dependencies]
dev = [
    "pytest>=8.0",
    "pytest-asyncio>=0.23",
    "ruff>=0.4",
]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"

[tool.hatch.build.targets.wheel]
packages = ["argus_sdk"]

[tool.ruff]
line-length = 100
target-version = "py312"

[tool.pytest.ini_options]
asyncio_mode = "auto"
```

- [ ] **Step 2: Install the updated dependencies**

```bash
cd /Users/prithviraj/Documents/CS/argus/sdk && pip install -e ".[dev]" -q
```

- [ ] **Step 3: Write failing tests for `status` and `projects`**

Append to `sdk/tests/test_cli.py`:

```python
from unittest.mock import MagicMock, patch

from click.testing import CliRunner

from argus_sdk.cli import cli


def test_status_not_logged_in(tmp_path, monkeypatch):
    monkeypatch.setenv("HOME", str(tmp_path))
    runner = CliRunner()
    result = runner.invoke(cli, ["status"])
    assert result.exit_code == 0
    assert "Not logged in" in result.output


def test_status_logged_in(tmp_path, monkeypatch):
    import argus_sdk._credentials as creds
    monkeypatch.setenv("HOME", str(tmp_path))
    creds.save("http://localhost:4000", "test-token", "user@example.com")

    mock_resp = MagicMock()
    mock_resp.status_code = 200
    mock_resp.json.return_value = {
        "id": "user-123",
        "email": "user@example.com",
        "projects": [{"id": "proj-1", "name": "my-api", "created_at": "2026-04-13T00:00:00Z"}],
    }

    with patch("httpx.get", return_value=mock_resp):
        runner = CliRunner()
        result = runner.invoke(cli, ["status"])

    assert result.exit_code == 0
    assert "user@example.com" in result.output
    assert "my-api" in result.output


def test_projects_not_logged_in(tmp_path, monkeypatch):
    monkeypatch.setenv("HOME", str(tmp_path))
    runner = CliRunner()
    result = runner.invoke(cli, ["projects"])
    assert result.exit_code == 0
    assert "Not logged in" in result.output


def test_projects_lists_projects(tmp_path, monkeypatch):
    import argus_sdk._credentials as creds
    monkeypatch.setenv("HOME", str(tmp_path))
    creds.save("http://localhost:4000", "test-token", "user@example.com")

    mock_resp = MagicMock()
    mock_resp.status_code = 200
    mock_resp.json.return_value = [
        {"id": "proj-1", "name": "production", "created_at": "2026-04-13T00:00:00Z"},
    ]

    with patch("httpx.get", return_value=mock_resp):
        runner = CliRunner()
        result = runner.invoke(cli, ["projects"])

    assert result.exit_code == 0
    assert "production" in result.output
    assert "proj-1" in result.output
```

- [ ] **Step 4: Run tests to verify they fail**

```bash
cd /Users/prithviraj/Documents/CS/argus/sdk && python -m pytest tests/test_cli.py -v 2>&1 | head -20
```

Expected: `ModuleNotFoundError` — `argus_sdk.cli` not found.

- [ ] **Step 5: Implement `cli.py` (status + projects only, login stub)**

Create `sdk/argus_sdk/cli.py`:

```python
from __future__ import annotations

import click
import httpx

from ._credentials import load, save


@click.group()
def cli() -> None:
    """Argus — LLM drift detection CLI."""


@cli.command()
def status() -> None:
    """Show the current logged-in user and their projects."""
    creds = load()
    if not creds:
        click.echo("Not logged in. Run: argus login")
        return

    try:
        resp = httpx.get(
            f"{creds['server']}/api/v1/me",
            headers={"Authorization": f"Bearer {creds['token']}"},
            timeout=5.0,
        )
    except Exception as e:
        click.echo(f"Error reaching server: {e}")
        return

    if resp.status_code == 401:
        click.echo("Session expired. Run: argus login")
        return
    if resp.status_code != 200:
        click.echo(f"Server error: {resp.status_code}")
        return

    data = resp.json()
    click.echo(f"Logged in as: {data['email']}")
    click.echo(f"Server:       {creds['server']}")
    projects = data.get("projects", [])
    if projects:
        click.echo(f"\nProjects ({len(projects)}):")
        for p in projects:
            click.echo(f"  {p['name']}  ({p['id']})")
    else:
        click.echo("\nNo projects yet. Run: argus projects")


@cli.command()
def projects() -> None:
    """List your projects."""
    creds = load()
    if not creds:
        click.echo("Not logged in. Run: argus login")
        return

    try:
        resp = httpx.get(
            f"{creds['server']}/api/v1/projects",
            headers={"Authorization": f"Bearer {creds['token']}"},
            timeout=5.0,
        )
    except Exception as e:
        click.echo(f"Error reaching server: {e}")
        return

    if resp.status_code == 401:
        click.echo("Session expired. Run: argus login")
        return
    if resp.status_code != 200:
        click.echo(f"Server error: {resp.status_code}")
        return

    data = resp.json()
    if not data:
        click.echo("No projects found.")
        return

    click.echo(f"{'Name':<20} {'ID'}")
    click.echo("-" * 50)
    for p in data:
        click.echo(f"{p['name']:<20} {p['id']}")


@cli.command()
@click.option("--server", default="http://localhost:4000", help="Argus server URL")
def login(server: str) -> None:
    """Log in via GitHub OAuth and save credentials locally."""
    _do_login(server)


def _do_login(server: str) -> None:
    import socket
    import threading
    import urllib.parse
    import webbrowser
    from http.server import BaseHTTPRequestHandler, HTTPServer

    received_code: list[str] = []
    srv: list[HTTPServer] = []

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            parsed = urllib.parse.urlparse(self.path)
            params = urllib.parse.parse_qs(parsed.query)
            code = params.get("code", [""])[0]
            if code:
                received_code.append(code)
            self.send_response(200)
            self.send_header("Content-Type", "text/html")
            self.end_headers()
            self.wfile.write(b"<h2>Logged in! You can close this tab.</h2>")
            threading.Thread(target=srv[0].shutdown, daemon=True).start()

        def log_message(self, *args):
            pass

    # Find a free port
    with socket.socket() as s:
        s.bind(("localhost", 0))
        port = s.getsockname()[1]

    local_server = HTTPServer(("localhost", port), Handler)
    srv.append(local_server)

    callback_url = f"http://localhost:{port}/callback"
    auth_url = f"{server.rstrip('/')}/auth/cli?redirect={urllib.parse.quote(callback_url)}"

    click.echo(f"Opening browser to log in via GitHub...")
    click.echo(f"If the browser doesn't open, visit:\n  {auth_url}")
    webbrowser.open(auth_url)

    local_server.serve_forever()

    if not received_code:
        click.echo("Login cancelled or timed out.")
        return

    # Exchange one-time code for JWT
    try:
        resp = httpx.post(
            f"{server.rstrip('/')}/api/v1/auth/token",
            json={"code": received_code[0]},
            timeout=10.0,
        )
    except Exception as e:
        click.echo(f"Error exchanging code: {e}")
        return

    if resp.status_code != 200:
        click.echo(f"Token exchange failed: {resp.status_code}")
        return

    data = resp.json()
    save(server.rstrip("/"), data["token"], data["email"])
    click.echo(f"Logged in as {data['email']}. Credentials saved.")
```

- [ ] **Step 6: Run tests and verify they pass**

```bash
cd /Users/prithviraj/Documents/CS/argus/sdk && python -m pytest tests/test_cli.py -v
```

Expected: all 7 CLI tests PASS.

- [ ] **Step 7: Smoke test the CLI**

```bash
cd /Users/prithviraj/Documents/CS/argus/sdk && argus --help
argus status
argus projects
```

Expected:
```
Usage: argus [OPTIONS] COMMAND [ARGS]...

  Argus — LLM drift detection CLI.

Options:
  --help  Show this message and exit.

Commands:
  login     Log in via GitHub OAuth and save credentials locally.
  projects  List your projects.
  status    Show the current logged-in user and their projects.
```

`argus status` → `Not logged in. Run: argus login`

- [ ] **Step 8: Run full test suite**

```bash
cd /Users/prithviraj/Documents/CS/argus/sdk && python -m pytest tests/ -v
```

Expected: all tests pass.

- [ ] **Step 9: Commit**

```bash
cd /Users/prithviraj/Documents/CS/argus && git add sdk/argus_sdk/cli.py sdk/argus_sdk/_credentials.py sdk/tests/test_cli.py sdk/pyproject.toml
git commit -m "feat(cli): add argus login, status, projects commands"
```

---

## Task 5: End-to-end manual verification

- [ ] **Step 1: Start the server**

```bash
cd /Users/prithviraj/Documents/CS/argus/server && export $(cat ../.env | xargs) && go run ./cmd/main.go &
```

- [ ] **Step 2: Run `argus login`**

```bash
argus login --server http://localhost:4000
```

Expected: browser opens to GitHub, after authorizing you see `Logged in as <email>. Credentials saved.`

- [ ] **Step 3: Run `argus status`**

```bash
argus status
```

Expected: prints your email and server URL.

- [ ] **Step 4: Create a project via API and list it**

```bash
TOKEN=$(cat ~/.config/argus/credentials.json | python3 -c "import json,sys; print(json.load(sys.stdin)['token'])")
curl -s -X POST http://localhost:4000/api/v1/projects \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"my-first-project"}' | python3 -m json.tool

argus projects
```

Expected: project appears in both curl output and `argus projects` output.

- [ ] **Step 5: Kill the background server**

```bash
lsof -ti :4000 | xargs kill -9
```

- [ ] **Step 6: Final commit if any cleanup needed**

```bash
cd /Users/prithviraj/Documents/CS/argus && git add -A && git commit -m "chore: plan 3 complete"
```
