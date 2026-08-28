# Research: Adding a "Saved Albums" section to the Spotify / Playlists sidebar

**Project:** `home-harlan-dev-clify` (cliamp-clify fork)
**Status:** Research only — no code proposed
**Date:** 2026-08-26

---

## 1. Current architecture (relevant only)

The TUI's left "Spotify / Playlists" pane is the provider list rendered by `Model.renderProviderList()` in `cliamp-clify/ui/model/view.go:822` (delegated from `View()` at `view.go:143` via `mainSections`). Each row is a `playlist.PlaylistInfo` and section headers are emitted automatically by the renderer whenever it sees a new `Section` value — there is no explicit allow-list of section names.

Key data shape (`cliamp-clify/playlist/provider.go:19-25`):

```go
type PlaylistInfo struct {
    ID           string
    Name         string
    TrackCount   int
    DurationSecs int
    Section      string
}
```

Provider entry point: `SpotifyProvider.Playlists()` at `cliamp-clify/external/spotify/provider.go:260-413`. It builds a flat `[]playlist.PlaylistInfo`, sorts by a `sectionOrder` map at `provider.go:396-405`, and prepends a `Recently Played` group produced by `withUnifiedRecent(...)` → `recentRows(...)`. Cached for `playlistListCacheTTL = 5 * time.Minute` (`provider.go:100`).

```go
// cliamp-clify/external/spotify/provider.go:396-405
sectionOrder := map[string]int{
    "Recently Played":    0,
    madeForYouSection:    1,    // "Made For You"
    "Library":            2,
    "Your playlists":     3,
    "Followed playlists": 4,
}
sort.SliceStable(all, func(i, j int) bool {
    return sectionOrder[all[i].Section] < sectionOrder[all[j].Section]
})
```

Saved albums are **already** paginated and cached — `SpotifyProvider.savedAlbums(ctx)` at `cliamp-clify/external/spotify/albums.go:107-160` walks `/v1/me/albums` in pages of `spotifyAlbumPageSize = 50` (`albums.go:19`) and caches in `p.albumCache` for `albumListCacheTTL = 5 * time.Minute` (`albums.go:20`). The full cache is loaded up-front; pagination is used only at render time inside `AlbumList(sortType, offset, size)` at `albums.go:165-200`. The two methods that today expose saved albums to the UI are `AlbumBrowser.AlbumList` and `AlbumTrackLoader.AlbumTracks` (`albums.go:165`, `albums.go:205`); they are wired into the separate `navBrowseModeByAlbum` view (driven by `N` → `Albums` → sort → enter; see `ui/model/keys_nav.go:234-317` and `ui/model/commands.go:356-367`).

The existing "Recently Played" synthetic-id scheme is `__clify_album__<id>` (`provider.go:106`) and `Tracks()` routes any `playlistID` with that prefix to `albumTracks(...)` (`provider.go:422`); `albumTracks(...)` is also reached directly via `AlbumTracks(albumID)` (`albums.go:205-210`).

The IPC protocol serialises a `PlaylistInfo` shape at `cliamp-clify/ipc/protocol.go:110-119` (carries `ID` / `Name` / `Section` / `TrackCount` / `DurationSecs` / `Favoritable` / `Favorite`); `cliamp-clify/ui/model/ipc_extended.go:376-390` and `cliamp-clify/daemon.go:773-787` already project `playlist.PlaylistInfo` into the IPC payload via `ipcProviderPlaylistInfos` and `providerPlaylistInfos`.

---

## 2. Files that would need to change

A minimal change to "fold saved albums into the Playlists view" is purely additive inside `external/spotify/`, plus a section header entry. Below is the file/line hit-list.

### 2.1 Core provider change

| File:line | One-line description |
|---|---|
| `cliamp-clify/external/spotify/provider.go:106` | Add (or reuse) a saved-album synthetic id prefix — see §3. |
| `cliamp-clify/external/spotify/provider.go:293-308` (right after the `Top Tracks` / `Your Music` rows) | Inject saved-album rows into the `[]playlist.PlaylistInfo` slice using `savedAlbums(ctx)` (existing pagination + cache, no new HTTP work). |
| `cliamp-clify/external/spotify/provider.go:396-405` | Add a new `"Albums"` entry to `sectionOrder` (or accept the existing "no rank = tail" fallback — see §4). |

