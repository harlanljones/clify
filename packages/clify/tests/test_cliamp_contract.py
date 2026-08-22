"""Contract tests pinning cliamp's JSON schemas (ROADMAP Phase 4).

These tests pin the exact payloads documented in docs/cliamp_schemas.md —
schema_version fields, the playlist track array shape, and the status payload
keys (including ``ok``) — and validate CliampClient parsing against schema
fixtures with a mocked subprocess boundary. If upstream cliamp drifts from
the documented schemas, these tests fail loudly.

Also verifies the AGENTS.md §5 alerting table under fault injection using
mocked telemetry: a killed daemon surfaces TOOL_ERROR payloads which raise
the validation_failure_rate alert, and latency/cost alerts fire at their
thresholds (no edits to monitoring.py required).
"""

import json
import subprocess
from unittest.mock import patch

import pytest

from cliamp_client import CliampClient, CliampParseError, ToolError
from cliamp_controller import CliampController
from monitoring import (
    COST_ALERT_USD,
    LATENCY_ALERT_SECONDS,
    VALIDATION_FAILURE_RATE_ALERT,
    TelemetryRegistry,
)

# ---------------------------------------------------------------------------
# Schema fixtures — verbatim from docs/cliamp_schemas.md (cliamp v1.63.2)
# ---------------------------------------------------------------------------

SCHEMA_VERSIONS = {
    "playlist_show": "cliamp.playlist.show/1",
    "playlist_list": "cliamp.playlist.list/1",
    "history": "cliamp.history/1",
    "history_unified": "cliamp.history.unified/1",
    "status": "cliamp.status/1",
}

PLAYLIST_SHOW_FIXTURE = [
    {
        "path": "/music/track1.flac",
        "title": "Track One",
        "duration": 213.5,
        "bookmarked": False,
    },
    {"path": "http://radio.cliamp.stream/lofi/stream"},  # optional fields absent
]

HISTORY_FIXTURE = [
    {
        "title": "Lofi Stream",
        "path": "http://radio.cliamp.stream/lofi/stream",
        "played_at": "2026-08-21T10:00:00Z",
        "stream": True,
    }
]

UNIFIED_HISTORY_FIXTURE = {
    "ok": True,
    "schema_version": "cliamp.history.unified/1",
    "history": [{
        "track": {"title": "A Song", "path": "spotify:track:abc"},
        "played_at": "2026-08-21T10:00:00Z",
        "sources": ["cliamp", "spotify"],
    }],
    "partial": False,
    "failed_sources": [],
}

STATUS_FIXTURE = {
    "ok": True,
    "state": "stopped",
    "track": {
        "title": "Lofi Stream",
        "path": "http://radio.cliamp.stream/lofi/stream",
        "stream": True,
    },
    "total": 11,
    "visualizer": "Bars",
    "shuffle": False,
    "repeat": "Off",
    "mono": False,
    "speed": 1,
    "eq_preset": "Custom",
    "theme": {"name": "Default - Terminal colors"},
    "eq_bands": [0, 0, 0, 0, 0, 0, 0, 0, 0, 0],
}

PLAYLIST_LIST_TEXT = "Synthwave Mix (2 tracks)\nCoding Focus (11 tracks)\n"


def _proc(stdout="", stderr="", returncode=0):
    return subprocess.CompletedProcess(
        args=["cliamp"], returncode=returncode, stdout=stdout, stderr=stderr
    )


@pytest.fixture
def client():
    return CliampClient()

# ---------------------------------------------------------------------------
# Contract: schema_version fields & payload shapes
# ---------------------------------------------------------------------------


class TestSchemaVersionPins:
    """Pinned schema_version strings from docs/cliamp_schemas.md."""

    def test_playlist_list_schema_version(self, client):
        with patch("cliamp_client.subprocess.run",
                   return_value=_proc(stdout=PLAYLIST_LIST_TEXT)):
            result = client.get_playlists()
        assert result["schema_version"] == SCHEMA_VERSIONS["playlist_list"]
        assert result["playlists"] == ["Synthwave Mix", "Coding Focus"]

    def test_history_schema_version(self, client):
        with patch("cliamp_client.subprocess.run",
                   return_value=_proc(stdout=json.dumps(HISTORY_FIXTURE))):
            result = client.get_recently_played()
        assert result["schema_version"] == SCHEMA_VERSIONS["history"]

    def test_unified_history_schema_version(self, client):
        with patch("cliamp_client.subprocess.run",
                   return_value=_proc(stdout=json.dumps(UNIFIED_HISTORY_FIXTURE))):
            result = client.get_unified_recently_played()
        assert result["schema_version"] == SCHEMA_VERSIONS["history_unified"]
        assert result["history"][0]["sources"] == ["cliamp", "spotify"]

    def test_status_schema_version(self, client):
        with patch("cliamp_client.subprocess.run",
                   return_value=_proc(stdout=json.dumps(STATUS_FIXTURE))):
            result = client.get_status()
        assert result["schema_version"] == SCHEMA_VERSIONS["status"]


