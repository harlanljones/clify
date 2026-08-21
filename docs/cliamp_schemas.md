# cliamp JSON Schemas & Subprocess Boundary

Grounding document for the clify ADD framework (Phase 0). All later phases
reference these schemas; Phase 4 contract tests pin them.

- **cliamp version pinned:** `v1.63.2` (minimum supported version)
- **Binary:** `/usr/bin/cliamp` (must be on `PATH`)
- **Transport decision:** subprocess + `--json` (stateless). See ROADMAP.md.
- **Daemon lifecycle decision:** fail-fast over auto-spawn. See ROADMAP.md.

Every invocation runs through:

```python
subprocess.run(argv, capture_output=True, timeout=2.0)
```

The 2.0s timeout keeps the §3.1 latency SLA (≤ 2.5s) achievable.

---

## 1. `cliamp playlist show "Name" --json`

- **schema_version:** `cliamp.playlist.show/1`
- Returns the track listing of one local playlist.

Success payload — **a bare JSON array of tracks** (verified in
`spikes/cliamp_spike.py`: an empty playlist returns `[]`, exit 0):

```json
[
  {
    "path": "string — file path or stream URL",
    "title": "string, optional",
    "duration": "number, seconds, optional",
    "bookmarked": "boolean, optional"
  }
]
```

The framework hard-depends only on the payload being a list; element fields
are pass-through.

### Failure modes

| Condition | Exit | stdout | stderr |
|---|---|---|---|
| Playlist not found | 1 | *(empty)* | `playlist "Name" not found` |
| Missing name argument | 0 | `usage: cliamp playlist show "Name" [--json]` | — |

⚠️ The missing-argument case exits **0** with a usage line on stdout — the
wrapper must validate arguments before spawning, not rely on exit codes.

## 2. `cliamp playlist list`

- **schema_version:** `cliamp.playlist.list/1`
- ⚠️ **No `--json` flag exists** (verified: `flag provided but not defined:
  -json`, exit 1). Output is human-readable text, one playlist per line with
  track counts; empty state prints `No playlists found.` (exit 0).
- The wrapper parses the text lines (name + count) and normalizes to:

```json
{
  "schema_version": "cliamp.playlist.list/1",
  "playlists": ["string — playlist names, in display order"]
}
```

## 3. `cliamp history --json`

- **schema_version:** `cliamp.history/1`
- Options: `--limit int` (default 50, 0 = all).
- Returns a JSON array (possibly empty: `[]`, exit 0). Element shape:

```json
[
  {
    "title": "string",
    "path": "string — file path or stream URL",
    "played_at": "string — timestamp, optional",
    "stream": "boolean, optional"
  }
]
```

Observed on discovery host: `[]` (empty history), exit 0.

## 4. `cliamp status --json`

- **schema_version:** `cliamp.status/1`
- Verified live payload (daemon running, v1.63.2):

```json
{
  "ok": true,
  "state": "stopped",
  "track": {
    "title": "Lofi Stream",
    "path": "http://radio.cliamp.stream/lofi/stream",
    "stream": true
  },
  "total": 11,
  "visualizer": "Bars",
  "shuffle": false,
  "repeat": "Off",
  "mono": false,
  "speed": 1,
  "eq_preset": "Custom",
  "theme": { "name": "Default - Terminal colors" },
  "eq_bands": [0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
}
```

Notes:
- `state` ∈ `playing | paused | stopped`.
- `track` may be absent/null when nothing is loaded.
- `eq_bands` is a fixed 10-element float array.
- Non-JSON form prints `State: … / Track: … / Volume: …` lines; never parse it.

## 4.1 Fork capability: `cliamp history unified --json --limit N`

- **schema_version:** `cliamp.history.unified/1`
- Available in `v1.63.2-clify.1`; stock `v1.63.2` rejects the subcommand.
- Requires a running fork TUI or daemon because it uses the shared IPC
  operation `{"cmd":"history.unified","limit":N}`.
- Merges cliamp local history with Spotify recently played, newest first,
  deduplicates canonical Spotify URIs, and reports per-source degradation.

```json
{
  "ok": true,
  "schema_version": "cliamp.history.unified/1",
  "history": [
    {
      "track": {"title": "A Song", "path": "spotify:track:abc"},
      "played_at": "2026-08-21T10:00:00Z",
      "sources": ["cliamp", "spotify"]
    }
  ]
}
```

When a source fails, `partial` is `true` and `failed_sources` names it. clify
probes this operation and falls back to its stock-cliamp plus direct-Spotify
merge when the command or schema is unavailable.

---

## 5. Subprocess failure modes (wrapper contract, AGENTS.md §4.2)

All failures surface as `ToolError`-compatible exceptions carrying
`{"status": "TOOL_ERROR", "retry_allowed": ...}`.

| Failure mode | Detection | retry_allowed |
|---|---|---|
| Binary missing from `PATH` | `FileNotFoundError` from `subprocess.run` | `false` |
| Non-zero exit code | `returncode != 0`; stderr carries message (e.g. `playlist "X" not found`) | `false` for unknown resource, `true` otherwise |
| Empty stdout where JSON expected | `stdout.strip() == ""` with exit 0 | `true` |
| Malformed JSON | `json.JSONDecodeError` | `true` |
| No daemon running | `status`/IPC commands fail non-zero or `ok: false` (see below) | `false` — fail-fast per lifecycle decision |
| Timeout | `subprocess.TimeoutExpired` after 2.0s | `true` |

### Daemon notes (observed v1.63.2)

- A daemon **was** running during discovery (`pgrep cliamp` → PID 3310189);
  `status --json` and IPC verbs (`pause`) succeed against it.
- When no daemon/instance is running, `cliamp status` reports the last saved
  state but playback-control verbs cannot be acted on; treat control-command
  failure as the daemon-down signal. **Fail-fast:** the framework never
  auto-spawns `cliamp --daemon`; it returns
  `{"status": "TOOL_ERROR", "retry_allowed": false}` and surfaces the
  remediation (`cliamp --daemon`) to the operator.
- Unknown flags (e.g. `--json` on `playlist list`) exit 1 with
  `flag provided but not defined: -json` on stderr.
