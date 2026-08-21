"""Credential-free recorded contract tests for Spotify Web API payloads."""

import json
from collections import deque

from spotify_client import SpotifyClient


PLAYLIST_PAGE = {
    "href": "https://api.spotify.com/v1/me/playlists?limit=50",
    "limit": 50,
    "next": None,
    "offset": 0,
    "previous": None,
    "total": 1,
    "items": [{
        "id": "playlist-1",
        "name": "Soft Rock",
        "type": "playlist",
        "uri": "spotify:playlist:playlist-1",
        "owner": {"id": "user-1", "display_name": "Listener"},
    }],
}

RECENT_PAGE = {
    "href": "https://api.spotify.com/v1/me/player/recently-played?limit=20",
    "limit": 20,
    "next": None,
    "cursors": {"after": "1", "before": "0"},
    "total": 1,
    "items": [{
        "played_at": "2026-08-21T10:00:00Z",
        "track": {
            "id": "track-1",
            "name": "A Song",
            "type": "track",
            "uri": "spotify:track:track-1",
            "artists": [{"id": "artist-1", "name": "An Artist"}],
        },
        "context": None,
    }],
}


class Response:
    status = 200
    headers = {}

    def __init__(self, payload):
        self.payload = payload

    def read(self):
        return json.dumps(self.payload).encode()


class RecordedTransport:
    def __init__(self, *payloads):
        self.payloads = deque(payloads)

    def __call__(self, _request, _timeout):
        return Response(self.payloads.popleft())


def client(*payloads):
    return SpotifyClient(access_token="recorded", transport=RecordedTransport(*payloads))


def test_playlist_contract_retains_identity_fields():
    result = client(PLAYLIST_PAGE).get_user_playlists()
    assert result["total"] == 1
    assert result["items"][0]["id"] == "playlist-1"
    assert result["items"][0]["name"] == "Soft Rock"


def test_recent_contract_retains_merge_fields():
    result = client(RECENT_PAGE).get_recently_played(limit=20)
    item = result["items"][0]
    assert item["played_at"] == "2026-08-21T10:00:00Z"
    assert item["track"]["name"] == "A Song"
    assert item["track"]["artists"][0]["name"] == "An Artist"
