# Implementation Plan — cliamp-clify Fork v2

Status: planned  
Baseline: fork release `v1.63.2-clify.1` (branch `clify/unified-recent`)  
Target release: `v1.63.2-clify.2`  
Implementation checkout: `cliamp-clify/`  
Automated verification: `make check`, `go test -race ./...`

Supersedes the pane-level behavior of `docs/cliamp_clify_fork_plan.md`
(the `cliamp.history.unified/1` IPC contract is preserved unchanged).

## 1. Default source = Spotify; TUI opens on the Spotify pane

- `main.go` (defaultProvider fallback): change hardcoded fallback from
  `"radio"` to `"spotify"`. Desired side effect: the radio seeding block
  (`defaultRadio`) no longer fires, the queue stays empty, and
  `StartInProvider()` focuses the provider pane resolved to Spotify.
- Graceful degradation: if Spotify is not configured it is never registered,
  so `model.New()` falls back to index 0 (radio) as today.
- Sync `config.toml.example` (`provider` key comment) and user-facing docs
  that state the startup default.

## 2. Recently Played: songs → albums/playlists (Spotify-derived only)

- Parse `context{type, uri}` from each `/v1/me/player/recently-played` item
  (`external/spotify/recent.go`); also add `id` to the parsed album object in
  `external/spotify/provider_shared.go`.
- Derive rows from the merged recent feed:
  - Albums: `context.uri = spotify:album:<id>` or `track.album.id`; name comes
    free with the track payload.
  - Playlists: `context.uri = spotify:playlist:<id>` only; names fetched once
    per unique ID via `GET /v1/playlists/{id}?fields=name` (best-effort,
    session-cached, fallback label for failures).
  - Dedupe by canonical URI, order by latest `played_at` desc, cap ~25 rows.
  - Plays without album/playlist context are skipped; local cliamp history no
    longer feeds the pane (it still feeds the IPC merged feed).
- Replace the single `"All sources"` row injected by `withUnifiedRecent()`
  with these rows under Section `"Recently Played"`. Delete the
  `UnifiedRecentPlaylistID` branch in `Tracks()`.
- Opening rows:
  - Playlist rows reuse the existing `/v1/playlists/{id}/items` path.
  - Album rows get a new branch keyed on a synthetic ID prefix
    (`__clify_album__<id>`) → paginated `GET /v1/albums/{id}/tracks`.
- Preserve the IPC contract: keep `unifiedRecent`, `recent.Merge`, and
  `UnifiedRecent()` intact so `cliamp.history.unified/1`,
  `daemon.handleHistory`, and the clify Python integration continue to see
  the merged song feed.

## 3. Fix broken/missing "Followed playlists"

Root causes identified:

- Header-blind scroll math for non-radio lists (`ui/model/view.go`): the
  naive `provCursor - visibleBudget + 1` branch ignores section headers, so
  bottom-of-list rows and the Followed header are clipped. Route the
  non-radio branch through header-aware `providerRowsFromScroll` (as the
  radio branch does) and clamp via `providerMaybeAdjustScroll` after
  `playlistsLoadedMsg` (covers Ctrl+R refresh).
- Filter mode drops section headers entirely; emit them per result group and
  fix the off-by-one that clips the result-count status line.
- Section rank map missing the `"Recently Played"` entry — make injection
  ordering explicit instead of relying on prepend-after-sort.
- `currentUserID()` caches a failed `/v1/me` for the whole session, collapsing
  every playlist into one section; only mark fetched on success.

Regression tests: 4-section scrolled render asserting the Followed header and
rows survive; filter-mode header test.

## 4. Tests & housekeeping

- Rework `external/spotify/recent_test.go` (row derivation, ordering, dedup,
  name-fetch caching, malformed context) and `ui/model/unified_recent_test.go`
  (no more "All sources"). Keep `TestPlaylistsIncludesFollowedPlaylists` and
  all daemon/IPC contract tests green.
- Windows stubs unaffected: touched files carry `//go:build !windows`.
- Sync docs per repo convention: `CLIFY_FORK.md` (describes the All sources
  song merge and Enter-on-All-sources acceptance step), `docs/spotify.md`,
  `site/index.html` if it references the feature, `CHANGELOG.md`.

## 5. Verification

```sh
cd cliamp-clify && make check && go test -race ./...
go build -trimpath -ldflags="-X main.version=v1.63.2-clify.2" -o bin/cliamp-clify .
```

Manual acceptance: open TUI → lands on Spotify pane; Recently Played shows
album/playlist rows sorted by recency and Enter opens tracks; Followed
playlists visible while scrolling and under `/` filter; Ctrl+R rebuilds rows.
