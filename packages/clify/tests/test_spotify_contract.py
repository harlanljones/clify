"""Credential-free recorded contract tests for Spotify Web API payloads."""

import json
from collections import deque

import pytest

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
        item = self.payloads.popleft()
        # Pre-built Response instances (e.g. non-200 statuses) pass through;
        # raw page payloads are wrapped in a default 200 response.
        return item if isinstance(item, Response) else Response(item)


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


GENERATED_ID_PREFIX = "37i9dQZF1E"
GENERATED_KINDS = {
    "daily_mix",
    "discover_weekly",
    "release_radar",
    "on_repeat",
    "repeat_rewind",
    "daylist",
    "other",
}


def generated_playlist(
    name,
    playlist_id,
    owner_id="spotify",
    tracks=12,
):
    return {
        "id": playlist_id,
        "name": name,
        "type": "playlist",
        "uri": f"spotify:playlist:{playlist_id}",
        "owner": {"id": owner_id, "display_name": "Spotify"},
        "tracks": {"href": f"https://api.spotify.com/v1/playlists/{playlist_id}/tracks", "total": tracks},
    }


GENERATED_PLAYLIST_PAGE = {
    "href": "https://api.spotify.com/v1/me/playlists?limit=50",
    "limit": 50,
    "next": None,
    "offset": 0,
    "previous": None,
    "total": 4,
    "items": [
        generated_playlist("Daily Mix 1", f"{GENERATED_ID_PREFIX}8xk2"),
        generated_playlist("Discover Weekly", f"{GENERATED_ID_PREFIX}4mQz"),
        generated_playlist("On Repeat", f"{GENERATED_ID_PREFIX}1vBn"),
        generated_playlist("Soft Rock", "playlist-1", owner_id="user-1"),
    ],
}

TRACKS_PAGE_FIRST = {
    "href": "https://api.spotify.com/v1/playlists/37i9dQZF1E8xk2/tracks?limit=100&offset=0",
    "limit": 100,
    "next": "https://api.spotify.com/v1/playlists/37i9dQZF1E8xk2/tracks?limit=100&offset=100",
    "offset": 0,
    "previous": None,
    "total": 101,
    "items": [{
        "added_at": "2026-08-01T09:00:00Z",
        "track": {"id": "track-1", "name": "Neon Skyline", "type": "track",
                  "uri": "spotify:track:track-1"},
    }],
}

TRACKS_PAGE_SECOND = {
    "href": "https://api.spotify.com/v1/playlists/37i9dQZF1E8xk2/tracks?limit=100&offset=100",
    "limit": 100,
    "next": None,
    "offset": 100,
    "previous": TRACKS_PAGE_FIRST["next"],
    "total": 101,
    "items": [{
        "added_at": "2026-08-02T09:00:00Z",
        "track": {"id": "track-2", "name": "Midnight Drive", "type": "track",
                  "uri": "spotify:track:track-2"},
    }],
}


class NotFoundResponse(Response):
    status = 404

    def __init__(self):
        super().__init__({})


class RecordingTransport(RecordedTransport):
    """RecordedTransport that also captures each request URL."""

    def __init__(self, *payloads):
        super().__init__(*payloads)
        self.urls = []

    def __call__(self, request, timeout):
        self.urls.append(request.full_url)
        return super().__call__(request, timeout)


def test_generated_playlists_contract_fields_and_types():
    result = client(GENERATED_PLAYLIST_PAGE).get_generated_playlists()
    assert set(result) == {"items", "total"}
    assert isinstance(result["items"], list)
    assert result["total"] == len(result["items"]) == 3
    for entry in result["items"]:
        assert isinstance(entry["id"], str)
        assert entry["id"].startswith(GENERATED_ID_PREFIX)
        assert isinstance(entry["owner_id"], str)
        assert entry["generated"] is True
        assert entry["kind"] in GENERATED_KINDS
        # Upstream Spotify identity fields survive the clify extension.
        assert isinstance(entry["name"], str)
        assert entry["type"] == "playlist"
        assert entry["uri"] == f"spotify:playlist:{entry['id']}"


@pytest.mark.parametrize(
    ("name", "kind"),
    [
        ("Daily Mix 5", "daily_mix"),
        ("Discover Weekly", "discover_weekly"),
        ("Release Radar", "release_radar"),
        ("On Repeat", "on_repeat"),
        ("Repeat Rewind", "repeat_rewind"),
        ("My Daylist", "daylist"),
        ("Chill Vibes", "other"),
    ],
)
def test_generated_playlists_contract_kind_enum(name, kind):
    page = {"items": [generated_playlist(name, f"{GENERATED_ID_PREFIX}aaaa")], "next": None}
    result = client(page).get_generated_playlists()
    assert result["items"][0]["kind"] == kind
    assert kind in GENERATED_KINDS


