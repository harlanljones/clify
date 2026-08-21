# cliamp Phase 5 Discovery (v1.63.2)

Discovery was performed on 2026-08-21 against the installed
`/usr/bin/cliamp` (`cliamp version v1.63.2`) and the exact upstream tag
`v1.63.2` (commit `c0f04563da12169b1340defcfed85736fe9e7bd1`). No
credentials or credential values were read or recorded.

## Decision

**No-go for cliamp TUI layout injection.** A v1.63.2 Lua plugin cannot add or
reorder sections in the Spotify provider browser. Phase 9 therefore closes via
its documented limitation path; `clify library` remains the planned supported
unified presentation surface.

The plugin API supports playback hooks, key bindings, callable commands,
messages, queue/player operations, and custom visualizers. Registration accepts
only `type = "hook"` or `type = "visualizer"`; the plugin object is wired to
event/config, keymap, command, and (for visualizers) render APIs. There is no
provider/browser/section registration hook. See the tagged
[plugin documentation](https://github.com/bjarneo/cliamp/blob/v1.63.2/docs/plugins.md#registration)
and
[`registerPluginAPI`](https://github.com/bjarneo/cliamp/blob/v1.63.2/luaplugin/luaplugin.go#L311-L386).
The four bundled examples (`auto-eq.lua`, `now-playing.lua`,
`status-messages.lua`, and `webhook.lua`) use those action/event surfaces and
provide no layout extension example.

A commands-only fallback is technically possible: a plugin may return text
from `p:command(...)`, invoked as `cliamp plugins call <plugin> <command>`.
That is an explicit action, not a passive browser section, so it does not meet
the Phase 9 layout requirement and is secondary to `clify library`.

## Provider inventory

`--provider` is a startup/default-provider selector, not a data-query command.
The public v1.63.2 CLI has no `provider list`, `provider playlists`, or
`provider tracks` subcommands. The running player does, however, implement
newline-delimited JSON IPC operations named `provider.list`,
`provider.playlists`, `provider.tracks`, and `provider.search`; they are handled
by the daemon but have no CLI wrapper. Evidence:
[`ipc/server.go`](https://github.com/bjarneo/cliamp/blob/v1.63.2/ipc/server.go#L325-L335)
and
[`ui/model/ipc_extended.go`](https://github.com/bjarneo/cliamp/blob/v1.63.2/ui/model/ipc_extended.go#L144-L277).

| Provider key(s) | Registration condition in v1.63.2 | This machine | Inventory/query capability |
|---|---|---|---|
| `radio` | Always registered | Available | playlists/tracks plus catalog search over IPC |
| `local` | Always registered when the config directory resolves | Available | playlists/tracks/search over IPC; omitted from the `--provider` help list despite being registered |
| `spotify` | `[spotify]` section present and not disabled | **Configured**; a mode-0600 credential cache exists | playlists, tracks, and search over IPC; the provider itself queries `/v1/me/tracks` and `/v1/me/playlists` |
| `navidrome` | Complete config or environment credentials | Not configured | playlists/tracks/search when registered |
| `plex`, `jellyfin`, `emby` | Complete server/auth config | Not configured | playlists/tracks/search when registered |
| `qobuz` | `[qobuz]` section present and not disabled | Not configured | playlists/tracks/search when registered |
| `soundcloud`, `netease` | Explicitly enabled | Not configured | playlists/tracks/search when registered |
| `yt`, `youtube`, `ytmusic` | YouTube Music credentials (configured or compiled fallback) and `yt-dlp` | `yt-dlp` present, but v1.63.2 source has an empty fallback credential pool and no config section | playlist/track browsing when registered; these providers do not implement `SearchTracks` |

Only the `[spotify]` section was present in this machine's config. Radio and
Local are built-ins. No cliamp process was running during discovery, so live
daemon IPC was not used; the inventory above is based on config section
presence, credential-file presence, executable availability, and the exact
tagged registration code in
[`main.go`](https://github.com/bjarneo/cliamp/blob/v1.63.2/main.go#L66-L175).

This IPC surface is a viable future alternative for provider playlist
inventory, but adopting it would revise the Phase 0 subprocess-only transport
decision and requires its own protocol contract tests. It is not exposed by the
current `CliampClient`.

## History semantics

There is no provider-scoped history operation. Both `cliamp history --json`
and IPC `{"cmd":"history"}` read the same single
`~/.config/cliamp/history.toml` store. History responses contain track metadata
and `played_at`, but no explicit provider field.

The store is local, but its contents are **provider-agnostic**, not
local-provider-only: v1.63.2 records a track after it crosses 50% playback
“regardless of provider.” Therefore Spotify tracks played through cliamp can
appear in `cliamp history`; source may sometimes be inferred from a canonical
path such as `spotify:track:...`, but that is not a provider field. See
[`maybeScrobble`](https://github.com/bjarneo/cliamp/blob/v1.63.2/ui/model/notifications.go#L121-L149)
and the
[`HistoryInfo` schema](https://github.com/bjarneo/cliamp/blob/v1.63.2/ipc/protocol.go#L153-L156).

The machine returned `[]` for `cliamp history --json --limit 3` and had no
`history.toml` at discovery time. That proves only that no qualifying cliamp
plays were persisted, not that Spotify playback is excluded.

Consequences for the later design:

- cliamp history is sufficient for qualifying plays made **through cliamp**,
  across its providers;
- it cannot be filtered reliably by provider from its schema alone;
- Spotify's `/me/player/recently-played` is still needed if the product must
  include Spotify listening performed outside cliamp;
- aggregation must deduplicate overlaps rather than assume the two histories
  are disjoint.
