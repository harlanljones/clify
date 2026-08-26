//go:build !windows

package spotify

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/bjarneo/cliamp/playlist"
	"github.com/bjarneo/cliamp/provider"
)

const (
	// topArtistsLimit caps the user's top artists fetched in one page; Spotify
	// allows 1..50 and a top list is not meaningfully paginated.
	topArtistsLimit = 50
	artistCacheTTL  = 5 * time.Minute
)

// Compile-time interface check for the "By Artist" browse (top artists).
var _ provider.ArtistBrowser = (*SpotifyProvider)(nil)

// Artists implements provider.ArtistBrowser. It returns the authenticated
// user's most-listened artists from /v1/me/top/artists (medium_term). AlbumCount
// is not returned by the top-artists endpoint, so it is left 0; selecting an
// artist drills into its albums via ArtistAlbums.
func (p *SpotifyProvider) Artists() ([]provider.ArtistInfo, error) {
	if err := p.ensureSession(); err != nil {
		return nil, err
	}

	p.mu.Lock()
	if p.artistCache != nil && time.Since(p.artistCacheAt) < artistCacheTTL {
		cache := make([]provider.ArtistInfo, len(p.artistCache))
		copy(cache, p.artistCache)
		p.mu.Unlock()
		return cache, nil
	}
	p.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	query := url.Values{
		"limit":      {strconv.Itoa(topArtistsLimit)},
		"time_range": {"medium_term"},
	}
	resp, err := p.webAPI(ctx, "GET", "/v1/me/top/artists", query)
	if err != nil {
		return nil, fmt.Errorf("spotify: top artists: %w", err)
	}
	var result struct {
		Items []spotifyArtistItem `json:"items"`
	}
	if err := decodeBody(resp, &result); err != nil {
		return nil, fmt.Errorf("spotify: parse top artists: %w", err)
	}

	var all []provider.ArtistInfo
	for _, item := range result.Items {
		if item.ID == "" {
			continue // skip malformed entries
		}
		all = append(all, provider.ArtistInfo{ID: item.ID, Name: item.Name})
	}

	p.mu.Lock()
	p.artistCache = all
	p.artistCacheAt = time.Now()
	p.mu.Unlock()

	return all, nil
}

// ArtistAlbums implements provider.ArtistBrowser. It returns the albums of the
// given artist (an id previously returned by Artists()) via
// /v1/artists/{id}/albums.
func (p *SpotifyProvider) ArtistAlbums(artistID string) ([]provider.AlbumInfo, error) {
	if err := p.ensureSession(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var all []provider.AlbumInfo
	offset := 0
	for {
		query := url.Values{
			"limit":          {strconv.Itoa(spotifyAlbumPageSize)},
			"offset":         {strconv.Itoa(offset)},
			"include_groups": {"album"},
		}
		path := fmt.Sprintf("/v1/artists/%s/albums", url.PathEscape(artistID))
		resp, err := p.webAPI(ctx, "GET", path, query)
		if err != nil {
			return nil, fmt.Errorf("spotify: artist albums: %w", err)
		}
		var result struct {
			Items []spotifyAlbumObject `json:"items"`
			Total int                  `json:"total"`
		}
		if err := decodeBody(resp, &result); err != nil {
			return nil, fmt.Errorf("spotify: parse artist albums: %w", err)
		}
		for _, item := range result.Items {
			if item.ID == "" {
				continue
			}
			all = append(all, albumInfoFromAlbum(item))
		}
		if offset+spotifyAlbumPageSize >= result.Total || len(result.Items) == 0 {
			break
		}
		offset += spotifyAlbumPageSize
	}

	return all, nil
}

// spotifyArtistItem is the raw artist object returned by /v1/me/top/artists.
type spotifyArtistItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// topTracksID is the synthetic library row id for the authenticated user's
// most-played tracks (personalized listening stats, distinct from the
// Spotify-generated "Made For You" mixes).
const topTracksID = "TOP TRACKS"

// topTracks returns the user's most-played tracks (/v1/me/top/tracks,
// medium_term). Items are flat track objects; one 50-item page is the full
// top list.
func (p *SpotifyProvider) topTracks(ctx context.Context) ([]playlist.Track, error) {
	query := url.Values{
		"limit":      {strconv.Itoa(spotifyTrackPageSize)},
		"offset":     {strconv.Itoa(0)},
		"time_range": {"medium_term"},
	}
	resp, err := p.webAPI(ctx, "GET", "/v1/me/top/tracks", query)
	if err != nil {
		return nil, fmt.Errorf("spotify: top tracks: %w", err)
	}
	var result struct {
		Items []*spotifyItem `json:"items"`
	}
	if err := decodeBody(resp, &result); err != nil {
		return nil, fmt.Errorf("spotify: parse top tracks: %w", err)
	}
	var all []playlist.Track
	for _, t := range result.Items {
		if t == nil || t.ID == "" {
			continue // skip local/unavailable tracks
		}
		all = append(all, trackFromItem(t))
	}
	return all, nil
}
