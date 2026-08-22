//go:build !windows

package spotify

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	playlist4pb "github.com/devgianlu/go-librespot/proto/spotify/playlist4"
	"google.golang.org/protobuf/proto"

	"github.com/bjarneo/cliamp/history"
	"github.com/bjarneo/cliamp/playlist"
)

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestRecentRequestsEndpointAndMapsTracks(t *testing.T) {
	p := New(&Session{}, "client", 160)
	p.webAPIFunc = func(_ context.Context, method, path string, query url.Values) (*http.Response, error) {
		if method != http.MethodGet || path != "/v1/me/player/recently-played" {
			t.Fatalf("request = %s %s", method, path)
		}
		if query.Get("limit") != "50" {
			t.Fatalf("limit = %q, want capped 50", query.Get("limit"))
		}
		body := `{"items":[` +
			`{"played_at":"2026-08-21T10:35:00Z","context":{"type":"album","uri":"spotify:album:ctx1"},"track":{"id":"abc","name":"Song","type":"track","uri":"spotify:track:abc","artists":[{"name":"Artist"}],"album":{"id":"ctx1","name":"Context Album","release_date":"2025-01-01"},"duration_ms":123000}},` +
			`{"played_at":"2026-08-21T10:30:00Z","context":{"type":"playlist","uri":"spotify:playlist:pl1"},"track":{"id":"def","name":"In List","type":"track","uri":"spotify:track:def","artists":[{"name":"Artist"}],"album":{"id":"alb2","name":"Whatever"},"duration_ms":1000}},` +
			`{"played_at":"2026-08-21T10:25:00Z","track":{"id":"ghi","name":"Bare","type":"track","uri":"spotify:track:ghi","artists":[{"name":"Artist"}],"album":{"id":"alb3","name":"Payload Album"},"duration_ms":1000}},` +
			`{"played_at":"2026-08-21T10:20:00Z","track":{"id":"jkl","name":"No Context","type":"track","uri":"spotify:track:jkl","duration_ms":1000}}` +
			`]}`
		return jsonResponse(200, body), nil
	}

	items, err := p.Recent(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 {
		t.Fatalf("items = %d, want 4", len(items))
	}
	first := items[0].Track
	if first.Path != "spotify:track:abc" || first.DurationSecs != 123 || first.Album != "Context Album" {
		t.Fatalf("first = %+v", first)
	}
	if items[0].PlayedAt.Format(time.RFC3339) != "2026-08-21T10:35:00Z" {
		t.Fatalf("played_at = %v", items[0].PlayedAt)
	}

	cases := []struct {
		path string
		kind string
		want string
	}{
		{"spotify:track:abc", "album", "ctx1"},
		{"spotify:track:def", "playlist", "pl1"},
		{"spotify:track:ghi", "album", "alb3"},
		{"spotify:track:jkl", "", ""},
	}
	for _, tc := range cases {
		for _, item := range items {
			if item.Track.Path != tc.path {
				continue
			}
			if got := item.Track.ProviderMeta["clify.recent.kind"]; got != tc.kind {
				t.Errorf("%s kind = %q, want %q", tc.path, got, tc.kind)
			}
			if got := item.Track.ProviderMeta["clify.recent.target"]; got != tc.want {
				t.Errorf("%s target = %q, want %q", tc.path, got, tc.want)
			}
		}
	}
}

func TestRecentRejectsMalformedResponse(t *testing.T) {
	p := New(&Session{}, "client", 160)
	p.webAPIFunc = func(context.Context, string, string, url.Values) (*http.Response, error) {
		return jsonResponse(200, `{"items":[{"played_at":"not-a-time","track":{"id":"abc"}}]}`), nil
	}
	if _, err := p.Recent(context.Background(), 20); err == nil {
		t.Fatal("Recent error = nil, want malformed played_at error")
	}
}

// recentFeedFixture wires a provider whose Web API serves a fixed recently-
// played feed plus library endpoints. Counters record how often each derived
// endpoint was hit so tests can assert caching behavior. statusByName forces
// a failure status for specific playlist-id name lookups, mirroring the
// non-2xx errors webAPIWithBody produces in production.
type recentFeedFixture struct {
	p            *SpotifyProvider
	feedJSON     string
	statusByName map[string]int

	mu            sync.Mutex
	recentCalls   int
	nameCalls     map[string]int
	requestedPath []string
}

func newRecentFeedFixture(t *testing.T, feedJSON string) *recentFeedFixture {
	t.Helper()
	f := &recentFeedFixture{nameCalls: map[string]int{}, statusByName: map[string]int{}}
	store := history.NewAt(t.TempDir() + "/history.toml")
	if err := store.Record(playlist.Track{Path: "/music/local.flac", Title: "Local Song"}, time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	f.p = New(&Session{}, "client", 160, WithHistory(store))
	f.p.webAPIFunc = f.handle
	f.feedJSON = feedJSON
	return f
}

func (f *recentFeedFixture) handle(_ context.Context, _ string, path string, query url.Values) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requestedPath = append(f.requestedPath, path+"?"+query.Encode())
	switch {
	case path == "/v1/me":
		return jsonResponse(200, `{"id":"me"}`), nil
	case path == "/v1/me/tracks":
		return jsonResponse(200, `{"items":[],"total":3}`), nil
	case path == "/v1/me/playlists":
		return jsonResponse(200, `{"items":[{"id":"mine","name":"Mine","snapshot_id":"s1","owner":{"id":"me"},"items":{"total":2}}],"total":1}`), nil
	case path == "/v1/me/player/recently-played":
		f.recentCalls++
		return jsonResponse(200, f.feedJSON), nil
	case strings.HasPrefix(path, "/v1/playlists/") && strings.HasSuffix(path, "/items"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/v1/playlists/"), "/items")
		return jsonResponse(200, fmt.Sprintf(`{"items":[{"track":{"id":"t-%s","name":"Track","type":"track","uri":"spotify:track:t-%s"}}],"total":1}`, id, id)), nil
	case strings.HasPrefix(path, "/v1/playlists/"):
		id := strings.TrimPrefix(path, "/v1/playlists/")
		f.nameCalls[id]++
		if status := f.statusByName[id]; status != 0 {
			return nil, &httpStatusError{
				StatusCode: status,
				msg:        fmt.Sprintf("http status %s: {}", http.StatusText(status)),
			}
		}
		return jsonResponse(200, fmt.Sprintf(`{"id":%q,"name":"Named %s"}`, id, id)), nil
	case strings.HasPrefix(path, "/v1/albums/"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/v1/albums/"), "/tracks")
		return jsonResponse(200, fmt.Sprintf(`{"items":[{"id":"a-%s","name":"Album Track","type":"track","uri":"spotify:track:a-%s","duration_ms":42000}],"total":1}`, id, id)), nil
	}
	return nil, fmt.Errorf("unexpected path %q", path)
}

func TestPlaylistsDerivesAlbumAndPlaylistRowsFromSpotifyFeed(t *testing.T) {
	f := newRecentFeedFixture(t, `{"items":[`+
		`{"played_at":"2026-08-21T11:05:00Z","context":{"type":"album","uri":"spotify:album:beta"},"track":{"id":"t1","name":"One","type":"track","uri":"spotify:track:t1","album":{"name":"Beta"}}},`+
		`{"played_at":"2026-08-21T11:10:00Z","track":{"id":"t2","name":"No Context","type":"track","uri":"spotify:track:t2"}},`+
		`{"played_at":"2026-08-21T11:15:00Z","track":{"id":"t3","name":"Two","type":"track","uri":"spotify:track:t3","album":{"id":"delta","name":"Delta"}}},`+
		`{"played_at":"2026-08-21T11:20:00Z","context":{"type":"album","uri":"spotify:album:beta"},"track":{"id":"t4","name":"Three","type":"track","uri":"spotify:track:t4","album":{"name":"Beta"}}},`+
		`{"played_at":"2026-08-21T11:25:00Z","context":{"type":"playlist","uri":"spotify:playlist:p1"},"track":{"id":"t5","name":"Four","type":"track","uri":"spotify:track:t5"}}`+
		`]}`)

	got, err := f.p.Playlists()
	if err != nil {
		t.Fatal(err)
	}

	want := []struct{ id, name, section string }{
		{"p1", "Named p1", "Recently Played"},
		{"__clify_album__beta", "Beta", "Recently Played"},
		{"__clify_album__delta", "Delta", "Recently Played"},
		{"YOUR MUSIC", "Your Music", "Library"},
		{"mine", "Mine", "Your playlists"},
	}
	if len(got) != len(want) {
		t.Fatalf("playlists = %+v", got)
	}
	for i, w := range want {
		if got[i].ID != w.id || got[i].Name != w.name || got[i].Section != w.section {
			t.Errorf("row[%d] = {%s %s %s}, want {%s %s %s}", i, got[i].ID, got[i].Name, got[i].Section, w.id, w.name, w.section)
		}
	}
	if got[0].TrackCount != 0 {
		t.Errorf("playlist row TrackCount = %d, want unset 0", got[0].TrackCount)
	}
	if f.nameCalls["p1"] != 1 {
		t.Errorf("playlist name fetches = %d, want cached 1", f.nameCalls["p1"])
	}

	// Second listing reuses caches: no extra recent or name fetches.
	if _, err := f.p.Playlists(); err != nil {
		t.Fatal(err)
	}
	if f.recentCalls != 1 {
		t.Errorf("recent calls = %d, want cached 1", f.recentCalls)
	}

	// Playlist rows open through the normal playlist-items path.
	tracks, err := f.p.Tracks("p1")
	if err != nil || len(tracks) != 1 || tracks[0].Path != "spotify:track:t-p1" {
		t.Fatalf("playlist tracks = %+v, err = %v", tracks, err)
	}
	// Album rows open through the paginated album-tracks path.
	tracks, err = f.p.Tracks("__clify_album__beta")
	if err != nil || len(tracks) != 1 || tracks[0].Path != "spotify:track:a-beta" {
		t.Fatalf("album tracks = %+v, err = %v", tracks, err)
	}

	f.p.Refresh()
	if _, err := f.p.Playlists(); err != nil {
		t.Fatal(err)
	}
	if f.recentCalls != 2 {
		t.Errorf("recent calls after Refresh = %d, want 2", f.recentCalls)
	}
}

func TestPlaylistsKeepsGeneratedRecentPlaylistRows(t *testing.T) {
	f := newRecentFeedFixture(t, `{"items":[`+
		`{"played_at":"2026-08-21T11:05:00Z","context":{"type":"playlist","uri":"spotify:playlist:mix404"},"track":{"id":"t1","name":"One","type":"track","uri":"spotify:track:t1"}},`+
		`{"played_at":"2026-08-21T11:10:00Z","context":{"type":"playlist","uri":"spotify:playlist:p500"},"track":{"id":"t2","name":"Two","type":"track","uri":"spotify:track:t2"}},`+
		`{"played_at":"2026-08-21T11:15:00Z","context":{"type":"album","uri":"spotify:album:beta"},"track":{"id":"t3","name":"Three","type":"track","uri":"spotify:track:t3","album":{"name":"Beta"}}}`+
		`]}`)
	f.statusByName["mix404"] = 404
	f.statusByName["p500"] = 500

	got, err := f.p.Playlists()
	if err != nil {
		t.Fatal(err)
	}
	recent := []playlist.PlaylistInfo{}
	for _, pl := range got {
		if pl.Section == "Recently Played" {
			recent = append(recent, pl)
		}
	}
	if len(recent) != 3 {
		t.Fatalf("recent rows = %+v, want album + generated + retryable playlist", recent)
	}
	if recent[0].ID != "__clify_album__beta" || recent[0].Name != "Beta" {
		t.Errorf("album row = {%s %s}, want unchanged", recent[0].ID, recent[0].Name)
	}
	if recent[1].ID != "p500" || recent[1].Name != "Playlist p500" {
		t.Errorf("retryable row = {%s %s}, want {p500 Playlist p500}", recent[1].ID, recent[1].Name)
	}
	if recent[2].ID != "mix404" || recent[2].Name != "Generated Mix" {
		t.Errorf("generated row = {%s %s}, want {mix404 Generated Mix}", recent[2].ID, recent[2].Name)
	}

	// A refresh re-derives rows; the known-generated id must not be
	// re-requested while the retryable one is.
	f.p.Refresh()
	if _, err := f.p.Playlists(); err != nil {
		t.Fatal(err)
	}
	if f.nameCalls["mix404"] != 1 {
		t.Errorf("generated name fetches = %d, want short-circuited 1", f.nameCalls["mix404"])
	}
	if f.nameCalls["p500"] != 2 {
		t.Errorf("retryable name fetches = %d, want retried 2", f.nameCalls["p500"])
	}
}

func TestResolvePlaylistNameCachesSuccessAndLabelsGenerated(t *testing.T) {
	f := newRecentFeedFixture(t, `{"items":[]}`)
	f.statusByName["dead"] = 404

	name, browsable := f.p.resolvePlaylistName("live")
	if !browsable || name != "Named live" {
		t.Fatalf("resolve live = (%q, %v), want success", name, browsable)
	}
	name, browsable = f.p.resolvePlaylistName("dead")
	if !browsable || name != "Generated Mix" {
		t.Fatalf("resolve dead = (%q, %v), want generated fallback", name, browsable)
	}

	// Cached success and recorded 404 both skip further HTTP calls.
	f.p.Refresh()
	if name, browsable = f.p.resolvePlaylistName("live"); !browsable || name != "Named live" {
		t.Fatalf("cached live = (%q, %v)", name, browsable)
	}
	if name, browsable = f.p.resolvePlaylistName("dead"); !browsable || name != "Generated Mix" {
		t.Fatalf("cached dead = (%q, %v)", name, browsable)
	}
	if f.nameCalls["live"] != 1 || f.nameCalls["dead"] != 1 {
		t.Fatalf("name fetches = live:%d dead:%d, want 1 each", f.nameCalls["live"], f.nameCalls["dead"])
	}
}

// TestResolvePlaylistNameFallsBackToSpclientName verifies a Web API 404 for a
// generated mix still yields its real display name via the spclient playlist4
// endpoint instead of the "Generated Mix" fallback label.
func TestResolvePlaylistNameFallsBackToSpclientName(t *testing.T) {
	f := newRecentFeedFixture(t, `{"items":[]}`)
	f.statusByName["mix1"] = 404
	fetches := 0
	f.p.playlistFetchFunc = func(_ context.Context, id string) *playlist4pb.SelectedListContent {
		fetches++
		if id != "mix1" {
			t.Errorf("playlist fetch id = %q, want mix1", id)
		}
		return &playlist4pb.SelectedListContent{
			Attributes: &playlist4pb.ListAttributes{Name: proto.String("Daily Mix 1")},
		}
	}

	name, browsable := f.p.resolvePlaylistName("mix1")
	if !browsable || name != "Daily Mix 1" {
		t.Fatalf("resolve mix1 = (%q, %v), want spclient name", name, browsable)
	}
	if !f.p.isGeneratedPlaylist("mix1") {
		t.Error("mix1 not registered as unbrowsable; Tracks() would miss the spclient route")
	}

	// The discovered name is cached; no further lookups of either kind.
	f.p.mu.Lock()
	cached := f.p.playlistNames["mix1"]
	after := fetches
	f.p.mu.Unlock()
	if cached != "Daily Mix 1" {
		t.Fatalf("cached name = %q, want Daily Mix 1", cached)
	}
	if after != 1 {
		t.Fatalf("playlist fetches = %d, want 1", after)
	}
}

func TestPlaylistsSkipsRowsWhenOnlyLocalHistoryExists(t *testing.T) {
	f := newRecentFeedFixture(t, `{"items":[{"played_at":"2026-08-21T09:00:00Z","track":{"id":"remote","name":"Remote","type":"track","uri":"spotify:track:remote"}}]}`)
	// Simulate the Spotify source failing: the merged feed keeps only cliamp
	// plays, and those must no longer produce Recently Played rows.
	f.p.webAPIFunc = func(ctx context.Context, method, path string, query url.Values) (*http.Response, error) {
		if path == "/v1/me/player/recently-played" {
			return nil, context.DeadlineExceeded
		}
		return f.handle(ctx, method, path, query)
	}

	got, err := f.p.Playlists()
	if err != nil {
		t.Fatal(err)
	}
	for _, pl := range got {
		if pl.Section == "Recently Played" {
			t.Fatalf("local history leaked into rows: %+v", got)
		}
	}
	if len(got) != 2 {
		t.Fatalf("playlists = %+v", got)
	}

	// The merged song feed itself still includes local history for IPC.
	result := f.p.UnifiedRecent(context.Background(), 50)
	if len(result.Items) != 1 || result.Items[0].Track.Title != "Local Song" {
		t.Fatalf("unified feed = %+v", result.Items)
	}
}

func TestRecentRowsCapAtTwentyFive(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"items":[`)
	for i := range 30 {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"played_at":"2026-08-21T%02d:%02d:00Z","context":{"type":"album","uri":"spotify:album:a%02d"},"track":{"id":"t%d","name":"T%d","type":"track","uri":"spotify:track:t%d","album":{"name":"A%d"}}}`,
			10+i/60, i%60, i, i, i, i, i)
	}
	b.WriteString(`]}`)
	f := newRecentFeedFixture(t, b.String())

	got, err := f.p.Playlists()
	if err != nil {
		t.Fatal(err)
	}
	rows := 0
	for _, pl := range got {
		if pl.Section == "Recently Played" {
			rows++
		}
	}
	if rows != 25 {
		t.Fatalf("recent rows = %d, want capped 25", rows)
	}
	if got[0].ID != "__clify_album__a29" {
		t.Fatalf("newest row = %q, want a29", got[0].ID)
	}
}

func TestUserIDRetriesAfterFailedMeCall(t *testing.T) {
	p := New(&Session{}, "client", 160)
	meCalls := 0
	p.webAPIFunc = func(_ context.Context, _ string, path string, _ url.Values) (*http.Response, error) {
		switch path {
		case "/v1/me":
			meCalls++
			if meCalls == 1 {
				return nil, context.DeadlineExceeded
			}
			return jsonResponse(200, `{"id":"me"}`), nil
		case "/v1/me/tracks":
			return jsonResponse(200, `{"total":0}`), nil
		case "/v1/me/playlists":
			return jsonResponse(200, `{"items":[{"id":"mine","name":"Mine","snapshot_id":"s1","owner":{"id":"me"},"items":{"total":1}}],"total":1}`), nil
		case "/v1/me/player/recently-played":
			return jsonResponse(200, `{"items":[]}`), nil
		}
		return nil, fmt.Errorf("unexpected path %q", path)
	}

	collapsed, err := p.Playlists()
	if err != nil {
		t.Fatal(err)
	}
	for _, pl := range collapsed {
		if pl.ID == "mine" && pl.Section != "Followed playlists" {
			t.Fatalf("before /v1/me recovery: %+v", collapsed)
		}
	}

	p.Refresh()
	recovered, err := p.Playlists()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, pl := range recovered {
		if pl.Section == "Your playlists" {
			found = true
		}
	}
	if !found {
		t.Fatalf("failed /v1/me cached for the session: %+v", recovered)
	}
}
