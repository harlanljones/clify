"""TDD lifecycle verification pipeline (AGENTS.md §4.1) + failure modes (§4.2)."""

import pytest
from unittest.mock import Mock, patch

# Define System Constants & SLAs
LATENCY_SLA_LIMIT = 2.5
COST_SLA_LIMIT = 0.02
ACCURACY_MIN_THRESHOLD = 0.90


class TestAgentDrivenLifecycle:

    @pytest.fixture
    def mock_spotify_sdk(self):
        """Mocks low-level API operations to ensure deterministic unit evaluations."""
        sdk = Mock()
        sdk.get_user_playlists.return_value = {
            "items": [{"name": "Synthwave Mix", "id": "4aV8"},
                      {"name": "Coding Focus", "id": "9zP1"}],
            "total": 2,
        }
        return sdk

    @pytest.fixture
    def active_agent(self, mock_spotify_sdk):
        """Initializes the production-grade target agent with strictly injected boundaries."""
        from core_agents import LibrarySyncAgent
        return LibrarySyncAgent(
            identity="LibrarySyncAgent",
            tools=[mock_spotify_sdk],
            configured_scopes=["library.read", "playlists.read"],
        )

    def test_scope_boundary_enforcement(self, active_agent):
        """STAGE 1: Verify the agent actively understands and rejects tasks outside its domain."""
        in_scope_task = "List out all playlists currently saved in my profile library."
        out_of_scope_task = "Skip this song, clear my queue, and increase the volume."

        # Assert valid input clears the boundary check
        assert active_agent.is_task_authorized(in_scope_task) is True

        # Assert illegal operations are caught and rejected immediately
        assert active_agent.is_task_authorized(out_of_scope_task) is False

    def test_execution_schema_conformance(self, active_agent, mock_spotify_sdk):
        """STAGE 2: Verify logic pipeline returns compliant structural payloads."""
        instruction = "Fetch my playlists."

        response = active_agent.process_instruction(instruction)

        # Verify appropriate tool was invoked exactly once
        mock_spotify_sdk.get_user_playlists.assert_called_once()

        # Enforce typing boundaries on output data payload
        assert "status" in response
        assert response["status"] == "SUCCESS"
        assert isinstance(response["data"], list)
        assert len(response["data"]) == 2

    @patch("time.time")
    def test_sla_metric_compliance(self, mock_time, active_agent):
        """STAGE 3: Verify the agent acts within runtime performance and budget guardrails."""
        # Mock timestamp sequence to capture a 1.2-second execution timeline
        mock_time.side_effect = [100.0, 101.2]

        telemetry = active_agent.execute_monitored_loop("Retrieve library data.")

        # Validate critical operational SLAs
        assert telemetry["metrics"]["duration_seconds"] <= LATENCY_SLA_LIMIT
        assert telemetry["metrics"]["calculated_cost_usd"] <= COST_SLA_LIMIT
        assert telemetry["metrics"]["confidence_rating"] >= ACCURACY_MIN_THRESHOLD


class TestFailureModes:
    """§4.2: agents must fail safely without polluting downstream workflows."""

    @pytest.fixture
    def failing_sdk(self):
        sdk = Mock()
        sdk.get_user_playlists.side_effect = RuntimeError("Spotify API unreachable")
        return sdk

    @pytest.fixture
    def agent(self, failing_sdk):
        from core_agents import LibrarySyncAgent
        return LibrarySyncAgent("LibrarySyncAgent", [failing_sdk],
                                ["library.read", "playlists.read"])

    def test_tool_error_returns_structured_payload(self, agent):
        response = agent.process_instruction("Fetch my playlists.")
        assert response["status"] == "TOOL_ERROR"
        assert response["retry_allowed"] is True

    def test_max_iteration_depth_guard(self, agent):
        from core_agents import MaxIterationExceeded
        with pytest.raises(MaxIterationExceeded):
            agent.execute_monitored_loop("Fetch my playlists.")
        assert agent.spotify.get_user_playlists.call_count == 3

    def test_context_overflow_interception(self, agent):
        from orchestrator import Orchestrator, ContextOverflowError
        orchestrator = Orchestrator(agent, window_size=1000)
        huge_prompt = "playlists " * 500  # ~1137 estimated tokens > 85% of 1000
        with pytest.raises(ContextOverflowError):
            orchestrator.run(huge_prompt)

    def test_out_of_scope_task_rejected_by_orchestrator(self, agent):
        from orchestrator import Orchestrator
        orchestrator = Orchestrator(agent)
        with pytest.raises(PermissionError):
            orchestrator.run("Skip this song and increase the volume.")


class TestMonitoring:
    """§5: telemetry streaming and alerting thresholds."""

    @pytest.fixture
    def registry(self):
        from monitoring import TelemetryRegistry
        return TelemetryRegistry()

    def _telemetry(self, duration, cost, confidence=1.0):
        return {"metrics": {"duration_seconds": duration,
                            "calculated_cost_usd": cost,
                            "confidence_rating": confidence}}

    def test_no_alerts_within_thresholds(self, registry):
        alerts = registry.record(self._telemetry(1.0, 0.001))
        assert alerts == []
        assert registry.avg_latency == 1.0

    def test_latency_alert(self, registry):
        registry.record(self._telemetry(4.0, 0.001))
        assert any(a["metric"] == "agent_latency_seconds" for a in registry.alerts)

    def test_cost_alert(self, registry):
        registry.record(self._telemetry(1.0, 0.10))
        assert any(a["metric"] == "agent_cost_per_task" for a in registry.alerts)

    def test_validation_failure_rate_alert(self, registry):
        for _ in range(50):
            registry.record(self._telemetry(1.0, 0.001, confidence=0.0))
        assert registry.validation_failure_rate == 1.0
        assert any(a["metric"] == "validation_failure_rate" for a in registry.alerts)
