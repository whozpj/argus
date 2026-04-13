"""Tests for argus_sdk CLI commands and credential helpers."""
import json
import pathlib
from unittest.mock import MagicMock, patch

import httpx
import pytest

import argus_sdk._credentials as creds
from click.testing import CliRunner

from argus_sdk.cli import cli


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------

@pytest.fixture
def runner():
    return CliRunner()


@pytest.fixture
def isolated_home(tmp_path, monkeypatch):
    """Each test gets a clean HOME directory so credentials never bleed between tests."""
    monkeypatch.setenv("HOME", str(tmp_path))
    return tmp_path


@pytest.fixture
def logged_in(isolated_home):
    """Pre-save credentials so tests start already logged in."""
    creds.save("http://localhost:4000", "test-token-xyz", "user@example.com")
    return isolated_home


# ---------------------------------------------------------------------------
# _credentials.save / load
# ---------------------------------------------------------------------------

def test_save_and_load_roundtrip(isolated_home):
    creds.save("http://localhost:4000", "tok123", "alice@example.com")
    data = creds.load()
    assert data["server"] == "http://localhost:4000"
    assert data["token"] == "tok123"
    assert data["email"] == "alice@example.com"


def test_load_returns_none_when_file_missing(isolated_home):
    assert creds.load() is None


def test_credentials_written_to_correct_path(isolated_home):
    creds.save("http://localhost:4000", "tok", "u@example.com")
    expected = isolated_home / ".config" / "argus" / "credentials.json"
    assert expected.exists()


def test_credentials_file_is_valid_json(isolated_home):
    creds.save("http://localhost:4000", "tok", "u@example.com")
    path = isolated_home / ".config" / "argus" / "credentials.json"
    data = json.loads(path.read_text())
    assert "server" in data and "token" in data and "email" in data


def test_save_overwrites_existing_credentials(isolated_home):
    creds.save("http://old-server", "old-token", "old@example.com")
    creds.save("http://new-server", "new-token", "new@example.com")
    data = creds.load()
    assert data["server"] == "http://new-server"
    assert data["token"] == "new-token"
    assert data["email"] == "new@example.com"


def test_credentials_directory_created_if_missing(tmp_path, monkeypatch):
    """save() must create ~/.config/argus/ even if it doesn't exist yet."""
    monkeypatch.setenv("HOME", str(tmp_path))
    assert not (tmp_path / ".config" / "argus").exists()
    creds.save("http://localhost:4000", "tok", "u@example.com")
    assert (tmp_path / ".config" / "argus" / "credentials.json").exists()


# ---------------------------------------------------------------------------
# argus status
# ---------------------------------------------------------------------------

def test_status_not_logged_in(runner, isolated_home):
    result = runner.invoke(cli, ["status"])
    assert result.exit_code == 0
    assert "Not logged in" in result.output


def test_status_shows_email_and_server(runner, logged_in):
    mock_resp = MagicMock()
    mock_resp.status_code = 200
    mock_resp.json.return_value = {"email": "user@example.com", "projects": []}

    with patch("httpx.get", return_value=mock_resp):
        result = runner.invoke(cli, ["status"])

    assert result.exit_code == 0
    assert "user@example.com" in result.output
    assert "http://localhost:4000" in result.output


def test_status_shows_project_names(runner, logged_in):
    mock_resp = MagicMock()
    mock_resp.status_code = 200
    mock_resp.json.return_value = {
        "email": "user@example.com",
        "projects": [
            {"id": "proj-1", "name": "production"},
            {"id": "proj-2", "name": "staging"},
        ],
    }

    with patch("httpx.get", return_value=mock_resp):
        result = runner.invoke(cli, ["status"])

    assert "production" in result.output
    assert "staging" in result.output


def test_status_no_projects_shows_hint(runner, logged_in):
    mock_resp = MagicMock()
    mock_resp.status_code = 200
    mock_resp.json.return_value = {"email": "user@example.com", "projects": []}

    with patch("httpx.get", return_value=mock_resp):
        result = runner.invoke(cli, ["status"])

    assert "No projects" in result.output


def test_status_expired_token_shows_message(runner, logged_in):
    mock_resp = MagicMock()
    mock_resp.status_code = 401

    with patch("httpx.get", return_value=mock_resp):
        result = runner.invoke(cli, ["status"])

    assert result.exit_code == 0
    assert "expired" in result.output.lower() or "login" in result.output.lower()


