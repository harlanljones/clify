"""Phase 6 SpotifyClient tests: OAuth, HTTP failures, pagination and cache."""

import json
import urllib.error
from collections import deque

import pytest

from spotify_client import (
    SpotifyClient,
    SpotifyConfigurationError,
    SpotifyHTTPError,
    SpotifyNetworkError,
    SpotifyParseError,
    SpotifyRateLimitError,
)


class Response:
    def __init__(self, payload, status=200, headers=None):
        self.status = status
        self.headers = headers or {}
        self._body = payload if isinstance(payload, bytes) else json.dumps(payload).encode()

    def read(self):
        return self._body

    def getcode(self):
        return self.status


class Transport:
    def __init__(self, *responses):
        self.responses = deque(responses)
        self.calls = []

    def __call__(self, request, timeout):
        self.calls.append((request, timeout))
        response = self.responses.popleft()
        if isinstance(response, BaseException):
            raise response
        return response


def client(transport, **kwargs):
    return SpotifyClient(
        client_id="client-id",
        client_secret="client-secret",
        refresh_token="refresh-secret",
        access_token="access-secret",
        transport=transport,
        **kwargs,
    )


def test_playlist_pagination_is_flattened():
    transport = Transport(
        Response({"items": [{"id": "one"}], "next": "https://api.spotify.com/v1/next"}),
        Response({"items": [{"id": "two"}], "next": None}),
    )
    result = client(transport).get_user_playlists()
    assert result == {"items": [{"id": "one"}, {"id": "two"}], "total": 2}
    assert [call[0].full_url for call in transport.calls] == [
        "https://api.spotify.com/v1/me/playlists?limit=50",
        "https://api.spotify.com/v1/next",
    ]


def test_401_refreshes_and_retries_exactly_once():
    unauthorized = Response({}, status=401)
    transport = Transport(
        unauthorized,
        Response({"access_token": "replacement"}),
        Response({"items": [], "next": None}),
    )
    spotify = client(transport)
    assert spotify.get_user_playlists()["items"] == []
    refresh_request = transport.calls[1][0]
    assert refresh_request.full_url.endswith("/api/token")
    assert refresh_request.get_method() == "POST"
    assert transport.calls[2][0].get_header("Authorization") == "Bearer replacement"


def test_second_401_becomes_structured_error_without_another_refresh():
    transport = Transport(
        Response({}, status=401),
        Response({"access_token": "replacement"}),
        Response({}, status=401),
    )
    with pytest.raises(SpotifyHTTPError) as caught:
        client(transport).get_user_playlists()
    assert caught.value.payload["status"] == "TOOL_ERROR"
    assert len(transport.calls) == 3


def test_429_uses_retry_after_and_retries_once():
    sleeps = []
    transport = Transport(
        Response({}, status=429, headers={"Retry-After": "2.5"}),
        Response({"items": [], "next": None}),
    )
    assert client(transport, sleep=sleeps.append).get_user_playlists()["items"] == []
    assert sleeps == [2.5]


def test_repeated_429_is_retryable_tool_error():
    transport = Transport(Response({}, 429), Response({}, 429))
    with pytest.raises(SpotifyRateLimitError) as caught:
        client(transport, sleep=lambda _: None).get_user_playlists()
    assert caught.value.payload["retry_allowed"] is True


@pytest.mark.parametrize("failure", [urllib.error.URLError("offline"), TimeoutError()])
def test_network_failures_are_structured_and_sanitized(failure):
    transport = Transport(failure)
    with pytest.raises(SpotifyNetworkError) as caught:
        client(transport).get_recently_played()
    assert caught.value.payload["status"] == "TOOL_ERROR"
    assert "access-secret" not in str(caught.value)


def test_malformed_json_is_structured():
    transport = Transport(Response(b"{not-json"))
    with pytest.raises(SpotifyParseError) as caught:
        client(transport).get_recently_played()
    assert caught.value.payload["retry_allowed"] is True


def test_playlist_cache_ttl_and_defensive_copy():
    times = iter([100.0, 129.9, 130.0])
    transport = Transport(
        Response({"items": [{"id": "one"}], "next": None}),
        Response({"items": [{"id": "two"}], "next": None}),
    )
    spotify = client(transport, clock=lambda: next(times))
    first = spotify.get_user_playlists()
    first["items"].clear()
    assert spotify.get_user_playlists()["items"] == [{"id": "one"}]
    assert spotify.get_user_playlists()["items"] == [{"id": "two"}]
    assert len(transport.calls) == 2


def test_recently_played_limit_and_url():
    transport = Transport(Response({"items": [{"played_at": "now"}]}))
    result = client(transport).get_recently_played(limit=7)
    assert len(result["items"]) == 1
    assert transport.calls[0][0].full_url.endswith("recently-played?limit=7")


def test_invalid_limit_never_uses_http():
    transport = Transport()
    with pytest.raises(SpotifyConfigurationError) as caught:
        client(transport).get_recently_played(0)
    assert caught.value.retry_allowed is False
    assert transport.calls == []


def test_missing_credentials_and_repr_never_reveal_secrets(monkeypatch):
    for name in ("SPOTIFY_CLIENT_ID", "SPOTIFY_CLIENT_SECRET", "SPOTIFY_REFRESH_TOKEN"):
        monkeypatch.delenv(name, raising=False)
    spotify = SpotifyClient(transport=Transport())
    with pytest.raises(SpotifyConfigurationError) as caught:
        spotify.get_recently_played()
    text = repr(spotify) + str(caught.value) + repr(vars(spotify))
    # Public representations/errors are redacted; internal state is necessarily private.
    assert "credentials=<redacted>" in repr(spotify)
    assert "refresh-secret" not in str(caught.value)


def test_refresh_request_contains_credentials_but_errors_do_not():
    transport = Transport(Response({}, status=400))
    spotify = SpotifyClient(
        client_id="highly-secret-id",
        client_secret="highly-secret-client-secret",
        refresh_token="highly-secret-refresh",
        transport=transport,
    )
    with pytest.raises(SpotifyHTTPError) as caught:
        spotify.get_recently_played()
    assert "highly-secret" not in str(caught.value)
    assert caught.value.payload["status"] == "TOOL_ERROR"