def test_generated_playlists_exclude_non_candidates():
    result = client(GENERATED_PLAYLIST_PAGE).get_generated_playlists()
    assert all(item["id"] != "playlist-1" for item in result["items"])


def test_generated_playlists_result_is_deep_copy_independent():
    spotify = client(GENERATED_PLAYLIST_PAGE)
    first = spotify.get_generated_playlists()["items"][0]
    first["name"] = "mutated"
    first["extra"] = True
    second = spotify.get_generated_playlists()["items"][0]
    assert second["name"] == "Daily Mix 1"
    assert "extra" not in second
    raw = spotify.get_user_playlists()["items"][0]
    assert "generated" not in raw
    assert "kind" not in raw
    assert "owner_id" not in raw


def test_playlist_tracks_success_contract_and_pagination_next_urls():
    recorded = RecordingTransport(TRACKS_PAGE_FIRST, TRACKS_PAGE_SECOND)
    result = SpotifyClient(access_token="recorded", transport=recorded).get_playlist_tracks(
        f"{GENERATED_ID_PREFIX}8xk2"
    )
    assert set(result) == {"items", "total", "unbrowsable"}
    assert result["unbrowsable"] is False
    assert isinstance(result["items"], list)
    assert [item["track"]["name"] for item in result["items"]] == [
        "Neon Skyline",
        "Midnight Drive",
    ]
    assert result["total"] == len(result["items"]) == 2
    # The second request must follow the page object's `next` URL verbatim.
    assert len(recorded.urls) == 2
    assert recorded.urls[1] == TRACKS_PAGE_FIRST["next"]


def test_playlist_tracks_404_unbrowsable_contract_shape():
    result = client(NotFoundResponse()).get_playlist_tracks(f"{GENERATED_ID_PREFIX}8xk2")
    assert set(result) == {"items", "total", "unbrowsable"}
    assert result["items"] == []
    assert result["total"] == 0
    assert result["unbrowsable"] is True


SAVED_TRACKS_PAGE = {
    "href": "https://api.spotify.com/v1/me/tracks?limit=50",
    "limit": 50,
    "next": None,
    "offset": 0,
    "previous": None,
    "total": 1,
    "items": [{
        "added_at": "2026-08-01T09:00:00Z",
        "track": {"id": "track-1", "name": "Saved Song", "type": "track",
                  "uri": "spotify:track:track-1"},
    }],
}

SAVED_ALBUMS_PAGE = {
    "href": "https://api.spotify.com/v1/me/albums?limit=50",
    "limit": 50,
    "next": None,
    "offset": 0,
    "previous": None,
    "total": 1,
    "items": [{
        "added_at": "2026-08-02T09:00:00Z",
        "album": {"id": "album-1", "name": "Saved Album", "type": "album",
                  "uri": "spotify:album:album-1"},
    }],
}

TOP_TRACKS_PAGE = {
    "items": [{"id": "top-track", "name": "Top Track", "type": "track",
               "uri": "spotify:track:top-track"}],
    "total": 1,
    "next": None,
}

TOP_ARTISTS_PAGE = {
    "items": [{"id": "top-artist", "name": "Top Artist", "type": "artist",
               "uri": "spotify:artist:top-artist"}],
    "total": 1,
    "next": None,
}


def test_saved_tracks_contract_retains_track_and_added_at():
    result = client(SAVED_TRACKS_PAGE).get_saved_tracks(limit=50)
    assert set(result) == {"items", "total"}
    item = result["items"][0]
    assert item["added_at"] == "2026-08-01T09:00:00Z"
    assert item["track"]["name"] == "Saved Song"
    assert item["track"]["uri"] == "spotify:track:track-1"
    assert result["total"] == 1


def test_saved_albums_contract_retains_album():
    result = client(SAVED_ALBUMS_PAGE).get_saved_albums(limit=50)
    assert set(result) == {"items", "total"}
    item = result["items"][0]
    assert item["album"]["name"] == "Saved Album"
    assert item["album"]["uri"] == "spotify:album:album-1"
    assert result["total"] == 1


def test_top_tracks_contract_is_flat_track_list():
    result = client(TOP_TRACKS_PAGE).get_top_tracks()
    assert set(result) == {"items", "total"}
    assert result["items"][0]["name"] == "Top Track"
    assert result["items"][0]["type"] == "track"
    assert result["total"] == 1


def test_top_artists_contract_is_flat_artist_list():
    result = client(TOP_ARTISTS_PAGE).get_top_artists()
    assert set(result) == {"items", "total"}
    assert result["items"][0]["name"] == "Top Artist"
    assert result["items"][0]["type"] == "artist"
    assert result["total"] == 1
