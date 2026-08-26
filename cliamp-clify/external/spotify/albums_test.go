//go:build !windows

package spotify

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// savedAlbumJSON builds one /v1/me/albums item.
func savedAlbumJSON(id, name, artistID, artistName, releaseDate string, totalTracks int, addedAt, genre string) string {
	return fmt.Sprintf(`{"added_at":%q,"album":{"id":%q,"name":%q,"artists":[{"id":%q,"name":%q}],"release_date":%q,"total_tracks":%d,"genres":[%q]}}`,
		addedAt, id, name, artistID, artistName, releaseDate, totalTracks, genre)
}

// savedAlbumsFixture fakes the /v1/me/albums and /v1/albums/{id}/tracks
// endpoints and records the offset values it was asked to fetch.
type savedAlbumsFixture struct {
	t             *testing.T
	p             *SpotifyProvider
	mu            sync.Mutex
	albumRequests []string // offsets requested on /v1/me/albums
	trackCalls    int
}

func newSavedAlbumsFixture(t *testing.T) *savedAlbumsFixture {
	t.Helper()
	f := &savedAlbumsFixture{t: t}
	f.p = New(&Session{}, "client", 160)
	f.p.webAPIFunc = func(_ context.Context, method, path string, query url.Values) (*http.Response, error) {
		switch {
		case path == "/v1/me/albums":
			f.mu.Lock()
			f.albumRequests = append(f.albumRequests, query.Get("offset"))
			f.mu.Unlock()
			if query.Get("limit") != "50" {
				t.Fatalf("limit = %q, want 50", query.Get("limit"))
			}
			switch query.Get("offset") {
			case "", "0":
				return jsonResponse(200, `{"items":[`+
					savedAlbumJSON("alb1", "Discovery", "a1", "Daft Punk", "2001-03-12", 14, "2026-08-01T00:00:00Z", "Electronic")+`,`+
					savedAlbumJSON("alb2", "Homework", "a1", "Daft Punk", "1997-01-20", 15, "2026-08-03T00:00:00Z", "House")+`],`+
					`"total":2}`), nil
			}
			t.Fatalf("unexpected offset %q", query.Get("offset"))
			return nil, nil
		case strings.HasPrefix(path, "/v1/albums/") && strings.HasSuffix(path, "/tracks"):
			f.mu.Lock()
			f.trackCalls++
			f.mu.Unlock()
			return jsonResponse(200, `{"items":[{"id":"t1","name":"One","type":"track","uri":"spotify:track:t1","album":{"id":"alb1","name":"Discovery","release_date":"2001-03-12"},"artists":[{"name":"Daft Punk"}],"duration_ms":123000}],"total":1}`), nil
		}
		t.Fatalf("unexpected request %s %s", method, path)
		return nil, fmt.Errorf("unexpected path %q", path)
	}
	return f
}

