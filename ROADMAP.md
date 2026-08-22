# Development Roadmap — clify Monorepo (`cliamp-clify` & `clify` CLI)

Agent-driven development plan for the `clify` Turborepo monorepo, uniting
[cliamp-clify](cliamp-clify/) (retro terminal music player fork with Spotify superpowers)
and [clify](packages/clify/) (companion CLI and Metric-Driven ADD framework). Every phase
follows the three-stage TDD lifecycle mandated by [AGENTS.md](AGENTS.md) §4
(Red → Green → Refactor), and every shipped agent and binary must pass the full
lifecycle verification pipeline before it is considered deployed.

**Status legend:** 🔲 not started · 🚧 in progress · ✅ done

---

## Monorepo Architecture Milestone — Turborepo Integration ✅

> **Done:** Repository transformed into a polyglot Turborepo monorepo managed by
> `pnpm` and `turbo`.
> - Root orchestration: `turbo.json`, `package.json`, `pnpm-workspace.yaml`.
> - Workspaces: `cliamp-clify` (Go 1.26+) and `packages/clify` (Python 3.10+).
> - Unified build & test pipelines: `pnpm build`, `pnpm test`, `pnpm check` running
>   Go and Python suites concurrently with full build artifact caching.


---

## Phase 0 — Discovery & Feasibility ✅

> **Decisions recorded (v1.63.2, `/usr/bin/cliamp`):**
> - **Transport:** subprocess + `--json` (stateless, recommended). No socket
>   protocol reimplementation; every call is an isolated
>   `subprocess.run(..., capture_output=True, timeout=2.0)`.
> - **Daemon lifecycle:** **fail-fast over auto-spawn.** The framework never
>   spawns `cliamp --daemon` itself; daemon-down surfaces as a structured
>   `TOOL_ERROR` (`retry_allowed: false`) with operator remediation.
> - Findings: schemas in [docs/cliamp_schemas.md](docs/cliamp_schemas.md);
>   spike in [spikes/cliamp_spike.py](spikes/cliamp_spike.py) (PASS).
>   ⚠️ `playlist list` has **no `--json` flag** — wrapper parses text output.
>   ⚠️ `playlist show` with no name exits **0** with a usage line.

Validate assumptions about cliamp's scriptable surface before writing
framework code. No agent changes; produces the grounding document all
later phases reference.

**cliamp capabilities to confirm (from upstream docs):**

| Surface | Mechanism | Machine-readable? |
|---|---|---|
| Playlist queries | `cliamp playlist show "Name" --json` | ✅ JSON |
| Listening history | `cliamp history --json` | ✅ JSON |
| Player state | `cliamp status --json` | ✅ JSON |
| Playback control | IPC: `cliamp play / pause / next / prev / volume / seek / queue / load …` | exit codes |
| Headless operation | `cliamp --daemon` (no TUI, IPC over Unix socket) | ✅ |

**Tasks**

- [x] Pin a minimum cliamp version and record the exact `--json` schemas
      for `playlist show`, `history`, and `status` in
      `docs/cliamp_schemas.md`. ✅ (v1.63.2)
- [x] Document failure modes of the subprocess boundary: non-zero exit
      codes, empty stdout, "no daemon running" errors, binary missing from
      `PATH`. ✅ (docs/cliamp_schemas.md §5)
- [x] Decide transport: **subprocess + `--json`** (recommended, stateless)
      vs. direct Unix-socket IPC (faster, but reimplements the protocol).
      Record the decision and rationale in this file. ✅ **subprocess + `--json`**
- [x] Confirm `cliamp --daemon` lifecycle management story (systemd user
      unit? framework-spawned child?) — needed by Phase 3. ✅ **fail-fast;
      daemon is operator-managed (e.g. systemd user unit), never
      framework-spawned**

**Exit criteria:** schemas documented; transport decision recorded; a
one-page spike script can round-trip `playlist show --json` and
`history --json` on a real cliamp install.

---