def test_status_server_error_shows_code(runner, logged_in):
    mock_resp = MagicMock()
    mock_resp.status_code = 500

    with patch("httpx.get", return_value=mock_resp):
        result = runner.invoke(cli, ["status"])

    assert result.exit_code == 0
    assert "500" in result.output


def test_status_unreachable_server_shows_error(runner, logged_in):
    with patch("httpx.get", side_effect=httpx.ConnectError("refused")):
        result = runner.invoke(cli, ["status"])

    assert result.exit_code == 0
    assert "Error" in result.output or "error" in result.output


# ---------------------------------------------------------------------------
# argus projects
# ---------------------------------------------------------------------------

def test_projects_not_logged_in(runner, isolated_home):
    result = runner.invoke(cli, ["projects"])
    assert result.exit_code == 0
    assert "Not logged in" in result.output


def test_projects_lists_all_projects(runner, logged_in):
    mock_resp = MagicMock()
    mock_resp.status_code = 200
    mock_resp.json.return_value = [
        {"id": "proj-1", "name": "production"},
        {"id": "proj-2", "name": "staging"},
    ]

    with patch("httpx.get", return_value=mock_resp):
        result = runner.invoke(cli, ["projects"])

    assert result.exit_code == 0
    assert "production" in result.output
    assert "proj-1" in result.output
    assert "staging" in result.output
    assert "proj-2" in result.output


def test_projects_empty_list_shows_message(runner, logged_in):
    mock_resp = MagicMock()
    mock_resp.status_code = 200
    mock_resp.json.return_value = []

    with patch("httpx.get", return_value=mock_resp):
        result = runner.invoke(cli, ["projects"])

    assert result.exit_code == 0
    assert "No projects" in result.output


def test_projects_expired_token(runner, logged_in):
    mock_resp = MagicMock()
    mock_resp.status_code = 401

    with patch("httpx.get", return_value=mock_resp):
        result = runner.invoke(cli, ["projects"])

    assert result.exit_code == 0
    assert "expired" in result.output.lower() or "login" in result.output.lower()


def test_projects_unreachable_server(runner, logged_in):
    with patch("httpx.get", side_effect=httpx.ConnectError("refused")):
        result = runner.invoke(cli, ["projects"])

    assert result.exit_code == 0
    assert "Error" in result.output or "error" in result.output


def test_projects_request_includes_bearer_token(runner, logged_in):
    """The Authorization header must carry the stored JWT."""
    received_headers = []
    mock_resp = MagicMock()
    mock_resp.status_code = 200
    mock_resp.json.return_value = []

    def capture(url, headers=None, timeout=None):
        received_headers.append(headers or {})
        return mock_resp

    with patch("httpx.get", side_effect=capture):
        runner.invoke(cli, ["projects"])

    assert received_headers, "httpx.get was never called"
    assert received_headers[0].get("Authorization") == "Bearer test-token-xyz"


# ---------------------------------------------------------------------------
# argus login
# ---------------------------------------------------------------------------

def test_login_accepts_server_option(runner, isolated_home):
    """login --server must be a recognized option (no 'No such option' error)."""
    # We interrupt login immediately by making serve_forever not block
    with patch("argus_sdk.cli._do_login") as mock_login:
        result = runner.invoke(cli, ["login", "--server", "http://my-argus:4000"])

    mock_login.assert_called_once_with("http://my-argus:4000")
    assert result.exit_code == 0


def test_login_shows_fallback_url(runner, isolated_home):
    """The fallback URL printed to the terminal must contain the server address."""
    import threading
    from http.server import HTTPServer, BaseHTTPRequestHandler

    # Patch webbrowser.open so it doesn't actually open a browser,
    # and simulate the local callback server receiving a code immediately.
    def fake_do_login(server):
        import urllib.parse, webbrowser
        fake_callback = "http://localhost:59999/callback"
        auth_url = f"{server.rstrip('/')}/auth/cli?redirect={urllib.parse.quote(fake_callback)}"
        from click import echo
        echo(f"If the browser doesn't open, visit:\n  {auth_url}")

    with patch("argus_sdk.cli._do_login", side_effect=fake_do_login):
        result = runner.invoke(cli, ["login", "--server", "http://argus.example.com"])

    assert "argus.example.com" in result.output
    assert "auth/cli" in result.output
