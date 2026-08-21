"""ADD Framework - Agent Boundary Core.

Implements AGENTS.md: Scope Guard (§2.2), Execution Unit (§4.2),
Metric Evaluator (§3).
"""

import time

# System constants & SLAs (§3)
LATENCY_SLA_LIMIT = 2.5          # seconds
COST_SLA_LIMIT = 0.02            # USD per execution loop
ACCURACY_MIN_THRESHOLD = 0.90
MAX_ITERATION_DEPTH = 3
CONTEXT_WINDOW_SIZE = 128_000    # tokens
CONTEXT_OVERFLOW_RATIO = 0.85    # intercept above 85% of the window

COST_PER_TOKEN_USD = 0.0000001
BASE_LOOP_TOKENS = 800

# Prohibited action verbs -> prohibited manifest scope (§2.2 keyword blocking).
PROHIBITED_TOKENS = {
    "skip": "playback.control",
    "pause": "playback.control",
    "play": "playback.control",
    "queue": "playback.control",
    "volume": "playback.control",
    "shuffle": "playback.control",
    "clear": "playback.control",
    "billing": "user.billing",
    "payment": "user.billing",
    "subscribe": "user.billing",
}

# Allowed entity tokens -> allowed manifest scope.
ALLOWED_TOKENS = {
    "library": "library.read",
    "saved": "library.read",
    "playlist": "playlists.read",
    "playlists": "playlists.read",
    "history": "history.read",
    "recently": "history.read",
    "played": "history.read",
}

_SCOPE_REFERENCE = "fetch list retrieve library playlists recently played history data"


class ScopeGuard:
    """Deterministic task gating before the LLM reasoning loop (§2.2)."""

    SIMILARITY_THRESHOLD = 0.30  # bag-of-words cosine; prod uses embeddings @ 0.82

    def __init__(self, allowed_scopes, prohibited_scopes,
                 allowed_tokens=None, prohibited_tokens=None,
                 scope_reference=None):
        self.allowed_scopes = set(allowed_scopes)
        self.prohibited_scopes = set(prohibited_scopes)
        # Per-agent keyword tables; default to module globals for
        # backward compatibility with existing manifests.
        self.allowed_tokens = dict(ALLOWED_TOKENS if allowed_tokens is None
                                   else allowed_tokens)
        self.prohibited_tokens = dict(PROHIBITED_TOKENS
                                      if prohibited_tokens is None
                                      else prohibited_tokens)
        # Pluggable similarity reference (default: legacy constant).
        self.scope_reference = scope_reference or _SCOPE_REFERENCE

    @staticmethod
    def _tokenize(text):
        return {t.strip(".,!?\"'").lower() for t in text.split()}

    @staticmethod
    def _cosine_similarity(tokens_a, tokens_b):
        vocab = tokens_a | tokens_b
        dot = sum(1 for t in vocab if t in tokens_a and t in tokens_b)
        norm = (len(tokens_a) ** 0.5) * (len(tokens_b) ** 0.5)
        return dot / norm if norm else 0.0

    def is_authorized(self, task: str, similarity_text: str = None) -> bool:
        tokens = self._tokenize(task)
        # 1) Keyword/token whitelisting: block lateral movement into
        #    prohibited operational zones.
        for token in tokens:
            scope = self.prohibited_tokens.get(token)
            if scope and scope in self.prohibited_scopes:
                return False
        # 2) Must touch at least one allowed scope entity.
        hits_allowed = any(
            self.allowed_tokens.get(t) in self.allowed_scopes for t in tokens
        )
        # 3) Semantic similarity gate against descriptive scope embedding.
        # Callers may pass `similarity_text` to score a stripped-down form of
        # the instruction (e.g. dropping a free-form argument like a playlist
        # name) so open-vocabulary arguments don't dilute the match.
        similarity = self._cosine_similarity(
            self._tokenize(similarity_text if similarity_text is not None else task),
            self._tokenize(self.scope_reference))
        return hits_allowed and similarity >= self.SIMILARITY_THRESHOLD


class ToolError(Exception):
    """Raised when an underlying tool call fails."""


class MaxIterationExceeded(Exception):
    """Raised when self-correction loops breach MAX_ITERATION_DEPTH."""


class BaseAgent:
    """Agent Boundary Core: Scope Guard + Execution Unit + Metric Evaluator."""

    def __init__(self, identity, tools, configured_scopes,
                 prohibited_scopes=None, max_iterations=MAX_ITERATION_DEPTH,
                 manifest=None):
        self.identity = identity
        self.tools = list(tools)
        self.configured_scopes = list(configured_scopes)
        manifest = manifest or {}
        self.manifest = dict(manifest)
        token_tables = manifest.get("token_tables", {})
        self.guard = ScopeGuard(
            configured_scopes,
            prohibited_scopes or [],
            allowed_tokens=token_tables.get("allowed_tokens"),
            prohibited_tokens=token_tables.get("prohibited_tokens"),
            scope_reference=manifest.get("description"),
        )
        self.max_iterations = max_iterations

    def is_task_authorized(self, task: str) -> bool:
        return self.guard.is_authorized(task)

    def _route_tool(self, instruction: str):
        raise NotImplementedError

    def process_instruction(self, instruction: str) -> dict:
        if not self.is_task_authorized(instruction):
            return {"status": "REJECTED", "reason": "OUT_OF_SCOPE", "data": []}
        try:
            data = self._route_tool(instruction)
        except Exception:
            # Fail safely without polluting downstream workflows (§4.2).
            return {"status": "TOOL_ERROR", "retry_allowed": True}
        return {"status": "SUCCESS", "data": data}

    def execute_monitored_loop(self, instruction: str) -> dict:
        """Runs the execution unit while capturing SLA telemetry (§3)."""
        start = time.time()
        iterations = 0
        response = None
        while iterations < self.max_iterations:
            iterations += 1
            response = self.process_instruction(instruction)
            if response.get("status") != "TOOL_ERROR" or not response.get("retry_allowed"):
                break
        else:
            raise MaxIterationExceeded(
                f"{self.identity} breached max iteration depth ({self.max_iterations})"
            )

        duration = time.time() - start
        tokens_used = BASE_LOOP_TOKENS + iterations * 50
        cost = tokens_used * COST_PER_TOKEN_USD
        confidence = 1.0 if response["status"] == "SUCCESS" else 0.0

        return {
            "agent": self.identity,
            "response": response,
            "metrics": {
                "duration_seconds": duration,
                "calculated_cost_usd": cost,
                "confidence_rating": confidence,
                "iterations": iterations,
                "tokens_used": tokens_used,
                "sla_compliant": (
                    duration <= LATENCY_SLA_LIMIT
                    and cost <= COST_SLA_LIMIT
                    and confidence >= ACCURACY_MIN_THRESHOLD
                ),
            },
        }


class LibrarySyncAgent(BaseAgent):
    """Read-only Spotify library agent (manifest: spotify_query_agent)."""

    def __init__(self, identity, tools, configured_scopes):
        super().__init__(
            identity=identity,
            tools=tools,
            configured_scopes=configured_scopes,
            prohibited_scopes=["playback.control", "user.billing"],
        )
        self.spotify = self.tools[0]

    def _route_tool(self, instruction: str) -> list:
        tokens = ScopeGuard._tokenize(instruction)
        if tokens & {"playlist", "playlists", "library", "saved"}:
            payload = self.spotify.get_user_playlists()
            return payload.get("items", [])
        if tokens & {"history", "recently", "played"}:
            payload = self.spotify.get_recently_played()
            return payload.get("items", [])
        raise ToolError(f"No tool route matched instruction: {instruction!r}")