## Phase 1 — `CliampClient` Tool Wrapper (Read-Only) ✅

> **Done:** `cliamp_client.py` + `tests/test_cliamp_client.py` (20 tests,
> all green). Structured `ToolError` hierarchy per §4.2; typed dataclasses
> (`Track`, `Playlist`, `HistoryEntry`, `PlayerStatus`) in the refactor pass.

*Spec refs: §2.1 (`required_tools`), §4.2 (tool error contract).*

A thin, injectable Python tool class — the analogue of `SpotifyClient` in
the existing manifest — that wraps cliamp's JSON-producing subcommands.
This is the single seam every agent will use; nothing else in the
framework shells out directly.

**Design**

```python
class CliampClient:
    def get_playlists(self) -> list[str]: ...      # cliamp playlist list
    def get_playlist(self, name: str) -> dict: ... # cliamp playlist show NAME --json
    def get_recently_played(self, limit=50) -> dict: ...  # cliamp history --json
    def get_status(self) -> dict: ...              # cliamp status --json
```

- All methods raise `ToolError`-compatible exceptions on non-zero exit,
  malformed JSON, or missing binary (§4.2 safe failure).
- Pure subprocess boundary → trivially mockable in tests, same as the
  current `mock_spotify_sdk` fixture pattern.

**TDD lifecycle**

- 🔴 **Red:** ✅ unit tests asserting (a) exact subprocess argv construction,
  (b) JSON parsing into documented schemas, (c) `RuntimeError` → structured
  `TOOL_ERROR` propagation, (d) missing-binary handling.
- 🟢 **Green:** ✅ implemented with `subprocess.run(..., capture_output=True,
  timeout=2.0)` — the timeout keeps the latency SLA (≤ 2.5s) achievable.
- ♻️ **Refactor:** ✅ parsed into typed dataclasses; SLA metrics intact.

**Exit criteria:** `CliampClient` is a drop-in tool for `BaseAgent`;
coverage of all four read methods; zero network access (local binary only).

---

## Phase 2 — `cliamp_query_agent` (Read-Only) ✅

*Spec refs: §2 (scope contract), §3 (SLAs), §4.1 (three-stage pipeline).*

The local-music analogue of the existing `LibrarySyncAgent`. First-class
proof that the framework generalizes beyond Spotify.

**Manifest (`agent_manifest.cliamp.json`):**

```json
{
  "agent_id": "cliamp_query_agent",
  "version": "1.0.0",
  "description": "Queries local cliamp playlists, listening history, and player status.",
  "allowed_scopes": ["playlists.read", "history.read", "player.status.read"],
  "prohibited_scopes": ["playback.control", "playlists.write", "player.config"],
  "required_tools": ["CliampClient"]
}
```

**Tasks**

