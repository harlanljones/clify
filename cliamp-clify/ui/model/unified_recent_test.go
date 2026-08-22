package model

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bjarneo/cliamp/ipc"
	"github.com/bjarneo/cliamp/playlist"
	"github.com/bjarneo/cliamp/recent"
)

type unifiedRecentTestProvider struct {
	result recent.Result
}

func (p *unifiedRecentTestProvider) Name() string { return "Spotify" }
func (p *unifiedRecentTestProvider) Playlists() ([]playlist.PlaylistInfo, error) {
	return nil, nil
}
func (p *unifiedRecentTestProvider) Tracks(string) ([]playlist.Track, error) { return nil, nil }
func (p *unifiedRecentTestProvider) UnifiedRecent(context.Context, int) recent.Result {
	return p.result
}

func TestRecentlyPlayedSectionRendersDerivedRowsFirst(t *testing.T) {
	provider := &unifiedRecentTestProvider{}
	m := Model{
		provider:  provider,
		plVisible: 14,
		providerLists: []playlist.PlaylistInfo{
			{ID: "__clify_album__beta", Name: "Beta", Section: "Recently Played"},
			{ID: "p1", Name: "Named p1", Section: "Recently Played"},
			{ID: "liked", Name: "Your Music", TrackCount: 61, Section: "Library"},
			{ID: "mine", Name: "My Playlist", TrackCount: 2, Section: "Your playlists"},
		},
	}
	view := m.renderProviderList()
	positions := []int{
		strings.Index(view, "Recently Played"),
		strings.Index(view, "Beta"),
		strings.Index(view, "Named p1"),
		strings.Index(view, "Library"),
		strings.Index(view, "Your Music"),
		strings.Index(view, "Your playlists"),
	}
	for i, position := range positions {
		if position < 0 || (i > 0 && position <= positions[i-1]) {
			t.Fatalf("unexpected provider rendering order:\n%s", view)
		}
	}
	if strings.Count(view, "Recently Played") != 1 {
		t.Fatalf("expected exactly one Recently Played header:\n%s", view)
	}
	if strings.Contains(view, "All sources") {
		t.Fatalf("legacy 'All sources' row still rendered:\n%s", view)
	}
	// Header + 4 selectable items render above the fold; headers stay out of
	// the selectable model.
	if rows := m.providerRowsFromScroll(0, 3); rows != 7 {
		t.Fatalf("rendered rows through fourth selectable item = %d, want 7", rows)
	}
	if len(m.providerLists) != 4 {
		t.Fatalf("headers entered selectable model: %+v", m.providerLists)
	}
}

func TestTUIUnifiedHistoryIPCContract(t *testing.T) {
	playedAt := time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)
	provider := &unifiedRecentTestProvider{result: recent.Result{
		Items: []recent.Item{{
			Track:    playlist.Track{Path: "spotify:track:abc", Title: "Song"},
			PlayedAt: playedAt, Sources: []string{"cliamp", "spotify"},
		}},
	}}
	reply := make(chan ipc.Response, 1)
	m := Model{providers: []ProviderEntry{{Key: "spotify", Provider: provider}}}
	cmd := m.handleIPCHistory(ipc.HistoryRequestMsg{Op: "history.unified", Limit: 20, Reply: reply})
	cmd()
	response := <-reply
	if !response.OK || response.SchemaVersion != "cliamp.history.unified/1" {
		t.Fatalf("response = %+v", response)
	}
	if len(response.History) != 1 || len(response.History[0].Sources) != 2 {
		t.Fatalf("history = %+v", response.History)
	}
}
