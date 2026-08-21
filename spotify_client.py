"""Small, injectable Spotify Web API client (ROADMAP Phase 6).

Credentials may be supplied explicitly or through ``SPOTIFY_CLIENT_ID``,
``SPOTIFY_CLIENT_SECRET`` and ``SPOTIFY_REFRESH_TOKEN``.  They are deliberately
excluded from exception messages and the client representation.
"""

from __future__ import annotations

import base64
import copy
import json
import os
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any, Callable, Mapping

from cliamp_client import ToolError

API_BASE_URL = "https://api.spotify.com/v1"
TOKEN_URL = "https://accounts.spotify.com/api/token"
HTTP_TIMEOUT_SECONDS = 2.0
PLAYLIST_CACHE_TTL_SECONDS = 30.0


class SpotifyConfigurationError(ToolError):
    """Required OAuth credentials are absent. This needs operator action."""

    def __init__(self, message: str):
        super().__init__(message, retry_allowed=False)


class SpotifyNetworkError(ToolError):
    """The Spotify HTTP boundary failed before yielding a usable response."""


class SpotifyParseError(ToolError):
    """Spotify returned malformed JSON or an unexpected payload shape."""


class SpotifyHTTPError(ToolError):
    """Spotify returned an unsuccessful HTTP response."""

    def __init__(self, status: int):
        self.status = status
        super().__init__(f"Spotify API request failed with HTTP {status}", True)


class SpotifyRateLimitError(SpotifyHTTPError):
    """Rate limiting persisted after the single permitted retry."""


Transport = Callable[[urllib.request.Request, float], Any]


def _default_transport(request: urllib.request.Request, timeout: float) -> Any:
    return urllib.request.urlopen(request, timeout=timeout)


