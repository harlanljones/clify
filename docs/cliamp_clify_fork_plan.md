# Implementation Plan — cliamp-clify Fork

Status: implemented; live Spotify acceptance and service cutover pending  
Baseline: cliamp `v1.63.2`, commit `c0f04563da12169b1340defcfed85736fe9e7bd1`  
Target release: `v1.63.2-clify.1`

Implementation checkout: `cliamp-clify/`, branch `clify/unified-recent`  
Toolchain: Go `1.26.5`  
Automated verification: upstream `make check`, `go test -race ./...`, and
clify's Python suite pass as of 2026-08-21. F5's private-account acceptance,
service cutover, and rollback drill remain operator-gated.

## 1. Outcome

Build and maintain a small cliamp fork that adds a selectable **Recently
Played** section above **Library** and **Your playlists** in the Spotify
provider browser. The section contains a merged, newest-first view of:

1. cliamp's provider-agnostic local history; and
2. Spotify's `/v1/me/player/recently-played` history.

The fork must remain usable without clify running. clify may consume the same
merged data through a versioned CLI/IPC contract, but cliamp's TUI must never
shell out to Python or depend on a clify daemon.

Expected TUI shape:

```text
── Spotify / Playlists ─────────────────────────────────────
  ── Recently Played ───────────────────────────────────────
> All sources · 20 tracks
  ── Library ────────────────────────────────────────────────
  Your Music · 61 tracks
  ── Your playlists ─────────────────────────────────────────
  My Playlist #19 · 2 tracks
  ...
```

Pressing Enter on `All sources` loads the merged tracks into the normal
playlist pane. All existing playback, queue, search, EQ, and visualizer
behavior then applies without a special playback path.

## 2. Architectural decision

### Native Go implementation, shared contract

The fork will implement aggregation inside cliamp and let clify consume it.
It will not invoke `clify library` from Go.

Reasons:

- cliamp already owns the TUI state and Spotify playback session;
- its OAuth scope list already includes `user-read-recently-played`;
- its `history.Store` already records qualifying plays regardless of provider;
- its provider browser already renders ordered `PlaylistInfo.Section` values;
- a Python subprocess would introduce lifecycle, latency, packaging, and error
  dependencies into every provider refresh;
- reusing cliamp's Spotify session avoids two independent logins inside one
  TUI process.

clify remains the automation and agent layer. A later phase adds a structured
`history.unified` IPC operation so clify can use the fork as a provider while
retaining its current direct-client fallback.

### Explicit non-goals

- No Lua-plugin layout work; v1.63.2 has no browser-section plugin API.
- No automatic cliamp daemon spawning.
- No modification or import of clify's mode-0600 Spotify credential file.
- No playlist mutation, history deletion, or Spotify library writes.
- No immediate overwrite of `/usr/bin/cliamp`; the fork is staged separately.

## 3. Existing extension points

The fork should use existing mechanisms rather than add a parallel UI system:

| Concern | Existing cliamp surface | Planned use |
|---|---|---|
| Section layout | `playlist.PlaylistInfo.Section` and `renderProviderList()` | Add a first `Recently Played` group |
| Selection/load | `playlist.Provider.Tracks(playlistID)` | Reserve one virtual playlist ID |
| Local history | `history.Store.Recent(limit)` | Preserve timestamps while merging |
| Spotify history | `SpotifyProvider.webAPI()` and existing OAuth session | Call `/v1/me/player/recently-played` |
| Playback | `playlist.Track` plus registered `spotify:` streamer | Load a normal mixed track slice |
| Refresh | `playlist.Refresher` checked by `Ctrl+R` | Add Spotify cache invalidation |
| Script access | IPC `provider.*` and `history` operations | Add `history.unified` compatibly |

Primary upstream files:

- `external/spotify/provider.go` — Spotify playlist enumeration and Web API
  boundary.
- `external/spotify/provider_shared.go` — Spotify JSON types and track
  conversion.