Notes:
- `Playlists()` already takes the 5-minute `playlistListCacheTTL` (`provider.go:267-271`) and the saved-albums `AlbumList` already takes its own 5-minute `albumListCacheTTL` (`albums.go:20`). Two overlapping caches, but no extra HTTP work is added (one `/v1/me/albums` walk per 5 min, same as today).
- `Playlists()` already runs inside a `5*time.Minute` timeout (`provider.go:278`), and `savedAlbums` itself is bounded by the same client context (`albums.go:122-128`). No timeout plumbing change is required.

### 2.2 Routing change

| File:line | One-line description |
|---|---|
| `cliamp-clify/external/spotify/provider.go:421-423` | `Tracks(id)` already branches on `unifiedAlbumIDPrefix`; see §3 for whether saved-album rows need a separate branch. |

### 2.3 UI (no code change required)

`renderProviderList()` at `cliamp-clify/ui/model/view.go:662-787` is **header-agnostic**: it auto-emits a `labeledSeparator("  ", p.Section)` whenever `p.Section != prevSection` (`view.go:772-778`) and likewise in filter mode (`view.go:706-712`). `playlistLabel` at `view.go:97-110` renders `Name` plus optional `TrackCount` / `DurationSecs`. **No UI-side changes are required** for an "Albums" section header to appear — adding the row with a non-empty `Section` value is sufficient.

If a distinct visual treatment is wanted (e.g. a "💿" glyph, an italic style, or different cursor highlight), `providerRowStyle` at `view.go:73-81` and `view_helpers_test.go:57-95` are the only two call sites that would need touching. The existing test `TestPlaylistLabel` would need an extension case for any new branch.

### 2.4 No changes required

- `cliamp-clify/playlist/provider.go` — `PlaylistInfo` already carries the required fields.
- `cliamp-clify/provider/interfaces.go` — no interface change; `AlbumBrowser` / `AlbumTrackLoader` are *used* but their signatures are sufficient.
- `cliamp-clify/ipc/protocol.go` — `ipc.PlaylistInfo` (lines 110-119) is a superset of the new row's fields.
- `cliamp-clify/ui/model/inline_overlays_nav.go` — only handles the separate `navBrowseModeByAlbum` screen (`view_nav.go`); unrelated.
- `cliamp-clify/ui/model/keys_nav.go` — the only `Enter` path that loads tracks is `handleProviderListEnter` which dispatches to `Tracks(id)` for the active provider; both synthetic-id routes already flow through there.

---

## 3. Proposed id scheme for saved-album rows

### 3.1 Option A — reuse `__clify_album__` (recommended)

The cleanest approach is to **reuse the existing `__clify_album__` prefix** and rely on Spotify-album-id uniqueness. The synthetic-id space is already shared between two sources:

- `recentRows` writes `__clify_album__<id>` for **recently-played** albums (deduped at `recent.go:158-159` against the merged feed; `recent.go:147-186`).
- `provider.Playlists()` would write `__clify_album__<id>` for **saved** albums.

A user with `alb42` both recently played and saved would produce one row (the `recentRows` one — it is prepended by `withUnifiedRecent` at `provider.go:269` and `provider.go:411` *before* the saved-album block is appended). The dedup is currently inside `recentRows` itself, so duplicates across the two sources are not blocked today. Two small risks:

1. **Name disagreement**: `recentRows` sets `row.Name = item.Track.Album` (or `"Album <id>"` fallback) at `recent.go:163-168`. Saved-album rows would set `row.Name = album.Name` (from `albumInfoFromAlbum`, `albums.go:57-85`). For a recently-played album that is also saved, the *saved* name is the more accurate one. To make the saved row win, the simplest path is to dedup at the `Playlists()` level: skip any `__clify_album__<id>` row whose id is already in the recently-played prepended slice.
2. **Different Section**: a `Recently Played` row is `Section: "Recently Played"`; a saved-album row would be `Section: "Albums"`. The dedup logic in (1) must use the *id* as the dedup key (not the section), and must run *after* the recently-played prefix is prepended. The natural place is inside `Playlists()` itself, after `withUnifiedRecent(...)` returns but before the `sectionOrder` sort at `provider.go:402-405`.

### 3.2 Option B — separate prefix `__clify_saved_album__`

Define a new constant alongside `unifiedAlbumIDPrefix` (next to `provider.go:106`), e.g. `savedAlbumIDPrefix = "__clify_saved_album__"`. Then:

