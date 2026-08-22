# Recently Played

cliamp keeps a local listening history in `~/.config/cliamp/history.toml`. A
play is recorded once you've listened to a track for at least 50% of its
duration — the same threshold Last.fm and the Navidrome scrobbler use, so
skipped tracks never enter the list.

## Browsing in the TUI

Open the **Local Playlists** provider. When at least one play has been
recorded, a virtual `Recently Played` entry appears at the top of the list.
Open it like any other playlist — the tracks are listed newest-first. The list
is read-only: bookmarking, removing tracks, or deleting the playlist itself is
rejected with a clear error.

To clear the list, run `cliamp history clear` (see below).

## CLI

```sh
cliamp history                # show the 50 most recent plays
cliamp history --limit 200    # show the 200 most recent
cliamp history --limit 0      # show all (capped at 200 entries on disk)
cliamp history --json         # machine-readable output
cliamp history clear          # wipe the history file
```

The relative timestamp (`3m ago`, `yesterday`, …) is local time. The JSON
output uses `played_at` in RFC 3339 UTC for portability.

## File format

`history.toml` uses the same minimal TOML dialect as cliamp's local playlists:

```toml
[[entry]]
played_at = "2026-05-06T22:09:11Z"
path = "/home/me/Music/AC-DC/Highway to Hell.flac"
title = "Highway to Hell"
artist = "AC/DC"
album = "Highway to Hell"
year = 1979
duration_secs = 208
```

Entries cap at 200 by default; older plays roll off FIFO. Consecutive replays
of the same track within 5 minutes update the existing top entry's timestamp
rather than duplicating it.

## What is not recorded

- Tracks you skipped before the 50% threshold.
- Live streams without a known duration (radio stations, ICY streams) — there
  is no "halfway through" to detect.
- Tracks with empty paths (defensive guard).
