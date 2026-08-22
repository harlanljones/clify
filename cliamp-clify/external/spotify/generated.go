//go:build !windows

package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	librespot "github.com/devgianlu/go-librespot"
	connectpb "github.com/devgianlu/go-librespot/proto/spotify/connectstate"
	extmetadatapb "github.com/devgianlu/go-librespot/proto/spotify/extendedmetadata"
	metadatapb "github.com/devgianlu/go-librespot/proto/spotify/metadata"
	playlist4pb "github.com/devgianlu/go-librespot/proto/spotify/playlist4"
	"google.golang.org/protobuf/proto"

	"github.com/bjarneo/cliamp/applog"
	"github.com/bjarneo/cliamp/playlist"
)

const (
	// madeForYouSection is the browser section for Spotify-generated
	// (algorithmic) playlists such as Daily Mixes and Discover Weekly.
	madeForYouSection = "Made For You"

	// Spotify owns every algorithmic playlist; since Nov 2024 the Web API
	// 404s on them, but their ids reliably start with this prefix.
	generatedOwnerID  = "spotify"
	generatedIDPrefix = "37i9dQZF1E"

	// generatedFallbackName labels Recently Played rows whose generated
	// playlist name is unknown because the Web API lookup 404'd.
	generatedFallbackName = "Generated Mix"

	// generatedMaxPages caps context-resolve page following.
	generatedMaxPages = 10

	// contextHydrateConcurrency bounds parallel spclient metadata lookups.
	contextHydrateConcurrency = 8
)

// generatedKind classifies a generated playlist from its display name.
func generatedKind(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "daily mix"):
		return "daily_mix"
	case strings.Contains(n, "discover weekly"):
		return "discover_weekly"
	case strings.Contains(n, "release radar"):
		return "release_radar"
	case strings.Contains(n, "on repeat"):
		return "on_repeat"
	case strings.Contains(n, "repeat rewind"):
		return "repeat_rewind"
	case strings.Contains(n, "daylist"):
		return "daylist"
	default:
		return "other"
	}
}

// isGeneratedPlaylistItem reports whether a /v1/me/playlists entry is a
// Spotify-generated playlist. Entries without an id are never generated.
func isGeneratedPlaylistItem(item spotifyPlaylistItem) bool {
	if item.ID == "" {
		return false
	}
	return item.Owner.ID == generatedOwnerID || strings.HasPrefix(item.ID, generatedIDPrefix)
}

// registerGeneratedPlaylist records id as resolvable via spclient. The set
// feeds Tracks() routing; unbrowsablePlaylists entries route too.
func (p *SpotifyProvider) registerGeneratedPlaylist(id string) {
	if p.generatedPlaylists == nil {
		p.generatedPlaylists = make(map[string]struct{})
	}
	p.generatedPlaylists[id] = struct{}{}
}

func (p *SpotifyProvider) isGeneratedPlaylist(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.generatedPlaylists[id]; ok {
		return true
	}
	_, ok := p.unbrowsablePlaylists[id]
	return ok
}

// resolveGeneratedTracks fetches tracks for a Spotify-generated playlist via
// spclient context-resolve and caches them with an empty snapshot_id sentinel.
// Failures keep the same error shape Tracks() uses for Web API fetches.
// Empty resolves are never cached: a transient spclient hiccup would
// otherwise blank the pane for the whole session (Refresh() keeps trackCache).
func (p *SpotifyProvider) resolveGeneratedTracks(playlistID string) ([]playlist.Track, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	tracks, err := p.fetchGeneratedTracks(ctx, playlistID)
	if err != nil {
		applog.Warn("spotify: resolve generated playlist %s failed: %v", playlistID, err)
		return nil, fmt.Errorf("spotify: list tracks: %w", err)
	}
	if len(tracks) == 0 {
		return nil, fmt.Errorf("spotify: list tracks: generated playlist %s resolved no tracks", playlistID)
	}

	p.hydrateContextTracks(ctx, tracks)

	p.mu.Lock()
	p.trackCache[playlistID] = &playlistCache{tracks: tracks}
	p.mu.Unlock()

	return slices.Clone(tracks), nil
}

// storePlaylistName records a display name discovered outside the standard
// name lookup so later Recently Played rows show real mix names.
func (p *SpotifyProvider) storePlaylistName(id, name string) {
	name = strings.TrimSpace(name)
	if id == "" || name == "" {
		return
	}
	p.mu.Lock()
	if p.playlistNames == nil {
		p.playlistNames = make(map[string]string)
	}
	p.playlistNames[id] = name
	p.mu.Unlock()
}