- `Tracks()` at `provider.go:421-423` needs a second branch: `if strings.HasPrefix(playlistID, savedAlbumIDPrefix) { return p.albumTracks(strings.TrimPrefix(playlistID, savedAlbumIDPrefix)) }`.
- `albumTracks(...)` cache key (`provider.go:524`) needs to consider the prefix to avoid colliding with `__clify_album__<id>` cache entries; in practice this is auto-handled because the cache key *is* the synthetic id (`provider.go:524` and `provider.go:570`).
- Tests for the new branch in `recent_test.go` / `albums_test.go` would need extension.

Option B is more explicit and makes the cache semantics trivially correct, but it doubles the synthetic-id namespace. Option A is closer to the existing design and reuses both the cache key and the test surface. **Recommendation: Option A** with a small `Playlists()`-level dedup.

### 3.3 Naming / display

`savedAlbums` returns `savedAlbumEntry{info: provider.AlbumInfo, addedAt}` (`albums.go:37-40`). The fields available for a row are `ID`, `Name`, `Artist`, `ArtistID`, `Year`, `TrackCount`, `Genre` (`albums.go:76-84`). The `PlaylistInfo` row needs only `ID`, `Name`, `TrackCount`, `Section`. Two minor adjustments:

- `Name` should probably be the artist-prefixed form (matching how the Albums nav view renders rows at `inline_overlays_nav.go:113-119`: `"%s — %s (%d)"` → `Name — Artist (Year)`). The current `playlistLabel` at `view.go:97-110` only formats `Name` + `TrackCount`; either accept the simpler form, or set `Name` itself to `"Name — Artist"` at injection time.
- `TrackCount` is available from `albumInfoFromAlbum` and can be passed through directly so the row shows `N tracks` in the sidebar.

---

## 4. UI section-header impact

`renderProviderList()` at `cliamp-clify/ui/model/view.go:662-787` is **fully data-driven** for non-radio providers:

```go
// cliamp-clify/ui/model/view.go:732-734
hasSections := !isRadio && slices.ContainsFunc(m.providerLists, func(p playlist.PlaylistInfo) bool {
    return p.Section != ""
})
```

```go
// cliamp-clify/ui/model/view.go:772-778
} else if hasSections && p.Section != prevSection {
    header := labeledSeparator("  ", p.Section)
    if len(lines) < visibleBudget {
        lines = append(lines, dimStyle.Render(header))
    }
    prevSection = p.Section
}
```

The filter-mode path at `view.go:700-712` is identical. **Any new `Section` value auto-emits a header** without UI changes. There is no allow-list of section names anywhere in `ui/model/`.

The only place a section name is *required* is the `sectionOrder` map at `provider.go:396-405` — but the `sort.SliceStable` falls back to a zero value for unknown sections, meaning the new "Albums" section would sort to the end by default. To position it deliberately (the v2 plan in §5 prescribes "Library and Your playlists follow"), the entry needs to be added to `sectionOrder`.

`headerAware` scroll math in `view.go:739-741` (`providerRowsFromScroll`) and the filter-mode `provSearchRenderedRows` at `view.go:800-820` both already count header rows correctly — they will pick up the new header automatically. The scroll-budget tests at `ui/model/provider_sections_test.go:33-65` (`TestProviderListScrollKeepsFollowedHeaderAndCursorRow`) and `provider_sections_test.go:70-92` (`TestProviderFilterShowsSectionHeaders`) cover the four-section case today; a new five-section case would be worth adding (and `sectionedProviderLists()` at `provider_sections_test.go:13-27` would gain an `Albums` fixture entry).

---

## 5. Existing plans and pre-existing direction

### 5.1 `docs/cliamp_clify_v2_plan.md` (the v2 plan, 2026)

This document (§2 "Recently Played: songs → albums/playlists", `cliamp-clify_v2_plan.md:23-47`) describes the **Recently Played → albums/playlists** injection (already implemented and shipped, per `CHANGELOG.md:12-15`). It does **not** cover folding saved albums into the Playlists view, but its pre-existing section-order list is the closest design precedent:

```
Recently Played  →  Made For You  →  Library  →  Your playlists  →  Followed playlists
```

§3 of the same plan ("Fix broken/missing Followed playlists", `cliamp-clify_v2_plan.md:49-67`) introduces the v2 section-rank map and the header-aware scroll math — both of which are now in place and would carry over to a new "Albums" section unchanged.

### 5.2 `docs/recently_played_research_2026-08-26.md`

This is a research report (also dated 2026-08-26) about the *empty* "Recently Played" sidebar; it establishes the full data flow for `recentRows` → `withUnifiedRecent` → `Playlists`. It does not address saved-album surfacing.

