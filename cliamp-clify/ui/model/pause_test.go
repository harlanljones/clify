package model

import (
	"testing"
	"time"

	"github.com/bjarneo/cliamp/playlist"
)

func TestShouldReconnectOnUnpause(t *testing.T) {
	tests := []struct {
		name  string
		track playlist.Track
		idx   int
		pause time.Duration
		want  bool
	}{
		{
			name: "live http stream reconnects",
			track: playlist.Track{
				Path:     "https://radio.example.com/stream",
				Stream:   true,
				Realtime: true,
			},
			idx:  0,
			want: true,
		},
		{
			name: "short-paused yt-dlp stream does not reconnect",
			track: playlist.Track{
				Path:   "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
				Stream: true,
			},
			idx:   0,
			pause: ytdlReconnectPauseThreshold - time.Second,
			want:  false,
		},
		{
			name: "long-paused yt-dlp stream reconnects",
			track: playlist.Track{
				Path:   "https://music.youtube.com/watch?v=dQw4w9WgXcQ",
				Stream: true,
			},
			idx:   0,
			pause: ytdlReconnectPauseThreshold,
			want:  true,
		},
		{
			name: "invalid current index does not reconnect",
			track: playlist.Track{
				Path:     "https://radio.example.com/stream",
				Stream:   true,
				Realtime: true,
			},
			idx:  -1,
			want: false,
		},
		{
			name: "known duration live stream still reconnects",
			track: playlist.Track{
				Path:         "https://radio.example.com/show.mp3",
				Stream:       true,
				Realtime:     true,
				DurationSecs: 120,
			},
			idx:  0,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldReconnectOnUnpause(tt.track, tt.idx, tt.pause); got != tt.want {
				t.Fatalf("shouldReconnectOnUnpause(...) = %v, want %v", got, tt.want)
			}
		})
	}
}
