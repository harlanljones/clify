# Spotify Web API contracts

Pinned for clify v1.6.0 from Spotify's official Web API reference on
2026-08-21. These are provider contracts, not clify-generated schema versions.

## Current user's playlists

`GET https://api.spotify.com/v1/me/playlists?limit=50`

Required authorization scope: `playlist-read-private`. The response is a page
object with an `items` array and nullable `next` URL. clify follows `next` until
it is null and returns a flattened `{"items": [...], "total": N}` payload.
Playlist entries retain Spotify fields; clify relies on `id` and `name`.

Reference: <https://developer.spotify.com/documentation/web-api/reference/get-a-list-of-current-users-playlists>

## Recently played

`GET https://api.spotify.com/v1/me/player/recently-played?limit=N`

Required authorization scope: `user-read-recently-played`. The response has an
`items` array. Each item contains an ISO-8601 `played_at` timestamp and a
`track` object; clify relies on `track.name`, `track.artists`, and optionally
`track.uri` when merging and deduplicating provider history.

Reference: <https://developer.spotify.com/documentation/web-api/reference/get-recently-played>

## Generated playlists

clify derives this listing from `GET https://api.spotify.com/v1/me/playlists?limit=50`
via `SpotifyClient.get_generated_playlists()`; it issues no separate Web API call.

Required authorization scope: `playlist-read-private` (inherited from the
playlists endpoint). A playlist is a generated candidate when its owner id is
`spotify` or its `id` or `uri` starts with the algorithmic prefix `37i9dQZF1E`.
Non-candidates are dropped. Each returned entry is a shallow-extended copy of
the cached playlist object with three clify-added fields:

| Field | Type | Meaning |
|---|---|---|
| `owner_id` | string | Owner id copied to the top level (`""` when absent or malformed). |
| `generated` | boolean | Always `true`; marks clify-added provenance. |
| `kind` | string | One of `daily_mix`, `discover_weekly`, `release_radar`, `on_repeat`, `repeat_rewind`, `daylist`, or `other`. |

`kind` is classified from a case-insensitive substring match on the playlist
name (`"Daily Mix 3"` → `daily_mix`); names matching none of the known
algorithmic families are `other`. Entries retain all upstream Spotify fields;
callers must not rely on `generated` appearing in raw `/me/playlists` results.
The result is a flattened `{"items": [...], "total": N}` payload served from
the shared 30-second user-playlist cache, so consecutive calls within the TTL
issue no HTTP requests.

## Playlist tracks

`GET https://api.spotify.com/v1/playlists/{playlist_id}/tracks?limit=100`

Required authorization scope: `playlist-read-private`. `playlist_id` must be a
non-empty string containing no slashes or whitespace; anything else is rejected
locally as a configuration error before any HTTP request. clify follows the
page object's nullable `next` URL until it is null and returns a flattened
payload. Success always carries `"unbrowsable": false`.

Since November 2024 Spotify answers HTTP 404 for its algorithmic playlists on
this endpoint. That outcome is a documented non-error path, not a failure:
`SpotifyClient.get_playlist_tracks()` returns
`{"items": [], "total": 0, "unbrowsable": true}` so callers can render such
playlists as present-but-not-browsable instead of surfacing a tool error. Any
other non-2xx status still becomes a structured `TOOL_ERROR`.

Reference: <https://developer.spotify.com/documentation/web-api/reference/get-playlists-tracks>

## OAuth and errors

Interactive setup uses Authorization Code with PKCE and requests only the two
scopes above. `clify spotify login` stores the public Client ID and refresh
token at `~/.config/clify/spotify.json` with mode 0600; it neither needs nor
stores a Client Secret. The client uses refresh-token OAuth after login. HTTP
401 refreshes and retries once; HTTP
429 observes `Retry-After` and retries once; any later failure becomes a
structured `TOOL_ERROR`. Environment-based legacy configuration remains
supported through `SPOTIFY_CLIENT_ID`, `SPOTIFY_CLIENT_SECRET`, and
`SPOTIFY_REFRESH_TOKEN`. Credentials are never included in telemetry or
exception messages.

Authorization reference:
<https://developer.spotify.com/documentation/web-api/tutorials/code-pkce-flow>