### 5.3 `cliamp-clify/docs/spotify.md`

The user-facing doc explicitly says (`spotify.md:72`):

> Your **saved albums** are available through the album browser (`b`/browse and sort). Press `b` while Spotify is the active provider, choose **Albums**, and pick a sort — `Recently Added` (default), `By Name`, or `By Artist`. Enter opens the album's track list. Albums you've *saved* in Spotify appear here only if you've liked/followed them in the app.

This is the existing contract. Folding saved albums into the Playlists pane would *augment* (not replace) the album browser, and `spotify.md` would need a follow-up sentence to that effect.

### 5.4 `CHANGELOG.md` ([Unreleased])

`CHANGELOG.md:12-15`:

> Spotified **saved albums** browse in the cliamp-clify TUI: the Spotify provider now implements `provider.AlbumBrowser` / `provider.AlbumTrackLoader` (`external/spotify/albums.go`), so saved albums appear in the album browser with `Recently Added` / `By Name` / `By Artist` sorts (`user-library-read`).

This is the *Albums nav view* work that already shipped. Surfacing saved albums in the Playlists pane is a follow-on that would add a new bullet under `[Unreleased] / Added`.

### 5.5 `docs/cliamp_clify_fork_plan.md`

§2 of this plan (`cliamp_clify_fork_plan.md:16-32`, `207-225`, `313`, `416`) describes the original v1 ordering: `Recently Played → Library → Your playlists → Followed playlists`, and §2 also calls out that the synthetic playlist must precede Library. The same ordering constraint should apply to a new "Albums" section; the question is whether it sits *above* Library (a "library of albums") or *below* Your playlists (an extension of "things I own"). Nothing in this plan answers that — it is an open design question for the user.

### 5.6 `packages/clify/unified_library.py:155-206`

The Python side already treats `saved_albums` as a first-class library section (`unified_library.py:189` registers it as a section, and `clify_cli.py:14` exposes it as the "Saved Albums" header). So the *data model* on the Python side already accepts "saved albums is a sibling of Liked Songs / Top Tracks". The TUI pane would be the natural place to mirror that hierarchy.

---

## 6. Test impact

The change has narrow blast radius. Tests that currently pin ordering will need to grow new expectations; tests that pin **count** of returned rows will need to extend their fixture.

### 6.1 Must update (assert on count / exact order of `Playlists()`)

| File:line | Test | What would change |
|---|---|---|
| `cliamp-clify/external/spotify/provider_nonwindows_test.go:21-71` | `TestPlaylistsIncludesFollowedPlaylists` | The fixture only stubs `/v1/me`, `/v1/me/tracks`, `/v1/me/playlists`; it does **not** stub `/v1/me/albums`. Adding the new section requires either (a) extending the `roundTripFunc` to handle `/v1/me/albums` and the saved-album rows become part of the assertion, or (b) keeping the test focused on Library / Your playlists / Followed playlists and explicitly excluding the Albums section from the count assertion. |
| `cliamp-clify/external/spotify/generated_test.go:104-146` | `TestPlaylistsBuildsMadeForYouSectionInOrder` | Same shape as 6.1.1 — extends `newGeneratedFixture` (`generated_test.go:79-102`) to handle `/v1/me/albums`. The `want` list at lines 121-126 enumerates the exact order and would grow new "Albums" rows. |
| `cliamp-clify/external/spotify/recent_test.go:163-226` | `TestPlaylistsDerivesAlbumAndPlaylistRowsFromSpotifyFeed` | Same fixture extension (`newRecentFeedFixture` returns). New saved-album rows would be added to `want` at lines 177-184. |
| `cliamp-clify/external/spotify/recent_test.go:369-398` | `TestRecentRowsCapAtTwentyFive` | This test pins the 25-row cap on `Recently Played`. Adding a saved-album block changes neither the cap nor the existing rows, so this test should remain green *if* `/v1/me/albums` is added to the fixture's allow-list (otherwise the test will fail with an "unexpected path" error). |
| `cliamp-clify/external/spotify/artists_test.go:169-197` | `TestTopTracksRowAndResolution` | Iterates rows to look for `TOP TRACKS`; not affected by an added "Albums" section as long as the iteration tolerates the extra rows. |

### 6.2 Must extend (render-side tests)

