import json
import pathlib
from unittest.mock import MagicMock, patch

import argus_sdk._credentials as creds
from click.testing import CliRunner

from argus_sdk.cli import cli


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


def test_status_not_logged_in(tmp_path, monkeypatch):
    monkeypatch.setenv("HOME", str(tmp_path))
    runner = CliRunner()
    result = runner.invoke(cli, ["status"])
    assert result.exit_code == 0
    assert "Not logged in" in result.output


def test_status_logged_in(tmp_path, monkeypatch):
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