- `external/spotify/session.go` — OAuth session and scopes.
- `history/history.go` — timestamped local history.
- `playlist/provider.go` — provider and section contracts.
- `ui/model/view.go`, `keys.go`, `providers.go`, `update.go` — rendering,
  selection, async loading, and refresh.
- `ipc/protocol.go`, `ipc/server.go`, `ui/model/ipc_extended.go` — structured
  external contract.

Upstream references:

- <https://github.com/bjarneo/cliamp/blob/v1.63.2/external/spotify/provider.go>
- <https://github.com/bjarneo/cliamp/blob/v1.63.2/history/history.go>
- <https://github.com/bjarneo/cliamp/blob/v1.63.2/ui/model/view.go>
- <https://github.com/bjarneo/cliamp/blob/v1.63.2/ipc/protocol.go>

## 4. Proposed code design

### 4.1 Pure recent-history package

Add `recent/` with no TUI dependency:

```go
package recent

type Item struct {
    Track    playlist.Track
    PlayedAt time.Time
    Sources  []string
}

type Source interface {
    Recent(ctx context.Context, limit int) ([]Item, error)
}

type Result struct {
    Items         []Item
    Partial       bool
    FailedSources []string
}

func Merge(limit int, sources ...NamedItems) Result
```

Merge rules:

1. Sort by `PlayedAt` descending; invalid/zero timestamps sort last.
2. Deduplicate by canonical `spotify:track:` URI first.
3. Otherwise use stable provider metadata IDs when present.
4. Do not collapse local files using title alone; prefer a duplicate over a
   false positive.
5. Keep the newest representation and union source names deterministically.
6. Never mutate source slices.
7. Apply the requested limit only after merge and deduplication.

Store provenance in `Track.ProviderMeta`, using namespaced keys such as
`clify.sources=cliamp,spotify` and `clify.played_at=<RFC3339>`. No tokens or
raw API payloads enter metadata or logs.

### 4.2 Local-history adapter

Add `recent/history_source.go`, adapting `history.Store.Recent()` into
`recent.Item` without dropping `PlayedAt`. Keep the current history file and
schema unchanged.

The first implementation may inject a read-only `history.New()` store into
Spotify independently. A follow-up refactor may share one store pointer among
the model, local provider, and Spotify provider; this is not required for the
feature because history writes are atomic and readers see either the old or
new complete file.

### 4.3 Spotify recent source

Add `external/spotify/recent.go`:

```go
func (p *SpotifyProvider) Recent(ctx context.Context, limit int) ([]recent.Item, error)
```

Responsibilities:

- ensure the existing Spotify session is authenticated;
- request `/v1/me/player/recently-played?limit=N`;
- parse `items[].played_at` and `items[].track`;
- reuse the existing Spotify item-to-`playlist.Track` conversion;
- preserve the canonical Spotify URI for playback and deduplication;
- cap API page size at Spotify's documented maximum;
- return structured errors without exposing OAuth data.

Spotify currently documents that this endpoint excludes podcast episodes;
the fork should document the same limitation rather than synthesize them.

### 4.4 Virtual playlist in SpotifyProvider

Use a collision-resistant internal ID:

```go
const UnifiedRecentPlaylistID = "__clify_unified_recent_v1__"
```

Extend `SpotifyProvider` with a variadic option so existing callers remain
source compatible:

```go
func New(session *Session, clientID string, bitrate int, opts ...Option) *SpotifyProvider
func WithHistory(store *history.Store) Option
```

`main.go` passes `spotify.WithHistory(history.New())`.

When `Playlists()` succeeds:

1. fetch/merge recent sources;
2. prepend one `PlaylistInfo` with `Section: "Recently Played"`,
   `Name: "All sources"`, and the merged count when non-empty;
3. retain existing ordering for Library, Your playlists, and Followed
   playlists;
4. continue returning normal playlists if recent-history retrieval partially
   or completely fails.

When `Tracks(UnifiedRecentPlaylistID)` is called, return the cached merged
tracks. All other IDs keep their existing behavior.