| File:line | Test | What would change |
|---|---|---|
| `cliamp-clify/ui/model/provider_sections_test.go:13-27` | `sectionedProviderLists` fixture | Add an `Albums` section with a row or two. |
| `cliamp-clify/ui/model/provider_sections_test.go:33-65` | `TestProviderListScrollKeepsFollowedHeaderAndCursorRow` | The `visible` cases (6/7/9) target the *Followed playlists* tail; with a new "Albums" group the math stays correct but the test should add a case at the *Albums* tail to confirm the new header is preserved on scroll. |
| `cliamp-clify/ui/model/provider_sections_test.go:70-92` | `TestProviderFilterShowsSectionHeaders` | Should be extended with an "Albums" row to confirm filter mode emits the Albums header. |
| `cliamp-clify/ui/model/unified_recent_test.go:27-67` | `TestRecentlyPlayedSectionRendersDerivedRowsFirst` | Not directly affected (it uses a stub provider with hard-coded `providerLists`); will stay green. |

### 6.3 No change expected (sanity check)

| File:line | Test |
|---|---|
| `cliamp-clify/external/spotify/albums_test.go:65-235` | All five `AlbumList*` and `TestAlbumTracksMapsAlbumEndpoint` tests use the `AlbumList` / `AlbumTracks` interfaces directly; unaffected by a `Playlists()` extension. |
| `cliamp-clify/daemon_ipc_test.go:73-90` | The `daemonWritableProvider` mock returns `nil, nil` from `Playlists`; unaffected. |
| `cliamp-clify/ui/model/help_test.go:10-...` | Categorised help test; unaffected. |
| `cliamp-clify/ui/model/view_state_test.go:140-...` | View-state pinning; unaffected. |
| `cliamp-clify/ui/model/layout_test.go:121` | Layout test; unaffected. |

### 6.4 `__clify_album__` collision test (Option A only)

If Option A is taken, `recent_test.go:179-180`, `recent_test.go:214`, `recent_test.go:250`, `recent_test.go:395` all assert on `__clify_album__<id>` ids. A deduped merged `Playlists()` would change the row order and the *count* of `Recently Played` rows when an album appears both in the feed and in the saved library. The simplest fix is to keep dedup inside `Playlists()` keyed on the *id only*, and update these tests to assert dedup behaviour explicitly (this is itself a useful regression test).

---

## 7. Open questions & risks

1. **Where does "Albums" sit in the section order?** The v2 plan puts `Library` after `Made For You` and before `Your playlists` (`cliamp-clify_v2_plan.md:23-24`, `provider.go:396-405`). The Python `unified_library.py:155-206` puts `saved_albums` *after* `your_playlists` and *before* `top_tracks`. Two reasonable options: (a) slot Albums between `Library` and `Your playlists` (most natural — saved albums *are* a library sub-collection), or (b) slot it as the new last group. (a) is the lower-risk choice because it leaves the existing tail (`Your playlists`, `Followed playlists`) untouched and only shifts `sectionOrder` indices above it by one. The screenshot in the user's task brief shows the order should match ROADMAP §271-282 ("Recently Played → Library → Your Playlists"), so (a) is also consistent with the visual spec.

2. **Library size / pagination.** The `Playlists()` view today is not paginated (it returns everything; the UI scrolls). Users with thousands of saved albums would see a *very* long "Albums" list. The `AlbumList` interface at `provider/interfaces.go:26-30` exists precisely for paging, but the Playlists pane does not currently use it. A long "Albums" block will also force the existing `providerRowsFromScroll` (`view.go:739`) to scroll hard. Risk: in the worst case, the user can no longer reach the bottom of `Followed playlists`. The `sectionedProviderLists` test at `provider_sections_test.go:33-65` already proves the header-aware scroll math survives header rows, so the *mechanism* is fine; only the worst-case *user experience* may need a follow-up cap (e.g. show first 50 saved albums with a `… +N more` placeholder row, mirroring the convention from `Radio` or `qobuz` if any).

3. **Snapshot caching vs. saved-album caching.** `Playlists()` is cached for 5 minutes (`provider.go:100`); `savedAlbums` is also cached for 5 minutes (`albums.go:20`). They are independent, so the Albums block could appear "stale" relative to the Recently Played group. This is identical to today's behaviour for Recently Played and is probably acceptable. Calling out for visibility.

