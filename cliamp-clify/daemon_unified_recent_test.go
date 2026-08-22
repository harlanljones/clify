package main

import (
	"context"
	"testing"
	"time"

	"github.com/bjarneo/cliamp/ipc"
	"github.com/bjarneo/cliamp/playlist"
	"github.com/bjarneo/cliamp/recent"
	"github.com/bjarneo/cliamp/ui/model"
)

type daemonUnifiedRecentProvider struct {
	result recent.Result
}

func (p daemonUnifiedRecentProvider) Name() string { return "Spotify" }
func (p daemonUnifiedRecentProvider) Playlists() ([]playlist.PlaylistInfo, error) {
	return nil, nil
}
func (p daemonUnifiedRecentProvider) Tracks(string) ([]playlist.Track, error) { return nil, nil }
func (p daemonUnifiedRecentProvider) UnifiedRecent(context.Context, int) recent.Result {
	return p.result
}

func TestDaemonUnifiedHistoryMatchesVersionedContract(t *testing.T) {
	provider := daemonUnifiedRecentProvider{result: recent.Result{Items: []recent.Item{{
		Track:    playlist.Track{Path: "spotify:track:abc", Title: "Song"},
		PlayedAt: time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC),
		Sources:  []string{"spotify"},
	}}}}
	d := daemon{providers: []model.ProviderEntry{{Key: "spotify", Provider: provider}}}
	replyChannel := make(chan ipc.Response, 1)
	d.handleHistory(ipc.HistoryRequestMsg{Op: "history.unified", Limit: 20, Reply: replyChannel})
	response := <-replyChannel
	if !response.OK || response.SchemaVersion != "cliamp.history.unified/1" {
		t.Fatalf("response = %+v", response)
	}
	if len(response.History) != 1 || response.History[0].PlayedAt != "2026-08-21T10:00:00Z" {
		t.Fatalf("history = %+v", response.History)
	}
}
