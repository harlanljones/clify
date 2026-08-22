package recent

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/bjarneo/cliamp/playlist"
)

func TestMergeOrdersDeduplicatesAndUnionsSources(t *testing.T) {
	old := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	newer := old.Add(time.Minute)
	local := []Item{{Track: playlist.Track{Path: "spotify:track:one", Title: "old"}, PlayedAt: old}}
	spotify := []Item{
		{Track: playlist.Track{Path: "spotify:track:one", Title: "new"}, PlayedAt: newer},
		{Track: playlist.Track{Path: "spotify:track:two", Title: "two"}, PlayedAt: old.Add(-time.Minute)},
	}

	got := Merge(10,
		NamedItems{Name: "cliamp", Items: local},
		NamedItems{Name: "spotify", Items: spotify},
	)

	if got.Partial || len(got.FailedSources) != 0 {
		t.Fatalf("unexpected partial result: %+v", got)
	}
	if len(got.Items) != 2 {
		t.Fatalf("len = %d, want 2", len(got.Items))
	}
	if got.Items[0].Track.Title != "new" {
		t.Fatalf("newest representation = %q, want new", got.Items[0].Track.Title)
	}
	if want := []string{"cliamp", "spotify"}; !reflect.DeepEqual(got.Items[0].Sources, want) {
		t.Fatalf("sources = %v, want %v", got.Items[0].Sources, want)
	}
	if got.Items[0].Track.Meta("clify.sources") != "cliamp,spotify" {
		t.Fatalf("metadata sources = %q", got.Items[0].Track.Meta("clify.sources"))
	}
	if got.Items[0].Track.Meta("clify.played_at") != newer.Format(time.RFC3339) {
		t.Fatalf("metadata played_at = %q", got.Items[0].Track.Meta("clify.played_at"))
	}
}

func TestMergeKeepsLocalTracksDistinctAndSortsZeroLast(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	got := Merge(0, NamedItems{Name: "cliamp", Items: []Item{
		{Track: playlist.Track{Path: "/music/a.flac", Title: "Same"}, PlayedAt: time.Time{}},
		{Track: playlist.Track{Path: "/music/b.flac", Title: "Same"}, PlayedAt: now},
	}})

	if len(got.Items) != 2 {
		t.Fatalf("len = %d, want 2", len(got.Items))
	}
	if got.Items[0].Track.Path != "/music/b.flac" || got.Items[1].Track.Path != "/music/a.flac" {
		t.Fatalf("order = %q, %q", got.Items[0].Track.Path, got.Items[1].Track.Path)
	}
}

func TestMergeAppliesLimitAfterDedupAndDoesNotMutateInputs(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	items := []Item{
		{Track: playlist.Track{Path: "spotify:track:one"}, PlayedAt: now},
		{Track: playlist.Track{Path: "spotify:track:one"}, PlayedAt: now.Add(-time.Second)},
		{Track: playlist.Track{Path: "spotify:track:two"}, PlayedAt: now.Add(-2 * time.Second)},
	}
	original := append([]Item(nil), items...)

	got := Merge(2, NamedItems{Name: "spotify", Items: items})
	if len(got.Items) != 2 || got.Items[1].Track.Path != "spotify:track:two" {
		t.Fatalf("items = %+v", got.Items)
	}
	if !reflect.DeepEqual(items, original) {
		t.Fatal("Merge mutated its source slice")
	}
}

func TestMergeReportsFailedSourceAndKeepsHealthyData(t *testing.T) {
	got := Merge(20,
		NamedItems{Name: "cliamp", Items: []Item{{Track: playlist.Track{Path: "/music/a.flac"}}}},
		NamedItems{Name: "spotify", Err: errors.New("offline")},
	)
	if !got.Partial || !reflect.DeepEqual(got.FailedSources, []string{"spotify"}) {
		t.Fatalf("partial result = %+v", got)
	}
	if len(got.Items) != 1 {
		t.Fatalf("healthy items lost: %+v", got.Items)
	}
}

func TestMergeDeduplicatesStableProviderID(t *testing.T) {
	now := time.Now().UTC()
	got := Merge(20,
		NamedItems{Name: "a", Items: []Item{{Track: playlist.Track{Path: "first", ProviderMeta: map[string]string{"navidrome.id": "42"}}, PlayedAt: now}}},
		NamedItems{Name: "b", Items: []Item{{Track: playlist.Track{Path: "second", ProviderMeta: map[string]string{"navidrome.id": "42"}}, PlayedAt: now.Add(-time.Minute)}}},
	)
	if len(got.Items) != 1 {
		t.Fatalf("len = %d, want provider ID duplicate collapsed", len(got.Items))
	}
}

func BenchmarkMergeRecent(b *testing.B) {
	now := time.Now().UTC()
	local := make([]Item, 200)
	spotify := make([]Item, 200)
	for i := range 200 {
		local[i] = Item{
			Track:    playlist.Track{Path: fmt.Sprintf("/music/%03d.flac", i)},
			PlayedAt: now.Add(-time.Duration(i) * time.Second),
		}
		spotify[i] = Item{
			Track:    playlist.Track{Path: fmt.Sprintf("spotify:track:%03d", i)},
			PlayedAt: now.Add(-time.Duration(i) * time.Second),
		}
	}
	b.ResetTimer()
	for range b.N {
		Merge(50,
			NamedItems{Name: "cliamp", Items: local},
			NamedItems{Name: "spotify", Items: spotify},
		)
	}
}
