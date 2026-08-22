"""Tests for framework refactors (ROADMAP: Framework refactors).

1. Per-agent token tables (manifest-driven ALLOWED/PROHIBITED maps).
2. Pluggable similarity reference (manifest description).
3. Multi-agent orchestration with scope-based routing.
"""

import pytest
from unittest.mock import Mock

from core_agents import BaseAgent, LibrarySyncAgent, ScopeGuard, _SCOPE_REFERENCE
from orchestrator import Orchestrator


class DummyAgent(BaseAgent):
    """Minimal agent that echoes a marker for routing verification."""

    def __init__(self, identity, marker, **kwargs):
        super().__init__(identity=identity, tools=[], **kwargs)
        self.marker = marker

    def _route_tool(self, instruction):
        return [self.marker]


PLAYBACK_MANIFEST = {
    "description": "skip play pause volume control playback",
    "token_tables": {
        "allowed_tokens": {
            "skip": "playback.control",
            "play": "playback.control",
            "pause": "playback.control",
            "volume": "playback.control",
        },
        "prohibited_tokens": {},
    },
}


@pytest.fixture
def read_agent():
    sdk = Mock()
    sdk.get_user_playlists.return_value = {
        "items": [{"name": "Synthwave Mix", "id": "4aV8"}],
        "total": 1,
    }
    return LibrarySyncAgent(
        identity="LibrarySyncAgent",
        tools=[sdk],
        configured_scopes=["library.read", "playlists.read"],
    )


@pytest.fixture
def playback_agent():
    return DummyAgent(
        identity="PlaybackAgent",
        marker="playback-handled",
        configured_scopes=["playback.control"],
        manifest=PLAYBACK_MANIFEST,
    )


class TestPerAgentTokenTables:
    def test_custom_allowed_tokens_authorize(self):
        agent = DummyAgent(
            identity="WidgetAgent",
            marker="ok",
            configured_scopes=["widgets.read"],
            manifest={
                "description": "widget gadget thing",
                "token_tables": {
                    "allowed_tokens": {"widget": "widgets.read"},
                    "prohibited_tokens": {},
                },
            },
        )
        assert agent.is_task_authorized("show the widget") is True

    def test_custom_tables_replace_module_defaults(self):
        # "playlist" is in the module-level ALLOWED_TOKENS but must not
        # authorize an agent with a custom table lacking it.
        agent = DummyAgent(
            identity="WidgetAgent",
            marker="ok",
            configured_scopes=["widgets.read"],
            manifest={
                "description": "widget gadget playlist",
                "token_tables": {
                    "allowed_tokens": {"widget": "widgets.read"},
                    "prohibited_tokens": {},
                },
            },
        )
        assert agent.is_task_authorized("show the playlist") is False

    def test_custom_prohibited_tokens_block(self):
        agent = DummyAgent(
            identity="PlaybackAgent",
            marker="ok",
            configured_scopes=["playback.control"],
            prohibited_scopes=["user.billing"],
            manifest={
                "description": "skip play pause volume control playback billing",
                "token_tables": {
                    "allowed_tokens": {"skip": "playback.control",
                                       "play": "playback.control"},
                    "prohibited_tokens": {"billing": "user.billing"},
                },
            },
        )
        assert agent.is_task_authorized("skip play billing") is False

    def test_defaults_preserved_without_manifest(self):
        guard = ScopeGuard(["playlists.read"], ["playback.control"])
        assert guard.allowed_tokens == dict(
            __import__("core_agents").ALLOWED_TOKENS)
        assert guard.is_authorized("list my playlists") is True
        assert guard.is_authorized("skip this song") is False


class TestPluggableSimilarityReference:
    def test_manifest_description_used_as_reference(self, playback_agent):
        assert playback_agent.guard.scope_reference == PLAYBACK_MANIFEST["description"]
        assert playback_agent.is_task_authorized("skip play pause") is True

    def test_default_reference_without_manifest(self):
        guard = ScopeGuard(["playlists.read"], [])
        assert guard.scope_reference == _SCOPE_REFERENCE


class TestMultiAgentOrchestration:
    def test_routes_read_task_to_read_agent(self, read_agent, playback_agent):
        orch = Orchestrator([read_agent, playback_agent])
        result = orch.run("list my playlists")
        assert result["agent"] == "LibrarySyncAgent"
        assert result["response"]["status"] == "SUCCESS"

    def test_routes_playback_task_to_playback_agent(self, read_agent, playback_agent):
        orch = Orchestrator([read_agent, playback_agent])
        result = orch.run("skip play pause")
        assert result["agent"] == "PlaybackAgent"
        assert result["response"]["data"] == ["playback-handled"]

    def test_raises_when_no_agent_authorizes(self, read_agent, playback_agent):
        orch = Orchestrator([read_agent, playback_agent])
        with pytest.raises(PermissionError):
            orch.run("unrelated gibberish zzz qqq")

    def test_single_agent_backward_compat(self, read_agent):
        orch = Orchestrator(read_agent)
        assert orch.agent is read_agent
        result = orch.run("list my playlists")
        assert result["agent"] == "LibrarySyncAgent"
        assert result["response"]["status"] == "SUCCESS"
        with pytest.raises(PermissionError):
            orch.run("skip this song")
