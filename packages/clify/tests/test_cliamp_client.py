"""Phase 1 tests: CliampClient subprocess boundary and schema conformance.

TDD stages per AGENTS.md §4 / ROADMAP Phase 1:
- Red/Green: argv construction, JSON parsing into documented schemas,
  structured TOOL_ERROR propagation for every documented failure mode
  (docs/cliamp_schemas.md §5).
"""

import json
import subprocess
from unittest.mock import Mock, patch

import pytest

from cliamp_client import (
    BinaryMissingError,
    CliampClient,
    CliampCommandError,
    CliampParseError,
    CliampTimeoutError,
    ToolError,
)


def make_proc(returncode=0, stdout="", stderr=""):
    proc = Mock(spec=subprocess.CompletedProcess)
    proc.returncode = returncode
    proc.stdout = stdout
    proc.stderr = stderr
    return proc


@pytest.fixture
def client():
    return CliampClient()


class TestArgvConstruction:
    """Exact argv, capture_output=True, text=True, timeout=2.0 on every call."""

    def check_run(self, mock_run, expected_argv):
        mock_run.assert_called_once_with(
            expected_argv, capture_output=True, text=True, timeout=2.0
        )

    @patch("cliamp_client.subprocess.run")
    def test_get_playlists_argv(self, mock_run, client):
        mock_run.return_value = make_proc(stdout="Mix (3 tracks)\nFocus (1 tracks)\n")
        client.get_playlists()
        self.check_run(mock_run, ["cliamp", "playlist", "list"])

    @patch("cliamp_client.subprocess.run")
    def test_get_playlist_argv(self, mock_run, client):
        mock_run.return_value = make_proc(stdout="[]")
        client.get_playlist("Synthwave Mix")
        self.check_run(
            mock_run, ["cliamp", "playlist", "show", "Synthwave Mix", "--json"]
        )

    @patch("cliamp_client.subprocess.run")
    def test_get_recently_played_default_limit(self, mock_run, client):
        mock_run.return_value = make_proc(stdout="[]")
        client.get_recently_played()
        self.check_run(mock_run, ["cliamp", "history", "--json", "--limit", "50"])

    @patch("cliamp_client.subprocess.run")
    def test_get_recently_played_custom_limit(self, mock_run, client):
        mock_run.return_value = make_proc(stdout="[]")
        client.get_recently_played(limit=10)
        self.check_run(mock_run, ["cliamp", "history", "--json", "--limit", "10"])

    @patch("cliamp_client.subprocess.run")
    def test_get_unified_recently_played_argv(self, mock_run, client):
        mock_run.return_value = make_proc(
            stdout='{"ok":true,"schema_version":"cliamp.history.unified/1","history":[]}'
        )
        client.get_unified_recently_played(limit=20)
        self.check_run(
            mock_run, ["cliamp", "history", "unified", "--json", "--limit", "20"]
        )

    @patch("cliamp_client.subprocess.run")
    def test_get_status_argv(self, mock_run, client):
        mock_run.return_value = make_proc(stdout='{"ok": true, "state": "stopped"}')
        client.get_status()
        self.check_run(mock_run, ["cliamp", "status", "--json"])


