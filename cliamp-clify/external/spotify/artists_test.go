//go:build !windows

package spotify

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// artistFixture fakes /v1/me/top/artists, /v1/artists/{id}/albums, and
// /v1/me/top/tracks, recording the top-artists request count.
type artistFixture struct {
	t            *testing.T
	p            *SpotifyProvider
	topArtists   string
	topTracks    string
	artistAlbums map[string]string // artistID -> albums JSON
	artistCalls  int
}

func newArtistFixture(t *testing.T) *artistFixture {
	f := &artistFixture{
		t:            t,
		topArtists:   `{"items":[{"id":"a1","name":"Daft Punk"},{"id":"a2","name":"Aphex Twin"}],"total":2}`,
		topTracks:    `{"items":[{"id":"t1","name":"One More Time","type":"track","uri":"spotify:track:t1","artists":[{"name":"Daft Punk"}],"album":{"id":"alb1","name":"Discovery","release_date":"2001-03-12"},"duration_ms":320000}],"total":1}`,
		artistAlbums: map[string]string{},
	}
	f.p = New(&Session{}, "client", 160)
	f.p.webAPIFunc = func(_ context.Context, _ string, path string, urlValues url.Values) (*http.Response, error) {
		switch {
		case path == "/v1/me":
			return jsonResponse(200, `{"id":"me"}`), nil
		case path == "/v1/me/tracks":
			return jsonResponse(200, `{"items":[],"total":0}`), nil
		case path == "/v1/me/playlists":
			return jsonResponse(200, `{"items":[],"total":0}`), nil
		case path == "/v1/me/player/recently-played":
			return jsonResponse(200, `{"items":[]}`), nil
		case path == "/v1/me/top/artists":
			f.artistCalls++
			return jsonResponse(200, f.topArtists), nil
		case path == "/v1/me/top/tracks":
			return jsonResponse(200, f.topTracks), nil
		case strings.HasPrefix(path, "/v1/artists/"):
			id := strings.TrimSuffix(strings.TrimPrefix(path, "/v1/artists/"), "/albums")
			body, ok := f.artistAlbums[id]
			if !ok {
				t.Fatalf("no artist albums fixture for %q", id)
			}
			return jsonResponse(200, body), nil
		}
		t.Fatalf("unexpected request %s", path)
		return nil, fmt.Errorf("unexpected path %q", path)
	}
	return f
}

func TestArtistsReturnsTopArtists(t *testing.T) {
	f := newArtistFixture(t)

	artists, err := f.p.Artists()
	if err != nil {
		t.Fatal(err)
	}
	if len(artists) != 2 {
		t.Fatalf("artists = %d, want 2", len(artists))
	}
	if artists[0].ID != "a1" || artists[0].Name != "Daft Punk" {
		t.Errorf("artists[0] = %+v", artists[0])
	}
	if artists[1].ID != "a2" || artists[1].Name != "Aphex Twin" {
		t.Errorf("artists[1] = %+v", artists[1])
	}
}

func TestArtistsRequestsMediumTermAndCaches(t *testing.T) {
	f := newArtistFixture(t)

	if _, err := f.p.Artists(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.p.Artists(); err != nil {
		t.Fatal(err)
	}
	if f.artistCalls != 1 {
		t.Fatalf("top-artists requests = %d, want 1 (second call served from cache)", f.artistCalls)
	}
}

func TestArtistsTopArtistsQuery(t *testing.T) {
	p := New(&Session{}, "client", 160)
	var requested bool
	p.webAPIFunc = func(_ context.Context, method, path string, query url.Values) (*http.Response, error) {
		if path == "/v1/me/top/artists" {
			requested = true
			if method != "GET" {
				t.Fatalf("method = %s", method)
			}
			if query.Get("limit") != "50" {
				t.Errorf("limit = %q, want 50", query.Get("limit"))
			}
			if query.Get("time_range") != "medium_term" {
				t.Errorf("time_range = %q, want medium_term", query.Get("time_range"))
			}
			return jsonResponse(200, `{"items":[],"total":0}`), nil
		}
		t.Fatalf("unexpected path %q", path)
		return nil, fmt.Errorf("unexpected path %q", path)
	}
	if _, err := p.Artists(); err != nil {
		t.Fatal(err)
	}
	if !requested {
		t.Fatal("top-artists endpoint was never requested")
	}
}

func TestArtistAlbumsMaps(t *testing.T) {
	f := newArtistFixture(t)
	f.artistAlbums["a1"] = `{"items":[` +
		`{"id":"alb1","name":"Discovery","artists":[{"id":"a1","name":"Daft Punk"}],"release_date":"2001-03-12","total_tracks":14,"genres":["Electronic"]}` +
		`],"total":1}`

	albums, err := f.p.ArtistAlbums("a1")
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 1 {
		t.Fatalf("albums = %d, want 1", len(albums))
	}
	if albums[0].ID != "alb1" || albums[0].Name != "Discovery" || albums[0].Artist != "Daft Punk" ||
		albums[0].Year != 2001 || albums[0].TrackCount != 14 || albums[0].Genre != "Electronic" {
		t.Errorf("album = %+v", albums[0])
	}
}

func TestArtistAlbumsRequestsIncludeGroupsAndMapsAll(t *testing.T) {
	// Artist albums are requested with include_groups=album and, with a 50-item
	// page, both albums arrive in one request when total <= 50.
	p := New(&Session{}, "client", 160)
	p.webAPIFunc = func(_ context.Context, _ string, path string, query url.Values) (*http.Response, error) {
		if path != "/v1/artists/a1/albums" {
			t.Fatalf("unexpected path %q", path)
		}
		if query.Get("include_groups") != "album" {
			t.Errorf("include_groups = %q, want album", query.Get("include_groups"))
		}
		if query.Get("limit") != "50" {
			t.Errorf("limit = %q, want 50", query.Get("limit"))
		}
		return jsonResponse(200, `{"items":[`+
			`{"id":"alb1","name":"Discovery","artists":[{"id":"a1","name":"Daft Punk"}],"release_date":"2001-03-12","total_tracks":14}`+`,`+
			`{"id":"alb2","name":"Homework","artists":[{"id":"a1","name":"Daft Punk"}],"release_date":"1997-01-20","total_tracks":15}`+
			`],"total":2}`), nil
	}
	albums, err := p.ArtistAlbums("a1")
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 2 || albums[0].Name != "Discovery" || albums[1].Name != "Homework" {
		t.Fatalf("albums = %+v", albums)
	}
}

func TestTopTracksRowAndResolution(t *testing.T) {
	f := newArtistFixture(t)

	rows, err := f.p.Playlists()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range rows {
		if row.ID == "TOP TRACKS" && row.Name == "Top Tracks" && row.Section == "Made For You" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Top Tracks row missing from Playlists(): %+v", rows)
	}

	tracks, err := f.p.Tracks("TOP TRACKS")
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 1 {
		t.Fatalf("tracks = %d, want 1", len(tracks))
	}
	if tracks[0].Path != "spotify:track:t1" || tracks[0].Title != "One More Time" ||
		tracks[0].Artist != "Daft Punk" || tracks[0].Album != "Discovery" || tracks[0].DurationSecs != 320 {
		t.Errorf("track = %+v", tracks[0])
	}
}
