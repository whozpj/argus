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

    with socket.socket() as s:
        s.bind(("localhost", 0))
        port = s.getsockname()[1]

    local_server = HTTPServer(("localhost", port), Handler)
    srv.append(local_server)

    callback_url = f"http://localhost:{port}/callback"
    auth_url = f"{server.rstrip('/')}/auth/cli?redirect={urllib.parse.quote(callback_url)}"

    click.echo("Opening browser to log in via GitHub...")
    click.echo(f"If the browser doesn't open, visit:\n  {auth_url}")
    webbrowser.open(auth_url)

    local_server.serve_forever()

    if not received_code:
        click.echo("Login cancelled or timed out.")
        return

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
