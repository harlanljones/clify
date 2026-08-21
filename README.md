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

## Using clify with cliamp

As of v1.5.0, clify ships a set of agents for
[cliamp](https://github.com/bjarneo/cliamp) (retro terminal music player),
integrated through a thin subprocess wrapper per the transport decision in
[ROADMAP.md](ROADMAP.md) Phase 0 and the schemas in
[docs/cliamp_schemas.md](docs/cliamp_schemas.md).

Prerequisites:

1. Install cliamp ≥ v1.63.2 so the `cliamp` binary is on `PATH`.
2. Start the daemon yourself — the framework is fail-fast and never
   auto-spawns it (playback mutations return
   `{"status": "TOOL_ERROR", "retry_allowed": false}` when no daemon runs):

   ```bash
   cliamp --daemon &
   ```

Quickstart:

```python
from cliamp_agents import CliampQueryAgent       # read-only: playlists, history, status
from cliamp_playback import CliampPlaybackAgent  # the only playback.control agent
from orchestrator import Orchestrator
from monitoring import TelemetryRegistry

query = CliampQueryAgent()        # tools default to CliampClient()
playback = CliampPlaybackAgent()  # tools default to CliampController()
orchestrator = Orchestrator([playback, query])  # scope-based routing
registry = TelemetryRegistry()

t = orchestrator.run("what's currently playing?")
registry.record(t)
print(t["response"])                    # {'status': 'SUCCESS', 'data': [...]}
print(t["metrics"]["sla_compliant"])    # True

t = orchestrator.run("pause the music")  # routed to cliamp_playback_agent
print(t["response"]["data"][0]["state"])  # 'paused' (post-command verification)
```

Read-only instructions (`playlist`, `history`, `status`) are routed to
`cliamp_query_agent`; unambiguous playback verbs (`pause`, `play`, `skip`,
`volume`, …) to `cliamp_playback_agent`. Destructive or ambiguous mutations
are rejected at the scope boundary before any subprocess runs. To run the
live integration tests, set `CLIAMP_INTEGRATION=1` (see the docstring in
`tests/test_cliamp_integration.py`).

## Unified library CLI

v1.6.0 adds a real Spotify Web API client and a provider-independent command
surface. Create a Spotify Developer app with
`http://127.0.0.1:8888/callback`, select Web API, and run the PKCE login:

```bash
pip install -e .
clify spotify login
```

The command asks only for the public Client ID, opens Spotify authorization,
and saves the refresh token to `~/.config/clify/spotify.json` with mode 0600.
It never needs or stores the Client Secret. The token requests only
`playlist-read-private` and `user-read-recently-played`. Credentials are
redacted from errors and telemetry. Then use:

```bash
clify library                 # Recently Played, Library, Your Playlists
clify library --json          # same fixed order, machine-readable
clify recent --limit 20
clify play "toggle playback"
clify status
```

The library command merges local cliamp history with Spotify history, sorts
by `played_at`, tags each item with its source, and deduplicates the same
track across providers. A provider outage returns healthy data with
`partial: true` and `failed_sources` metadata. Spotify response fields pinned
by contract tests are documented in
[docs/spotify_schemas.md](docs/spotify_schemas.md).

cliamp's Lua plugin API does not expose an API for modifying the built-in
Spotify browser tree, so clify cannot safely inject a passive Recently Played
sidebar section into the closed TUI. The supported unified surface is
`clify library`.

## Repository layout

| File | Spec Section | Purpose |
|---|---|---|
| `core_agents.py` | §2, §3, §4 | Agent Boundary Core: `ScopeGuard` (keyword whitelisting + similarity gating), `BaseAgent` (execution unit + SLA metric evaluator), `LibrarySyncAgent` (read-only Spotify agent) |
| `orchestrator.py` | §1, §4.2 | Task validation, context-overflow interception at 85% of the token window, telemetry-returning dispatch |
| `monitoring.py` | §5 | `TelemetryRegistry`: sliding-window latency, cost accumulator, validation failure rate with alerting thresholds |
| `agent_manifest.json` | §2.1 | Scope contract for `spotify_query_agent` |
| `tests/test_lifecycle.py` | §4.1, §4.2, §5 | Three-stage TDD lifecycle tests plus failure-mode and monitoring tests |
| `cliamp_client.py` | §4.2 | `CliampClient`: thin subprocess wrapper around `cliamp --json` commands with the structured `ToolError` hierarchy |
| `cliamp_agents.py` | §2.1, §2.2 | `CliampQueryAgent`: read-only cliamp agent (playlists, history, status) driven by `agent_manifest.cliamp.json` |
| `cliamp_controller.py` | §4.2 | `CliampController`: mutation-capable IPC verb wrapper with fail-fast daemon precondition |
| `cliamp_playback.py` | §2.2, §3.3 | `CliampPlaybackAgent`: the only `playback.control` agent, with unambiguous verb routing and a post-command verification loop |
| `agent_manifest.cliamp.json` / `agent_manifest.cliamp_playback.json` | §2.1 | Scope contracts for the cliamp agents |
| `spotify_client.py` | §4.2 | Real Spotify Web API client with refresh OAuth, pagination, rate-limit handling, and short-lived caching |
| `spotify_auth.py` | §4.2 | Interactive PKCE login with state validation, loopback callback, and mode-0600 credential storage |
| `unified_library.py` / `agent_manifest.cliamp_library.json` | §2.1, §4.2 | Cross-provider aggregation with fixed ordering and failure isolation |
| `clify_cli.py` | §1 | Installed CLI for unified library, recent history, playback, and status |
| `docs/cliamp_schemas.md` | §4 | Pinned cliamp JSON schemas (v1.63.2) and subprocess failure-mode contract |
| `docs/spotify_schemas.md` | §4 | Pinned Spotify response fields and OAuth/error contract |
| `tests/test_cliamp_contract.py` | §4, §5 | Contract tests pinning cliamp JSON schemas plus §5 alert verification under fault injection |
| `tests/test_cliamp_integration.py` | §4 | Live-daemon integration tests (`@pytest.mark.integration`, gated by `CLIAMP_INTEGRATION`) |

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
