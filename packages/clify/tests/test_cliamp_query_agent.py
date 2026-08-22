"""TDD lifecycle verification pipeline for CliampQueryAgent (AGENTS.md §4.1).

Mirrors tests/test_lifecycle.py's three-stage structure against a mocked
CliampClient (ROADMAP Phase 2).
"""

import pytest
from unittest.mock import Mock, patch

# Define System Constants & SLAs
LATENCY_SLA_LIMIT = 2.5
COST_SLA_LIMIT = 0.02
ACCURACY_MIN_THRESHOLD = 0.90


class TestCliampAgentDrivenLifecycle:

    @pytest.fixture
    def mock_cliamp_client(self):
        """Mocks the subprocess boundary to ensure deterministic unit evaluations."""
        client = Mock()
        client.get_playlists.return_value = {
            "schema_version": "cliamp.playlist.list/1",
            "playlists": ["Synthwave Mix", "Coding Focus"],
        }
        client.get_playlist.return_value = {
            "name": "Synthwave Mix",
            "tracks": [{"path": "/music/a.flac", "title": "A", "duration": 200.0,
                        "bookmarked": False}],
        }
        client.get_recently_played.return_value = {
            "schema_version": "cliamp.history/1",
            "entries": [
                {"title": "Lofi Stream", "path": "http://x/stream",
                 "played_at": "2024-01-01T10:00:00", "stream": True},
                {"title": "Track B", "path": "/music/b.flac",
                 "played_at": None, "stream": None},
            ],
        }
        client.get_status.return_value = {
            "schema_version": "cliamp.status/1",
            "ok": True,
            "state": "playing",
            "track": {"title": "Lofi Stream", "path": "http://x/stream",
                      "stream": True},
        }
        return client

    @pytest.fixture
    def active_agent(self, mock_cliamp_client):
        """Initializes the production-grade target agent with strictly injected boundaries."""
        from cliamp_agents import CliampQueryAgent
        return CliampQueryAgent(tools=[mock_cliamp_client])

    def test_scope_boundary_enforcement(self, active_agent):
        """STAGE 1: Verify the agent actively understands and rejects tasks outside its domain."""
        # Assert valid input clears the boundary check
        assert active_agent.is_task_authorized("Show my cliamp playlists") is True
        assert active_agent.is_task_authorized("What did I listen to recently?") is True
        assert active_agent.is_task_authorized("What is currently playing?") is True

        # Assert illegal operations are caught and rejected immediately
        assert active_agent.is_task_authorized("Skip this song") is False
        assert active_agent.is_task_authorized("Turn the volume up") is False

    def test_execution_schema_conformance_playlists(self, active_agent, mock_cliamp_client):
        """STAGE 2: playlist terms route to get_playlists with a compliant payload."""
        response = active_agent.process_instruction("Show my cliamp playlists")

        # Verify appropriate tool was invoked exactly once
        mock_cliamp_client.get_playlists.assert_called_once()

        # Enforce typing boundaries on output data payload
        assert "status" in response
        assert response["status"] == "SUCCESS"
        assert isinstance(response["data"], dict)
        assert len(response["data"]["playlists"]) == 2

    def test_execution_schema_conformance_named_playlist(self, active_agent, mock_cliamp_client):
        """STAGE 2: a quoted playlist name routes to get_playlist(name)."""
        response = active_agent.process_instruction(
            'Show my cliamp playlist "Synthwave Mix"')

        mock_cliamp_client.get_playlist.assert_called_once_with("Synthwave Mix")
        assert response["status"] == "SUCCESS"
        assert response["data"]["name"] == "Synthwave Mix"
        assert isinstance(response["data"]["tracks"], list)

    def test_execution_schema_conformance_history(self, active_agent, mock_cliamp_client):
        """STAGE 2: history terms route to get_recently_played."""
        response = active_agent.process_instruction("What did I listen to recently?")

        mock_cliamp_client.get_recently_played.assert_called_once()
        assert response["status"] == "SUCCESS"
        assert response["data"]["schema_version"] == "cliamp.history/1"
        assert len(response["data"]["entries"]) == 2

    def test_execution_schema_conformance_status(self, active_agent, mock_cliamp_client):
        """STAGE 2: status terms route to get_status."""
        response = active_agent.process_instruction("What is currently playing?")

        mock_cliamp_client.get_status.assert_called_once()
        assert response["status"] == "SUCCESS"
        assert response["data"]["state"] == "playing"

    @patch("time.time")
    def test_sla_metric_compliance(self, mock_time, active_agent):
        """STAGE 3: Verify the agent acts within runtime performance and budget guardrails."""
        # Mock timestamp sequence to capture a 1.2-second execution timeline
        mock_time.side_effect = [100.0, 101.2]

        telemetry = active_agent.execute_monitored_loop("What is currently playing?")

        # Validate critical operational SLAs
        assert telemetry["metrics"]["duration_seconds"] <= LATENCY_SLA_LIMIT
        assert telemetry["metrics"]["calculated_cost_usd"] <= COST_SLA_LIMIT
        assert telemetry["metrics"]["confidence_rating"] >= ACCURACY_MIN_THRESHOLD
        assert telemetry["metrics"]["sla_compliant"] is True
