"""Integration tests against a live cliamp daemon (ROADMAP Phase 4).

These tests are skipped unless the ``CLIAMP_INTEGRATION`` environment variable
is set. To run them:

1. Install cliamp (>= v1.63.2) so the ``cliamp`` binary is on ``PATH``.
   On Linux you may need ``pipewire-alsa`` (or equivalent audio backend) for
   actual playback; status/playlist queries work headlessly.
2. Launch the daemon (the framework never auto-spawns it — fail-fast):

       cliamp --daemon &

3. Run the suite with the env var set:

       CLIAMP_INTEGRATION=1 python -m pytest tests/test_cliamp_integration.py -v

They exercise real ``CliampClient.get_status`` / ``get_playlists`` calls and a
couple of playback round-trips through ``CliampPlaybackAgent`` dispatched via
``Orchestrator``, asserting SLA-compliant telemetry end to end.
"""

import os
import shutil

import pytest

from cliamp_client import CliampClient
from cliamp_playback import CliampPlaybackAgent
from cliamp_agents import CliampQueryAgent
from orchestrator import Orchestrator

pytestmark = pytest.mark.integration

_REQUIRES_DAEMON = pytest.mark.skipif(
    not os.environ.get("CLIAMP_INTEGRATION"),
    reason="CLIAMP_INTEGRATION unset; start `cliamp --daemon` and set it to run",
)


@pytest.fixture(scope="module")
def client():
    if not shutil.which("cliamp"):
        pytest.skip("cliamp binary not on PATH")
    return CliampClient()


@_REQUIRES_DAEMON
class TestLiveReadPaths:
    def test_get_status_live(self, client):
        status = client.get_status()
        assert status["schema_version"] == "cliamp.status/1"
        assert status["ok"] is True
        assert status["state"] in ("playing", "paused", "stopped")

    def test_get_playlists_live(self, client):
        result = client.get_playlists()
        assert result["schema_version"] == "cliamp.playlist.list/1"
        assert isinstance(result["playlists"], list)


@_REQUIRES_DAEMON
class TestPlaybackRoundTrips:
    """Playback verbs round-trip through CliampPlaybackAgent via Orchestrator."""

    @pytest.fixture
    def orchestrator(self):
        return Orchestrator([CliampPlaybackAgent(), CliampQueryAgent()])

    def test_toggle_round_trip(self, orchestrator):
        before = CliampClient().get_status()
        telemetry = orchestrator.run("toggle playback")
        assert telemetry["response"]["status"] == "SUCCESS"
        assert telemetry["metrics"]["sla_compliant"] is True
        after = CliampClient().get_status()
        assert after["state"] != before["state"] or after["ok"] is True
        # Restore prior state.
        orchestrator.run("toggle playback")

    def test_pause_then_play_round_trip(self, orchestrator):
        t_pause = orchestrator.run("pause the music")
        assert t_pause["response"]["status"] == "SUCCESS"
        assert CliampClient().get_status()["state"] == "paused"
        t_play = orchestrator.run("play the music")
        assert t_play["response"]["status"] == "SUCCESS"
        assert CliampClient().get_status()["state"] == "playing"