- [x] Register the manifest; keep `spotify_query_agent` untouched. ✅
- [x] Refactor `ScopeGuard` to load token tables **per-agent from the
      manifest** instead of module-level globals (see "Framework
      refactors" below). Player-specific terms (`queue`, `seek`, `daemon`)
      map to prohibited scopes here. ✅ (`token_tables` in
      `agent_manifest.cliamp.json`; manifest `description` doubles as the
      ScopeGuard similarity reference)
- [x] Implement `CliampQueryAgent(BaseAgent)` with `_route_tool` dispatch:
      playlist/library terms → `get_playlist(s)`, history/recently/played →
      `get_recently_played`, status/playing/current → `get_status`. ✅
      (`cliamp_agents.py`; quoted names route to `get_playlist(name)`)
- [x] TDD pipeline mirroring `tests/test_lifecycle.py`: ✅
      (`tests/test_cliamp_query_agent.py`, 6/6 green; full suite 47 passed)
      1. **Stage 1:** in-scope ("Show my cliamp playlists", "What did I
         listen to recently?") authorized; out-of-scope ("Skip this song",
         "Turn the volume up") rejected.
      2. **Stage 2:** schema conformance — correct tool invoked exactly
         once, `{"status": "SUCCESS", "data": [...]}` contract holds.
      3. **Stage 3:** SLA compliance under the subprocess boundary —
         latency ≤ 2.5s, cost ≤ $0.02, confidence ≥ 0.90.

**Exit criteria:** agent passes the full §4.1 pipeline; README layout table
gains a row; telemetry streams to `TelemetryRegistry` without alerts.

---

## Phase 3 — `cliamp_playback_agent` (Playback Control) ✅

> **Done:** `cliamp_controller.py` (CliampController, 12 IPC verbs, daemon
> precondition fail-fast), `cliamp_playback.py` (CliampPlaybackAgent,
> manifest-driven ScopeGuard, unambiguous routing, 3-attempt verification
> loop), `agent_manifest.cliamp_playback.json`, and
> `tests/test_cliamp_playback_agent.py` (53 tests, all green; full suite
> 100 green).

*Spec refs: §2.2 (mutation gating), §3.3 (max iteration depth), §5 (alerts).*

The framework's first **mutation-capable** agent — deliberately its own
unit so read-only consumers never inherit control authority. Highest-risk
phase; lands only after Phases 0–2 are green.

**Manifest excerpt:**

```json
{
  "agent_id": "cliamp_playback_agent",
  "allowed_scopes": ["playback.control"],
  "prohibited_scopes": ["playlists.write", "user.billing"],
  "required_tools": ["CliampController"]
}
```

**Design**

- [x] `CliampController` tool wrapping IPC verbs: `play`, `pause`, `toggle`,
      `stop`, `next`, `prev`, `volume <db>`, `seek <s>`, `queue <path>`,
      `load <playlist>`, `shuffle`, `repeat`. Each returns the post-command
      `status --json` payload for verification.
- [x] Daemon precondition: `_route_tool` verifies a running instance
      (`cliamp status` succeeds); if not, either fail with structured
      `TOOL_ERROR` (`retry_allowed: false`) or auto-spawn
      `cliamp --daemon --auto-play` per the Phase 0 lifecycle decision.
- [x] **Confirmation semantics:** destructive/ambiguous instructions
      (e.g. `history clear`, `playlist delete`) stay prohibited outright;
      playback verbs are authorized only when they map unambiguously to one
      IPC command.
- [x] Telemetry: mutation commands must still honor cost/latency SLAs;
      alerting thresholds in `TelemetryRegistry` unchanged.

**TDD lifecycle**

- 🔴 **Red:** boundary tests (playback verbs authorized **only** for this
      agent; `LibrarySyncAgent` and `cliamp_query_agent` still reject them);
      routing tests (each verb → exactly one IPC call); verification-loop
      tests (`MaxIterationExceeded` after 3 failed status confirmations).
- 🟢 **Green:** implement with mocked controller; then integration-test
      against a real `cliamp --daemon` in CI (marked
      `@pytest.mark.integration`, skippable via env var).
- ♻️ **Refactor:** tighten confidence scoring for mutations (require
      post-command state match for `confidence = 1.0`).

**Exit criteria:** "Skip this song", "Pause", and "Volume up" execute
end-to-end through `Orchestrator` with SLA-compliant telemetry, while the
same instructions remain rejected by both read-only agents.

---

## Phase 4 — Hardening & Release ✅

- [x] **Integration CI job:** install cliamp (Linux build + `pipewire-alsa`
      note for devs), launch `--daemon`, run the full suite against it.
      ✅ (`tests/test_cliamp_integration.py`, `@pytest.mark.integration`,
      gated by `CLIAMP_INTEGRATION`; setup documented in the module
      docstring)
- [x] **Contract tests:** pin cliamp's JSON schemas; fail loudly on
      upstream format drift (schema version field in
      `docs/cliamp_schemas.md`). ✅ (`tests/test_cliamp_contract.py`)
- [x] **Docs:** README section "Using clify with cliamp" with quickstart;
      CHANGELOG entry; bump to v1.5.0 (minor — new feature, no breaking
      changes to the Spotify agent). ✅
- [x] **Monitoring:** confirm all three §5 alerts fire correctly under a
      fault-injected cliamp (killed daemon → latency/tool-error alerts).
      ✅ (`TestSection5Alerts` in `tests/test_cliamp_contract.py`)

---

## Framework refactors unlocked along the way

These emerge from Phase 2/3 and should be tracked as their own tasks:

1. **Per-agent token tables** — move `ALLOWED_TOKENS`/`PROHIBITED_TOKENS`
   out of module globals into manifest-driven configuration so multiple
   agents with different vocabularies coexist cleanly.
2. **Pluggable similarity reference** — `_SCOPE_REFERENCE` is currently a
   Spotify-flavored constant; derive it from the manifest `description`.
3. **Multi-agent orchestration** — `Orchestrator` currently wraps one
   agent; add scope-based routing so "what's playing?" and "skip it" can be
   dispatched to `cliamp_query_agent` and `cliamp_playback_agent`
   respectively within one registry/monitored loop.

## Risks & open questions

| Risk | Mitigation |
|---|---|
| Upstream JSON schema drift between cliamp releases | Phase 4 contract tests; pin minimum version |
| Subprocess latency eats the 2.5s SLA | 2.0s subprocess timeout; measure in Stage 3 tests; consider direct socket transport if needed |
| Daemon lifecycle ownership ambiguity | Resolve in Phase 0; default to fail-fast over auto-spawn |
| Mutation agents raise `validation_failure_rate` | Tightened confidence scoring in Phase 3; watch §5 alerts in staging |

---

# Development Roadmap — Unified Library CLI (v1.6.0)

Follow-on initiative to the v1.5.0 cliamp integration above. Prompted by
live-usage findings: `CliampClient`/`CliampController` only ever see
**local** cliamp playlists (`~/.config/cliamp/playlists/*.toml`) and local
`cliamp history`. They have no visibility into Spotify (or any other
provider) content, even though the cliamp **TUI** already renders it —
screenshot reference: a "Spotify / Playlists" browser with `Library` and
`Your playlists` sections, no cross-provider "Recently Played" section.
Goal: give clify's CLI a unified, provider-agnostic view of "everything the
user can play," with **Recently Played listed above Library and Your
Playlists** — matching the requested section order.

**Status legend:** 🔲 not started · 🚧 in progress · ✅ done

**Two independent tracks, deliberately separated because they have
different failure domains:**

- **Track A (data):** clify learns to *see* all playlists + a merged
  recently-played feed, exposed through clify's own CLI/agents.
- **Track B (presentation):** whether that data can additionally be
  injected into cliamp's *own* TUI screen (the thing in the screenshot),
  which is a closed Go binary clify does not control.

---

## Phase 5 — Discovery: Lua plugin surface & provider inventory ✅

> **Decision (cliamp v1.63.2): Phase 9 is a no-go.** Lua plugins can
> register hooks, key bindings, callable commands, and visualizers, but no
> provider-browser sections or layout components. Provider inventory and
> history findings are recorded in
> [docs/cliamp_phase5_discovery.md](docs/cliamp_phase5_discovery.md).

Before committing to an architecture, establish what's actually possible.
`cliamp plugins` (list/install/trust/remove/call/commands) is the **only**
extension point into the running cliamp process/TUI that isn't plain
subprocess IPC — everything decided here gates Phase 9.

**Tasks**

- [x] Read cliamp's Lua plugin API docs/examples (`cliamp plugins list`,
      any bundled sample plugins). Determine: can a plugin register a new
      sidebar section in the Spotify/library browser tree, or only
      register invocable `commands` (action-style, not layout-style)? ✅
      **No layout API; hooks/commands/key bindings/visualizers only.**
