# Research: Why the "Recently Played" sidebar is empty

**Date:** 2026-08-26
**Project:** `home-harlan-dev-clify` (cliamp-clify fork)
**Status:** Research only — no code changes proposed. See "Open questions" at the end for items that need user-side data to resolve.

---

## TL;DR

The "Recently Played" section in clify's Spotify/Playlists sidebar is **derived exclusively from `GET /v1/me/player/recently-played`** via `SpotifyProvider.Recent` → `unifiedRecent` → `recentRows`. The cliamp local history is fed into the same merge pipeline (`unifiedRecent`) for IPC consumers, but it is **silently dropped** before the row-derivation step, because `recentRows` requires `clify.recent.kind` / `clify.recent.target` metadata that is only attached for items the Spotify endpoint returned. Three plausible root causes — each independently sufficient to produce the observed empty sidebar — and the diagnostic commands to disambiguate are listed in §5.

The most likely single cause, given that the user has been "listening for hours" yet no rows appear, is **(a) the Spotify feed returns items but neither `context.uri` nor `track.album.id` is populated for the user's plays**, causing every item to be filtered out at `cliamp-clify/external/spotify/recent.go:154`. The diagnostic check is to dump the raw feed (§6).

---

## 1. Flow diagram: Spotify play → sidebar row

Every `file:line` is the *only* site of the gate it describes.

```
Spotify Web API
  GET /v1/me/player/recently-played?limit=50
        │
        ▼  cliamp-clify/external/spotify/recent.go:52
SpotifyProvider.Recent(ctx, limit)
        │  - cap limit to spotifyRecentPageSize=50     (recent.go:46-50)
        │  - JSON parse, items[].track.id required     (recent.go:68)
        │  - skip items with empty/malformed played_at  (recent.go:71-74)
        │
        ▼  per item
deriveRecentContext(ctxPtr, track.Album.ID)            (recent.go:80)
        │  returns ("album", id)         if context.uri is "spotify:album:<id>"
        │  returns ("playlist", id)      if context.uri is "spotify:playlist:<id>"
        │  returns ("album", albumID)    if track.Album.ID != ""  (FALLBACK)
        │  returns ("", "")              otherwise  → DROPPED below
        │
        ▼  recent.go:82-86
attach ProviderMeta { RecentKindMetaKey, RecentTargetMetaKey }   ONLY if kind != ""
        │
        ▼
emit recent.Item { Track, PlayedAt, Sources: ["spotify"] }
        │
        ╔═══════════════════════════════════════════════════════╗
        ║  IN PARALLEL — cliamp local history (unrelated path)  ║
        ║  HistorySource.Recent(ctx, limit)                     ║
        ║      cliamp-clify/recent/history_source.go:14         ║
        ║  returns recent.Item { Track, PlayedAt, ["cliamp"] }  ║
        ║  NO ProviderMeta of any kind is set.                 ║
        ║      (history_source.go:27 — only Track/PlayedAt/Src)║
        ╚═══════════════════════════════════════════════════════╝
        │
        ▼  cliamp-clify/external/spotify/recent.go:257
unifiedRecent(ctx, limit)
        │  cache check: recentCacheTTL = 30s              (recent.go:259, provider.go:101)
        │  fan out both sources concurrently              (recent.go:273-288)
        │  sort sources by name, then:
        ▼  recent/recent.go:50
recent.Merge(limit, cliamp..., spotify...)
        │  canonicalKey(track)                            (recent/recent.go:118-133)
        │      key = "spotify:track:<id>"  (lowercased)   if track.Path starts with that
        │      key = "<meta>.id=<value>"   (lowercased)   for first ProviderMeta key ending in ".id"
        │      key = ""                    otherwise
        │  if key != "" → dedupe (newer wins, sources union)   (recent.go:71-82)
        │  if key == ""  → keep BOTH (no dedup)               (recent.go:67-69)
        │  sort.SliceStable by PlayedAt desc                 (recent.go:88-90)
        │  setMetadata → inject clify.sources, clify.played_at  (recent.go:162-170)
        │  cap to limit (50)                                 (recent.go:96-98)
        ▼
cached & returned as recent.Result
        │
        ▼  cliamp-clify/external/spotify/recent.go:137
recentRows(result.Items)
        │  ┌─────────────────────────────────────────────────────────────┐
        │  │  GATE 1 — recent.go:154                                     │
        │  │  if kind == "" || target == ""  →  skip                    │
        │  │  Drops:                                                    │
        │  │    • every cliamp-history item (no ProviderMeta)           │
        │  │    • every spotify item where BOTH context.uri is empty    │
        │  │      AND track.album.id is empty                           │
        │  └─────────────────────────────────────────────────────────────┘
        │  ┌─────────────────────────────────────────────────────────────┐
        │  │  GATE 2 — recent.go:158-159                                 │
        │  │  dedup by (kind, target) → skip subsequent items            │
        │  │  Two plays of the same album / same playlist collapse      │
        │  │  to one row (newest wins, order preserved by Merge sort).  │
        │  └─────────────────────────────────────────────────────────────┘
        │  ┌─────────────────────────────────────────────────────────────┐
        │  │  GATE 3 — recent.go:170-173  (kind == "playlist")           │
        │  │  if !resolvePlaylistName(target).browsable → skip           │
        │  │  (404 → still kept under "Generated Mix" fallback,         │
        │  │   5xx/transport → kept under "Playlist <id>" retry label) │
        │  └─────────────────────────────────────────────────────────────┘
        │  ┌─────────────────────────────────────────────────────────────┐
        │  │  GATE 4 — recent.go:181-183                                 │
        │  │  cap at unifiedRecentRowCap = 25 rows                       │
        │  └─────────────────────────────────────────────────────────────┘
        │
        ▼
[]playlist.PlaylistInfo{Section: "Recently Played", ...}
        │
        ▼  provider.go:412  (and provider.go:269 for cache hit)
Playlists() returns withUnifiedRecent(slices.Clone(all))
        │
        ▼  ui/model/notifications.go:128 (section header)
        │  Sort: sectionOrder["Recently Played"] = 0    (provider.go:396-405)
        ▼
TUI sidebar renders "Recently Played" group
```

