package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestVisualizerFitsTinyRectangles(t *testing.T) {
	for mode := VisMode(0); mode < VisCount; mode++ {
		t.Run(visModes[mode].name, func(t *testing.T) {
			for _, rect := range []struct{ cols, rows int }{{0, 0}, {1, 1}, {8, 2}} {
				v := NewVisualizer(44100)
				v.Mode = mode
				v.Cols = rect.cols
				v.Rows = rect.rows
				v.Tick(VisTickContext{})
				got := v.Render()
				if rect.cols == 0 || rect.rows == 0 || mode == VisNone {
					if got != "" {
						t.Fatalf("%dx%d render = %q, want empty", rect.cols, rect.rows, got)
					}
					continue
				}
				lines := strings.Split(got, "\n")
				if len(lines) != rect.rows {
					t.Fatalf("%dx%d line count = %d, want %d: %q", rect.cols, rect.rows, len(lines), rect.rows, got)
				}
				for _, line := range lines {
					if width := lipgloss.Width(line); width != rect.cols {
						t.Fatalf("%dx%d line width = %d, want %d: %q", rect.cols, rect.rows, width, rect.cols, line)
					}
				}
			}
		})
	}
}

func TestVisualizerFrameClipsPluginOutput(t *testing.T) {
	frame := fitVisualizerFrame(strings.Repeat("界", 20)+"\n"+strings.Repeat("plugin output ", 20), 8, 2)
	lines := strings.Split(frame, "\n")
	if len(lines) != 2 {
		t.Fatalf("line count = %d, want 2", len(lines))
	}
	for _, line := range lines {
		if width := lipgloss.Width(line); width != 8 {
			t.Fatalf("line width = %d, want 8: %q", width, line)
		}
	}
}