- [x] Confirm which providers configured on this machine are actually
      queryable at all (`--provider` list from `cliamp --help`: radio,
      navidrome, plex, jellyfin, emby, **spotify**, qobuz, soundcloud,
      netease, yt, youtube, ytmusic) — start with Spotify since it's the
      one in active use. ✅ **Spotify is the only explicitly configured
      remote provider; Radio and Local are built in. Provider inventory is
      queryable through daemon JSON IPC, but v1.63.2 has no public CLI
      wrapper for those operations.**
- [x] Determine whether cliamp's `history --json` (currently empty on this
      machine) has any
      provider-scoped equivalent, or whether "recently played" must be
      reconstructed entirely from provider APIs (e.g. Spotify's own
      `/me/player/recently-played`) since cliamp doesn't track it. ✅
      **No provider-scoped history exists. The single local store records
      qualifying cliamp playback regardless of provider; Spotify's API is
      needed only to include plays outside cliamp (then deduplicate).**
- [x] Record findings + a go/no-go on Phase 9 (TUI injection) in this file.
      ✅ **No-go.**

**Exit criteria:** written answer to "can a Lua plugin alter the TUI's
Spotify browser layout, yes/no" with evidence; provider inventory table.

---

## Phase 6 — `SpotifyClient` tool (real, not a test mock) ✅

