# clify — Metric-Driven Agent-Driven Development (ADD) Framework

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Python 3.10+](https://img.shields.io/badge/python-3.10%2B-blue.svg)](pyproject.toml)

`clify` is a reference implementation of the Metric-Driven Agent-Driven
Development (ADD) technical specification in [AGENTS.md](AGENTS.md). It
decouples autonomous agents into specialized, single-responsibility units
whose control, guardrails, and evaluations are managed by a centralized
framework engine rather than the agents themselves.

## Features

- **Scope boundary enforcement (§2.2)** — deterministic pre-LLM gating via
  keyword/token whitelisting and semantic similarity scoring; prohibited
  operations (e.g. `playback.control`, `user.billing`) are rejected before
  any tool runs.
- **SLA telemetry (§3)** — every execution reports latency, cost, token
  usage, confidence, and SLA compliance. Targets: latency ≤ 2.5s,
  cost ≤ $0.02/execution, confidence ≥ 0.90.
- **Safe failure modes (§4.2)** — tool errors return
  `{"status": "TOOL_ERROR", "retry_allowed": true}`; self-correction is
  capped at 3 iterations; prompts breaching 85% of the context window are
  intercepted by the orchestrator.
- **Runtime monitoring (§5)** — a central time-series registry tracks
  sliding-window latency, cost per task, and validation failure rate with
  alerting thresholds and mitigating actions.
- **TDD lifecycle verification (§4)** — a three-stage (Red → Green →
  Refactor) pytest pipeline that an agent must pass before deployment.

## Installation

```bash
pip install clify
```

Or from source, using [uv](https://docs.astral.sh/uv/):

```bash
git clone https://github.com/harlan/clify.git
cd clify
uv venv .venv && uv pip install -e ".[test]" --python .venv/bin/python
```

Requires Python ≥ 3.10. The only runtime dependency is the standard
library; `pytest` is needed for the test suite.

## Quickstart

```python
from core_agents import LibrarySyncAgent
from orchestrator import Orchestrator
from monitoring import TelemetryRegistry

class SpotifyClient:  # substitute a real SDK client
    def get_user_playlists(self):
        return {"items": [{"name": "Synthwave Mix", "id": "4aV8"}], "total": 1}
    def get_recently_played(self):
        return {"items": [], "total": 0}

agent = LibrarySyncAgent(
    identity="LibrarySyncAgent",
    tools=[SpotifyClient()],
    configured_scopes=["library.read", "playlists.read"],
)

orchestrator = Orchestrator(agent)
registry = TelemetryRegistry()

telemetry = orchestrator.run("Fetch my playlists.")
registry.record(telemetry)

print(telemetry["response"])                  # {'status': 'SUCCESS', 'data': [...]}
print(telemetry["metrics"]["sla_compliant"])  # True
```

Out-of-scope tasks are rejected by the scope guard before any tool runs:

```python
agent.is_task_authorized("Skip this song and increase the volume.")  # False
```

## Repository layout

| File | Spec Section | Purpose |
|---|---|---|
| `core_agents.py` | §2, §3, §4 | Agent Boundary Core: `ScopeGuard` (keyword whitelisting + similarity gating), `BaseAgent` (execution unit + SLA metric evaluator), `LibrarySyncAgent` (read-only Spotify agent) |
| `orchestrator.py` | §1, §4.2 | Task validation, context-overflow interception at 85% of the token window, telemetry-returning dispatch |
| `monitoring.py` | §5 | `TelemetryRegistry`: sliding-window latency, cost accumulator, validation failure rate with alerting thresholds |
| `agent_manifest.json` | §2.1 | Scope contract for `spotify_query_agent` |
| `tests/test_lifecycle.py` | §4.1, §4.2, §5 | Three-stage TDD lifecycle tests plus failure-mode and monitoring tests |

## Running the tests

```bash
.venv/bin/python -m pytest -v
```

The suite implements the spec's TDD lifecycle verification pipeline:

1. **Stage 1 (scope boundary):** asserts in-scope tasks are authorized and
   prohibited tasks are rejected.
2. **Stage 2 (schema conformance):** asserts the correct tool is invoked
   exactly once and the structured payload contract holds.
3. **Stage 3 (SLA compliance):** asserts latency, cost, and confidence
   metrics stay within guardrails.

## Guardrails enforced

- **Scope boundary (§2.2):** prohibited verbs (skip/queue/volume/billing…)
  are rejected before any tool runs; tasks must hit an allowed scope entity
  and clear the similarity threshold.
- **Failure modes (§4.2):** tool errors return
  `{"status": "TOOL_ERROR", "retry_allowed": true}`; self-correction is
  capped at 3 iterations (`MaxIterationExceeded`).
- **Context overflow (§4.2):** the orchestrator intercepts prompts whose
  estimated token count exceeds 85% of the context window
  (`ContextOverflowError`).
- **SLAs (§3):** latency ≤ 2.5s, cost ≤ $0.02/execution, confidence ≥ 0.90 —
  reported in every `execute_monitored_loop` telemetry payload.
- **Monitoring alerts (§5):** sliding-window average latency > 3.0s, cost
  per task > $0.05, or validation failure rate > 2% raise alerts with the
  spec-defined mitigating actions.

## Publishing

```bash
uv pip install build --python .venv/bin/python
.venv/bin/python -m build
# then upload dist/* with twine or: uv publish
```

## Changelog

See [CHANGELOG.md](CHANGELOG.md).

## License

[MIT](LICENSE) © 2026 harlan
