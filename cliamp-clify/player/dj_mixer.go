package player

import "github.com/gopxl/beep/v2"

// djMixer combines the two deck streams. A deck may be nil; nil decks are
// treated as silence, which makes loading and unloading the idle deck safe.
type djMixer struct {
	decks [2]*faderStreamer
}

func newDJMixer(a, b beep.Streamer) *djMixer {
	m := &djMixer{}
	if a != nil {
		m.decks[0] = newFaderStreamer(a)
	}
	if b != nil {
		m.decks[1] = newFaderStreamer(b)
	}
	return m
}

func (m *djMixer) Stream(samples [][2]float64) (int, bool) {
	for i := range samples {
		samples[i] = [2]float64{}
	}
	active := false
	for _, deck := range m.decks {
		if deck == nil {
			continue
		}
		buf := make([][2]float64, len(samples))
		n, ok := deck.Stream(buf)
		if n > 0 {
			active = true
		}
		for i := 0; i < n; i++ {
			samples[i][0] += buf[i][0]
			samples[i][1] += buf[i][1]
		}
		if !ok && n == 0 {
			deck = nil
		}
	}
	return len(samples), active
}

func (m *djMixer) Err() error {
	for _, deck := range m.decks {
		if deck != nil && deck.Err() != nil {
			return deck.Err()
		}
	}
	return nil
}
