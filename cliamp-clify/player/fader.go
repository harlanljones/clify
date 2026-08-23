package player

import (
	"math"
	"sync/atomic"

	"github.com/gopxl/beep/v2"
)

// faderStreamer applies a gain that can be changed safely from the control
// thread. Changes are ramped over a block so crossfades do not introduce a
// discontinuity at a block boundary.
type faderStreamer struct {
	s      beep.Streamer
	gain   atomic.Uint64 // current linear gain
	target atomic.Uint64
	step   atomic.Uint64 // maximum gain change per sample; zero means instant
}

func newFaderStreamer(s beep.Streamer) *faderStreamer {
	f := &faderStreamer{s: s}
	f.gain.Store(math.Float64bits(1))
	f.target.Store(math.Float64bits(1))
	return f
}

func (f *faderStreamer) SetGain(gain float64, samples int) {
	gain = max(0, min(1, gain))
	current := math.Float64frombits(f.gain.Load())
	f.target.Store(math.Float64bits(gain))
	if samples <= 0 {
		f.step.Store(math.Float64bits(0))
		f.gain.Store(math.Float64bits(gain))
		return
	}
	f.step.Store(math.Float64bits(math.Abs(gain-current) / float64(samples)))
}

func (f *faderStreamer) Gain() float64 { return math.Float64frombits(f.target.Load()) }

func (f *faderStreamer) Stream(samples [][2]float64) (int, bool) {
	n, ok := f.s.Stream(samples)
	current := math.Float64frombits(f.gain.Load())
	target := math.Float64frombits(f.target.Load())
	step := math.Float64frombits(f.step.Load())
	for i := 0; i < n; i++ {
		if step > 0 && current != target {
			if current < target {
				current = min(target, current+step)
			} else {
				current = max(target, current-step)
			}
		}
		samples[i][0] *= current
		samples[i][1] *= current
	}
	if current == target {
		f.step.Store(math.Float64bits(0))
	}
	f.gain.Store(math.Float64bits(current))
	return n, ok
}

func (f *faderStreamer) Err() error { return f.s.Err() }

// equalPowerGains returns gains for a normalized crossfader position.
func equalPowerGains(position float64) (a, b float64) {
	position = max(0, min(1, position))
	a, b = math.Cos(position*math.Pi/2), math.Sin(position*math.Pi/2)
	if math.Abs(a) < 1e-12 {
		a = 0
	}
	if math.Abs(b) < 1e-12 {
		b = 0
	}
	return a, b
}
