"""Interactive Spotify OAuth login using Authorization Code with PKCE."""

from __future__ import annotations

import base64
import hashlib
import json
import os
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path
import secrets
import urllib.error
import urllib.parse
import urllib.request
import webbrowser

from cliamp_client import ToolError


AUTHORIZE_URL = "https://accounts.spotify.com/authorize"
TOKEN_URL = "https://accounts.spotify.com/api/token"
DEFAULT_REDIRECT_URI = "http://127.0.0.1:8888/callback"
SCOPES = ("playlist-read-private", "user-read-recently-played")
CALLBACK_TIMEOUT_SECONDS = 180


class SpotifyLoginError(ToolError):
    """A safe, structured failure during interactive authorization."""

    def __init__(self, message):
        super().__init__(message, retry_allowed=False)


def default_config_path() -> Path:
    root = os.environ.get("XDG_CONFIG_HOME")
    return Path(root) / "clify" / "spotify.json" if root else (
        Path.home() / ".config" / "clify" / "spotify.json"
    )


def generate_code_verifier() -> str:
    return secrets.token_urlsafe(64)


def _code_challenge(verifier: str) -> str:
    digest = hashlib.sha256(verifier.encode("ascii")).digest()
    return base64.urlsafe_b64encode(digest).rstrip(b"=").decode("ascii")


def build_authorization_url(client_id: str, state: str, verifier: str,
                            redirect_uri: str = DEFAULT_REDIRECT_URI) -> str:
    query = urllib.parse.urlencode({
        "client_id": client_id,
        "response_type": "code",
        "redirect_uri": redirect_uri,
        "scope": " ".join(SCOPES),
        "state": state,
        "code_challenge_method": "S256",
        "code_challenge": _code_challenge(verifier),
    })
    return f"{AUTHORIZE_URL}?{query}"


def _receive_callback(expected_state: str, redirect_uri: str,
                      timeout: int = CALLBACK_TIMEOUT_SECONDS) -> str:
    parsed = urllib.parse.urlsplit(redirect_uri)
    result = {}

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            request = urllib.parse.urlsplit(self.path)
            params = urllib.parse.parse_qs(request.query)
            result.update({key: values[0] for key, values in params.items()})
            ok = request.path == parsed.path and result.get("state") == expected_state
            self.send_response(200 if ok else 400)
            self.send_header("Content-Type", "text/plain; charset=utf-8")
            self.end_headers()
            message = "Spotify authorization complete. You may close this tab." if ok \
                else "Spotify authorization failed. Return to the terminal."
            self.wfile.write(message.encode("utf-8"))

        def log_message(self, _format, *_args):
            return

    server = HTTPServer((parsed.hostname, parsed.port), Handler)
    server.timeout = timeout
    server.handle_request()
    server.server_close()
    if result.get("error"):
        raise SpotifyLoginError(f"Spotify authorization denied: {result['error']}")
    if result.get("state") != expected_state:
        raise SpotifyLoginError("Spotify OAuth state mismatch or callback timed out")
    if not result.get("code"):
        raise SpotifyLoginError("Spotify callback did not contain an authorization code")
    return result["code"]


def exchange_code(client_id: str, code: str, verifier: str,
                  redirect_uri: str = DEFAULT_REDIRECT_URI,
                  transport=urllib.request.urlopen) -> dict:
    body = urllib.parse.urlencode({
        "grant_type": "authorization_code",
        "code": code,
        "redirect_uri": redirect_uri,
        "client_id": client_id,
        "code_verifier": verifier,
    }).encode("ascii")
    request = urllib.request.Request(
        TOKEN_URL,
        data=body,
        method="POST",
        headers={"Content-Type": "application/x-www-form-urlencoded"},
    )
    try:
        response = transport(request, timeout=10)
        payload = json.loads(response.read())
    except (urllib.error.URLError, OSError, ValueError, TypeError) as exc:
        raise SpotifyLoginError("Spotify token exchange failed") from exc
    if not isinstance(payload, dict) or not payload.get("refresh_token"):
        raise SpotifyLoginError("Spotify token response lacked a refresh token")
    return payload


def save_credentials(client_id: str, refresh_token: str,
                     path: Path | None = None) -> Path:
    path = Path(path or default_config_path())
    path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    path.write_text(json.dumps({
        "client_id": client_id,
        "refresh_token": refresh_token,
        "oauth_flow": "pkce",
    }) + "\n", encoding="utf-8")
    path.chmod(0o600)
    return path


def login(client_id: str | None = None, *, redirect_uri=DEFAULT_REDIRECT_URI,
          browser_open=webbrowser.open, callback_receiver=_receive_callback,
          transport=urllib.request.urlopen, config_path=None) -> Path:
    client_id = client_id or os.environ.get("SPOTIFY_CLIENT_ID")
    if not client_id:
        client_id = input("Spotify Client ID: ").strip()
    if not client_id:
        raise SpotifyLoginError("Spotify Client ID is required")

    state = secrets.token_urlsafe(24)
    verifier = generate_code_verifier()
    url = build_authorization_url(client_id, state, verifier, redirect_uri)
    print("Opening Spotify authorization in your browser...")
    print(f"If it does not open, visit:\n{url}")
    browser_open(url)
    code = callback_receiver(state, redirect_uri)
    tokens = exchange_code(client_id, code, verifier, redirect_uri, transport)
    return save_credentials(client_id, tokens["refresh_token"], config_path)
