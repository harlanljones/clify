"""Phase 3 TDD suite: cliamp_playback_agent (mutation-capable).

Red: boundary, routing, verification-loop, daemon-down contracts.
Spec refs: AGENTS.md §2.2, §3, §4.2; ROADMAP Phase 3.
"""

import json
from unittest.mock import Mock, patch

import pytest

from cliamp_client import CliampCommandError
from cliamp_controller import CliampController, CliampDaemonError
from cliamp_playback import CliampPlaybackAgent
from core_agents import LibrarySyncAgent, MaxIterationExceeded

PLAYING = {"ok": True, "state": "playing", "track": {"title": "X"}}
PAUSED = {"ok": True, "state": "paused", "track": {"title": "X"}}
STOPPED = {"ok": True, "state": "stopped", "track": None}


def _completed(stdout="", returncode=0, stderr=""):
    proc = Mock()
    proc.stdout = stdout
    proc.stderr = stderr
    proc.returncode = returncode
    return proc


@pytest.fixture
def mock_controller():
    return Mock(spec=CliampController)


@pytest.fixture
def agent(mock_controller):
    return CliampPlaybackAgent(tools=[mock_controller])


@pytest.fixture
def library_agent():
    return LibrarySyncAgent(
        identity="LibrarySyncAgent",
        tools=[Mock()],
        configured_scopes=["library.read", "playlists.read", "history.read"],
    )


# --------------------------------------------------------------------------
# Stage 1 — boundary tests
# --------------------------------------------------------------------------

class TestBoundary:
    @pytest.mark.parametrize("instruction", [
        "play the track",
        "pause the music",
        "toggle the music",
        "stop the music",
        "skip this song",
        "next track please",
        "previous track please",
        "volume up please",
        "seek the song 30",
        "queue the song file.mp3",
        "load the playlist Chill",
        "shuffle the playlist",
        "repeat this song",
    ])
    def test_playback_verbs_authorized(self, agent, instruction):
        assert agent.is_task_authorized(instruction) is True

    @pytest.mark.parametrize("instruction", [
        "clear the queue",          # destructive -> playlists.write
        "delete the playlist",      # destructive -> playlists.write
        "clear my history",         # destructive -> playlists.write
        "update my billing info",   # user.billing
        "cancel the payment",       # user.billing
        "show me the playlists",    # read-only query scope, not playback
        "fetch my listening history",
    ])
    def test_out_of_scope_rejected(self, agent, instruction):
        assert agent.is_task_authorized(instruction) is False
        response = agent.process_instruction(instruction)
        assert response == {"status": "REJECTED", "reason": "OUT_OF_SCOPE", "data": []}

    def test_ambiguous_instruction_rejected_at_boundary(self, agent, mock_controller):
        # Two distinct verbs -> no unambiguous IPC command.
        assert agent.is_task_authorized("play and shuffle the music") is False
        assert agent.process_instruction("play and shuffle the music")["status"] == "REJECTED"
        mock_controller.play.assert_not_called()
        mock_controller.shuffle.assert_not_called()

    @pytest.mark.parametrize("instruction", [
        "skip this song",
        "pause the music",
        "volume up please",
        "queue the song file.mp3",
        "play the track",
    ])
    def test_library_sync_agent_rejects_playback_verbs(self, library_agent, instruction):
        assert library_agent.is_task_authorized(instruction) is False

    @pytest.mark.parametrize("instruction", [
        "skip this song",
        "pause the music",
        "volume up please",
    ])
    def test_cliamp_query_agent_rejects_playback_verbs(self, instruction):
        query_mod = pytest.importorskip("cliamp_agents", reason="Phase 2 agent not present")
        query_agent = query_mod.CliampQueryAgent(tools=[Mock()])
        assert query_agent.is_task_authorized(instruction) is False


# --------------------------------------------------------------------------
# Stage 2 — routing tests: each verb -> exactly one controller call
# --------------------------------------------------------------------------

