# Development Roadmap — cliamp Integration

Agent-driven development plan for integrating [cliamp](https://github.com/bjarneo/cliamp)
(retro terminal music player) into the clify ADD framework. Every phase
follows the three-stage TDD lifecycle mandated by [AGENTS.md](AGENTS.md) §4
(Red → Green → Refactor), and every shipped agent must pass the full
lifecycle verification pipeline before it is considered deployed.

**Status legend:** 🔲 not started · 🚧 in progress · ✅ done

---

## Phase 0 — Discovery & Feasibility 🔲

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

- [ ] Pin a minimum cliamp version and record the exact `--json` schemas
      for `playlist show`, `history`, and `status` in
      `docs/cliamp_schemas.md`.
- [ ] Document failure modes of the subprocess boundary: non-zero exit
      codes, empty stdout, "no daemon running" errors, binary missing from
      `PATH`.
- [ ] Decide transport: **subprocess + `--json`** (recommended, stateless)
      vs. direct Unix-socket IPC (faster, but reimplements the protocol).
      Record the decision and rationale in this file.
- [ ] Confirm `cliamp --daemon` lifecycle management story (systemd user
      unit? framework-spawned child?) — needed by Phase 3.

**Exit criteria:** schemas documented; transport decision recorded; a
one-page spike script can round-trip `playlist show --json` and
`history --json` on a real cliamp install.

---

## Phase 1 — `CliampClient` Tool Wrapper (Read-Only) 🔲

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

- 🔴 **Red:** unit tests asserting (a) exact subprocess argv construction,
  (b) JSON parsing into documented schemas, (c) `RuntimeError` → structured
  `TOOL_ERROR` propagation, (d) missing-binary handling.
- 🟢 **Green:** implement with `subprocess.run(..., capture_output=True,
  timeout=2.0)` — the timeout keeps the latency SLA (≤ 2.5s) achievable.
- ♻️ **Refactor:** parse into typed dataclasses; verify SLA metrics remain
  intact.

**Exit criteria:** `CliampClient` is a drop-in tool for `BaseAgent`;
coverage of all four read methods; zero network access (local binary only).

---

## Phase 2 — `cliamp_query_agent` (Read-Only) 🔲

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

- [ ] Register the manifest; keep `spotify_query_agent` untouched.
- [ ] Refactor `ScopeGuard` to load token tables **per-agent from the
      manifest** instead of module-level globals (see "Framework
      refactors" below). Player-specific terms (`queue`, `seek`, `daemon`)
      map to prohibited scopes here.
- [ ] Implement `CliampQueryAgent(BaseAgent)` with `_route_tool` dispatch:
      playlist/library terms → `get_playlist(s)`, history/recently/played →
      `get_recently_played`, status/playing/current → `get_status`.
- [ ] TDD pipeline mirroring `tests/test_lifecycle.py`:
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

## Phase 3 — `cliamp_playback_agent` (Playback Control) 🔲

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

- [ ] `CliampController` tool wrapping IPC verbs: `play`, `pause`, `toggle`,
      `stop`, `next`, `prev`, `volume <db>`, `seek <s>`, `queue <path>`,
      `load <playlist>`, `shuffle`, `repeat`. Each returns the post-command
      `status --json` payload for verification.
- [ ] Daemon precondition: `_route_tool` verifies a running instance
      (`cliamp status` succeeds); if not, either fail with structured
      `TOOL_ERROR` (`retry_allowed: false`) or auto-spawn
      `cliamp --daemon --auto-play` per the Phase 0 lifecycle decision.
- [ ] **Confirmation semantics:** destructive/ambiguous instructions
      (e.g. `history clear`, `playlist delete`) stay prohibited outright;
      playback verbs are authorized only when they map unambiguously to one
      IPC command.
- [ ] Telemetry: mutation commands must still honor cost/latency SLAs;
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

## Phase 4 — Hardening & Release 🔲

- [ ] **Integration CI job:** install cliamp (Linux build + `pipewire-alsa`
      note for devs), launch `--daemon`, run the full suite against it.
- [ ] **Contract tests:** pin cliamp's JSON schemas; fail loudly on
      upstream format drift (schema version field in
      `docs/cliamp_schemas.md`).
- [ ] **Docs:** README section "Using clify with cliamp" with quickstart;
      CHANGELOG entry; bump to v1.5.0 (minor — new feature, no breaking
      changes to the Spotify agent).
- [ ] **Monitoring:** confirm all three §5 alerts fire correctly under a
      fault-injected cliamp (killed daemon → latency/tool-error alerts).

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
