# clify — Metric-Driven Agent-Driven Development & Spotify CLI

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Python 3.10+](https://img.shields.io/badge/python-3.10%2B-blue.svg)](https://pypi.org/project/clify/)
[![PyPI version](https://img.shields.io/pypi/v/clify.svg)](https://pypi.org/project/clify/)

`clify` is a Python CLI and reference implementation of the **Metric-Driven Agent-Driven Development (ADD)** framework specified in [AGENTS.md](https://github.com/harlan/clify/blob/main/AGENTS.md). It provides extended Spotify library aggregation, natural-language playback control, and deterministic SLA guardrails, designed as the companion CLI to [`cliamp-clify`](https://github.com/harlan/clify/tree/main/cliamp-clify).

---

## Features

- **Unified Music Library (`clify library`):** Merges local `cliamp` listening history and Spotify Web API data into a single structured view (`Recently Played` → `Library` → `Your Playlists` → `Made For You`).
- **Cross-Provider Recent History (`clify recent`):** Deduplicates tracks across providers with timestamped ordering and per-source failure isolation (`partial: true`).
- **Natural Language Playback (`clify play`):** Agent-orchestrated command routing with strict scope guardrails (§2.2) and post-action verification.
- **Headless Spotify PKCE Login (`clify spotify login`):** OAuth 2.0 PKCE with local loopback callback, mode-0600 storage, and automatic token refresh (no Client Secret needed).
- **Scope Boundary Enforcement (§2.2):** Pre-LLM gating via token whitelisting and semantic similarity scoring; unauthorized actions are rejected before executing any tools.
- **SLA Telemetry & Guardrails (§3, §5):** Latency ($\le 2.5s$), cost ($\le \$0.02$), confidence ($\ge 0.90$), context window overflow interception, and sliding-window health monitoring.

---

## Installation

From PyPI:
```bash
pip install clify
```

Or from the monorepo source:
```bash
git clone https://github.com/harlan/clify.git
cd clify/packages/clify
pip install -e ".[test]"
```

Requires Python $\ge 3.10$. The only runtime dependency is the Python standard library; `pytest` is required for testing.

---

## CLI Quickstart

1. **Authorize with Spotify:**
   ```bash
   clify spotify login --client-id <YOUR_SPOTIFY_CLIENT_ID>
   ```

2. **View unified library and history:**
   ```bash
   # Formatted text output
   clify library

   # Machine-readable JSON output
   clify library --json

   # Merged listening history
   clify recent --limit 25
   ```

3. **Control playback and inspect player state:**
   ```bash
   # Show current status
   clify status

   # Natural language playback instruction via ADD agent orchestrator
   clify play "pause the music"
   ```

---

## Python API Quickstart

```python
from cliamp_agents import CliampQueryAgent
from cliamp_playback import CliampPlaybackAgent
from orchestrator import Orchestrator
from monitoring import TelemetryRegistry

query_agent = CliampQueryAgent()        # read-only: playlists, history, status
playback_agent = CliampPlaybackAgent()  # playback.control agent
orchestrator = Orchestrator([playback_agent, query_agent])
registry = TelemetryRegistry()

# Execute monitored agent loop
telemetry = orchestrator.run("what is currently playing?")
registry.record(telemetry)

print(telemetry["response"])                  # {'status': 'SUCCESS', 'data': [...]}
print(telemetry["metrics"]["sla_compliant"])  # True
print(telemetry["metrics"]["duration_seconds"]) # e.g. 0.04
```

Out-of-scope tasks are rejected by the scope guard before any tool runs:
```python
query_agent.is_task_authorized("Skip track and delete account.")  # False
```

---

## Testing

Run the full test suite (200+ unit, contract, and lifecycle tests):

```bash
pytest
```

---

## License

[MIT](https://github.com/harlan/clify/blob/main/LICENSE) © 2026 harlan