4. **Album art URL.** `provider.AlbumInfo` at `albums.go:76-84` and `playlist.PlaylistInfo` at `playlist/provider.go:19-25` both **do not** carry an album-art URL. The `TrackInfo` IPC type does (`ipc/protocol.go:98` — `AlbumArtURL`), but the *playlist row* type does not. The TUI's `playlistLabel` at `view.go:97-110` doesn't fetch or display art for the sidebar; it would only show `Name · N tracks`. This is consistent with how `Your Music`, `Top Tracks`, and existing playlists render today. The art would be fetched on `Enter` (when the actual `Track` list is loaded) via `Tracks(id)` → `albumTracks(...)` (`provider.go:522-579`), which already returns `playlist.Track` with the full `ProviderMeta` (including art on the *track* level, not the playlist level). No art fetching change is needed at the row level.

5. **Refresh on `/v1/me/albums` failure.** `Playlists()` returns an error if any single sub-fetch fails (`provider.go:294-298` for `/v1/me/tracks`, `provider.go:336-340` for `/v1/me/playlists`). If `/v1/me/albums` fails (e.g. transient network), the right behaviour is "show the rest of the lists but log + footer warn for albums". This is a policy decision: do we treat albums as a hard error (matching `/v1/me/tracks`) or as a soft partial (matching the `recent.go:130-139` precedent)? The existing `withUnifiedRecent` soft-fails (`recent.go:134-136` only warns on `result.Partial`), which is the better precedent for a non-essential library.

6. **Cache invalidation on `Refresh()`.** `Playlists()` checks `p.listCache` and `p.listCacheAt` (`provider.go:267-271`); `Refresh()` already calls `p.resetSessionScopedStateLocked` which presumably drops both caches. Need to verify that `p.albumCache` is also dropped by `Refresh()` — currently unverified in this research. (Easy to confirm by reading `provider.go:140-160` if needed.) If `Refresh()` does **not** drop `p.albumCache`, then Ctrl+R will refresh Recently Played / playlists but not saved albums, which would be inconsistent.

7. **Idempotence with `recentRows`.** The dedup contract for `recentRows` is "two plays of the same album collapse to one row" (`recent.go:158-159`). If Option A reuses `__clify_album__<id>`, then a *saved album that the user also recently played* will produce a row from `recentRows` (because `recentRows` runs first via `withUnifiedRecent` at `provider.go:269`/`provider.go:411`) and *no* corresponding saved-album row (because the dedup logic at the `Playlists()` level sees the id already in the recently-played slice). The recently-played row carries `Section: "Recently Played"`; the saved row would have carried `Section: "Albums"`. The user gets one row, under "Recently Played", with the *recent* name (from `item.Track.Album`), not the *saved* name (from `albumInfoFromAlbum`). That is a small visible artefact and the user may or may not care. Option B sidesteps it.

8. **Test fixture explosion.** The four `TestPlaylists*` tests in `recent_test.go` and `generated_test.go` already each build a custom `webAPIFunc`. Adding `/v1/me/albums` to four separate fixtures is duplicative; consider a small helper in `albums_test.go` (or a new shared test helper) to keep the fixtures DRY.

9. **Docs surface area.** `cliamp-clify/docs/spotify.md:68-77` lists every section. It currently says "Your **saved albums** are available through the album browser" (`spotify.md:72`). The line must be updated to reflect the new dual location. Likewise `CHANGELOG.md:12-15` would gain a follow-up bullet; `site/index.html` is not in this repo's `site/` (it is mentioned in `AGENTS.md` for cliamp upstream), so check upstream before editing. `ROADMAP.md:271-282` describes the v1.6 plan that the screenshot is from; it would gain a one-line update noting that the TUI pane now also includes saved albums in the Playlists view.

10. **CLAUDE.md "Prefer in-place edits" rule.** The cliamp-clify `AGENTS.md` (`/home/harlan/dev/clify/cliamp-clify/AGENTS.md`) requires minimal diffs and discourages speculative abstractions. The change here is small enough to honour that — most of the work is in `provider.go` (one new block, one new `sectionOrder` entry) plus a handful of test fixture extensions.

---

## 8. Evidence appendix (key snippets)

### 8.1 The `Playlists()` build, with `sectionOrder` at the tail

```go
// cliamp-clify/external/spotify/provider.go:391-405
// Group playlists by section so the UI can emit one header per group.
// Recently Played rows are injected ahead of the sorted list by
// withUnifiedRecent; the rank entry keeps that ordering explicit should
// the combined list ever be re-sorted.
sectionOrder := map[string]int{
    "Recently Played":    0,
    madeForYouSection:    1,
    "Library":            2,
    "Your playlists":     3,
    "Followed playlists": 4,
}
sort.SliceStable(all, func(i, j int) bool {
    return sectionOrder[all[i].Section] < sectionOrder[all[j].Section]
})
```

