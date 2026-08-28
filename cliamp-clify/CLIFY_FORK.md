# cliamp-clify fork

Baseline: cliamp `v1.63.2` (`c0f04563da12169b1340defcfed85736fe9e7bd1`)  
Fork release: `v1.63.2-clify.2`  
Go toolchain: `1.26.5`

This fork adds a native **Recently Played** section to Spotify's provider
browser, makes Spotify the startup default, syncs visualizer colors with
Omarchy desktop themes, and fixes Spotify Top Artists album counts in browse.

## What changed in clify.2 (continued)

- **Omarchy live theme sync.** When `theme` is unset in config, cliamp reads
  `~/.config/omarchy/current/theme/colors.toml`, maps Omarchy palette keys onto
  cliamp's six-color theme, and hot-reloads UI + spectrum visualizer colors when
  the desktop theme file changes (~2s poll). The live palette appears in the
  theme picker as `omarchy`. See [docs/themes.md](docs/themes.md).
- **Spotify Top Artists album counts.** `/v1/me/top/artists` omits album totals;
  browse now enriches each artist via `/v1/artists/{id}/albums` (concurrent,
  cached) so **By Artist** rows show real counts instead of `(0 albums)`.

## What changed in clify.2

- **Made For You section with resolvable Spotify mixes.** Spotify-generated
  playlists (Daily Mixes, Discover Weekly, Release Radar, On Repeat, Repeat
  Rewind, daylist) are discovered from `/v1/me/playlists` (owner `spotify` or
  id prefix `37i9dQZF1E`) into a new "Made For You" section right after
  Recently Played, instead of being listed under Followed playlists where
  opening them failed. Since Nov 2024 the Web API 404s on these ids; their
  tracks now resolve through go-librespot's spclient context-resolve
  (`/context-resolve/v1/{uri}`, up to 10 pages), cached per session.
  Recently Played playlist rows whose name lookup 404s stay visible under a
  "Generated Mix" fallback label and resolve the same way; the recorded 404
  set is now only a routing hint, not a visibility filter.
- **Spotify-derived Recently Played rows.** The single `All sources` song row
  is replaced by up to 25 album/playlist rows derived from the merged recent
  feed's Spotify listening context (`context.type` / `context.uri`, falling
  back to the track's own album), deduplicated by canonical URI, newest first.
  Album rows open through paginated `/v1/albums/{id}/tracks`; playlist rows
  reuse the existing playlist-items path, with names fetched once per unique
  id (best-effort, session-cached). Local cliamp history no longer feeds the
  pane but still feeds the IPC merged feed.
- **Default provider is Spotify.** Without a `provider = "..."` config line,
  the TUI opens on the Spotify pane (radio seeding no longer fires). If
  Spotify is not configured, cliamp falls back to the first registered
  provider as before.
- **Followed playlists visibility fixes.** Section-aware scroll math keeps
  bottom-of-list rows inside the viewport; filter mode (`/`) renders one
  header per result group with an always-visible result-count line; a failed
  `/v1/me` call no longer collapses every playlist into one section for the
  whole session.
- **`spotify login` subcommand.** `clify spotify login [--client-id <id>]`
  authorizes headlessly: it persists `client_id` under `[spotify]` when given,
  then runs the same browser OAuth journey as TUI sign-in (Web API + playback
  in one tab journey) and caches credentials — no player initialization, so
  the next TUI start is silent. Port 19872 must be free during the flow.
- **Fork branding.** The TUI header, terminal window title, pixel-art logo
  visualizer, and CLI help now identify as `clify` (`clify version ...`,
  `retro terminal music player (cliamp fork)`). Functional identity is
  unchanged on purpose: config stays at `~/.config/cliamp/`, env vars keep
  the `CLIAMP_*` prefix, Go module and IPC method names (including
  `cliamp.history.unified/1`) are untouched, so the fork remains
  drop-in compatible with stock cliamp state and clify clients.

The versioned IPC contract is unchanged: `unifiedRecent`, `recent.Merge`, and
`UnifiedRecent()` still expose the merged timestamped song feed to clify via
`cliamp.history.unified/1`:

```sh
cliamp-clify history unified --json --limit 20
```

The fork does not invoke clify or require a Python daemon.

## Build and verify

```sh
make check
go test -race ./...
go build -trimpath \
  -ldflags="-s -w -X main.version=v1.63.2-clify.2" \
  -o bin/cliamp-clify .
```

## Staging and rollback

Stage without replacing the system binary:

```sh
install -Dm755 bin/cliamp-clify ~/.local/bin/cliamp-clify
```

Stop the stock daemon before starting the fork against the default socket, or
use a separately configured service during acceptance. Rollback is immediate:
stop the fork service and restart the existing service that executes
`/usr/bin/cliamp`. The fork never overwrites `/usr/bin/cliamp`.

Live acceptance requires an operator-controlled Spotify account. Verify empty
history, offline startup, expired OAuth, rate limiting, section order, Enter
on a Recently Played album row and a playlist row, Followed playlists
visibility while scrolling and under `/` filter, Ctrl+R invalidation, and the
rollback procedure before cutover.
