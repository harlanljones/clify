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