---

## 2. Exact conditions under which a row is dropped

Every drop site in the Spotify/Playlists sidebar path, with the line that performs it and the resulting behaviour.

| # | File:line | Condition | Effect |
|---|---|---|---|
| 1 | `external/spotify/recent.go:68-70` | `input.Track == nil \|\| input.Track.ID == ""` | Item silently skipped — the play is in the feed but unusable. |
| 2 | `external/spotify/recent.go:71-74` | `played_at` does not parse as RFC3339Nano | **Whole call returns error** — entire merged feed is empty for this refresh. |
| 3 | `external/spotify/recent.go:81-86` | `deriveRecentContext` returned `("", "")` | **No `ProviderMeta` is attached**, so the item later fails Gate 1. |
| 4 | `external/spotify/recent.go:107-124` (fallback branch) | `context == nil` AND `albumID == ""` | Returns `("", "")` → leads to drop #3. |
| 5 | `external/spotify/recent.go:154-156` | `kind == "" \|\| target == ""` in `recentRows` | **Item skipped.** This is the dominant drop point for local-history plays. |
| 6 | `external/spotify/recent.go:158-159` | `seen[kind,target]` already set | Subsequent plays of the same album/playlist are deduped — they don't add rows, but they do not remove the existing one. |
| 7 | `external/spotify/recent.go:170-173` | `kind == "playlist"` AND `resolvePlaylistName(target)` returns `browsable=false` | Row skipped. (Currently unreachable in production: `resolvePlaylistName` *always* returns `browsable=true` — see `recent.go:195-249` — even on 404 and on transport errors, it falls through to a labelled placeholder.) |
| 8 | `external/spotify/recent.go:181-183` | `len(rows) == unifiedRecentRowCap` (25) | Further items skipped. Not the cause for an empty list. |
| 9 | `external/spotify/recent.go:130-139` `withUnifiedRecent` | `unifiedRecent` errors out (e.g. `ensureSession` fails, both sources error) | `result.Items` is empty → `recentRows` returns no rows. The `applog.UserWarn` at `recent.go:135` only fires when the result is *partial* (one source failed, the other succeeded) — if both sources return zero data with no error, **no warning is emitted at all**. |