### 4.5 Cache and refresh policy

- Cache merged recent items for 30 seconds.
- Fetch local and Spotify histories concurrently with bounded contexts.
- Do not add an unbounded retry loop; reuse the existing Web API retry policy.
- Implement `SpotifyProvider.Refresh()` to clear both playlist-list and recent
  caches. This also makes the existing `Ctrl+R` provider path meaningful for
  Spotify.
- If Spotify recent history fails, show local data and record a warning.
- If local history fails, show Spotify data and record a warning.
- If both fail, omit the virtual playlist but still show Library/playlists.

## 5. IPC and clify convergence

After the TUI works natively, add a backward-compatible structured operation:

```json
{
  "cmd": "history.unified",
  "limit": 20
}
```

Response:

```json
{
  "ok": true,
  "schema_version": "cliamp.history.unified/1",
  "history": [
    {
      "track": {"title": "A Song", "path": "spotify:track:..."},
      "played_at": "2026-08-21T10:00:00Z",
      "sources": ["cliamp", "spotify"]
    }
  ],
  "partial": false,
  "failed_sources": []
}
```

Required fork changes:

- extend `ipc.HistoryInfo` with `sources,omitempty`;
- extend `ipc.Response` with `schema_version`, `partial`, and
  `failed_sources` using `omitempty` to preserve old JSON;
- route `history.unified` in `ipc/server.go`;
- handle it asynchronously in TUI and daemon dispatchers;
- add `cliamp history unified --json --limit N` as a public wrapper.

Required clify changes:

- add capability detection using `cliamp version` or a lightweight IPC probe;
- prefer `history.unified/1` when the fork exposes it;
- retain the current `CliampClient + SpotifyClient` merge as fallback for
  stock cliamp;
- pin the new schema in clify contract tests;
- never treat absence of the new operation as an error on stock cliamp.

## 6. TDD execution plan

Every phase follows Red → Green → Refactor.

### F0 — Fork bootstrap

- Fork `bjarneo/cliamp` and add `upstream` remote.
- Branch from the installed baseline commit as `clify/unified-recent`.
- Build an uninstalled binary at `bin/cliamp-clify`.
- Record upstream commit and Go toolchain version.
- Run the untouched upstream `make check` before feature work.

Exit: baseline tests pass and the fork binary starts against a temporary config.

### F1 — Merge contract

Red tests:

- chronological ordering;
- URI-based deduplication and unioned provenance;
- non-Spotify local tracks remain distinct;
- malformed timestamps sort last;
- limit-after-dedup behavior;
- source slices remain unchanged;
- one failed source produces `Partial=true` and healthy data.

Green: implement `recent.Merge` and source adapters.

Refactor: table-driven cases plus `BenchmarkMergeRecent`; target under 1 ms
for 400 entries.

### F2 — Spotify provider integration

Red tests with `httptest.Server`:

- exact recent endpoint and limit query;
- JSON-to-track conversion with RFC3339 timestamps;
- auth and malformed-response errors;
- local-only and Spotify-only degradation;
- synthetic playlist precedes Library and Your playlists;
- reserved ID loads the merged tracks;
- 30-second cache and `Refresh()` invalidation.

Green: implement native API source, provider option, virtual playlist, cache,
and failure isolation.

Refactor: concurrent source fetch with bounded context and no duplicated
Spotify conversion logic.

### F3 — TUI behavior

Red tests in `ui/model/`:

- golden rendering order exactly matches the proposed TUI shape;
- section headers consume viewport rows but are not selectable;
- cursor, scrolling, page-up/down, and filtering remain correct;
- Enter on `All sources` loads the reserved playlist;
- empty recents omit the section;
- recent-loading failure leaves the existing Spotify browser usable;
- narrow and compact layouts do not overflow.

Green: use the existing `PlaylistInfo.Section` renderer; avoid a bespoke
overlay.

Refactor: keep section-order knowledge inside the provider and generic UI
section rendering inside the model.

