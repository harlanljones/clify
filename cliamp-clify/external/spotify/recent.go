//go:build !windows

package spotify

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bjarneo/cliamp/applog"
	"github.com/bjarneo/cliamp/playlist"
	"github.com/bjarneo/cliamp/recent"
)

const spotifyRecentPageSize = 50

// ProviderMeta keys stashing the browse-row derivation for each play. They
// intentionally do not end in ".id" so recent.Merge's canonical dedup key
// (which scans *.id metadata) is unaffected.
const (
	RecentKindMetaKey   = "clify.recent.kind"   // "album" or "playlist"
	RecentTargetMetaKey = "clify.recent.target" // Spotify album/playlist id
)

// recentKind marks the derived row type stored under RecentKindMetaKey.
const (
	recentKindAlbum    = "album"
	recentKindPlaylist = "playlist"
)

// Recent loads Spotify's track-only recently-played feed. Spotify does not
// include podcast episodes in this endpoint. Each item carries its listening
// context (album or playlist) in ProviderMeta so the provider browser can
// derive Recently Played rows from the merged feed.
func (p *SpotifyProvider) Recent(ctx context.Context, limit int) ([]recent.Item, error) {
	if err := p.ensureSession(); err != nil {
		return nil, err
	}
	if limit < 1 {
		limit = spotifyRecentPageSize
	} else if limit > spotifyRecentPageSize {
		limit = spotifyRecentPageSize
	}
	query := url.Values{"limit": {strconv.Itoa(limit)}}
	resp, err := p.webAPI(ctx, http.MethodGet, "/v1/me/player/recently-played", query)
	if err != nil {
		return nil, fmt.Errorf("spotify: recently played: %w", err)
	}
	var payload struct {
		Items []struct {
			PlayedAt string       `json:"played_at"`
			Context  *recentCtx   `json:"context"`
			Track    *spotifyItem `json:"track"`
		} `json:"items"`
	}
	if err := decodeBody(resp, &payload); err != nil {
		return nil, fmt.Errorf("spotify: parse recently played: %w", err)
	}
	items := make([]recent.Item, 0, len(payload.Items))
	for _, input := range payload.Items {
		if input.Track == nil || input.Track.ID == "" {
			continue
		}
		playedAt, err := time.Parse(time.RFC3339Nano, input.PlayedAt)
		if err != nil {
			return nil, fmt.Errorf("spotify: parse recently played timestamp: %w", err)
		}
		track := trackFromItem(input.Track)
		var ctxPtr *recentCtx
		if input.Context != nil && input.Context.URI != "" {
			ctxPtr = input.Context
		}
		kind, target := deriveRecentContext(ctxPtr, input.Track.Album.ID)
		if kind != "" {
			track.ProviderMeta = map[string]string{
				RecentKindMetaKey:   kind,
				RecentTargetMetaKey: target,
			}
		}
		items = append(items, recent.Item{
			Track:    track,
			PlayedAt: playedAt,
			Sources:  []string{"spotify"},
		})
	}
	return items, nil
}

// recentCtx is the listening context Spotify attaches to recently-played
// entries ("album" or "playlist"; track radios surface as non-playable URIs
// and are ignored by the prefix checks).
type recentCtx struct {
	Type string `json:"type"`
	URI  string `json:"uri"`
}

// deriveRecentContext maps a play's context to a browse-row kind and target.
// Album contexts win; playlist contexts come from the context URI only;
// plays without either fall back to the track's own album when known.
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

// withUnifiedRecent injects the Spotify-derived Recently Played rows ahead of
// the library sections. Rows are self-ordered newest-first and injected at a
// single explicit point; the section rank map in Playlists lists the section
// too so any combined sort keeps it first.
func (p *SpotifyProvider) withUnifiedRecent(playlists []playlist.PlaylistInfo) []playlist.PlaylistInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()
	result := p.unifiedRecent(ctx, unifiedRecentLimit)
	if result.Partial {
		applog.UserWarn("spotify: recently played source unavailable: %v", result.FailedSources)
	}
	rows := p.recentRows(result.Items)
	return append(rows, playlists...)
}

// unifiedRecentRowCap bounds how many Recently Played rows are offered.
const unifiedRecentRowCap = 25

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
		case recentKindPlaylist:
			name, browsable := p.resolvePlaylistName(target)
			if !browsable {
				continue
			}
			row.ID = target
			row.Name = name
		default:
			continue
		}
		seen[key] = struct{}{}
		rows = append(rows, row)
		if len(rows) == unifiedRecentRowCap {
			break
		}
	}
	return rows
}