class SpotifyClient:
    """Read-only Spotify API tool using refresh-token OAuth2.

    A 401 triggers one token refresh and one retry. A 429 sleeps for the
    server's ``Retry-After`` duration and retries once. Playlist results are
    cached per client instance for 30 seconds.
    """

    def __init__(
        self,
        client_id: str | None = None,
        client_secret: str | None = None,
        refresh_token: str | None = None,
        access_token: str | None = None,
        *,
        transport: Transport | None = None,
        sleep: Callable[[float], None] = time.sleep,
        clock: Callable[[], float] = time.monotonic,
        timeout: float = HTTP_TIMEOUT_SECONDS,
        oauth_flow: str = "secret",
    ):
        self._client_id = client_id or os.environ.get("SPOTIFY_CLIENT_ID")
        self._client_secret = client_secret or os.environ.get("SPOTIFY_CLIENT_SECRET")
        self._refresh_token = refresh_token or os.environ.get("SPOTIFY_REFRESH_TOKEN")
        self._access_token = access_token
        self._transport = transport or _default_transport
        self._sleep = sleep
        self._clock = clock
        self._timeout = timeout
        self._oauth_flow = oauth_flow
        self._playlist_cache: tuple[float, dict[str, Any]] | None = None

    @classmethod
    def from_config(cls, path: str | os.PathLike | None = None, **kwargs):
        """Load PKCE credentials saved by ``clify spotify login``.

        Explicit constructor keyword arguments and SPOTIFY_* environment
        variables take precedence over the file.
        """
        if path is None:
            from spotify_auth import default_config_path
            path = default_config_path()
        try:
            payload = json.loads(Path(path).read_text(encoding="utf-8"))
        except FileNotFoundError:
            payload = {}
        except (OSError, json.JSONDecodeError) as exc:
            raise SpotifyConfigurationError("Spotify credential file is invalid") from exc
        return cls(
            client_id=(kwargs.pop("client_id", None)
                       or os.environ.get("SPOTIFY_CLIENT_ID")
                       or payload.get("client_id")),
            refresh_token=(kwargs.pop("refresh_token", None)
                           or os.environ.get("SPOTIFY_REFRESH_TOKEN")
                           or payload.get("refresh_token")),
            oauth_flow=kwargs.pop("oauth_flow", None) or payload.get("oauth_flow", "secret"),
            **kwargs,
        )

    def __repr__(self) -> str:
        return "SpotifyClient(credentials=<redacted>)"

    @staticmethod
    def _headers(response: Any) -> Mapping[str, str]:
        return getattr(response, "headers", {}) or {}

    def _decode_json(self, response: Any) -> Any:
        try:
            raw = response.read()
            if isinstance(raw, bytes):
                raw = raw.decode("utf-8")
            return json.loads(raw)
        except (UnicodeDecodeError, json.JSONDecodeError, TypeError, ValueError) as exc:
            raise SpotifyParseError("Spotify returned malformed JSON") from exc

    def _send(self, request: urllib.request.Request) -> tuple[int, Mapping[str, str], Any]:
        try:
            response = self._transport(request, self._timeout)
            status_value = getattr(response, "status", None)
            if status_value is None:
                status_value = response.getcode()
            status = int(status_value)
            return status, self._headers(response), self._decode_json(response)
        except urllib.error.HTTPError as exc:
            return exc.code, exc.headers or {}, None
        except (urllib.error.URLError, TimeoutError, OSError) as exc:
            raise SpotifyNetworkError("Spotify network request failed") from exc

    def _require_refresh_credentials(self) -> None:
        required = [
            ("SPOTIFY_CLIENT_ID", self._client_id),
            ("SPOTIFY_REFRESH_TOKEN", self._refresh_token),
        ]
        if self._oauth_flow != "pkce":
            required.append(("SPOTIFY_CLIENT_SECRET", self._client_secret))
        missing = [
            name
            for name, value in required
            if not value
        ]
        if missing:
            raise SpotifyConfigurationError(
                "Missing Spotify OAuth configuration: " + ", ".join(missing)
            )

    def _refresh_access_token(self) -> None:
        self._require_refresh_credentials()
        fields = {"grant_type": "refresh_token", "refresh_token": self._refresh_token}
        headers = {"Content-Type": "application/x-www-form-urlencoded"}
        if self._oauth_flow == "pkce":
            fields["client_id"] = self._client_id
        else:
            credentials = f"{self._client_id}:{self._client_secret}".encode("utf-8")
            headers["Authorization"] = (
                "Basic " + base64.b64encode(credentials).decode("ascii")
            )
        data = urllib.parse.urlencode(fields).encode("ascii")
        request = urllib.request.Request(
            TOKEN_URL,
            data=data,
            method="POST",
            headers=headers,
        )
        status, _headers, payload = self._send(request)
        if status < 200 or status >= 300:
            raise SpotifyHTTPError(status)
        if not isinstance(payload, dict) or not isinstance(payload.get("access_token"), str):
            raise SpotifyParseError("Spotify token response lacked an access token")
        self._access_token = payload["access_token"]

    def _get_json(self, url: str) -> dict[str, Any]:
        if not self._access_token:
            self._refresh_access_token()

        refreshed = False
        rate_retried = False
        while True:
            request = urllib.request.Request(
                url,
                method="GET",
                headers={"Authorization": f"Bearer {self._access_token}"},
            )
            status, headers, payload = self._send(request)
            if status == 401 and not refreshed:
                self._refresh_access_token()
                refreshed = True
                continue
            if status == 429 and not rate_retried:
                try:
                    delay = max(0.0, float(headers.get("Retry-After", "1")))
                except (TypeError, ValueError):
                    delay = 1.0
                self._sleep(delay)
                rate_retried = True
                continue
            if status == 429:
                raise SpotifyRateLimitError(status)
            if status < 200 or status >= 300:
                raise SpotifyHTTPError(status)
            if not isinstance(payload, dict):
                raise SpotifyParseError("Spotify returned a non-object JSON payload")
            return payload

    def get_user_playlists(self) -> dict[str, Any]:
        """Return all of the current user's playlists in one ``items`` list."""
        now = self._clock()
        if self._playlist_cache and now - self._playlist_cache[0] < PLAYLIST_CACHE_TTL_SECONDS:
            return copy.deepcopy(self._playlist_cache[1])

        url: str | None = f"{API_BASE_URL}/me/playlists?limit=50"
        items: list[Any] = []
        while url:
            page = self._get_json(url)
            page_items = page.get("items")
            if not isinstance(page_items, list):
                raise SpotifyParseError("Spotify playlist response lacked an items list")
            items.extend(page_items)
            next_url = page.get("next")
            if next_url is not None and not isinstance(next_url, str):
                raise SpotifyParseError("Spotify playlist response had an invalid next URL")
            url = next_url

        result = {"items": items, "total": len(items)}
        self._playlist_cache = (now, copy.deepcopy(result))
        return copy.deepcopy(result)

    def get_recently_played(self, limit: int = 50) -> dict[str, Any]:
        """Return the current user's recently played tracks."""
        if isinstance(limit, bool) or not isinstance(limit, int) or not 1 <= limit <= 50:
            raise SpotifyConfigurationError("recently played limit must be between 1 and 50")
        url = f"{API_BASE_URL}/me/player/recently-played?limit={limit}"
        payload = self._get_json(url)
        if not isinstance(payload.get("items"), list):
            raise SpotifyParseError("Spotify recently-played response lacked an items list")
        return payload
