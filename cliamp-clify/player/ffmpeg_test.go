package player

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"testing"
)

// TestFFmpegPipeLiveEOF verifies that an unexpected EOF on an infinite radio
// stream is surfaced as an error (so auto-reconnect fires), while a finite
// stream treats EOF as a clean end-of-track.
func TestFFmpegPipeLiveEOF(t *testing.T) {
	tests := []struct {
		name    string
		live    bool
		wantErr bool
	}{
		{name: "live stream EOF is an error", live: true, wantErr: true},
		{name: "finite stream EOF is clean", live: false, wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &ffmpegPipe{
				reader: bufio.NewReader(bytes.NewReader(nil)), // immediate EOF
				live:   tt.live,
			}
			samples := make([][2]float64, 8)
			n, ok := f.Stream(samples)
			if n != 0 || ok {
				t.Fatalf("Stream at EOF: got n=%d ok=%v, want 0, false", n, ok)
			}
			if gotErr := f.Err() != nil; gotErr != tt.wantErr {
				t.Fatalf("Err()=%v, wantErr=%v", f.Err(), tt.wantErr)
			}
			if tt.wantErr && !errors.Is(f.Err(), io.ErrUnexpectedEOF) {
				t.Fatalf("Err()=%v, want io.ErrUnexpectedEOF", f.Err())
			}
		})
	}
}