class TestSchemaConformance:
    @patch("cliamp_client.subprocess.run")
    def test_get_playlists_parses_text_output(self, mock_run, client):
        mock_run.return_value = make_proc(
            stdout="Synthwave Mix (12 tracks)\nCoding Focus (4 tracks)\n"
        )
        result = client.get_playlists()
        assert result["schema_version"] == "cliamp.playlist.list/1"
        assert result["playlists"] == ["Synthwave Mix", "Coding Focus"]

    @patch("cliamp_client.subprocess.run")
    def test_get_playlists_empty_state(self, mock_run, client):
        mock_run.return_value = make_proc(stdout="No playlists found.\n")
        assert client.get_playlists()["playlists"] == []

    @patch("cliamp_client.subprocess.run")
    def test_get_playlist_bare_array(self, mock_run, client):
        tracks = [
            {"path": "/music/a.mp3", "title": "A", "duration": 180.0,
             "bookmarked": False},
            {"path": "http://stream/x", "title": "X"},
        ]
        mock_run.return_value = make_proc(stdout=json.dumps(tracks))
        result = client.get_playlist("Mix")
        assert result["name"] == "Mix"
        assert len(result["tracks"]) == 2
        assert result["tracks"][0]["path"] == "/music/a.mp3"
        assert result["tracks"][0]["duration"] == 180.0
        assert result["tracks"][1]["title"] == "X"

    @patch("cliamp_client.subprocess.run")
    def test_get_playlist_empty(self, mock_run, client):
        mock_run.return_value = make_proc(stdout="[]")
        result = client.get_playlist("Empty")
        assert result["tracks"] == []

    @patch("cliamp_client.subprocess.run")
    def test_get_recently_played_entries(self, mock_run, client):
        history = [
            {"title": "Lofi", "path": "http://x", "played_at": "2025-01-01T00:00:00Z",
             "stream": True},
        ]
        mock_run.return_value = make_proc(stdout=json.dumps(history))
        result = client.get_recently_played()
        assert result["schema_version"] == "cliamp.history/1"
        assert result["entries"][0]["title"] == "Lofi"
        assert result["entries"][0]["stream"] is True

    @patch("cliamp_client.subprocess.run")
    def test_get_unified_recently_played_contract(self, mock_run, client):
        payload = {
            "ok": True,
            "schema_version": "cliamp.history.unified/1",
            "history": [{
                "track": {"title": "Song", "path": "spotify:track:abc"},
                "played_at": "2026-08-21T10:00:00Z",
                "sources": ["cliamp", "spotify"],
            }],
            "partial": False,
            "failed_sources": [],
        }
        mock_run.return_value = make_proc(stdout=json.dumps(payload))
        assert client.get_unified_recently_played() == payload

    @patch("cliamp_client.subprocess.run")
    def test_get_status_payload(self, mock_run, client):
        payload = {
            "ok": True, "state": "stopped",
            "track": {"title": "Lofi Stream", "path": "http://x", "stream": True},
            "shuffle": False, "repeat": "Off", "eq_bands": [0] * 10,
        }
        mock_run.return_value = make_proc(stdout=json.dumps(payload))
        result = client.get_status()
        assert result["schema_version"] == "cliamp.status/1"
        assert result["ok"] is True
        assert result["state"] == "stopped"
        assert result["track"]["title"] == "Lofi Stream"
        assert result["repeat"] == "Off"  # unknown fields pass through


class TestFailureModes:
    """docs/cliamp_schemas.md §5: all failures are ToolError-compatible."""

    @patch("cliamp_client.subprocess.run")
    def test_nonzero_exit_raises_tool_error(self, mock_run, client):
        mock_run.return_value = make_proc(
            returncode=1, stderr='playlist "Nope" not found'
        )
        with pytest.raises(ToolError) as exc_info:
            client.get_playlist("Nope")
        assert exc_info.value.payload["status"] == "TOOL_ERROR"
        assert exc_info.value.payload["retry_allowed"] is False

    @patch("cliamp_client.subprocess.run")
    def test_nonzero_exit_retryable_otherwise(self, mock_run, client):
        mock_run.return_value = make_proc(returncode=2, stderr="boom")
        with pytest.raises(CliampCommandError) as exc_info:
            client.get_status()
        assert exc_info.value.retry_allowed is True

    @patch("cliamp_client.subprocess.run")
    def test_malformed_json_raises(self, mock_run, client):
        mock_run.return_value = make_proc(stdout="{not json")
        with pytest.raises(CliampParseError) as exc_info:
            client.get_recently_played()
        assert exc_info.value.payload["retry_allowed"] is True

    @patch("cliamp_client.subprocess.run")
    def test_empty_stdout_raises(self, mock_run, client):
        mock_run.return_value = make_proc(stdout="  \n")
        with pytest.raises(CliampParseError):
            client.get_status()

    @patch("cliamp_client.subprocess.run")
    def test_wrong_json_type_raises(self, mock_run, client):
        mock_run.return_value = make_proc(stdout='{"unexpected": true}')
        with pytest.raises(CliampParseError):
            client.get_playlist("Mix")

    @patch("cliamp_client.subprocess.run")
    def test_missing_binary_raises(self, mock_run, client):
        mock_run.side_effect = FileNotFoundError("cliamp")
        with pytest.raises(BinaryMissingError) as exc_info:
            client.get_status()
        assert exc_info.value.payload == {
            "status": "TOOL_ERROR",
            "retry_allowed": False,
            "error": exc_info.value.message,
        }

    @patch("cliamp_client.subprocess.run")
    def test_timeout_raises(self, mock_run, client):
        mock_run.side_effect = subprocess.TimeoutExpired(cmd="cliamp", timeout=2.0)
        with pytest.raises(CliampTimeoutError) as exc_info:
            client.get_status()
        assert exc_info.value.retry_allowed is True

    def test_empty_playlist_name_rejected_without_subprocess(self, client):
        # cliamp playlist show with no name exits 0 with a usage line —
        # validate before spawning (docs/cliamp_schemas.md §1).
        with patch("cliamp_client.subprocess.run") as mock_run:
            with pytest.raises(CliampCommandError):
                client.get_playlist("  ")
            mock_run.assert_not_called()

    def test_all_errors_are_tool_error_compatible(self):
        for err_cls in (BinaryMissingError, CliampCommandError,
                        CliampParseError, CliampTimeoutError):
            assert issubclass(err_cls, ToolError)