// contextDisplayName extracts a human-readable playlist name from
// context-resolve metadata. Spotify prefixes the key ("spotify:context_name"),
// so the match is by suffix rather than exact key.
func contextDisplayName(metadata map[string]string) string {
	for key, val := range metadata {
		if strings.Contains(key, "context_name") && strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
	}
	return ""
}

// spclientPlaylistName resolves a playlist display name through the spclient
// playlist4 endpoint, which — unlike the Web API — still serves Spotify-owned
// mixes. Falls back to context-resolve metadata when the playlist fetch is
// unavailable. Returns "" when neither works.
func (p *SpotifyProvider) spclientPlaylistName(id string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if content := p.playlistFetch(ctx, id); content != nil {
		if name := strings.TrimSpace(content.GetAttributes().GetName()); name != "" {
			return name
		}
	}
	spotCtx, err := p.contextResolve(ctx, "spotify:playlist:"+id)
	if err != nil {
		applog.Warn("spotify: spclient name resolve for %s failed: %v", id, err)
		return ""
	}
	return contextDisplayName(spotCtx.Metadata)
}

// playlistFetch loads a playlist's list4 content (attributes include the
// display name) via the session's spclient. Overridable in tests; nil result
// means unavailable.
func (p *SpotifyProvider) playlistFetch(ctx context.Context, id string) *playlist4pb.SelectedListContent {
	if p.playlistFetchFunc != nil {
		return p.playlistFetchFunc(ctx, id)
	}
	sp := p.session.Sp()
	if sp == nil {
		return nil
	}
	resp, err := sp.Request(ctx, http.MethodGet, "/playlist/v2/playlist/"+id, url.Values{"decorate": {"attributes"}}, nil, nil)
	if err != nil {
		applog.Warn("spotify: playlist4 fetch for %s failed: %v", id, err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		applog.Warn("spotify: playlist4 fetch for %s: status %d", id, resp.StatusCode)
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return nil
	}
	var content playlist4pb.SelectedListContent
	if err := proto.Unmarshal(body, &content); err != nil {
		applog.Warn("spotify: parse playlist4 content for %s: %v", id, err)
		return nil
	}
	return &content
}

func (p *SpotifyProvider) fetchGeneratedTracks(ctx context.Context, playlistID string) ([]playlist.Track, error) {
	spotCtx, err := p.contextResolve(ctx, "spotify:playlist:"+playlistID)
	if err != nil {
		return nil, fmt.Errorf("context resolve: %w", err)
	}
	if spotCtx.Loading {
		return nil, fmt.Errorf("context %s is loading", spotCtx.Uri)
	}
	if name := contextDisplayName(spotCtx.Metadata); name != "" {
		p.storePlaylistName(playlistID, name)
	}

	queue := slices.Clone(spotCtx.Pages)
	var all []playlist.Track
	for i := 0; i < len(queue) && i < generatedMaxPages; i++ {
		for _, track := range queue[i].Tracks {
			if track == nil {
				continue
			}
			if tr, ok := trackFromContextTrack(track); ok {
				all = append(all, tr)
			}
		}
		if i == len(queue)-1 && queue[i].NextPageUrl != "" && len(queue) < generatedMaxPages {
			next, err := p.contextPage(ctx, queue[i].NextPageUrl)
			if err != nil {
				return nil, fmt.Errorf("fetch next page: %w", err)
			}
			queue = append(queue, next)
		}
	}
	applog.Debug("spotify: generated playlist %s resolved %d tracks from %d pages", playlistID, len(all), len(queue))
	return all, nil
}

// trackMetadata fetches full track metadata via the session's spclient
// extended-metadata endpoint, overridable in tests.
func (p *SpotifyProvider) trackMetadata(ctx context.Context, id librespot.SpotifyId, meta *metadatapb.Track) error {
	if p.trackMetadataFunc != nil {
		return p.trackMetadataFunc(ctx, id, meta)
	}
	sp := p.session.Sp()
	if sp == nil {
		return fmt.Errorf("spotify: spclient unavailable")
	}
	return sp.ExtendedMetadataSimple(ctx, id, extmetadatapb.ExtensionKind_TRACK_V4, meta)
}

// Context-track metadata key names, verified against real context-resolve /
// connect-state payloads (spicetify TrackMetadata capture). go-librespot's
// ids.go reads artist_uri / album_uri from the same map.
const (
	ctxMetaTitle      = "title"
	ctxMetaArtistName = "artist_name"
	ctxMetaAlbumTitle = "album_title"
	ctxMetaDurationMs = "duration" // milliseconds, as a string
)

// trackFromContextTrack converts a connectpb.ContextTrack into a
// playlist.Track. Non-track entries (episodes, local files) are skipped.
func trackFromContextTrack(t *connectpb.ContextTrack) (playlist.Track, bool) {
	uri := t.Uri
	if uri == "" && len(t.Gid) == 16 {
		uri = librespot.SpotifyIdFromGid(librespot.SpotifyIdTypeTrack, t.Gid).Uri()
	}
	id := strings.TrimPrefix(uri, "spotify:track:")
	if uri == "" || id == "" || id == uri {
		return playlist.Track{}, false // non-track or unidentifiable entry
	}

	durationMs, _ := strconv.Atoi(t.Metadata[ctxMetaDurationMs])
	return playlist.Track{
		Path:         uri,
		Title:        t.Metadata[ctxMetaTitle],
		Artist:       t.Metadata[ctxMetaArtistName],
		Album:        t.Metadata[ctxMetaAlbumTitle],
		DurationSecs: durationMs / 1000,
	}, true
}

// hydrateContextTracks fills missing song names (title, artist, album,
// duration) for context-resolve results using the spclient extended-metadata
// endpoint. The Web API catalog endpoints (/v1/tracks, /v1/search) are
// blocked for Development-Mode apps (403), so spclient is the only reliable
// name source. Hydration failures are logged and otherwise ignored — partial
// metadata still plays.
func (p *SpotifyProvider) hydrateContextTracks(ctx context.Context, tracks []playlist.Track) {
	type job struct {
		idx int
		id  librespot.SpotifyId
	}
	var jobs []job
	for i, t := range tracks {
		if t.Title != "" && t.Artist != "" && t.Album != "" && t.DurationSecs > 0 {
			continue
		}
		spotID, err := librespot.SpotifyIdFromUri(t.Path)
		if err != nil || spotID.Type() != librespot.SpotifyIdTypeTrack {
			continue
		}
		jobs = append(jobs, job{idx: i, id: *spotID})
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, contextHydrateConcurrency)
	var failed atomic.Int32
	for _, j := range jobs {
		wg.Add(1)
		go func(j job) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			var meta metadatapb.Track
			if err := p.trackMetadata(ctx, j.id, &meta); err != nil {
				failed.Add(1)
				return
			}
			t := &tracks[j.idx]
			if t.Title == "" {
				t.Title = meta.GetName()
			}
			if t.Artist == "" {
				for _, a := range meta.Artist {
					if t.Artist != "" {
						t.Artist += ", "
					}
					t.Artist += a.GetName()
				}
			}
			if t.Album == "" {
				t.Album = meta.GetAlbum().GetName()
			}
			if t.DurationSecs <= 0 {
				t.DurationSecs = int(meta.GetDuration()) / 1000
			}
		}(j)
	}
	wg.Wait()
	if n := failed.Load(); n > 0 {
		applog.Warn("spotify: %d/%d generated-track names unresolved via spclient", n, len(jobs))
	}
}