class TestPlaylistTrackArrayShape:
    """cliamp.playlist.show/1: a bare JSON array of tracks; path required."""

    def test_bare_array_parsed_to_tracks(self, client):
        with patch("cliamp_client.subprocess.run",
                   return_value=_proc(stdout=json.dumps(PLAYLIST_SHOW_FIXTURE))):
            result = client.get_playlist("Synthwave Mix")
        assert result["name"] == "Synthwave Mix"
        assert len(result["tracks"]) == 2
        first = result["tracks"][0]
        assert first["path"] == "/music/track1.flac"
        assert first["title"] == "Track One"
        assert first["duration"] == 213.5
        assert first["bookmarked"] is False
        # Optional fields are pass-through (absent -> None)
        second = result["tracks"][1]
        assert second["path"] == "http://radio.cliamp.stream/lofi/stream"
        assert second["title"] is None

    def test_empty_playlist_is_empty_array(self, client):
        with patch("cliamp_client.subprocess.run", return_value=_proc(stdout="[]")):
            result = client.get_playlist("Empty")
        assert result["tracks"] == []

    def test_non_array_payload_fails_loudly(self, client):
        """Upstream drift: object instead of bare array must raise."""
        with patch("cliamp_client.subprocess.run",
                   return_value=_proc(stdout='{"tracks": []}')):
            with pytest.raises(CliampParseError):
                client.get_playlist("Drifted")


class TestStatusPayloadKeys:
    """cliamp.status/1: status payload must carry ok/state/track keys."""

    def test_status_keys_including_ok(self, client):
        with patch("cliamp_client.subprocess.run",
                   return_value=_proc(stdout=json.dumps(STATUS_FIXTURE))):
            result = client.get_status()
        assert result["ok"] is True
        assert result["state"] == "stopped"
        assert result["track"]["title"] == "Lofi Stream"
        # Extra documented fields pass through untouched.
        assert result["eq_bands"] == [0] * 10
        assert result["repeat"] == "Off"

    def test_status_ok_false_signals_daemon_down(self, client):
        payload = dict(STATUS_FIXTURE, ok=False)
        with patch("cliamp_client.subprocess.run",
                   return_value=_proc(stdout=json.dumps(payload))):
            result = client.get_status()
        assert result["ok"] is False

    def test_non_object_status_fails_loudly(self, client):
        with patch("cliamp_client.subprocess.run", return_value=_proc(stdout="[]")):
            with pytest.raises(CliampParseError):
                client.get_status()


class TestHistoryShape:
    """cliamp.history/1: JSON array of entries with title/path required."""

    def test_history_entries(self, client):
        with patch("cliamp_client.subprocess.run",
                   return_value=_proc(stdout=json.dumps(HISTORY_FIXTURE))):
            result = client.get_recently_played()
        entry = result["entries"][0]
        assert entry["title"] == "Lofi Stream"
        assert entry["path"] == "http://radio.cliamp.stream/lofi/stream"
        assert entry["stream"] is True

    def test_empty_history(self, client):
        with patch("cliamp_client.subprocess.run", return_value=_proc(stdout="[]")):
            assert client.get_recently_played()["entries"] == []


# ---------------------------------------------------------------------------
# §5 alert verification under fault injection (mocked telemetry)
# ---------------------------------------------------------------------------


def _telemetry(duration=0.5, cost=0.01, confidence=1.0):
    return {
        "metrics": {
            "duration_seconds": duration,
            "calculated_cost_usd": cost,
            "confidence_rating": confidence,
        }
    }


class TestSection5Alerts:
    """Prove the §5 alerting table fires under fault-injected cliamp runs."""

    def test_killed_daemon_raises_validation_failure_rate_alert(self):
        """Killed daemon -> TOOL_ERROR (confidence < 1.0) -> >2% failure rate."""
        # Fault injection: daemon down; controller mutation fails fast.
        controller = CliampController()
        with patch("cliamp_client.subprocess.run",
                   return_value=_proc(returncode=1, stderr="no daemon")):
            with pytest.raises(ToolError) as excinfo:
                controller.pause()
        assert excinfo.value.payload["status"] == "TOOL_ERROR"
        assert excinfo.value.payload["retry_allowed"] is False

        # Feed the registry 48 healthy runs + 1 killed-daemon failure (2.04% > 2%).
        registry = TelemetryRegistry()
        for _ in range(48):
            registry.record(_telemetry())
        alerts = registry.record(_telemetry(confidence=0.0))
        assert registry.validation_failure_rate > VALIDATION_FAILURE_RATE_ALERT
        assert any(a["metric"] == "validation_failure_rate" for a in alerts)
        assert any(
            a["action"] == "Route tasks back to previous safe software build"
            for a in alerts
        )

    def test_latency_alert_fires_above_threshold(self):
        registry = TelemetryRegistry()
        registry.record(_telemetry(duration=LATENCY_ALERT_SECONDS + 0.5))
        assert any(a["metric"] == "agent_latency_seconds" for a in registry.alerts)
        assert any(
            a["action"] == "Fall back to lightweight prompt template"
            for a in registry.alerts
        )

    def test_cost_alert_fires_above_threshold(self):
        registry = TelemetryRegistry()
        registry.record(_telemetry(cost=COST_ALERT_USD + 0.01))
        assert any(a["metric"] == "agent_cost_per_task" for a in registry.alerts)
        assert any(
            a["action"] == "Kill runtime engine execution thread"
            for a in registry.alerts
        )

    def test_healthy_telemetry_raises_no_alerts(self):
        registry = TelemetryRegistry()
        for _ in range(10):
            registry.record(_telemetry())
        assert registry.alerts == []