### 8.2 Existing synthetic-id scheme and the `Tracks()` dispatch

```go
// cliamp-clify/external/spotify/provider.go:104-106
// unifiedAlbumIDPrefix marks synthetic Recently Played album rows; the
// remainder is the Spotify album id served by /v1/albums/{id}/tracks.
unifiedAlbumIDPrefix = "__clify_album__"
```

```go
// cliamp-clify/external/spotify/provider.go:418-424
func (p *SpotifyProvider) Tracks(playlistID string) ([]playlist.Track, error) {
    if err := p.ensureSession(); err != nil {
        return nil, err
    }
    if strings.HasPrefix(playlistID, unifiedAlbumIDPrefix) {
        return p.albumTracks(strings.TrimPrefix(playlistID, unifiedAlbumIDPrefix))
    }
```

### 8.3 `recentRows` for the Recently Played injection (and the prefix it uses)

```go
// cliamp-clify/external/spotify/recent.go:147-186
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
        key := rowKey{kind, target}
        if _, dup := seen[key]; dup {
            continue
        }
        row := playlist.PlaylistInfo{Section: "Recently Played"}
        switch kind {
        case recentKindAlbum:
            row.ID = unifiedAlbumIDPrefix + target
            row.Name = item.Track.Album
            if row.Name == "" {
                row.Name = "Album " + target
            }
```

### 8.4 The data shape already available for a saved-album row

```go
// cliamp-clify/external/spotify/albums.go:57-85
func albumInfoFromAlbum(alb spotifyAlbumObject) provider.AlbumInfo {
    var year int
    if len(alb.ReleaseDate) >= 4 {
        if y, err := strconv.Atoi(alb.ReleaseDate[:4]); err == nil {
            year = y
        }
    }
    artistNames := make([]string, len(alb.Artists))
    artistID := ""
    for i, a := range alb.Artists {
        artistNames[i] = a.Name
        if i == 0 {
            artistID = a.ID
        }
    }
    genre := ""
    if len(alb.Genres) > 0 {
        genre = alb.Genres[0]
    }
    return provider.AlbumInfo{
        ID:         alb.ID,
        Name:       alb.Name,
        Artist:     strings.Join(artistNames, ", "),
        ArtistID:   artistID,
        Year:       year,
        TrackCount: alb.TotalTracks,
        Genre:      genre,
    }
}
```

### 8.5 The UI is data-driven for non-radio sections

```go
// cliamp-clify/ui/model/view.go:732-734
hasSections := !isRadio && slices.ContainsFunc(m.providerLists, func(p playlist.PlaylistInfo) bool {
    return p.Section != ""
})
```

```go
// cliamp-clify/ui/model/view.go:772-778
} else if hasSections && p.Section != prevSection {
    header := labeledSeparator("  ", p.Section)
    if len(lines) < visibleBudget {
        lines = append(lines, dimStyle.Render(header))
    }
    prevSection = p.Section
}
```

### 8.6 The IPC projection already works for any `playlist.PlaylistInfo`

```go
// cliamp-clify/ipc/protocol.go:110-119
type PlaylistInfo struct {
    ID           string `json:"id"`
    Name         string `json:"name"`
    Provider     string `json:"provider"`
    Section      string `json:"section,omitempty"`
    TrackCount   int    `json:"track_count,omitempty"`
    DurationSecs int    `json:"duration_secs,omitempty"`
    Favoritable  bool   `json:"favoritable,omitempty"`
    Favorite     bool   `json:"favorite,omitempty"`
}
```

```go
// cliamp-clify/ui/model/ipc_extended.go:376-390
items[i] = ipc.PlaylistInfo{ID: list.ID, Name: list.Name, Provider: entry.Key, Section: list.Section, TrackCount: list.TrackCount, DurationSecs: list.DurationSecs}
```

### 8.7 Tests that pin the current `sectionOrder` ordering

```go
// cliamp-clify/external/spotify/provider_nonwindows_test.go:54-70
want := []struct {
    id      string
    section string
}{
    {id: "TOP TRACKS", section: "Made For You"},
    {id: "YOUR MUSIC", section: "Library"},
    {id: "owned", section: "Your playlists"},
    {id: "followed", section: "Followed playlists"},
}
```

