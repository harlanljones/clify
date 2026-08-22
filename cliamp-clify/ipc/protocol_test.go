package ipc

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRequestMarshal(t *testing.T) {
	req := Request{Cmd: "play"}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Request
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded.Cmd != "play" {
		t.Errorf("Cmd = %q, want play", decoded.Cmd)
	}
}

func TestRequestWithValue(t *testing.T) {
	req := Request{Cmd: "volume", Value: -5.0}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Request
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded.Cmd != "volume" || decoded.Value != -5.0 {
		t.Errorf("got Cmd=%q Value=%f, want volume -5.0", decoded.Cmd, decoded.Value)
	}
}

func TestRequestOmitsEmptyFields(t *testing.T) {
	req := Request{Cmd: "next"}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if _, ok := raw["value"]; ok {
		t.Error("zero value should be omitted")
	}
	if _, ok := raw["playlist"]; ok {
		t.Error("empty playlist should be omitted")
	}
	if _, ok := raw["path"]; ok {
		t.Error("empty path should be omitted")
	}
}

func TestResponseMarshal(t *testing.T) {
	track := &TrackInfo{Title: "Song", Artist: "Artist", Path: "/music/song.mp3"}
	resp := Response{
		OK:       true,
		State:    "playing",
		Track:    track,
		Position: 30.5,
		Duration: 180.0,
		Volume:   -10.0,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Response
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if !decoded.OK {
		t.Error("OK should be true")
	}
	if decoded.State != "playing" {
		t.Errorf("State = %q, want playing", decoded.State)
	}
	if decoded.Track == nil {
		t.Fatal("Track should not be nil")
	}
	if decoded.Track.Title != "Song" {
		t.Errorf("Track.Title = %q, want Song", decoded.Track.Title)
	}
	if decoded.Position != 30.5 {
		t.Errorf("Position = %f, want 30.5", decoded.Position)
	}
}

func TestResponseError(t *testing.T) {
	resp := Response{OK: false, Error: "track not found"}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Response
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded.OK {
		t.Error("OK should be false")
	}
	if decoded.Error != "track not found" {
		t.Errorf("Error = %q, want 'track not found'", decoded.Error)
	}
}

func TestUnifiedHistoryResponseJSONContract(t *testing.T) {
	resp := Response{
		OK:            true,
		SchemaVersion: "cliamp.history.unified/1",
		History: []HistoryInfo{{
			Track:    TrackInfo{Title: "Song", Path: "spotify:track:abc"},
			PlayedAt: "2026-08-21T10:00:00Z",
			Sources:  []string{"cliamp", "spotify"},
		}},
		Partial:       true,
		FailedSources: []string{"spotify"},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["schema_version"] != "cliamp.history.unified/1" || raw["partial"] != true {
		t.Fatalf("response = %s", data)
	}
	history := raw["history"].([]any)
	entry := history[0].(map[string]any)
	if len(entry["sources"].([]any)) != 2 {
		t.Fatalf("sources = %#v", entry["sources"])
	}
}

func TestLegacyHistoryResponseOmitsUnifiedFields(t *testing.T) {
	data, err := json.Marshal(Response{OK: true, History: []HistoryInfo{{Track: TrackInfo{Path: "/a"}}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"schema_version", "partial", "failed_sources", "sources"} {
		if strings.Contains(string(data), `"`+field+`"`) {
			t.Fatalf("legacy response unexpectedly contains %q: %s", field, data)
		}
	}
}

func TestDispatcherFunc(t *testing.T) {
	var received any
	fn := DispatcherFunc(func(msg any) {
		received = msg
	})

	fn.Send("test message")

	if received != "test message" {
		t.Errorf("received = %v, want 'test message'", received)
	}
}

func TestTrackInfoMarshal(t *testing.T) {
	info := TrackInfo{Title: "Song", Artist: "Artist", Path: "/path/song.mp3"}
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded TrackInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded.Title != "Song" || decoded.Artist != "Artist" || decoded.Path != "/path/song.mp3" {
		t.Errorf("decoded = %+v, want Title=Song Artist=Artist Path=/path/song.mp3", decoded)
	}
}

func TestResponseBoolPointerFields(t *testing.T) {
	// Shuffle and Mono are *bool so they can distinguish unset from false
	trueBool := true
	resp := Response{
		OK:      true,
		Shuffle: &trueBool,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Response
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Shuffle == nil || !*decoded.Shuffle {
		t.Error("Shuffle should be *true")
	}
	if decoded.Mono != nil {
		t.Error("Mono should be nil when unset")
	}
}

func TestStructuredLibraryResponseRoundTrip(t *testing.T) {
	resp := Response{
		OK:        true,
		Providers: []ProviderInfo{{Key: "local", Name: "Local", Searchable: true}},
		Playlists: []PlaylistInfo{{ID: "mix", Name: "Mix", Provider: "local", TrackCount: 3}},
		Tracks:    []TrackInfo{{Title: "Song", Album: "Album", Path: "/song.flac", Index: 2}},
		Lyrics:    []LyricLine{{Start: 12.5, Text: "Line"}},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Response
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Providers) != 1 || decoded.Playlists[0].TrackCount != 3 || decoded.Tracks[0].Album != "Album" || decoded.Lyrics[0].Start != 12.5 {
		t.Fatalf("structured response did not round trip: %#v", decoded)
	}
}