// contextResolve resolves a context URI through the session's spclient,
// overridable in tests. Returns an error rather than panicking when no
// session/spclient is live.
func (p *SpotifyProvider) contextResolve(ctx context.Context, uri string) (*connectpb.Context, error) {
	if p.contextResolveFunc != nil {
		return p.contextResolveFunc(ctx, uri)
	}
	sp := p.session.Sp()
	if sp == nil {
		return nil, fmt.Errorf("spotify: spclient unavailable")
	}
	return sp.ContextResolve(ctx, uri)
}

// contextPage loads a hm:// context page URL through the session's spclient,
// overridable in tests.
func (p *SpotifyProvider) contextPage(ctx context.Context, pageURL string) (*connectpb.ContextPage, error) {
	if p.contextPageFunc != nil {
		return p.contextPageFunc(ctx, pageURL)
	}
	sp := p.session.Sp()
	if sp == nil {
		return nil, fmt.Errorf("spotify: spclient unavailable")
	}
	resp, err := sp.Request(ctx, http.MethodGet, strings.TrimPrefix(pageURL, "hm://"), nil, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("request context page: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("invalid status code from context page: %d", resp.StatusCode)
	}
	var page connectpb.ContextPage
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBody)).Decode(&page); err != nil {
		return nil, fmt.Errorf("parse context page: %w", err)
	}
	return &page, nil
}