> **Done:** `spotify_client.py` uses the standard-library HTTP stack with
> refresh-token OAuth, pagination, one-time 401/429 retry paths, structured
> errors, credential redaction, and a defensive 30-second playlist cache.
> `tests/test_spotify_client.py` covers all boundary paths without live secrets.
> `clify spotify login` now implements the initial Authorization Code with PKCE
> flow and stores the resulting refresh token outside the repository.

> Before Phase 6, `LibrarySyncAgent.spotify` was only ever a `Mock()` in tests.
> Phase 5 found an
> alternative playlist-inventory route through cliamp's undocumented daemon
> JSON IPC, but a direct Spotify client remains necessary for Spotify plays
> made outside cliamp and preserves the Phase 0 subprocess-only boundary.

**Design**

```python
class SpotifyClient:
    def get_user_playlists(self) -> dict: ...      # GET /me/playlists (paginated)
    def get_recently_played(self, limit=50) -> dict: ...  # GET /me/player/recently-played
```

- OAuth2 (authorization-code + refresh token) — credentials never touch
  logs/telemetry; store refresh token outside the repo (env var or OS
  keychain), never committed.
- Same `ToolError` contract as `CliampClient` (§4.2): rate-limit (429),
  expired-token (401 → refresh-and-retry once), network failure all map to
  structured errors, not raw exceptions.
- Pagination handled internally; callers get a flat list.

**TDD lifecycle**

- 🔴 Red: ✅ mocked HTTP layer — token refresh path, pagination, 429 backoff,
  malformed-JSON handling.
- 🟢 Green: ✅ implemented against the Spotify Web API contract.
- ♻️ Refactor: ✅ cache playlist listing for 30s to avoid
  hammering the API on repeated clify CLI invocations.

**Exit criteria:** `LibrarySyncAgent` can be constructed with a real
`SpotifyClient` (no mock). Recorded contract fixtures verify playlist listing;
matching a user's live TUI content is an operator smoke test requiring that
user's OAuth environment and is intentionally not run in CI.

---

## Phase 7 — Unified library aggregation layer ✅

> **Done:** `unified_library.py`, `agent_manifest.cliamp_library.json`, and
> `tests/test_unified_library.py`; provider failures are isolated and merged
> results retain source provenance.

*Spec refs: §2.1 (`required_tools`), existing `CliampQueryAgent` pattern.*

A new read-only agent/module that merges every source clify can see into
one ordered structure, replacing the local-only `get_playlists()` call
users currently hit.

**Design**

```python
class UnifiedLibraryClient:
    def get_recently_played(self, limit=20) -> list[dict]: ...
    #   merges CliampClient.get_recently_played() (local) +
    #   SpotifyClient.get_recently_played() (+ any other configured
    #   provider), sorted by played_at desc, source-tagged.

    def get_library_sections(self) -> dict:
        # {"recently_played": [...], "library": [...], "your_playlists": [...]}
        # section order is fixed: recently_played first, per the
        # requested layout — Library and Your Playlists follow.
```

