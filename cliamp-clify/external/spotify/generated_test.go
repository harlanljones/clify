//go:build !windows

package spotify

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	librespot "github.com/devgianlu/go-librespot"
	metadatapb "github.com/devgianlu/go-librespot/proto/spotify/metadata"
	"google.golang.org/protobuf/proto"

	connectpb "github.com/devgianlu/go-librespot/proto/spotify/connectstate"
)

func TestGeneratedKindClassification(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"Daily Mix 1", "daily_mix"},
		{"daily mix 3", "daily_mix"},
		{"Discover Weekly", "discover_weekly"},
		{"Release Radar", "release_radar"},
		{"On Repeat", "on_repeat"},
		{"Repeat Rewind", "repeat_rewind"},
		{"Your daylist for August", "daylist"},
		{"My Shazam Tracks", "other"},
		{"", "other"},
	}
	for _, tc := range cases {
		if got := generatedKind(tc.name); got != tc.want {
			t.Errorf("generatedKind(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestIsGeneratedPlaylistItem(t *testing.T) {
	cases := []struct {
		item spotifyPlaylistItem
		want bool
	}{
		{spotifyPlaylistItem{ID: "37i9dQZF1E4mX", Owner: struct {
			ID string `json:"id"`
		}{ID: "someone"}}, true},
		{spotifyPlaylistItem{ID: "whatever123", Owner: struct {
			ID string `json:"id"`
		}{ID: "spotify"}}, true},
		{spotifyPlaylistItem{ID: "whatever123", Owner: struct {
			ID string `json:"id"`
		}{ID: "me"}}, false},
		{spotifyPlaylistItem{Owner: struct {
			ID string `json:"id"`
		}{ID: "spotify"}}, false}, // malformed: missing id
	}
	for i, tc := range cases {
		if got := isGeneratedPlaylistItem(tc.item); got != tc.want {
			t.Errorf("case %d: isGeneratedPlaylistItem(%+v) = %v, want %v", i, tc.item, got, tc.want)
		}
	}
}

// generatedFixture serves a fixed /v1/me/playlists listing plus library
// endpoints, and records whether Web API playlist-items were requested.
type generatedFixture struct {
	p          *SpotifyProvider
	mu         sync.Mutex
	itemsCall  int // /v1/playlists/{id}/items hits
	pagesFetch int // context-resolve next-page fetches
}

func newGeneratedFixture(t *testing.T, playlistsJSON string) *generatedFixture {
	t.Helper()
	f := &generatedFixture{}
	f.p = New(&Session{}, "client", 160)
	f.p.webAPIFunc = func(_ context.Context, _ string, path string, _ url.Values) (*http.Response, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case path == "/v1/me":
			return jsonResponse(200, `{"id":"me"}`), nil
		case path == "/v1/me/tracks":
			return jsonResponse(200, `{"items":[],"total":0}`), nil
		case path == "/v1/me/playlists":
			return jsonResponse(200, fmt.Sprintf(`{"items":[%s],"total":1}`, playlistsJSON)), nil
		case strings.HasPrefix(path, "/v1/playlists/") && strings.HasSuffix(path, "/items"):
			f.itemsCall++
			return jsonResponse(200, `{"items":[{"track":{"id":"web","name":"Web Track","type":"track","uri":"spotify:track:web"}}],"total":1}`), nil
		case path == "/v1/me/player/recently-played":
			return jsonResponse(200, `{"items":[]}`), nil
		}
		return nil, fmt.Errorf("unexpected path %q", path)
	}
	return f
}

func TestPlaylistsBuildsMadeForYouSectionInOrder(t *testing.T) {
	f := newGeneratedFixture(t,
		`{"id":"regular1","name":"Buddy List","snapshot_id":"s1","owner":{"id":"me"},"items":{"total":4}},`+
			`{"id":"37i9dQZF1E8XDailyMix","name":"Daily Mix 1","owner":{"id":"spotify"},"items":{"total":30}},`+
			`{"id":"37i9dQZF1ExUserOwned","name":"Discover Weekly","owner":{"id":"someone-else"},"items":{"total":2}},`+
			`{"name":"No Id At All","owner":{"id":"spotify"},"items":{"total":7}},`+
			`{"id":"followed1","name":"Followed Thing","snapshot_id":"s2","owner":{"id":"other-user"},"items":{"total":1}}`)

	got, err := f.p.Playlists()
	if err != nil {
		t.Fatal(err)
	}

	var want []struct{ id, name, section string }
	add := func(id, name, section string) {
		want = append(want, struct{ id, name, section string }{id, name, section})
	}
	add("37i9dQZF1E8XDailyMix", "Daily Mix 1", "Made For You")
	add("37i9dQZF1ExUserOwned", "Discover Weekly", "Made For You")
	add("YOUR MUSIC", "Your Music", "Library")
	add("regular1", "Buddy List", "Your playlists")
	add("followed1", "Followed Thing", "Followed playlists")

	if len(got) != len(want) {
		t.Fatalf("playlists = %+v, want %d rows", got, len(want))
	}
	for i, w := range want {
		if got[i].ID != w.id || got[i].Name != w.name || got[i].Section != w.section {
			t.Errorf("row[%d] = {%s %s %s}, want {%s %s %s}", i, got[i].ID, got[i].Name, got[i].Section, w.id, w.name, w.section)
		}
	}

	// Discovered ids are registered so Tracks() routes them via spclient.
	for _, id := range []string{"37i9dQZF1E8XDailyMix", "37i9dQZF1ExUserOwned"} {
		if !f.p.isGeneratedPlaylist(id) {
			t.Errorf("isGeneratedPlaylist(%q) = false, want true", id)
		}
	}
	if f.p.isGeneratedPlaylist("regular1") || f.p.isGeneratedPlaylist("followed1") {
		t.Error("non-generated ids registered as generated")
	}
}

func TestTracksRoutesGeneratedThroughContextResolve(t *testing.T) {
	f := newGeneratedFixture(t, `{"id":"mix1","name":"Daily Mix 1","owner":{"id":"spotify"},"items":{"total":1}}`)
	resolves := 0
	f.p.contextResolveFunc = func(_ context.Context, uri string) (*connectpb.Context, error) {
		resolves++
		if uri != "spotify:playlist:mix1" {
			t.Errorf("resolve uri = %q, want spotify:playlist:mix1", uri)
		}
		return &connectpb.Context{
			Uri: uri,
			Pages: []*connectpb.ContextPage{{
				Tracks: []*connectpb.ContextTrack{
					{
						Uri: "spotify:track:t1",
						Metadata: map[string]string{
							"title":       "Song One",
							"artist_name": "Artist",
							"album_title": "Album",
							"duration":    "150000",
						},
					},
					{Uri: "spotify:episode:e1"}, // non-track entries are skipped
					{Gid: []byte{0x01, 0x02}},
				},
			}},
		}, nil
	}

	if _, err := f.p.Playlists(); err != nil {
		t.Fatal(err)
	}

	tracks, err := f.p.Tracks("mix1")
	if err != nil {
		t.Fatal(err)
	}
	if resolves != 1 {
		t.Fatalf("context resolves = %d, want 1", resolves)
	}
	if len(tracks) != 1 || tracks[0].Title != "Song One" || tracks[0].Artist != "Artist" ||
		tracks[0].Album != "Album" || tracks[0].DurationSecs != 150 || tracks[0].Path != "spotify:track:t1" {
		t.Fatalf("tracks = %+v", tracks)
	}

	// Second open is served from cache without another resolve or Web API hit.
	if _, err := f.p.Tracks("mix1"); err != nil {
		t.Fatal(err)
	}
	if resolves != 1 {
		t.Fatalf("context resolves after repeat = %d, want cached 1", resolves)
	}

	// Normal playlists still go through the Web API items endpoint.
	if _, err := f.p.Tracks("regular1"); err != nil {
		t.Fatal(err)
	}
	if f.itemsCall != 1 || resolves != 1 {
		t.Fatalf("routing mixed up: web items = %d, resolves = %d", f.itemsCall, resolves)
	}

	// Unbrowsable (404'd) ids route through spclient too.
	if f.p.unbrowsablePlaylists == nil {
		f.p.unbrowsablePlaylists = make(map[string]struct{})
	}
	f.p.unbrowsablePlaylists["mix404"] = struct{}{}
	if !f.p.isGeneratedPlaylist("mix404") {
		t.Error("unbrowsable id not treated as resolvable-generated")
	}
}

func TestGeneratedTracksFollowPagesUpToCap(t *testing.T) {
	f := newGeneratedFixture(t, `{"id":"big","name":"Daily Mix 5","owner":{"id":"spotify"},"items":{"total":99}}`)
	page := func(n int) *connectpb.ContextPage {
		return &connectpb.ContextPage{
			NextPageUrl: fmt.Sprintf("hm://context-resolve/v1/pages/%d", n),
			Tracks:      []*connectpb.ContextTrack{{Uri: "spotify:track:p" + strconv.Itoa(n)}},
		}
	}
	f.p.contextResolveFunc = func(context.Context, string) (*connectpb.Context, error) {
		return &connectpb.Context{Uri: "spotify:playlist:big", Pages: []*connectpb.ContextPage{page(0)}}, nil
	}
	f.p.contextPageFunc = func(_ context.Context, pageURL string) (*connectpb.ContextPage, error) {
		f.mu.Lock()
		f.pagesFetch++
		f.mu.Unlock()
		if !strings.HasPrefix(pageURL, "hm://") {
			t.Errorf("page url = %q, want hm:// prefix preserved for caller", pageURL)
		}
		return page(f.pagesFetch), nil
	}

	f.p.registerGeneratedPlaylist("big")
	tracks, err := f.p.Tracks("big")
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != generatedMaxPages {
		t.Fatalf("tracks = %d, want capped %d", len(tracks), generatedMaxPages)
	}
	if got := f.pagesFetch; got != generatedMaxPages-1 {
		t.Fatalf("next-page fetches = %d, want %d", got, generatedMaxPages-1)
	}
}

// TestGeneratedTracksHydrateMissingSongNames verifies context-resolve results
// whose metadata maps are empty still get real song names via spclient
// extended metadata, instead of rendering blank rows. The Web API catalog is
// 403-blocked for Development-Mode apps, so hydration must not touch it.
func TestGeneratedTracksHydrateMissingSongNames(t *testing.T) {
	f := newGeneratedFixture(t, `{"id":"blank","name":"On Repeat","owner":{"id":"spotify"},"items":{"total":2}}`)
	f.p.contextResolveFunc = func(context.Context, string) (*connectpb.Context, error) {
		return &connectpb.Context{
			Uri: "spotify:playlist:blank",
			Pages: []*connectpb.ContextPage{{
				Tracks: []*connectpb.ContextTrack{
					{Uri: "spotify:track:1111111111111111111111"},
					{Uri: "spotify:track:2222222222222222222222"},
				},
			}},
		}, nil
	}
	f.p.registerGeneratedPlaylist("blank")
	f.p.trackMetadataFunc = func(_ context.Context, id librespot.SpotifyId, meta *metadatapb.Track) error {
		switch id.Base62() {
		case "1111111111111111111111":
			meta.Name = proto.String("Real Song One")
			meta.Artist = []*metadatapb.Artist{{Name: proto.String("A One")}}
			meta.Album = &metadatapb.Album{Name: proto.String("Album One")}
			meta.Duration = proto.Int32(111000)
		case "2222222222222222222222":
			meta.Name = proto.String("Real Song Two")
			meta.Artist = []*metadatapb.Artist{{Name: proto.String("A Two")}, {Name: proto.String("A Three")}}
			meta.Album = &metadatapb.Album{Name: proto.String("Album Two")}
			meta.Duration = proto.Int32(222000)
		default:
			return errors.New("unexpected track")
		}
		return nil
	}

	tracks, err := f.p.Tracks("blank")
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 2 || tracks[0].Title != "Real Song One" || tracks[0].Artist != "A One" ||
		tracks[0].Album != "Album One" || tracks[0].DurationSecs != 111 ||
		tracks[1].Title != "Real Song Two" || tracks[1].Artist != "A Two, A Three" || tracks[1].DurationSecs != 222 {
		t.Fatalf("tracks = %+v", tracks)
	}
}

// TestGeneratedTracksEmptyResolveNotCached verifies a context-resolve that
// yields zero usable tracks surfaces an error and is retried later instead of
// being cached as an empty pane for the session.
func TestGeneratedTracksEmptyResolveNotCached(t *testing.T) {
	f := newGeneratedFixture(t, `{"id":"empty","name":"Daily Mix 2","owner":{"id":"spotify"},"items":{"total":9}}`)
	resolves := 0
	f.p.registerGeneratedPlaylist("empty")
	f.p.contextResolveFunc = func(context.Context, string) (*connectpb.Context, error) {
		resolves++
		return &connectpb.Context{Uri: "spotify:playlist:empty"}, nil
	}

	if _, err := f.p.Tracks("empty"); err == nil || !strings.Contains(err.Error(), "resolved no tracks") {
		t.Fatalf("err = %v, want no-tracks failure", err)
	}
	if _, err := f.p.Tracks("empty"); err == nil {
		t.Fatal("second Tracks error = nil, want retry")
	}
	if resolves != 2 {
		t.Fatalf("context resolves = %d, want retried 2 (empty result must not be cached)", resolves)
	}
}

func TestGeneratedTracksDegradeGracefullyOnResolverFailure(t *testing.T) {
	f := newGeneratedFixture(t, `{"id":"broken","name":"Release Radar","owner":{"id":"spotify"},"items":{"total":5}}`)
	f.p.contextResolveFunc = func(context.Context, string) (*connectpb.Context, error) {
		return nil, errors.New("spclient exploded")
	}

	if _, err := f.p.Playlists(); err != nil {
		t.Fatal(err)
	}
	_, err := f.p.Tracks("broken")
	if err == nil {
		t.Fatal("Tracks error = nil, want resolver failure surfaced")
	}
	if !strings.HasPrefix(err.Error(), "spotify: list tracks:") {
		t.Fatalf("err = %v, want standard fetch-failure shape", err)
	}
	// The Web API items endpoint must not have been tried as a fallback.
	if f.itemsCall != 0 {
		t.Fatalf("web api items calls = %d, want 0", f.itemsCall)
	}

	// Page-fetch failures degrade the same way.
	f2 := newGeneratedFixture(t, `{"id":"paged","name":"On Repeat","owner":{"id":"spotify"},"items":{"total":5}}`)
	f2.p.contextResolveFunc = func(context.Context, string) (*connectpb.Context, error) {
		return &connectpb.Context{
			Uri:   "spotify:playlist:paged",
			Pages: []*connectpb.ContextPage{{NextPageUrl: "hm://next"}},
		}, nil
	}
	f2.p.contextPageFunc = func(context.Context, string) (*connectpb.ContextPage, error) {
		return nil, errors.New("page fetch failed")
	}
	if _, err := f2.p.Playlists(); err != nil {
		t.Fatal(err)
	}
	if _, err := f2.p.Tracks("paged"); err == nil || !strings.Contains(err.Error(), "fetch next page") {
		t.Fatalf("err = %v, want wrapped next-page failure", err)
	}
}

func TestTrackFromContextTrack(t *testing.T) {
	cases := []struct {
		name    string
		track   *connectpb.ContextTrack
		wantOK  bool
		wantURI string
	}{
		{"uri track", &connectpb.ContextTrack{Uri: "spotify:track:abc"}, true, "spotify:track:abc"},
		{"episode skipped", &connectpb.ContextTrack{Uri: "spotify:episode:xyz"}, false, ""},
		{"local skipped", &connectpb.ContextTrack{Uri: "spotify:local:"}, false, ""},
		{"empty skipped", &connectpb.ContextTrack{}, false, ""},
	}
	for _, tc := range cases {
		got, ok := trackFromContextTrack(tc.track)
		if ok != tc.wantOK {
			t.Errorf("%s: ok = %v, want %v", tc.name, ok, tc.wantOK)
			continue
		}
		if ok && got.Path != tc.wantURI {
			t.Errorf("%s: path = %q, want %q", tc.name, got.Path, tc.wantURI)
		}
	}

	// A well-formed 16-byte gid converts to a canonical track URI.
	wantURI := librespot.SpotifyIdFromGid(librespot.SpotifyIdTypeTrack, make([]byte, 16)).Uri()
	got, ok := trackFromContextTrack(&connectpb.ContextTrack{Gid: make([]byte, 16)})
	if !ok || got.Path != wantURI {
		t.Errorf("gid track = (%+v, %v), want uri %s", got, ok, wantURI)
	}
}