### F4 — IPC and clify compatibility

Red tests:

- JSON round-trip for `cliamp.history.unified/1`;
- old `history` response remains byte/schema compatible;
- daemon and TUI dispatch produce equivalent responses;
- clify prefers the fork capability and falls back cleanly on stock v1.63.2;
- partial metadata reaches clify monitoring.

Green: implement the new operation, command wrapper, and clify adapter.

Refactor: keep one mapping function from `recent.Result` to IPC response.

### F5 — Live acceptance and packaging

- Run all Go tests, race detector, vet/static checks, and clify's Python suite.
- Run opt-in live Spotify tests with a private test account; never CI secrets
  from a personal account.
- Verify the displayed order and load behavior against the reference screen.
- Verify offline startup, expired OAuth, empty history, and 429 behavior.
- Build `v1.63.2-clify.1` with reproducible version ldflags.
- Install initially as `~/.local/bin/cliamp-clify`, leaving
  `/usr/bin/cliamp` untouched.
- Use a separate user service name/socket during staging, or stop the stock
  daemon before pointing the existing service at the fork.
- Document one-command rollback to the stock binary/service.

Exit: the reference TUI visibly contains Recently Played first, Enter loads
merged tracks, all tests pass, and rollback is verified.

### F6 — Upstream and maintenance

- Split commits into: merge package, Spotify source, provider/TUI, IPC, docs.
- Open the native Spotify-recent implementation upstream without clify
  branding where possible.
- Keep fork-only IPC compatibility in a separate commit if upstream declines it.
- Rebase regularly; never merge upstream blindly across `provider.go`,
  `view.go`, `ipc/protocol.go`, or OAuth/session changes.
- Publish a compatibility table mapping fork releases to upstream commits and
  clify schema versions.

## 7. Verification gates and SLAs

| Gate | Required result |
|---|---|
| Go unit/integration suite | 100% pass |
| `go test -race ./...` | no races |
| Static checks / upstream `make check` | pass |
| clify Python suite | 100% pass except explicitly gated live tests |
| Warm recent-list load | < 50 ms from cache |
| Cold local merge | < 10 ms excluding network |
| Cold provider refresh | bounded to 2.5 s before partial fallback |
| Duplicate rate | zero duplicate canonical Spotify URIs |
| Unsupported stock cliamp | clify fallback succeeds |
| Credential leakage | zero tokens/secrets in logs, IPC, fixtures, or metadata |

## 8. Risks and mitigations

| Risk | Mitigation |
|---|---|
| Spotify endpoint or quota failure blocks browser | Treat recents as optional; render normal playlists and local history |
| Duplicate plays across local and Spotify histories | URI-first dedup with deterministic source union |
| False dedup of unrelated local tracks | Never dedup local files on title alone |
| Async response changes active provider after navigation | Reuse generation IDs and provider-name guards |
| Section header breaks cursor/scroll math | Extend existing viewport and navigation table tests |
| Fork falls behind upstream security/playback fixes | Small isolated commits, upstream remote, compatibility matrix |
| Fork overwrites working system binary | Stage under a distinct name and verify rollback before service cutover |
| Two OAuth implementations diverge | TUI uses cliamp's session; clify consumes IPC when available and retains fallback |
| Recent Spotify podcast episodes missing | Document Spotify endpoint limitation; do not fabricate coverage |

## 9. Definition of done

The fork is complete when:

1. Spotify provider view shows Recently Played above Library and Your
   playlists.
2. Enter loads a merged, timestamp-sorted, deduplicated track list.
3. Spotify/local failures degrade independently without hiding playlists.
4. `Ctrl+R` invalidates and refreshes recent data.
5. TUI and daemon expose `cliamp.history.unified/1` consistently.
6. clify consumes that schema when present and remains compatible with stock
   cliamp.
7. Unit, race, static, contract, and live acceptance gates pass.
8. The fork is staged separately, and rollback to `/usr/bin/cliamp` is tested.
