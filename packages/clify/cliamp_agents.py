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

_DJ_TERMS = {"dj", "deck", "blend", "crossfade", "mix", "pitch", "nudge", "sync", "cue", "loop", "record", "brake", "turntable"}

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


_DJ_MANIFEST_PATH = os.path.join(
    os.path.dirname(os.path.abspath(__file__)), "agent_manifest.dj.json"
)


class DjAgent(BaseAgent):
    """Scope-gated DJ controller with one deterministic command per request."""

    def __init__(self, identity="dj_agent", tools=None, manifest=None,
                 manifest_path=None):
        manifest = manifest if manifest is not None else load_manifest(
            manifest_path or _DJ_MANIFEST_PATH
        )
        if tools is None:
            from cliamp_controller import CliampController
            tools = [CliampController()]
        super().__init__(identity, tools, manifest["allowed_scopes"],
                         manifest["prohibited_scopes"], manifest=manifest)
        self.controller = self.tools[0]
        # DJ commands are intentionally terse (often just "sync deck B" or
        # "crossfade 0.75"); retain the deterministic keyword gate while
        # allowing those short commands through the semantic check.
        self.guard.SIMILARITY_THRESHOLD = 0.15

    @staticmethod
    def _command(instruction):
        text = instruction.lower()
        if "status" in text:
            return "dj_status", ()
        patterns = [
            (r"\b(blend|crossfade|mix)\b.*?([0-9]+(?:\.[0-9]+)?)", "dj_crossfade", 1),
            (r"\b(sync)\b.*?\b(deck\s*[ab]|[01])\b", "dj_sync", 1),
            (r"\b(pitch)\b.*?\b(deck\s*[ab]|[01])\b.*?([0-9.]+)", "dj_pitch", 2),
            (r"\b(cue)\b.*?\b(deck\s*[ab]|[01])\b.*?([0-9.]+)", "dj_cue", 2),
            (r"\b(record)\b.*?\b(start|stop)\b", "dj_record", 1),
        ]
        matches = []
        for pattern, method, arg_count in patterns:
            match = re.search(pattern, text)
            if match:
                values = list(match.groups())[-arg_count:]
                values = [v[-1] if v.startswith("deck") else v for v in values]
                matches.append((method, tuple(values)))
        if not matches and re.search(r"\b(blend|crossfade|mix)\b", text):
            # A qualitative blend request uses a conservative one-second
            # crossfade when no explicit duration/position was supplied.
            matches.append(("dj_crossfade", ("0.5",)))
        if len(matches) != 1:
            raise ToolError("ambiguous or unroutable DJ instruction", retry_allowed=False)
        return matches[0]

    def _route_tool(self, instruction):
        method, args = self._command(instruction)
        return getattr(self.controller, method)(*args)
