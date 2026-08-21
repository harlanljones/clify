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
    assert manifest["allowed_scopes"] == ["playlists.read", "history.read"]
    assert "playback.control" in manifest["prohibited_scopes"]
