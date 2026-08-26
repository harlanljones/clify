# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- DJ mode foundation in `cliamp-clify/player`: optional `DJEngine`, dual-deck
  state, ramped faders, equal-power crossfade curves, transition state, BPM
  estimation, and confidence-gated sync.
- Python `DjAgent`, `agent_manifest.dj.json`, and DJ controller methods for the
  future `dj.*` command surface.
- Help screen (`?` / `Ctrl+K`) redesign in `cliamp-clify/ui/model`: bindings are
  now grouped into labeled sections (Playback, Navigation, Playlist & Queue,
  Providers & Source, Search & Filter, EQ & Visuals, General) with the
  fork-only DJ engine keys under a dedicated **DJ mode (clify fork)** section.
  Key pills are column-aligned and `/` filtering is now a fuzzy, highlighted
  search with a live match count.

### Documentation

- Added [`docs/dj.md`](docs/dj.md) with implementation status, boundaries, and
  build/test instructions.
- Updated [`docs/keybindings.md`](docs/keybindings.md) to describe the
  categorized help screen and the clify-fork section.

## [1.7.0] - 2026-08-22

### Added

- **Turborepo Monorepo Architecture:** Converted the repository into a high-performance
  polyglot Turborepo monorepo with `pnpm` workspaces, unifying `cliamp-clify` (Go)
  and `clify` (Python) with cached build, test, and check pipelines (`pnpm build`,
  `pnpm test`, `pnpm check`).
- **Dual-Product Branding & Documentation:** Repositioned the repository around its two
  primary complementary products: `cliamp-clify` (retro terminal music player with
  native Spotify mixes & recently played) and `clify` CLI (extended Spotify querying,
  natural-language playback agents, and ADD framework).
- Native `cliamp-clify` fork integration with a merged Recently Played section,
  30-second cache, independent source degradation, and normal playlist loading.
- Versioned `cliamp.history.unified/1` IPC/CLI contract. clify prefers this
  capability and remains compatible with stock cliamp through its existing
  direct merge fallback.
- `SpotifyClient.get_generated_playlists()` and
  `SpotifyClient.get_playlist_tracks()` surfacing Spotify's algorithmic
  playlists (Daily Mix, Discover Weekly, Release Radar, On Repeat, Repeat
  Rewind, Daylist) with a `kind` classification, a documented non-error
  payload for playlists made unbrowsable by Spotify's November 2024 Web API
  restriction, and recorded contract tests pinning those wire shapes ahead of
  the upcoming Made For You section. Schemas are pinned in
  [docs/spotify_schemas.md](docs/spotify_schemas.md).


## [1.6.0] - 2026-08-21

### Added

- A standard-library `SpotifyClient` with refresh-token OAuth, one-time
  401/429 retries, pagination, structured errors, credential redaction, and a
  30-second playlist cache.
- `clify spotify login`, using Authorization Code with PKCE, OAuth state
  validation, a loopback callback, and mode-0600 refresh-token storage. No
  Client Secret is required or stored.
- `UnifiedLibraryClient` and a read-only agent that merge and deduplicate
  local/Spotify history, preserve Recently Played → Library → Your Playlists
  ordering, and isolate provider failures.
- The installed `clify` CLI with `library`, `recent`, `play`, and `status`
  commands plus machine-readable JSON output.
- Recorded Spotify contracts, cross-version CI, and provider-degradation
  monitoring.

### Documentation

- Recorded the cliamp Lua-plugin feasibility decision: plugins cannot extend
  the built-in Spotify browser layout, so `clify library` is the supported
  unified presentation surface.

## [1.5.0] - 2026-08-21

Minor release adding [cliamp](https://github.com/bjarneo/cliamp) integration
per [ROADMAP.md](ROADMAP.md) Phases 0–4. No breaking changes to the existing
Spotify agent or public APIs.

### Added

- `cliamp_client.py` — `CliampClient`, a thin injectable subprocess wrapper
  around cliamp's `--json` subcommands (`playlist show`, `playlist list`,
  `history`, `status`), with a structured `ToolError` hierarchy per §4.2
  (binary missing, non-zero exit, timeout, parse errors, daemon down).
- `cliamp_agents.py` — `CliampQueryAgent`, a read-only cliamp agent
  (playlists, listening history, player status) scoped by
  `agent_manifest.cliamp.json`.
- `cliamp_controller.py` — `CliampController`, the mutation-capable seam
  wrapping cliamp IPC verbs (`play`, `pause`, `toggle`, `stop`, `next`,
  `prev`, `volume`, `seek`, `queue`, `load`, `shuffle`, `repeat`) with a
  fail-fast daemon precondition (no auto-spawn).
- `cliamp_playback.py` — `CliampPlaybackAgent`, the only agent authorized
  for `playback.control`, with unambiguous verb routing and a post-command
  status verification loop capped at the §3.3 max iteration depth.
- Multi-agent scope-based routing in `Orchestrator` (accepts a list of
  agents; single-agent usage unchanged).
- `docs/cliamp_schemas.md` — pinned cliamp JSON schemas (v1.63.2) and the
  subprocess failure-mode contract.
- `tests/test_cliamp_contract.py` — contract tests pinning cliamp's JSON
  schemas (fail loudly on upstream drift) plus §5 alert verification under
  fault injection.
- `tests/test_cliamp_integration.py` — live-daemon integration tests,
  marked `@pytest.mark.integration` and skipped unless `CLIAMP_INTEGRATION`
  is set.
- README section "Using clify with cliamp" with quickstart.

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

[Unreleased]: https://github.com/harlanljones/clify/compare/v1.7.0...HEAD
[1.7.0]: https://github.com/harlanljones/clify/compare/v1.6.0...v1.7.0
[1.6.0]: https://github.com/harlanljones/clify/releases/tag/v1.6.0
[1.5.0]: https://github.com/harlanljones/clify/releases/tag/v1.5.0
[1.4.2]: https://github.com/harlanljones/clify/releases/tag/v1.4.2