The merge itself (`recent/recent.go:50-100`) does **not** drop items. The local-history items remain in the merged `Result.Items` stream — they just fail Gate 1. The test `TestPlaylistsSkipsRowsWhenOnlyLocalHistoryExists` (`recent_test.go:338-367`) explicitly encodes this contract:

```go
// recent_test.go:340-341
// Simulate the Spotify source failing: the merged feed keeps only cliamp
// plays, and those must no longer produce Recently Played rows.
```

and the assertion at line 363-365 confirms the merged feed still contains the local play for IPC consumers — only the sidebar row derivation is filtered.

### The two hidden-but-critical fall-throughs

1. **Spotify source succeeds with 0 items but no error** — no `applog.UserWarn` fires. The user gets an empty "Recently Played" section silently. Compare lines 134-136:

   ```go
   // external/spotify/recent.go:134-136
   if result.Partial {
       applog.UserWarn("spotify: recently played source unavailable: %v", result.FailedSources)
   }
   ```

2. **Every item in the feed has neither `context.uri` nor `track.album.id`** — each item is added to the merged feed (so the IPC `UnifiedRecent` returns them with timestamps), but each fails the `kind != ""` test at `recent.go:154` and produces no row. No log line is emitted at this drop site.

---

## 3. Is the cliamp local history part of the problem?

**Yes, but only in the sense that the design intentionally excludes it.** It is not a bug; it is the documented contract from `recent_test.go:341`.

Evidence trail:

- **Where history is written:** `cliamp-clify/daemon.go:908-928` `recordHistory()`. Called from the player loop when a track reaches ≥50% played (`daemon.go:924`). The store is `~/.config/cliamp/history.toml` (`history/history.go:64`).
- **How it's read:** `cliamp-clify/recent/history_source.go:14-30` `HistorySource.Recent` returns `[]recent.Item` with `Sources: []string{"cliamp"}` and **no `ProviderMeta`** of any kind. The Track struct's `Path`, `Album`, `Artist`, etc. are preserved verbatim from the original track.
- **Why the row is dropped anyway:** `external/spotify/recent.go:152-153` reads only `ProviderMeta[RecentKindMetaKey]` / `ProviderMeta[RecentTargetMetaKey]`. These keys are *only ever set* inside `Recent()` itself (`recent.go:82-85`):

  ```go
  // external/spotify/recent.go:82-86
  if kind != "" {
      track.ProviderMeta = map[string]string{
          RecentKindMetaKey:   kind,
          RecentTargetMetaKey: target,
      }
  }
  ```

  There is no other writer in the repo (verified by grep: 8 total hits for `clify.recent.kind` / `RecentKindMetaKey` / `RecentTargetMetaKey` across the Go code — all in `external/spotify/recent.go` and one test assertion at `recent_test.go:80`).

- **Consequence:** Even if the user has 200 cliamp history entries from a local-only listening session, zero Recently Played sidebar rows can ever come from them. The sidebar derives rows **only from the Spotify feed.**

This matches the architecture intent stated in `docs/cliamp_clify_v2_plan.md:23-24`:
> "-Recently-Played:-songs-→-albums/playlists-(Spotify-derived-only)"

So the cliamp history is the *source of truth* for the "Recently played tracks" IPC feed, but for the *album/playlist sidebar rows* the Spotify endpoint is authoritative.

