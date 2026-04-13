import json
import pathlib

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
