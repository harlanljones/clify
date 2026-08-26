"""Phase 7 TDD coverage for unified, provider-independent library data."""

import json
from unittest.mock import Mock, patch

import pytest

from unified_library import (
    UnifiedLibraryAgent,
    UnifiedLibraryClient,
    merge_recently_played,
)


def test_merge_recently_played_sorts_tags_and_deduplicates():
    local = [
        {"title": "Shared Song", "artist": "Artist", "played_at": "2025-01-01T10:00:00Z"},
        {"title": "Local Only", "played_at": "2025-01-01T09:00:00Z"},
    ]
    spotify = [
        {
            "track": {"name": "Shared Song", "artists": [{"name": "Artist"}]},
            "played_at": "2025-01-01T11:00:00+00:00",
        },
        {"track": {"name": "Newest"}, "played_at": "2025-01-01T12:00:00Z"},
    ]

    result = merge_recently_played([("cliamp", local), ("spotify", spotify)])

    assert [item["played_at"] for item in result] == [
        "2025-01-01T12:00:00Z",
        "2025-01-01T11:00:00+00:00",
        "2025-01-01T09:00:00Z",
    ]
    assert [item["source"] for item in result] == ["spotify", "spotify", "cliamp"]
    assert result[1]["sources"] == ["cliamp", "spotify"]
    assert local[0].get("source") is None  # the pure function does not mutate inputs


def test_bare_source_lists_get_conventional_source_names():
    result = merge_recently_played([
        [{"title": "Local", "played_at": None}],
        [{"track": {"name": "Remote"}, "played_at": "bad timestamp"}],
    ])
    assert [item["source"] for item in result] == ["cliamp", "spotify"]


@pytest.fixture
def providers():
    cliamp = Mock()
    cliamp.get_recently_played.return_value = {
        "entries": [{"title": "Local", "played_at": "2025-01-01T09:00:00Z"}]
    }
    cliamp.get_playlists.return_value = {"playlists": ["Road Trip", "Focus"]}
    spotify = Mock()
    spotify.get_recently_played.return_value = {
        "items": [{"track": {"name": "Remote"}, "played_at": "2025-01-01T10:00:00Z"}]
    }
    spotify.get_user_playlists.return_value = {
        "items": [{"id": "p1", "name": "Discoveries"}], "total": 1
    }
    spotify.get_generated_playlists.return_value = {
        "items": [{
            "id": "g1",
            "name": "Daily Mix 1",
            "owner_id": "spotify",
            "generated": True,
            "kind": "daily_mix",
        }],
        "total": 1,
    }
    spotify.get_saved_tracks.return_value = {
        "items": [{"added_at": "2025-06-01T00:00:00Z", "track": {"name": "Liked One"}}],
        "total": 1,
    }
    spotify.get_saved_albums.return_value = {
        "items": [{"added_at": "2025-06-02T00:00:00Z", "album": {"name": "Album One"}}],
        "total": 1,
    }
    spotify.get_top_tracks.return_value = {
        "items": [{"name": "Top Track", "type": "track"}],
        "total": 1,
    }
    spotify.get_top_artists.return_value = {
        "items": [{"name": "Top Artist", "type": "artist"}],
        "total": 1,
    }
    return cliamp, spotify


def test_get_recently_played_merges_providers_and_honors_limit(providers):
    cliamp, spotify = providers
    client = UnifiedLibraryClient(cliamp, spotify)

    result = client.get_recently_played(limit=1)

    cliamp.get_recently_played.assert_called_once_with(limit=1)
    spotify.get_recently_played.assert_called_once_with(limit=1)
    assert len(result) == 1
    assert result[0]["source"] == "spotify"


def test_get_recently_played_prefers_fork_unified_contract(providers):
    cliamp, spotify = providers
    cliamp.get_unified_recently_played.return_value = {
        "ok": True,
        "schema_version": "cliamp.history.unified/1",
        "history": [{
            "track": {"title": "Native", "path": "spotify:track:native"},
            "played_at": "2026-08-21T10:00:00Z",
            "sources": ["cliamp", "spotify"],
        }],
        "partial": False,
        "failed_sources": [],
    }

    result = UnifiedLibraryClient(cliamp, spotify).get_recently_played(limit=7)

    cliamp.get_unified_recently_played.assert_called_once_with(limit=7)
    cliamp.get_recently_played.assert_not_called()
    spotify.get_recently_played.assert_not_called()
    assert result[0]["track"]["title"] == "Native"
    assert result[0]["sources"] == ["cliamp", "spotify"]


