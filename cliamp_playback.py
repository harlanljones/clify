"""CliampPlaybackAgent — the only agent authorized for playback.control.

ROADMAP Phase 3. Loads agent_manifest.cliamp_playback.json (allowed_scopes
["playback.control"], prohibited ["playlists.write", "user.billing"]) into
BaseAgent's manifest-driven ScopeGuard.

Routing: only unambiguous instructions map to exactly one IPC command; a task
touching zero or two+ distinct verbs is rejected at the boundary. Destructive
instructions (history clear, playlist delete) are rejected by the prohibited
token table before routing.

Verification loop: after each mutation the post-command status payload is
checked against the expected state; on mismatch the mutation is retried up to
MAX_ITERATION_DEPTH, then MaxIterationExceeded is raised. confidence = 1.0
only on a confirmed state match (BaseAgent's metric evaluator: SUCCESS only).
"""

import json
import os
import re

from cliamp_client import ToolError
from cliamp_controller import CliampController
from core_agents import BaseAgent, MaxIterationExceeded, ScopeGuard

MANIFEST_PATH = os.path.join(
    os.path.dirname(os.path.abspath(__file__)), "agent_manifest.cliamp_playback.json"
)

_NUMERIC_RE = re.compile(r"^-?\d+(\.\d+)?$")

# token -> controller method name. Aliases (skip/next, prev/previous) collapse
# to one method so they stay unambiguous.
VERB_ROUTES = {
    "play": "play",
    "pause": "pause",
    "toggle": "toggle",
    "stop": "stop",
    "next": "next",
    "skip": "next",
    "prev": "prev",
    "previous": "prev",
    "volume": "volume",
    "seek": "seek",
    "queue": "queue",
    "load": "load",
    "shuffle": "shuffle",
    "repeat": "repeat",
}

# Verbs whose post-command confirmation checks status["state"].
EXPECTED_STATE = {"play": "playing", "pause": "paused", "stop": "stopped"}

# Verbs that take a single argument.
_ARG_VERBS = {"volume", "seek", "queue", "load"}

# Marker words after which the remainder of the instruction is the free-form
# argument (playlist/track name may contain spaces and punctuation, so we
# can't just take the last whitespace token).
_FREE_ARG_MARKERS = {"load": ("playlist",), "queue": ("song", "track", "file")}


class CliampPlaybackAgent(BaseAgent):
    """Mutation agent for cliamp playback (scope: playback.control)."""

    def __init__(self, identity="cliamp_playback_agent", tools=None,
                 manifest_path=MANIFEST_PATH, manifest=None, **kwargs):
        if manifest is None:
            with open(manifest_path) as fh:
                manifest = json.load(fh)
        tools = list(tools) if tools else [CliampController()]
        super().__init__(
            identity=identity,
            tools=tools,
            configured_scopes=manifest["allowed_scopes"],
            prohibited_scopes=manifest["prohibited_scopes"],
            manifest=manifest,
            **kwargs,
        )
        self.controller = self.tools[0]

    # -- boundary ------------------------------------------------------------

    def _matched_methods(self, task: str) -> set:
        tokens = ScopeGuard._tokenize(task)
        return {VERB_ROUTES[t] for t in tokens if t in VERB_ROUTES}

    def is_task_authorized(self, task: str) -> bool:
        """ScopeGuard gate + unambiguity: exactly one IPC command must match.

        For load/queue, the similarity gate is scored against the instruction
        template with the free-form argument (playlist/track name) stripped,
        since open-vocabulary names would otherwise dilute the bag-of-words
        match against the fixed scope-reference description.
        """
        methods = self._matched_methods(task)
        similarity_text = task
        if len(methods) == 1:
            (method,) = methods
            similarity_text = self._template_text(method, task)
        if not self.guard.is_authorized(task, similarity_text=similarity_text):
            return False
        return len(methods) == 1

    @staticmethod
    def _template_text(method: str, instruction: str) -> str:
        markers = _FREE_ARG_MARKERS.get(method)
        if not markers:
            return instruction
        words = instruction.split()
        lowered = [w.strip(".,!?\"'").lower() for w in words]
        for marker in markers:
            if marker in lowered:
                return " ".join(words[: lowered.index(marker) + 1])
        return instruction

    # -- routing + verification loop ------------------------------------------

    @staticmethod
    def _extract_arg(method: str, instruction: str) -> str:
        tokens = instruction.split()
        if method in ("volume", "seek"):
            for tok in tokens:
                if _NUMERIC_RE.match(tok):
                    return tok
            raise ToolError(
                f"{method} requires a numeric argument", retry_allowed=False
            )
        # queue/load: the target is everything after the marker word (e.g.
        # "playlist"/"song"), since playlist and track names may contain
        # spaces ("My Playlist #24"). Falls back to the last token if no
        # marker is present.
        markers = _FREE_ARG_MARKERS.get(method, ())
        lowered = [w.strip(".,!?\"'").lower() for w in tokens]
        for marker in markers:
            if marker in lowered:
                remainder = tokens[lowered.index(marker) + 1:]
                if remainder:
                    return " ".join(remainder)
        return tokens[-1]

    @staticmethod
    def _confirmed(method: str, payload: dict) -> bool:
        if not isinstance(payload, dict):
            return False
        if method in EXPECTED_STATE:
            return payload.get("ok", True) and (
                payload.get("state") == EXPECTED_STATE[method]
            )
        return bool(payload.get("ok", True))

    def _route_tool(self, instruction: str):
        methods = self._matched_methods(instruction)
        if len(methods) != 1:
            raise ToolError(
                f"ambiguous or unroutable playback instruction: {instruction!r}",
                retry_allowed=False,
            )
        method = methods.pop()
        arg = None
        if method in _ARG_VERBS:
            arg = self._extract_arg(method, instruction)

        # Verification loop: mutate, confirm post-command status, retry up to
        # MAX_ITERATION_DEPTH, then fail hard.
        for _attempt in range(self.max_iterations):
            payload = getattr(self.controller, method)(arg) if arg is not None \
                else getattr(self.controller, method)()
            if self._confirmed(method, payload):
                return payload
        raise MaxIterationExceeded(
            f"{self.identity}: post-command status never matched expected state "
            f"for {method!r} after {self.max_iterations} attempts"
        )

    # -- failure contract (§4.2) ----------------------------------------------

    def process_instruction(self, instruction: str) -> dict:
        if not self.is_task_authorized(instruction):
            return {"status": "REJECTED", "reason": "OUT_OF_SCOPE", "data": []}
        try:
            data = self._route_tool(instruction)
        except MaxIterationExceeded:
            raise
        except ToolError as exc:
            # Structured payload; daemon-down surfaces retry_allowed=False.
            return exc.payload
        except Exception:
            return {"status": "TOOL_ERROR", "retry_allowed": True}
        return {"status": "SUCCESS", "data": [data]}