func TestAlbumListMapsSavedAlbums(t *testing.T) {
	f := newSavedAlbumsFixture(t)

	albums, err := f.p.AlbumList("recent", 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 2 {
		t.Fatalf("albums = %d, want 2", len(albums))
	}
	// Default "Recently Added" sort is newest-first.
	if albums[0].ID != "alb2" || albums[1].ID != "alb1" {
		t.Fatalf("recent order = %s,%s, want alb2,alb1", albums[0].ID, albums[1].ID)
	}
	first := albums[0]
	if first.Name != "Homework" || first.Artist != "Daft Punk" || first.ArtistID != "a1" ||
		first.Year != 1997 || first.TrackCount != 15 || first.Genre != "House" {
		t.Errorf("first = %+v", first)
	}
	if albums[1].TrackCount != 14 || albums[1].Genre != "Electronic" {
		t.Errorf("second = %+v", albums[1])
	}
}

func TestAlbumListSortByNameAndArtist(t *testing.T) {
	// Distinct names exercise the client-side name comparator.
	nameFixture := New(&Session{}, "client", 160)
	nameFixture.webAPIFunc = func(_ context.Context, _ string, path string, _ url.Values) (*http.Response, error) {
		if path != "/v1/me/albums" {
			t.Fatalf("unexpected path %q", path)
		}
		return jsonResponse(200, `{"items":[`+
			savedAlbumJSON("alb-z", "Zeta", "a1", "Artist", "2020-01-01", 10, "2026-08-01T00:00:00Z", "")+`,`+
			savedAlbumJSON("alb-a", "Alpha", "a2", "Artist", "2019-01-01", 8, "2026-08-02T00:00:00Z", "")+`],"total":2}`), nil
	}
	nameSorted, err := nameFixture.AlbumList("name", 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(nameSorted) != 2 || nameSorted[0].ID != "alb-a" || nameSorted[1].ID != "alb-z" {
		t.Errorf("name sort = %+v, want [alb-a alb-z]", nameSorted)
	}

	// Distinct artists exercise the client-side artist comparator.
	artistFixture := New(&Session{}, "client", 160)
	artistFixture.webAPIFunc = func(_ context.Context, _ string, path string, _ url.Values) (*http.Response, error) {
		if path != "/v1/me/albums" {
			t.Fatalf("unexpected path %q", path)
		}
		return jsonResponse(200, `{"items":[`+
			savedAlbumJSON("alb-x", "X", "a1", "Zebra", "2020-01-01", 10, "2026-08-01T00:00:00Z", "")+`,`+
			savedAlbumJSON("alb-y", "Y", "a2", "Apple", "2019-01-01", 8, "2026-08-02T00:00:00Z", "")+`],"total":2}`), nil
	}
	byArtist, err := artistFixture.AlbumList("artist", 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(byArtist) != 2 || byArtist[0].ID != "alb-y" || byArtist[1].ID != "alb-x" {
		t.Errorf("artist sort = %+v, want [alb-y alb-x]", byArtist)
	}
}

func TestAlbumSortTypesAndDefault(t *testing.T) {
	f := newSavedAlbumsFixture(t)

	if got := f.p.DefaultAlbumSort(); got != "recent" {
		t.Fatalf("DefaultAlbumSort = %q, want recent", got)
	}
	sorts := f.p.AlbumSortTypes()
	if len(sorts) != 3 {
		t.Fatalf("AlbumSortTypes = %d, want 3", len(sorts))
	}
	if sorts[0].ID != "recent" || sorts[0].Label != "Recently Added" {
		t.Errorf("sorts[0] = %+v", sorts[0])
	}
}

func TestAlbumListCachesWithinTTLAndPagination(t *testing.T) {
	f := newSavedAlbumsFixture(t)

	if _, err := f.p.AlbumList("recent", 0, 50); err != nil {
		t.Fatal(err)
	}
	if _, err := f.p.AlbumList("recent", 0, 50); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	requests := append([]string(nil), f.albumRequests...)
	f.mu.Unlock()
	if len(requests) != 1 {
		t.Fatalf("second call re-hit the API: offsets = %v, want 1 request", requests)
	}
}

func TestAlbumTracksMapsAlbumEndpoint(t *testing.T) {
	f := newSavedAlbumsFixture(t)

	tracks, err := f.p.AlbumTracks("alb1")
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 1 {
		t.Fatalf("tracks = %d, want 1", len(tracks))
	}
	if tracks[0].Path != "spotify:track:t1" || tracks[0].Title != "One" || tracks[0].Artist != "Daft Punk" ||
		tracks[0].Album != "Discovery" || tracks[0].Year != 2001 {
		t.Errorf("track = %+v", tracks[0])
	}
	f.mu.Lock()
	callCount := f.trackCalls
	f.mu.Unlock()
	if callCount != 1 {
		t.Fatalf("album tracks requests = %d, want 1", callCount)
	}
}

func TestAlbumListSupportsPagingOffsets(t *testing.T) {
	page := func(offset, limit int) string {
		_ = offset
		all := []string{
			savedAlbumJSON("a1", "One", "x", "Artist", "2020-01-01", 1, "2026-08-01T00:00:00Z", ""),
			savedAlbumJSON("a2", "Two", "x", "Artist", "2020-01-01", 1, "2026-08-02T00:00:00Z", ""),
			savedAlbumJSON("a3", "Three", "x", "Artist", "2020-01-01", 1, "2026-08-03T00:00:00Z", ""),
		}
		if limit < 1 {
			limit = 50
		}
		start := offset
		if start < 0 {
			start = 0
		}
		if start > len(all) {
			start = len(all)
		}
		end := start + limit
		if end > len(all) {
			end = len(all)
		}
		items := strings.Join(all[start:end], ",")
		return `{"items":[` + items + `],"total":3}`
	}

	p := New(&Session{}, "client", 160)
	p.webAPIFunc = func(_ context.Context, _ string, path string, query url.Values) (*http.Response, error) {
		if path != "/v1/me/albums" {
			t.Fatalf("unexpected path %q", path)
		}
		offset := 0
		if o := query.Get("offset"); o != "" {
			if _, err := fmt.Sscanf(o, "%d", &offset); err != nil {
				t.Fatalf("bad offset %q", o)
			}
		}
		return jsonResponse(200, page(offset, 50)), nil
	}

	first, err := p.AlbumList("recent", 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 || first[0].ID != "a3" || first[1].ID != "a2" {
		t.Fatalf("first page = %+v", first)
	}
	second, err := p.AlbumList("recent", 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 || second[0].ID != "a1" {
		t.Fatalf("second page = %+v", second)
	}
}