- `agent_manifest.cliamp_library.json`: `allowed_scopes: ["playlists.read",
  "history.read"]`, same prohibited set as `cliamp_query_agent`.
- Merge/sort is a pure function (`merge_recently_played(sources: list[list[dict]])
  -> list[dict]`) — unit-testable without any live provider.
- A provider outage (e.g. Spotify token expired) must degrade gracefully:
  return the other sources' data with a `partial: true` flag + which
  source failed, never a hard failure for the whole call (§4.2 spirit).

**TDD lifecycle**

- 🔴 Red: ✅ merge-order tests (recently-played always first regardless of
  section sizes); partial-failure tests (one provider down → others still
  returned); dedup tests (same track played on two providers).
- 🟢 Green: ✅ implemented against `CliampClient` + `SpotifyClient` (Phase 6).
- ♻️ Refactor: ✅ confidence/SLA metrics per existing `BaseAgent` contract.

**Exit criteria:** one call returns recently-played + library + playlists,
correctly ordered, with per-source failure isolation.

---

## Phase 8 — Robust clify CLI entrypoint ✅

> **Done:** `clify_cli.py` and the `clify` console script implement all five
> documented command forms. The v1.6.0 wheel was installed into a fresh
> virtual environment and its generated `clify --help` entrypoint passed.

> Before Phase 8 there was **no CLI entrypoint** in this repo at all. Every
> prior interaction was ad
> hoc `python3 -c "..."`. This phase is what "more robust CLI" concretely
> means: a real, documented command surface.

**Design**

```
clify library                 # unified view: recently played / library / your playlists
clify library --json          # machine-readable, same section order
clify recent [--limit N]      # recently-played only, merged across providers
clify play "toggle playback"  # existing Orchestrator dispatch, from a shell
clify status                  # cliamp status passthrough
```

- New `clify_cli.py` + `[project.scripts] clify = "clify_cli:main"` in
  `pyproject.toml`.
- Text-mode output visually mirrors the requested TUI ordering (Recently
  Played above Library, Library above Your Playlists) so the CLI is a
  faithful preview of what Phase 9 would (if feasible) push into the TUI
  itself.
- Errors surface the same structured `{status, reason}` shape agents
  already return — no new error taxonomy.

**TDD lifecycle**

- 🔴 Red: ✅ argument-parsing tests; output-ordering tests; exit-code
  contract (0 success, non-zero on `TOOL_ERROR`/`REJECTED`).
- 🟢 Green: ✅ implemented with `argparse`, reusing `Orchestrator` +
  `UnifiedLibraryClient` — no new business logic in the CLI layer itself.
- ♻️ Refactor: ✅ `--json` output validated by CLI tests and the documented
  provider contracts
  (mirrors the `docs/cliamp_schemas.md` pattern).

**Exit criteria:** the v1.6.0 wheel installs into a fresh virtual environment
and exposes `clify`; CLI tests verify Recently Played / Library / Your
Playlists end-to-end with deterministic provider fixtures. Live content uses
the operator's configured cliamp daemon and Spotify OAuth environment.

---

## Phase 9 — TUI injection (conditional on Phase 5 findings) ✅

> **Disposition: NO-GO / closed by exit criterion (b).** The tagged v1.63.2
> registration code has no provider-browser or section-rendering API, so a
> plugin cannot place "Recently Played" above Spotify's Library/Your
> Playlists sections. Do not build or install an injection prototype. The
> supported unified surface will be `clify library` from Phase 8. Evidence:
> [Phase 5 discovery](docs/cliamp_phase5_discovery.md).

**Only proceed if Phase 5 confirms the Lua plugin API can alter section
layout, not just register callable commands.** If it can't, this phase is
replaced by documenting the limitation and pointing users at `clify
library` (Phase 8) as the supported "see everything" surface instead of
promising a TUI change clify cannot deliver.

