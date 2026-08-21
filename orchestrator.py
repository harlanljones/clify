"""ADD Framework - Orchestration Layer.

1. Validate Task  ->  2. Dispatch to Agent Boundary Core  ->  3. Return
Output & Telemetry. Also intercepts context overflows before processing
when token counts breach 85% of the LLM's absolute window (§4.2).
"""

from core_agents import CONTEXT_OVERFLOW_RATIO, CONTEXT_WINDOW_SIZE

CHARS_PER_TOKEN = 4  # rough estimator; production uses a real tokenizer


class ContextOverflowError(Exception):
    """Raised when a prompt breaches the context-window guardrail."""


class Orchestrator:
    def __init__(self, agent, window_size=CONTEXT_WINDOW_SIZE):
        self.agent = agent
        self.window_size = window_size

    def estimate_tokens(self, text: str) -> int:
        return max(1, len(text) // CHARS_PER_TOKEN)

    def validate_task(self, task: str) -> None:
        """Pre-LLM gating: context overflow check then scope authorization."""
        if self.estimate_tokens(task) > self.window_size * CONTEXT_OVERFLOW_RATIO:
            raise ContextOverflowError(
                f"Prompt exceeds {CONTEXT_OVERFLOW_RATIO:.0%} of the "
                f"{self.window_size}-token context window."
            )
        if not self.agent.is_task_authorized(task):
            raise PermissionError(f"Task rejected by scope guard: {task!r}")

    def run(self, task: str) -> dict:
        self.validate_task(task)
        return self.agent.execute_monitored_loop(task)
