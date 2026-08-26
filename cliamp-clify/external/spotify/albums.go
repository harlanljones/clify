//go:build !windows

package spotify

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bjarneo/cliamp/playlist"
	"github.com/bjarneo/cliamp/provider"
)

const (
	spotifyAlbumPageSize = 50
	albumListCacheTTL    = 5 * time.Minute
	// spots the /v1/me/albums "recently added" ordering; other sort options
	// are applied client-side because the Web API only paginates by added_at.
	albumSortRecent = "recent"
	albumSortName   = "name"
	albumSortArtist = "artist"
)

// Compile-time interface checks for the saved-albums browse surface.
var (
	_ provider.AlbumBrowser     = (*SpotifyProvider)(nil)
	_ provider.AlbumTrackLoader = (*SpotifyProvider)(nil)
)

// savedAlbumEntry is a saved album plus its local added_at timestamp so the
// "Recently Added" sort (the Web API's native ordering) is preserved after the
// client-side sorts that the other sort options require.
type savedAlbumEntry struct {
	info    provider.AlbumInfo
	addedAt time.Time
}

// spotifyAlbumObject is a Spotify album object shared by /v1/me/albums and
// /v1/artists/{id}/albums.
type spotifyAlbumObject struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Artists []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"artists"`
	ReleaseDate string   `json:"release_date"`
	TotalTracks int      `json:"total_tracks"`
	Genres      []string `json:"genres"`
}

// albumInfoFromAlbum maps a Spotify album object to provider.AlbumInfo.
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

// spotifySavedAlbumItem is the raw entry returned by GET /v1/me/albums.
type spotifySavedAlbumItem struct {
	AddedAt string             `json:"added_at"`
	Album   spotifyAlbumObject `json:"album"`
}

// AlbumSortTypes implements provider.AlbumBrowser.
func (p *SpotifyProvider) AlbumSortTypes() []provider.SortType {
	return []provider.SortType{
		{ID: albumSortRecent, Label: "Recently Added"},
		{ID: albumSortName, Label: "By Name"},
		{ID: albumSortArtist, Label: "By Artist"},
	}
}

// DefaultAlbumSort implements provider.AlbumBrowser.
func (p *SpotifyProvider) DefaultAlbumSort() string { return albumSortRecent }

// savedAlbums fetches and caches every album in the authenticated user's
// library. Results are cached per session for albumListCacheTTL.
func (p *SpotifyProvider) savedAlbums(ctx context.Context) ([]savedAlbumEntry, error) {
	p.mu.Lock()
	if p.albumCache != nil && time.Since(p.albumCacheAt) < albumListCacheTTL {
		cache := make([]savedAlbumEntry, len(p.albumCache))
		copy(cache, p.albumCache)
		p.mu.Unlock()
		return cache, nil
	}
	p.mu.Unlock()

	var entries []savedAlbumEntry
	offset := 0
	for {
		query := url.Values{
			"limit":  {strconv.Itoa(spotifyAlbumPageSize)},
			"offset": {strconv.Itoa(offset)},
		}
		resp, err := p.webAPI(ctx, "GET", "/v1/me/albums", query)
		if err != nil {
			return nil, fmt.Errorf("spotify: saved albums: %w", err)
		}

		var result struct {
			Items []spotifySavedAlbumItem `json:"items"`
			Total int                     `json:"total"`
		}
		if err := decodeBody(resp, &result); err != nil {
			return nil, fmt.Errorf("spotify: parse saved albums: %w", err)
		}

		for _, item := range result.Items {
			if item.Album.ID == "" {
				continue // skip malformed entries
			}
			addedAt, _ := time.Parse(time.RFC3339, item.AddedAt)
			entries = append(entries, savedAlbumEntry{
				info:    albumInfoFromAlbum(item.Album),
				addedAt: addedAt,
			})
		}

		if offset+spotifyAlbumPageSize >= result.Total || len(result.Items) == 0 {
			break
		}
		offset += spotifyAlbumPageSize
	}

	p.mu.Lock()
	p.albumCache = entries
	p.albumCacheAt = time.Now()
	p.mu.Unlock()

	return entries, nil
}

// AlbumList implements provider.AlbumBrowser. It fetches the full saved-album
// catalog, applies the requested client-side sort, and returns the requested
// page. Invalid or negative offsets behave as an empty first page.
func (p *SpotifyProvider) AlbumList(sortType string, offset, size int) ([]provider.AlbumInfo, error) {
	if err := p.ensureSession(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	entries, err := p.savedAlbums(ctx)
	if err != nil {
		return nil, err
	}

	all := make([]savedAlbumEntry, len(entries))
	copy(all, entries)
	sortAlbumEntries(all, sortType)

	if offset < 0 {
		offset = 0
	}
	if size <= 0 {
		size = spotifyAlbumPageSize
	}
	if offset >= len(all) {
		return []provider.AlbumInfo{}, nil
	}
	end := offset + size
	if end > len(all) {
		end = len(all)
	}
	out := make([]provider.AlbumInfo, 0, end-offset)
	for _, entry := range all[offset:end] {
		out = append(out, entry.info)
	}
	return out, nil
}

// AlbumTracks implements provider.AlbumTrackLoader. It returns the tracks of a
// saved album via the same /v1/albums/{id}/tracks path used by unified-album
// rows; the id is a raw Spotify album id (not the __clify_album__ prefix).
func (p *SpotifyProvider) AlbumTracks(albumID string) ([]playlist.Track, error) {
	if err := p.ensureSession(); err != nil {
		return nil, err
	}
	return p.albumTracks(albumID)
}

func sortAlbumEntries(entries []savedAlbumEntry, sortType string) {
	switch sortType {
	case albumSortName:
		// Stable sort keeps the native added_at order for equal names.
		sort.SliceStable(entries, func(i, j int) bool {
			return strings.ToLower(entries[i].info.Name) < strings.ToLower(entries[j].info.Name)
		})
	case albumSortArtist:
		sort.SliceStable(entries, func(i, j int) bool {
			return strings.ToLower(entries[i].info.Artist) < strings.ToLower(entries[j].info.Artist)
		})
	default: // albumSortRecent
		sort.SliceStable(entries, func(i, j int) bool {
			return entries[i].addedAt.After(entries[j].addedAt)
		})
	}
}