def test_stock_cliamp_falls_back_when_unified_contract_is_absent(providers):
    cliamp, spotify = providers
    cliamp.get_unified_recently_played.side_effect = RuntimeError("unknown command")

    result = UnifiedLibraryClient(cliamp, spotify).get_recently_played(limit=2)

    assert len(result) == 2
    cliamp.get_recently_played.assert_called_once_with(limit=2)
    spotify.get_recently_played.assert_called_once_with(limit=2)


def test_library_sections_have_fixed_order(providers):
    client = UnifiedLibraryClient(*providers)

    result = client.get_library_sections()

    assert list(result)[:3] == ["recently_played", "library", "your_playlists"]
    assert result["library"] == ["Road Trip", "Focus"]
    assert result["your_playlists"] == [{"id": "p1", "name": "Discoveries"}]
    assert result["partial"] is False
    assert result["failed_sources"] == []


def test_library_sections_include_made_for_you(providers):
    client = UnifiedLibraryClient(*providers)

    result = client.get_library_sections()

    assert list(result) == [
        "recently_played", "library", "your_playlists", "made_for_you",
        "saved_tracks", "saved_albums", "top_tracks", "top_artists",
        "partial", "failed_sources",
    ]
    assert result["made_for_you"] == [{
        "id": "g1",
        "name": "Daily Mix 1",
        "owner_id": "spotify",
        "generated": True,
        "kind": "daily_mix",
    }]
    assert result["partial"] is False
    assert result["failed_sources"] == []


def test_library_sections_expose_saved_and_top(providers):
    result = UnifiedLibraryClient(*providers).get_library_sections()

    assert result["saved_tracks"] == [{
        "added_at": "2025-06-01T00:00:00Z",
        "track": {"name": "Liked One"},
    }]
    assert result["saved_albums"] == [{
        "added_at": "2025-06-02T00:00:00Z",
        "album": {"name": "Album One"},
    }]
    assert result["top_tracks"] == [{"name": "Top Track", "type": "track"}]
    assert result["top_artists"] == [{"name": "Top Artist", "type": "artist"}]
    assert result["partial"] is False
    assert result["failed_sources"] == []


def test_saved_and_top_provider_failure_is_isolated(providers):
    cliamp, spotify = providers
    spotify.get_saved_tracks.side_effect = RuntimeError("offline")
    spotify.get_top_artists.side_effect = RuntimeError("offline")

    result = UnifiedLibraryClient(cliamp, spotify).get_library_sections()

    assert result["saved_tracks"] == []
    assert result["saved_albums"] == [{
        "added_at": "2025-06-02T00:00:00Z",
        "album": {"name": "Album One"},
    }]
    assert result["top_tracks"] == [{"name": "Top Track", "type": "track"}]
    assert result["top_artists"] == []
    assert result["partial"] is True
    assert set(result["failed_sources"]) == {"spotify.saved", "spotify.top"}


def test_missing_generated_capability_is_absent_not_failed(providers):
    cliamp, spotify = providers
    del spotify.get_generated_playlists

    result = UnifiedLibraryClient(cliamp, spotify).get_library_sections()

    assert "made_for_you" not in result
    assert result["partial"] is False
    assert result["failed_sources"] == []


def test_generated_playlist_failure_is_isolated(providers):
    cliamp, spotify = providers
    spotify.get_generated_playlists.side_effect = RuntimeError("offline")

    result = UnifiedLibraryClient(cliamp, spotify).get_library_sections()

    assert result["made_for_you"] == []
    assert result["library"] == ["Road Trip", "Focus"]
    assert result["your_playlists"] == [{"id": "p1", "name": "Discoveries"}]
    assert result["partial"] is True
    assert result["failed_sources"] == ["spotify.generated"]


def test_generated_malformed_payload_is_tolerated(providers):
    cliamp, spotify = providers
    spotify.get_generated_playlists.return_value = None

    result = UnifiedLibraryClient(cliamp, spotify).get_library_sections()

    assert result["made_for_you"] == []
    assert result["partial"] is False


