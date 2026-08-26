import json
import os
import urllib.parse

from spotify_auth import (
    SCOPES,
    build_authorization_url,
    exchange_code,
    save_credentials,
)


def test_scopes_expose_library_and_top_history():
    assert SCOPES == (
        "playlist-read-private",
        "user-read-recently-played",
        "user-library-read",
        "user-top-read",
    )
from spotify_client import SpotifyClient


class Response:
    status = 200
    headers = {}

    def __init__(self, payload):
        self.payload = payload

    def read(self):
        return json.dumps(self.payload).encode()


def test_authorization_url_uses_pkce_state_and_required_scopes():
    url = build_authorization_url("public-id", "csrf-state", "v" * 64)
    query = urllib.parse.parse_qs(urllib.parse.urlsplit(url).query)
    assert query["response_type"] == ["code"]
    assert query["state"] == ["csrf-state"]
    assert query["code_challenge_method"] == ["S256"]
    assert set(query["scope"][0].split()) == set(SCOPES)
    assert "v" * 64 not in url


def test_code_exchange_uses_pkce_without_client_secret():
    captured = {}

    def transport(request, timeout):
        captured["request"] = request
        captured["timeout"] = timeout
        return Response({"access_token": "access", "refresh_token": "refresh"})

    result = exchange_code("public-id", "code", "v" * 64, transport=transport)
    fields = urllib.parse.parse_qs(captured["request"].data.decode())
    assert result["refresh_token"] == "refresh"
    assert fields["client_id"] == ["public-id"]
    assert fields["code_verifier"] == ["v" * 64]
    assert captured["request"].get_header("Authorization") is None


def test_credentials_are_mode_600_and_exclude_secret(tmp_path):
    path = save_credentials("public-id", "refresh", tmp_path / "spotify.json")
    payload = json.loads(path.read_text())
    assert payload == {
        "client_id": "public-id",
        "refresh_token": "refresh",
        "oauth_flow": "pkce",
    }
    assert os.stat(path).st_mode & 0o777 == 0o600


def test_client_loads_pkce_config_and_refreshes_without_secret(tmp_path):
    path = save_credentials("public-id", "refresh", tmp_path / "spotify.json")
    requests = []

    def transport(request, _timeout):
        requests.append(request)
        if request.full_url.endswith("/api/token"):
            return Response({"access_token": "access"})
        return Response({"items": []})

    spotify = SpotifyClient.from_config(path, transport=transport)
    assert spotify.get_recently_played()["items"] == []
    fields = urllib.parse.parse_qs(requests[0].data.decode())
    assert fields["client_id"] == ["public-id"]
    assert requests[0].get_header("Authorization") is None