```go
// cliamp-clify/external/spotify/generated_test.go:121-126
add("TOP TRACKS", "Top Tracks", "Made For You")
add("37i9dQZF1E8XDailyMix", "Daily Mix 1", "Made For You")
add("37i9dQZF1ExUserOwned", "Discover Weekly", "Made For You")
add("YOUR MUSIC", "Your Music", "Library")
add("regular1", "Buddy List", "Your playlists")
add("followed1", "Followed Thing", "Followed playlists")
```

```go
// cliamp-clify/external/spotify/recent_test.go:177-184
want := []struct{ id, name, section string }{
    {"p1", "Named p1", "Recently Played"},
    {"__clify_album__beta", "Beta", "Recently Played"},
    {"__clify_album__delta", "Delta", "Recently Played"},
    {"TOP TRACKS", "Top Tracks", "Made For You"},
    {"YOUR MUSIC", "Your Music", "Library"},
    {"mine", "Mine", "Your playlists"},
}
```

### 8.8 v2 plan: section-ordering precedent

```
// docs/cliamp_clify_v2_plan.md:23-47
## 2. Recently Played: songs → albums/playlists (Spotify-derived only)
...
- Replace the single `"All sources"` row injected by `withUnifiedRecent()`
  with these rows under Section `"Recently Played"`. Delete the
  `UnifiedRecentPlaylistID` branch in `Tracks()`.
- Opening rows:
  - Playlist rows reuse the existing `/v1/playlists/{id}/items` path.
  - Album rows get a new branch keyed on a synthetic ID prefix
    (`__clify_album__<id>`) → paginated `GET /v1/albums/{id}/tracks`.
```

### 8.9 v2 plan: header-blind scroll fix (already shipped)

```
// docs/cliamp_clify_v2_plan.md:49-67
## 3. Fix broken/missing "Followed playlists"
...
- Header-blind scroll math for non-radio lists (`ui/model/view.go`): the
  naive `provCursor - visibleBudget + 1` branch ignores section headers, so
  bottom-of-list rows and the Followed header are clipped. Route the
  non-radio branch through header-aware `providerRowsFromScroll` (as the
  radio branch does) and clamp via `providerMaybeAdjustScroll` after
  `playlistsLoadedMsg` (covers Ctrl+R refresh).
```

### 8.10 Python side already treats saved albums as a library section

```python
# packages/clify/unified_library.py:187-202
for key, method, label in (
    ("saved_tracks", "get_saved_tracks", "spotify.saved"),
    ("saved_albums", "get_saved_albums", "spotify.saved"),
    ("top_tracks", "get_top_tracks", "spotify.top"),
    ("top_artists", "get_top_artists", "spotify.top"),
):
```

```python
# packages/clify/clify_cli.py:10-14
(
    ("library", "Library"),
    ...
    ("saved_albums", "Saved Albums"),
```

### 8.11 User-facing doc explicitly references the current (browser-only) surface

```
// cliamp-clify/docs/spotify.md:68-72
Your Spotify playlists are listed in the provider panel under `── Library ──`,
`── Your playlists ──`, and `── Followed playlists ──` headers. Navigate with
the arrow keys and press `Enter` to load one. Tracks are streamed through
cliamp's audio pipeline, so EQ, visualizer, mono, and all other effects work
exactly as with local files.

Your **saved albums** are available through the album browser (`b`/browse and
sort). Press `b` while Spotify is the active provider, choose **Albums**, and
pick a sort — `Recently Added` (default), `By Name`, or `By Artist`. Enter
opens the album's track list. Albums you've *saved* in Spotify appear here
only if you've liked/followed them in the app.
```

---

## 9. Bottom line

The change is small (one new block in `Playlists()`, one entry in `sectionOrder`, an optional dedup step for `__clify_album__` collisions). It requires **no UI code changes** (`view.go` is fully data-driven for non-radio sections), **no interface changes** (`AlbumBrowser` / `AlbumTrackLoader` are sufficient as-is), and **no IPC changes** (`ipc.PlaylistInfo` is a superset). The only meaningful risks are: large-library scroll behaviour (worst case), id-collision dedup semantics between Recently Played and Saved (Option A), and a soft-fail policy decision for when `/v1/me/albums` errors. The pre-existing `sectionOrder` test fixtures (`provider_nonwindows_test.go`, `generated_test.go`, `recent_test.go`) all need an Albums block added; the render-side tests in `ui/model/provider_sections_test.go` need an Albums case to keep the header-aware scroll math covered.