// resolvePlaylistName resolves a playlist display name once per session and
// reports whether the id is browsable at all. Some context playlist ids
// (Spotify-generated mixes) are not addressable via /v1/playlists/{id};
// their 404 name lookups are remembered as a Tracks() routing hint (those
// resolve via spclient instead), but the rows stay visible under a fallback
// label. Transient failures keep the row under its own fallback label so a
// later listing can retry.
func (p *SpotifyProvider) resolvePlaylistName(id string) (name string, browsable bool) {
	p.mu.Lock()
	if cached, ok := p.playlistNames[id]; ok {
		p.mu.Unlock()
		return cached, true
	}
	_, knownGenerated := p.unbrowsablePlaylists[id]
	p.mu.Unlock()
	if knownGenerated {
		return generatedFallbackName, true
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var payload struct {
		Name string `json:"name"`
	}
	path := "/v1/playlists/" + url.PathEscape(id)
	resp, err := p.webAPI(ctx, http.MethodGet, path, url.Values{"fields": {"name"}})
	if err != nil {
		var statusErr *httpStatusError
		if errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusNotFound {
			p.mu.Lock()
			if p.unbrowsablePlaylists == nil {
				p.unbrowsablePlaylists = make(map[string]struct{})
			}
			p.unbrowsablePlaylists[id] = struct{}{}
			p.mu.Unlock()
			// Spotify-owned mixes 404 here; spclient context metadata still
			// carries their display names (e.g. "Daily Mix 1").
			if name := p.spclientPlaylistName(id); name != "" {
				p.storePlaylistName(id, name)
				return name, true
			}
			return generatedFallbackName, true
		}
		return "Playlist " + id, true
	}
	if err := decodeBody(resp, &payload); err != nil {
		return "Playlist " + id, true
	}

	fetched := strings.TrimSpace(payload.Name)
	if fetched == "" {
		fetched = "Playlist " + id
	}

	p.mu.Lock()
	if p.playlistNames == nil {
		p.playlistNames = make(map[string]string)
	}
	p.playlistNames[id] = fetched
	p.mu.Unlock()
	return fetched, true
}

// UnifiedRecent exposes the cached merged feed to the IPC layer without
// coupling that layer to SpotifyProvider.
func (p *SpotifyProvider) UnifiedRecent(ctx context.Context, limit int) recent.Result {
	return cloneRecentResult(p.unifiedRecent(ctx, limit))
}

func (p *SpotifyProvider) unifiedRecent(ctx context.Context, limit int) recent.Result {
	p.mu.Lock()
	if !p.recentCacheAt.IsZero() && time.Since(p.recentCacheAt) < recentCacheTTL {
		cached := cloneRecentResult(p.recentCache)
		p.mu.Unlock()
		return cached
	}
	store := p.historyStore
	p.mu.Unlock()

	type sourceResult struct {
		name  string
		items []recent.Item
		err   error
	}
	results := make(chan sourceResult, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		items, err := (recent.HistorySource{Store: store}).Recent(ctx, limit)
		results <- sourceResult{name: "cliamp", items: items, err: err}
	}()
	go func() {
		defer wg.Done()
		items, err := p.Recent(ctx, limit)
		results <- sourceResult{name: "spotify", items: items, err: err}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	named := make([]recent.NamedItems, 0, 2)
	for result := range results {
		named = append(named, recent.NamedItems{Name: result.name, Items: result.items, Err: result.err})
	}
	slices.SortFunc(named, func(a, b recent.NamedItems) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})
	merged := recent.Merge(limit, named...)
	p.mu.Lock()
	p.recentCache = cloneRecentResult(merged)
	p.recentCacheAt = time.Now()
	p.mu.Unlock()
	return merged
}

func cloneRecentResult(input recent.Result) recent.Result {
	output := recent.Result{
		Partial:       input.Partial,
		FailedSources: slices.Clone(input.FailedSources),
		Items:         make([]recent.Item, len(input.Items)),
	}
	for i, item := range input.Items {
		output.Items[i] = item
		output.Items[i].Sources = slices.Clone(item.Sources)
		output.Items[i].Track = cloneTrack(item.Track)
	}
	return output
}

func cloneTrack(track playlist.Track) playlist.Track {
	if track.ProviderMeta != nil {
		metadata := make(map[string]string, len(track.ProviderMeta))
		for key, value := range track.ProviderMeta {
			metadata[key] = value
		}
		track.ProviderMeta = metadata
	}
	return track
}
