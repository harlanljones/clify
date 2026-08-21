# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.4.2] - 2026-08-21

Initial public release of the Metric-Driven Agent-Driven Development (ADD)
framework, implementing the full technical specification in
[AGENTS.md](AGENTS.md).

### Added

- `core_agents.py` — Agent Boundary Core (spec §2, §3, §4):
  - `ScopeGuard` with keyword/token whitelisting and semantic similarity
    gating (§2.2).
  - `BaseAgent` execution unit with SLA metric evaluator, tool-error safe
    failure mode, and max-iteration guard (§3.3, §4.2).
  - `LibrarySyncAgent`, a read-only Spotify agent conforming to the
    `spotify_query_agent` manifest contract.
- `orchestrator.py` — Orchestration Layer (spec §1, §4.2): task validation,
  context-overflow interception at 85% of the token window, and
  telemetry-returning dispatch.
- `monitoring.py` — Runtime Monitoring & Observability (spec §5):
  `TelemetryRegistry` with sliding-window latency tracking, cost accumulator,
  validation failure rate, and alerting thresholds with mitigating actions.
- `agent_manifest.json` — scope contract for `spotify_query_agent` (§2.1).
- `tests/test_lifecycle.py` — three-stage TDD lifecycle verification pipeline
  (§4.1) plus failure-mode and monitoring tests (§4.2, §5).
- Packaging metadata (`pyproject.toml`), MIT license, and this changelog.

[1.4.2]: https://github.com/harlan/clify/releases/tag/v1.4.2
