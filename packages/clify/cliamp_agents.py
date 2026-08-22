"""CliampQueryAgent — read-only cliamp agent (ROADMAP Phase 2).

Loads its scope contract from ``agent_manifest.cliamp.json`` (§2.1). The
manifest's ``token_tables`` and ``description`` feed the ``ScopeGuard``
per-agent keyword gating and similarity reference (§2.2). All cliamp
interaction goes through the injected ``CliampClient`` (Phase 1 seam).
"""

import json
import os
import re

from core_agents import BaseAgent, ScopeGuard, ToolError

_MANIFEST_PATH = os.path.join(
    os.path.dirname(os.path.abspath(__file__)), "agent_manifest.cliamp.json"
)

_PLAYLIST_TERMS = {"playlist", "playlists", "library"}
_HISTORY_TERMS = {"history", "recently", "played"}
_STATUS_TERMS = {"status", "playing", "current"}

_QUOTED_NAME_RE = re.compile(r"[\"']([^\"']+)[\"']")


def load_manifest(path=None):
    """Reads the cliamp_query_agent manifest (§2.1 schema)."""
    with open(path or _MANIFEST_PATH, "r", encoding="utf-8") as fh:
        return json.load(fh)


class CliampQueryAgent(BaseAgent):
    """Read-only cliamp agent: playlists, listening history, player status."""

    def __init__(self, identity="cliamp_query_agent", tools=None,
                 manifest=None, manifest_path=None):
        manifest = manifest if manifest is not None else load_manifest(manifest_path)
        if tools is None:
            from cliamp_client import CliampClient
            tools = [CliampClient()]
        super().__init__(
            identity=identity,
            tools=tools,
            configured_scopes=manifest["allowed_scopes"],
            prohibited_scopes=manifest["prohibited_scopes"],
            manifest=manifest,
        )
        self.client = self.tools[0]

    @staticmethod
    def _extract_quoted_name(instruction: str):
        match = _QUOTED_NAME_RE.search(instruction)
        return match.group(1) if match else None

    def _route_tool(self, instruction: str):
        tokens = ScopeGuard._tokenize(instruction)
        if tokens & _PLAYLIST_TERMS:
            name = self._extract_quoted_name(instruction)
            if name:
                return self.client.get_playlist(name)
            return self.client.get_playlists()
        if tokens & _HISTORY_TERMS:
            return self.client.get_recently_played()
        if tokens & _STATUS_TERMS:
            return self.client.get_status()
        raise ToolError(f"No tool route matched instruction: {instruction!r}")