class TestRouting:
    @pytest.mark.parametrize("instruction,method,args,payload", [
        ("play the track", "play", (), PLAYING),
        ("pause the music", "pause", (), PAUSED),
        ("toggle the music", "toggle", (), PLAYING),
        ("stop the music", "stop", (), STOPPED),
        ("skip this song", "next", (), PLAYING),
        ("next track please", "next", (), PLAYING),
        ("previous track please", "prev", (), PAUSED),
        ("volume up please 5", "volume", ("5",), PLAYING),
        ("seek the song 30", "seek", ("30",), PLAYING),
        ("queue the song file.mp3", "queue", ("file.mp3",), PLAYING),
        ("load the playlist Chill", "load", ("Chill",), PLAYING),
        ("shuffle the playlist", "shuffle", (), PLAYING),
        ("repeat this song", "repeat", (), PLAYING),
    ])
    def test_verb_routes_to_exactly_one_call(
        self, agent, mock_controller, instruction, method, args, payload
    ):
        getattr(mock_controller, method).return_value = payload
        response = agent.process_instruction(instruction)
        assert response["status"] == "SUCCESS"
        getattr(mock_controller, method).assert_called_once_with(*args)
        # No other controller verb was touched.
        for other in ("play", "pause", "toggle", "stop", "next", "prev",
                      "volume", "seek", "queue", "load", "shuffle", "repeat"):
            if other != method:
                getattr(mock_controller, other).assert_not_called()

    def test_success_payload_contains_post_command_status(self, agent, mock_controller):
        mock_controller.pause.return_value = PAUSED
        response = agent.process_instruction("pause the music")
        assert response["data"] == [PAUSED]

    def test_confidence_is_1_on_state_match(self, agent, mock_controller):
        mock_controller.pause.return_value = PAUSED
        telemetry = agent.execute_monitored_loop("pause the music")
        assert telemetry["metrics"]["confidence_rating"] == 1.0
        assert telemetry["metrics"]["sla_compliant"] is True


# --------------------------------------------------------------------------
# Stage 2/3 — verification loop
# --------------------------------------------------------------------------

class TestVerificationLoop:
    def test_max_iteration_exceeded_after_3_failed_confirmations(
        self, agent, mock_controller
    ):
        # Post-command status never reaches "paused".
        mock_controller.pause.return_value = PLAYING
        with pytest.raises(MaxIterationExceeded):
            agent.process_instruction("pause the music")
        assert mock_controller.pause.call_count == 3

    def test_retry_succeeds_on_second_confirmation(self, agent, mock_controller):
        mock_controller.pause.side_effect = [PLAYING, PAUSED]
        response = agent.process_instruction("pause the music")
        assert response["status"] == "SUCCESS"
        assert mock_controller.pause.call_count == 2

    def test_toggle_confirms_on_ok_status(self, agent, mock_controller):
        mock_controller.toggle.return_value = {"ok": True, "state": "paused"}
        response = agent.process_instruction("toggle the music")
        assert response["status"] == "SUCCESS"


# --------------------------------------------------------------------------
# Daemon-down — fail-fast, structured TOOL_ERROR, retry_allowed=false
# --------------------------------------------------------------------------

class TestDaemonDown:
    def test_agent_returns_structured_tool_error_no_retry(self, agent, mock_controller):
        mock_controller.play.side_effect = CliampDaemonError("no daemon running")
        response = agent.process_instruction("play the track")
        assert response["status"] == "TOOL_ERROR"
        assert response["retry_allowed"] is False

    def test_monitored_loop_does_not_retry_daemon_down(self, agent, mock_controller):
        mock_controller.play.side_effect = CliampDaemonError("no daemon running")
        telemetry = agent.execute_monitored_loop("play the track")
        assert telemetry["response"]["retry_allowed"] is False
        assert telemetry["metrics"]["iterations"] == 1
        assert mock_controller.play.call_count == 1

    @patch("subprocess.run")
    def test_controller_fail_fast_when_status_fails(self, mock_run):
        mock_run.return_value = _completed(returncode=1, stderr="no daemon running")
        controller = CliampController()
        with pytest.raises(CliampDaemonError) as excinfo:
            controller.pause()
        assert excinfo.value.retry_allowed is False
        # Never spawned a control verb; never spawned a daemon.
        for call in mock_run.call_args_list:
            assert call.args[0][1] == "status"

    @patch("subprocess.run")
    def test_controller_fail_fast_when_status_not_ok(self, mock_run):
        mock_run.return_value = _completed(stdout=json.dumps({"ok": False}))
        controller = CliampController()
        with pytest.raises(CliampDaemonError) as excinfo:
            controller.next()
        assert excinfo.value.retry_allowed is False

    @patch("subprocess.run")
    def test_controller_fail_fast_when_control_command_fails(self, mock_run):
        mock_run.side_effect = [
            _completed(stdout=json.dumps(PLAYING)),          # precondition status
            _completed(returncode=1, stderr="no daemon"),    # the verb itself
        ]
        controller = CliampController()
        with pytest.raises(CliampDaemonError) as excinfo:
            controller.pause()
        assert excinfo.value.retry_allowed is False

    @patch("subprocess.run")
    def test_controller_returns_post_command_status(self, mock_run):
        mock_run.side_effect = [
            _completed(stdout=json.dumps(PLAYING)),   # precondition
            _completed(stdout=""),                    # pause verb
            _completed(stdout=json.dumps(PAUSED)),    # post-command status
        ]
        controller = CliampController()
        payload = controller.pause()
        assert payload["state"] == "paused"
        assert mock_run.call_count == 3
        # All subprocess calls honor the 2.0s timeout (§3.1 latency SLA).
        for call in mock_run.call_args_list:
            assert call.kwargs["timeout"] == 2.0