---

## 4. Is the issue (a) empty context, (b) empty cliamp history, (c) missing scope, or (d) a recent code change?

| Hypothesis | P(Bug In Code) | P(Environmental / User Account) | Notes |
|---|---|---|---|
| **(a) Spotify feed returns items but no `context.uri` AND no `track.album.id`** | Low — the fallback path at `recent.go:120-122` explicitly handles this by keying on `track.Album.ID`. The fallback only fails when **both** are empty. | **High** if the user listens primarily via the Spotify mobile/desktop client with `context` omitted, or via a third-party client (Spotify's `/v1/me/player/recently-played` historically omits `context` for some client types). | Tests cover the happy path (`recent_test.go:163-226`) and the no-context-but-has-album-id path (item `ghi` at `recent_test.go:44`, derived as `album/alb3` at `recent_test.go:72`). There is **no test fixture** for the fully-empty `{"track": {"id":"jkl"}}` path that *reaches* Gate 1 — only a unit-level check at `recent_test.go:73` confirms `("", "")` derivation. This is a real test-coverage gap (see §7). |
| **(b) cliamp history is empty** | Irrelevant for the sidebar. | Irrelevant. | The sidebar doesn't read cliamp history for row derivation. (It does read it for the IPC `UnifiedRecent` feed and for the local "Recently Played" playlist in the `local` provider — those are unaffected.) |
| **(c) User lacks the OAuth scope** | Low — `user-read-recently-played` is in `oauthScopes` at `external/spotify/session.go:222` and is requested on every code path (silent refresh `session.go:246`, persisted-token `session.go:282`, interactive `session.go:454, 466, 469`). | Only possible if the user signed in *before* the scope list included it and never re-authenticated. The endpoint is gated by `user-read-recently-played`; without it, the API returns `401` / `403`, which would propagate up to `result.FailedSources` and trigger the `applog.UserWarn` at `recent.go:135`. The user would see a footer warning. If they don't see one, this is unlikely. | `playlist-read-private` and `user-library-read` are also in the scope list (`session.go:206, 213`). |
| **(d) Recent code change silently broke the merge** | Low — `recent.Merge` is unchanged in the spotify context; the only changes that would affect rows are in `Recent` / `recentRows` / `deriveRecentContext`. None of those have been touched in a way that would silently break the empty-context case. | n/a | Grep shows `clify.recent.kind` / `clify.recent.target` are only set in `recent.go:82-86`; no fallback enrichment happens elsewhere. |

**Most likely: (a)**, with `context` populated but pointing at something unhandled (e.g. an `episode` or `show` context from a podcast — the comment at `recent.go:97-98` notes these are ignored), or with the Web API omitting `context` and the track's `album.id` also empty. The diagnostic in §6 will disambiguate in 30 seconds.

---

## 5. Diagnostic commands & log lines

These can be run by the user with no code changes.

### 5.1 Confirm the sidebar is being computed at all

The TUI surfaces a footer message whenever the source is partial:

```go
// external/spotify/recent.go:134-136
if result.Partial {
    applog.UserWarn("spotify: recently played source unavailable: %v", result.FailedSources)
}
```

`applog.UserWarn` writes **both** to the log file `~/.config/cliamp/cliamp.log` *and* to the in-memory footer buffer (`applog/applog.go:135-139`):

```go
// applog/applog.go:132-139
// UserWarn logs at warn level and pushes the same message into the footer.
func UserWarn(format string, args ...any) {
    msg := fmt.Sprintf(format, args...)
    logger.Load().Warn(msg)
    pushFooter(msg)
}
```

So the user can either:
- **Watch the in-app footer** (the in-memory buffer, drained by the TUI) while opening Spotify/Playlists — if any source failed, the message will be there.
- **`tail -f ~/.config/cliamp/cliamp.log`** for the same string, plus all other `applog.Warn` / `applog.Error` / `applog.Debug` events. `applog.Init` opens this file at startup (`applog/applog.go:68-99`).

To get more signal at debug level, the user would need to launch the binary with the log level set — see `applog.ParseLevel` (`applog/applog.go:103-116`).

### 5.2 Confirm the raw feed shape

The cleanest end-to-end check is a one-off curl against the same endpoint using the same refresh token the app uses. The user can:

1. Find the stored refresh token (location is `appdir.Dir() + "/creds.json"` per the OAuth flow at `session.go:481`; `appdir.Dir()` resolves to `~/.config/cliamp/` per `internal/appdir/`).
2. Exchange it for an access token using the same `oauthScopes`:

   ```sh
   # refresh token exchange (mirrors session.go:245-249)
   curl -s -X POST https://accounts.spotify.com/api/token \
     -d grant_type=refresh_token \
     -d refresh_token="$REFRESH" \
     -d client_id=... # the cliamp DefaultClientID
   ```

3. Hit the endpoint:

   ```sh
   curl -s -H "Authorization: Bearer $ACCESS" \
     "https://api.spotify.com/v1/me/player/recently-played?limit=50" \
     | jq '.items[] | {played_at, context, track: {id, name, album: .track.album.id}}'
   ```

   For each item, the question is: **does `context.uri` start with `spotify:album:` or `spotify:playlist:`, or is `track.album.id` non-empty?** If both are blank, that item will be dropped at `recent.go:154`. If *every* item is dropped, the sidebar will be empty.

4. The same check works for a non-`user-read-recently-played` scope problem: a 401/403 response from this curl means re-authentication is required.

### 5.3 Confirm cliamp history is or isn't being written

```sh
ls -la ~/.config/cliamp/history.toml
# if it exists, the file is non-empty, and the user has been listening locally,
# the local history is fine — and the sidebar still ignores it (by design).
wc -l ~/.config/cliamp/history.toml
head -20 ~/.config/cliamp/history.toml
```

If `history.toml` is missing, the daemon's `recordHistory` has never fired for this user (no local playback has crossed the 50% threshold, or the daemon never had a `historyStore` configured). For the *sidebar* this is irrelevant; for IPC `unified-recent` it would matter.

### 5.4 Confirm what rows `recentRows` would have produced (no test needed)

A one-shot test written against `newRecentFeedFixture` is not needed by the user. The existing test `TestPlaylistsDerivesAlbumAndPlaylistRowsFromSpotifyFeed` (`recent_test.go:163-226`) is the canonical happy path. If the user's own `feedJSON` (from 5.2) substituted in there produces a non-empty `want` list, the gate is *not* in `recentRows` — it's upstream (feed shape, scope, or auth).

---

## 6. Direct quotes for the report

Five- to ten-line snippets that anchor the most important claims above. (Re-quoted from the files for the report's own self-containment.)

### The "what's required to be a row" gate

```go
// external/spotify/recent.go:144-160
// recentRows derives deduplicated album/playlist rows from the merged feed,
// preserving its newest-first order. Plays without usable context — including
// every local cliamp history play — are skipped.
func (p *SpotifyProvider) recentRows(items []recent.Item) []playlist.PlaylistInfo {
    type rowKey struct{ kind, target string }
    seen := make(map[rowKey]struct{}, unifiedRecentRowCap)
    rows := make([]playlist.PlaylistInfo, 0, unifiedRecentRowCap)
    for _, item := range items {
        kind := item.Track.ProviderMeta[RecentKindMetaKey]
        target := item.Track.ProviderMeta[RecentTargetMetaKey]
        if kind == "" || target == "" {
            continue
        }
```

### The context-derivation rules

```go
// external/spotify/recent.go:107-124
func deriveRecentContext(context *recentCtx, albumID string) (kind, target string) {
    if context != nil {
        switch {
        case strings.HasPrefix(context.URI, "spotify:album:"):
            if id := strings.TrimPrefix(context.URI, "spotify:album:"); id != "" {
                return recentKindAlbum, id
            }
        case strings.HasPrefix(context.URI, "spotify:playlist:"):
            if id := strings.TrimPrefix(context.URI, "spotify:playlist:"); id != "" {
                return recentKindPlaylist, id
            }
        }
    }
    if albumID != "" {
        return recentKindAlbum, albumID
    }
    return "", ""
}
```

Note the explicit comment: "track radios surface as non-playable URIs and are ignored by the prefix checks" (`recent.go:97-98`). A user listening mostly to a "radio" station's child tracks will get `context.uri` like `spotify:station:...` or similar — the prefix checks will all fail, and the fallback to `track.Album.ID` will be the only remaining option.

### The documented "local history cannot produce rows" contract

```go
// external/spotify/recent_test.go:338-356
func TestPlaylistsSkipsRowsWhenOnlyLocalHistoryExists(t *testing.T) {
    f := newRecentFeedFixture(t, `{"items":[{"played_at":"2026-08-21T09:00:00Z","track":{"id":"remote","name":"Remote","type":"track","uri":"spotify:track:remote"}}]}`)
    // Simulate the Spotify source failing: the merged feed keeps only cliamp
    // plays, and those must no longer produce Recently Played rows.
    f.p.webAPIFunc = func(ctx context.Context, method, path string, query url.Values) (*http.Response, error) {
        if path == "/v1/me/player/recently-played" {
            return nil, context.DeadlineExceeded
        }
        return f.handle(ctx, method, path, query)
    }

    got, err := f.p.Playlists()
    if err != nil {
        t.Fatal(err)
    }
    for _, pl := range got {
        if pl.Section == "Recently Played" {
            t.Fatalf("local history leaked into rows: %+v", got)
        }
    }
```

### The full OAuth scope list including `user-read-recently-played`

```go
// external/spotify/session.go:201-227
// oauthScopes are the Spotify Web API scopes needed for cliamp.
// See: https://developer.spotify.com/documentation/web-api/concepts/scopes
var oauthScopes = []string{
    // Playlist browsing
    "playlist-read-collaborative",
    "playlist-read-private",
    // Playlist modification (save queue, create playlists)
    "playlist-modify-public",
    "playlist-modify-private",
    // Streaming audio
    "streaming",
    // Library (liked songs, saved albums)
    "user-library-read",
    "user-library-modify",
    // User profile
    "user-read-private",
    // Playback state (current track, queue)
    "user-read-playback-state",
    "user-modify-playback-state",
    "user-read-currently-playing",
    // Recently played / top tracks
    "user-read-recently-played",
    "user-top-read",
    // Following (artists, users)
    "user-follow-read",
    "user-follow-modify",
}
```

`playlist-read-private` and `user-library-read` (the two scopes the task brief flagged as "would be needed for non-owned playlists and saved albums") are both present and requested on every auth path (`session.go:246, 282, 454, 466, 469`). So the *code* is not missing scope coverage; the *token* in storage may be stale if the user has never re-authenticated since the scope was added.

### The merge contract (no scope for `ProviderMeta`)

```go
// recent/recent.go:118-133
func canonicalKey(track playlist.Track) string {
    if strings.HasPrefix(strings.ToLower(track.Path), "spotify:track:") {
        return strings.ToLower(track.Path)
    }
    keys := make([]string, 0, len(track.ProviderMeta))
    for key, value := range track.ProviderMeta {
        if value != "" && strings.HasSuffix(strings.ToLower(key), ".id") {
            keys = append(keys, strings.ToLower(key)+"="+value)
        }
    }
    if len(keys) == 0 {
        return ""
    }
    slices.Sort(keys)
    return keys[0]
}
```

Combined with the `RecentKindMetaKey`/`RecentTargetMetaKey` *not* ending in `.id` (deliberately — see the comment at `recent.go:24-26`), this means `clify.recent.kind` / `clify.recent.target` **do not affect the merge key at all**. Two plays of the same album/playlist from different sources are deduped by `spotify:track:<id>` if both plays have a track-level URI, but the *rows* in the sidebar are still keyed only on `(kind, target)` (`recent.go:157-159`).

---

## 7. Open questions & risks

### Open questions requiring user-side data

1. **What does the raw feed look like for this user?** (See §5.2 — the curl exchange is the only definitive way to know whether `context.uri` and/or `track.album.id` are present in the user's account.)
2. **Was the OAuth flow re-run after the scope list grew?** The scope list in `session.go:201-227` is the *current* one. If the user signed in months ago when a different subset was in effect, their stored refresh token won't have `user-read-recently-played`. The user can confirm by re-running sign-in and re-checking the consent screen.
3. **Are plays happening from inside the cliamp TUI (which would hit the same `/v1/me/player/recently-played` endpoint that is the source for the sidebar) or only from the Spotify mobile/desktop app?** Items played *only* outside cliamp still appear in the endpoint (Spotify populates it from the user's overall listening history), but the `context.uri` for an external play reflects where the play *actually happened* — which may be a context the cliamp prefix checks do not recognise (e.g. `spotify:station:...` from a radio session, or `spotify:user:<id>:collection` from "Liked Songs" which the prefix checks do handle — it parses as `spotify:user:` and falls through to the `albumID` fallback).
4. **Is the user listening to a lot of podcasts?** The endpoint does include podcast episodes in the feed, and those have `track.type == "episode"`, no `album`, and a `context.uri` of `spotify:show:...` or `spotify:episode:...` — none of which `deriveRecentContext` handles. This is a structural reason for an empty sidebar even with healthy feed data.

### Risks / gaps

1. **Test coverage gap:** No test fixture exercises the "context empty AND album.id empty" path through `recentRows` (i.e. an item where `deriveRecentContext` returns `("", "")`). The closest is `TestRecentRequestsEndpointAndMapsTracks` at `recent_test.go:32-88`, which confirms the *derivation* produces `("", "")` for the `jkl` track (`recent_test.go:73`) but does not assert that this item is filtered out of the sidebar rows. A future contributor could easily regress Gate 1 by reading a different metadata key.
2. **Silent failure mode:** The `applog.UserWarn` at `recent.go:135` only fires on `result.Partial == true`. If both sources return *zero data with no error* (e.g. valid token, scope granted, empty feed), the user gets no signal. A `Debug`-level log on `len(result.Items) == 0 && !result.Partial` would help future debugging.
3. **Cache invalidation:** `recentCacheTTL = 30s` (`provider.go:101`) plus `playlistListCacheTTL = 5min` (`provider.go:100`) means the user has to wait up to 5 min 30 s after a play for it to surface. This is not the cause of a *persistently* empty section, but it is the cause of a "I just played something and it didn't show up" perception.
4. **Fallback to `track.Album.ID` always produces "album" rows:** Even if the play was a non-owned, non-saved-album track (e.g. from Discover Weekly), the row will be a synthetic album row with `__clify_album__<id>`, with name from `track.Album` (which the feed does populate for tracks) or `"Album <id>"` (`recent.go:163-168`). This is a deliberate design choice (see `docs/cliamp_clify_v2_plan.md:23-24` "Spotify-derived-only"), but it is worth noting for the user: **non-owned, non-saved albums do still produce rows** under this fallback — so the absence of rows implies the feed items lack `album.id` entirely, not that the user lacks saved-album privileges.
5. **No replay of recent plays the cliamp daemon recorded but that originated from non-Spotify sources.** The `HistorySource` items do carry `Album`, `Title`, `Artist` fields (from the local track tags) but no Spotify IDs. There is no code path that would let a local-history play surface as a Spotify album/playlist row even if that local track *did* come from Spotify originally (e.g. a downloaded Spotify track played locally). This is consistent with the v2 plan but worth flagging for the user.