**Tasks (conditional; not applicable after no-go)**

- [x] ~~Prototype a `cliamp plugins install` Lua plugin that reads clify's
      `UnifiedLibraryClient` output (via a small local socket/file the
      plugin can poll) and renders a "Recently Played" section above
      Library/Your Playlists in the Spotify browser view.~~ **Not applicable:
      no layout registration API exists.**
- [x] ~~`cliamp plugins trust` + `cliamp plugins call` round-trip tested
      against a live daemon (integration-marked, same pattern as
      `CLIAMP_INTEGRATION`).~~ **Not run: it cannot validate an unsupported
      layout capability.**
- [x] Fallback path documented because the plugin API is
      commands-only: expose "recently played" as a `cliamp plugins call`
      action instead of a passive section. ✅ A callable text action is
      technically possible but optional; `clify library` is the supported
      surface because a command still cannot create a passive TUI section.

**Exit criteria:** either (a) the TUI screenshot's sidebar visibly gains a
"Recently Played" section above Library/Your playlists when clify data
changes, or (b) a written note in this roadmap explaining why that's out
of reach and directing users to `clify library`.

---

## Phase 10 — Hardening & Release (v1.6.0) ✅

- [x] Integration CI: `SpotifyClient` against a sandboxed/test Spotify
      account or recorded cassette (never live user credentials in CI).
      ✅ Recorded, credential-free HTTP fixtures run in `.github/workflows/test.yml`.
- [x] Contract tests: pin Spotify API response schema the same way
      `docs/cliamp_schemas.md` pins cliamp's. ✅ `docs/spotify_schemas.md` and
      `tests/test_spotify_contract.py`.
- [x] Docs: README "Unified library" section, `clify --help` output,
      CHANGELOG entry, version bump. ✅ v1.6.0.
- [x] Monitoring: extend §5 alert rules to cover `SpotifyClient` failures
      and merge-layer `partial: true` degradation. ✅ Structured Spotify tool
      failures feed existing validation alerts; partial merges emit
      `provider_degradation`.

---

## New risks & open questions (v1.6.0)

| Risk | Mitigation |
|---|---|
| cliamp's Lua plugin API does not support layout injection | Resolved in Phase 5: Phase 9 is a no-go; Phase 8 CLI is the supported presentation surface |
| Spotify OAuth token storage/security | keep refresh token out of the repo and out of telemetry; document required scopes minimally (`playlist-read-private`, `user-read-recently-played`) |
| Spotify API rate limits under repeated CLI invocations | short-TTL cache in `SpotifyClient` (Phase 6) |
| "Recently played" has no single source of truth (provider-agnostic cliamp playback history vs. Spotify-wide history) | source-tag when evidence permits, deduplicate overlaps, and keep the merge additive |
| Scope-similarity gate (see v1.5.0 `agent_manifest.cliamp_playback.json` fix) rejecting real playlist/track names again in the new library manifest | reuse the `similarity_text` template-stripping pattern already added to `ScopeGuard.is_authorized` |

---

## Follow-on — cliamp-clify native TUI fork

**Status: 🚧 implementation complete; live acceptance/cutover pending.** The
pinned source is checked out at `cliamp-clify/` on branch
`clify/unified-recent`. Merge, Spotify provider, TUI ordering, cache/refresh,
versioned IPC, CLI wrapper, and stock-cliamp fallback tests are green. The
remaining work requires a configured private Spotify test account and an
operator-approved service cutover/rollback drill.

The implementation plan for adding a native merged Recently Played section to
the closed cliamp TUI is tracked in
[docs/cliamp_clify_fork_plan.md](docs/cliamp_clify_fork_plan.md). The plan keeps
the TUI implementation in Go, uses cliamp's existing Spotify session and local
history, exposes a versioned IPC contract back to clify, stages the fork under
a separate binary, and preserves a tested rollback path.