def test_provider_failure_returns_other_source_and_metadata(providers):
    cliamp, spotify = providers
    spotify.get_recently_played.side_effect = RuntimeError("expired token")
    spotify.get_user_playlists.side_effect = RuntimeError("offline")
    client = UnifiedLibraryClient(cliamp, spotify)

    result = client.get_library_sections()

    assert [entry["title"] for entry in result["recently_played"]] == ["Local"]
    assert result["library"] == ["Road Trip", "Focus"]
    assert result["your_playlists"] == []
    assert result["partial"] is True
    assert result["failed_sources"] == ["spotify"]


def test_recent_failure_does_not_raise(providers):
    cliamp, spotify = providers
    cliamp.get_recently_played.side_effect = RuntimeError("daemon down")
    client = UnifiedLibraryClient(cliamp, spotify)
    assert client.get_recently_played()[0]["source"] == "spotify"
    assert client.failed_sources == ["cliamp"]


class TestUnifiedLibraryAgent:
    def test_manifest_and_scope_contract(self, providers):
        agent = UnifiedLibraryAgent(tools=list(providers))
        assert agent.is_task_authorized("Show my unified library") is True
        assert agent.is_task_authorized("What was recently played?") is True
        assert agent.is_task_authorized("Skip this song") is False
        assert agent.manifest["required_tools"] == ["CliampClient", "SpotifyClient"]

    def test_routes_library_and_recent_queries(self, providers):
        agent = UnifiedLibraryAgent(tools=list(providers))
        library = agent.process_instruction("Show my unified library")
        recent = agent.process_instruction("Show recently played history")
        assert library["status"] == "SUCCESS"
        assert list(library["data"])[:3] == [
            "recently_played", "library", "your_playlists"
        ]
        assert recent["status"] == "SUCCESS"
        assert isinstance(recent["data"], list)

    def test_mix_and_radio_vocabulary_authorization(self, providers):
        agent = UnifiedLibraryAgent(tools=list(providers))
        assert agent.is_task_authorized("Show my daily mixes.") is True
        assert agent.is_task_authorized("List my discover weekly.") is True
        assert agent.is_task_authorized("Open my discover weekly radio") is True
        # Prohibited playback tokens win over in-vocabulary library terms.
        assert agent.is_task_authorized("Skip this song in my daily mix.") is False

    def test_saved_and_top_vocabulary_authorization(self, providers):
        agent = UnifiedLibraryAgent(tools=list(providers))
        assert agent.is_task_authorized("Show my liked songs.") is True
        assert agent.is_task_authorized("List my saved albums.") is True
        assert agent.is_task_authorized("Show my top artists.") is True
        assert agent.is_task_authorized("What are my top tracks?") is True
        # Prohibited playback tokens still win.
        assert agent.is_task_authorized("Skip my liked songs.") is False

    def test_saved_and_top_queries_route_to_library_sections(self, providers):
        agent = UnifiedLibraryAgent(tools=list(providers))
        liked = agent.process_instruction("Show my liked songs")
        assert liked["status"] == "SUCCESS"
        assert "saved_tracks" in liked["data"]
        top = agent.process_instruction("Show my top artists")
        assert top["status"] == "SUCCESS"
        assert "top_artists" in top["data"]

    def test_mix_queries_route_to_library_sections(self, providers):
        agent = UnifiedLibraryAgent(tools=list(providers))
        result = agent.process_instruction("Show my daily mixes")
        assert result["status"] == "SUCCESS"
        assert "made_for_you" in result["data"]

    @patch("time.time", side_effect=[100.0, 101.2])
    def test_base_agent_sla_contract(self, _mock_time, providers):
        telemetry = UnifiedLibraryAgent(tools=list(providers)).execute_monitored_loop(
            "Show my unified library"
        )
        assert telemetry["metrics"]["sla_compliant"] is True
        assert telemetry["metrics"]["iterations"] == 1


def test_manifest_is_valid_and_read_only():
    with open("agent_manifest.cliamp_library.json", encoding="utf-8") as stream:
        manifest = json.load(stream)
    assert manifest["allowed_scopes"] == [
        "playlists.read", "history.read", "library.read"
    ]
    assert "playback.control" in manifest["prohibited_scopes"]
    assert manifest["token_tables"]["allowed_tokens"]["liked"] == "library.read"
    assert manifest["token_tables"]["allowed_tokens"]["top"] == "history.read"
