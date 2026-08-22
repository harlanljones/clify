package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/bjarneo/cliamp/playlist"
)

// sectionedProviderLists builds a Spotify-shaped provider list with four
// sections; row indexes map 1:1 onto the returned slice.
func sectionedProviderLists() []playlist.PlaylistInfo {
	lists := []playlist.PlaylistInfo{
		{ID: "__clify_album__beta", Name: "Recent Album", Section: "Recently Played"},
		{ID: "liked", Name: "Your Music", Section: "Library"},
		{ID: "mine", Name: "My Playlist", Section: "Your playlists"},
	}
	for i := range 8 {
		lists = append(lists, playlist.PlaylistInfo{
			ID:      strings.Repeat("f", i+1),
			Name:    fmt.Sprintf("Followed %c", 'A'+i),
			Section: "Followed playlists",
		})
	}
	return lists
}

// TestProviderListScrollKeepsFollowedHeaderAndCursorRow pins §3 of the v2
// plan: scrolling to the bottom of a four-section list keeps the cursor row
// inside the budget, never over-clips trailing rows, and renders the Followed
// playlists header whenever the whole trailing group fits.
func TestProviderListScrollKeepsFollowedHeaderAndCursorRow(t *testing.T) {
	lists := sectionedProviderLists()
	cases := []struct {
		visible    int
		wantHeader bool
	}{
		{visible: 6, wantHeader: false}, // header cannot fit beside the tail
		{visible: 7, wantHeader: false},
		{visible: 9, wantHeader: true}, // header + all 8 followed rows fit
	}
	for _, tc := range cases {
		m := Model{
			provider:      commandsTestProvider{name: "Spotify"},
			plVisible:     tc.visible,
			providerLists: lists,
		}
		m.provCursor = len(lists) - 1

		view := m.renderProviderList()
		if !strings.Contains(view, "Followed H") {
			t.Errorf("visible=%d clipped the cursor row:\n%s", tc.visible, view)
		}
		if got := strings.Contains(view, "Followed playlists"); got != tc.wantHeader {
			t.Errorf("visible=%d header present = %t, want %t:\n%s", tc.visible, got, tc.wantHeader, view)
		}
		if tc.wantHeader && (!strings.Contains(view, "Followed A") || !strings.Contains(view, "Followed G")) {
			t.Errorf("visible=%d header rendered without its full group:\n%s", tc.visible, view)
		}
		if lines := strings.Split(strings.TrimRight(view, "\n"), "\n"); len(lines) > tc.visible {
			t.Errorf("visible=%d rendered %d lines, over budget", tc.visible, len(lines))
		}
	}
}

// TestProviderFilterShowsSectionHeaders pins filter-mode behavior: results
// are grouped under their real section headers and the result-count line
// stays inside the viewport instead of being clipped by an off-by-one.
func TestProviderFilterShowsSectionHeaders(t *testing.T) {
	m := Model{
		provider:      commandsTestProvider{name: "Spotify"},
		plVisible:     10,
		providerLists: sectionedProviderLists(),
	}
	m.provSearch.active = true
	m.provSearch.query = "o" // matches "Your Music" plus every Followed row
	m.updateProvSearch()

	view := m.renderProviderList()
	for _, want := range []string{"Your Music", "Library", "Followed playlists", "Followed B"} {
		if !strings.Contains(view, want) {
			t.Fatalf("filter output missing %q:\n%s", want, view)
		}
	}
	if !strings.Contains(view, "playlists") || !strings.Contains(view, "/") {
		t.Fatalf("filter dropped result count line:\n%s", view)
	}
	if lines := strings.Split(strings.TrimRight(view, "\n"), "\n"); len(lines) > m.effectivePlaylistVisible() {
		t.Fatalf("filter rendered %d lines over budget %d", len(lines), m.effectivePlaylistVisible())
	}
}

// TestPlaylistsLoadedClampsScroll pins the post-load clamp: replacing the list
// while the cursor sits past its end must keep the render in bounds instead of
// leaving stale scroll state that clips or panics.
func TestPlaylistsLoadedClampsScroll(t *testing.T) {
	current := commandsTestProvider{name: "Spotify"}
	m := Model{
		provider:      current,
		plVisible:     5,
		providerLists: sectionedProviderLists(),
	}
	m.requests.provider = 1
	m.provCursor = 10 // last row of the previous, longer list

	short := []playlist.PlaylistInfo{
		{ID: "liked", Name: "Your Music", Section: "Library"},
		{ID: "mine", Name: "My Playlist", Section: "Your playlists"},
	}
	updated, _ := m.Update(playlistsLoadedMsg{playlists: short, providerName: "Spotify", gen: 1})
	m = updated.(Model)

	if m.provCursor >= len(m.providerLists) {
		t.Fatalf("cursor %d out of bounds for %d lists", m.provCursor, len(m.providerLists))
	}
	m.providerMaybeAdjustScroll()
	if rows := m.providerRowsFromScroll(m.provScroll, m.provCursor); rows > 5 {
		t.Fatalf("cursor needs %d rendered rows, over budget 5", rows)
	}
}
