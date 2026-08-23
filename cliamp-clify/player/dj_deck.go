package player

import (
	"sync"
	"time"

	"github.com/gopxl/beep/v2"
)

// DJDeck is the state associated with one independently loaded track.
type DJDeck struct {
	mu       sync.RWMutex
	pipeline *trackPipeline
	path     string
	position time.Duration
	paused   bool
	cue      time.Duration
	bpm      BPMResult
}

func (d *DJDeck) Path() string            { d.mu.RLock(); defer d.mu.RUnlock(); return d.path }
func (d *DJDeck) Position() time.Duration { d.mu.RLock(); defer d.mu.RUnlock(); return d.position }
func (d *DJDeck) SetCue(position time.Duration) {
	d.mu.Lock()
	d.cue = maxDuration(0, position)
	d.mu.Unlock()
}
func (d *DJDeck) Cue() time.Duration { d.mu.RLock(); defer d.mu.RUnlock(); return d.cue }
func (d *DJDeck) BPM() BPMResult     { d.mu.RLock(); defer d.mu.RUnlock(); return d.bpm }

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

func (d *DJDeck) streamer() beep.Streamer {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.pipeline == nil {
		return nil
	}
	return d.pipeline.stream
}
