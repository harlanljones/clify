package player

import (
	"testing"
	"time"
)

type sliceStreamer struct{ samples [][2]float64 }

func (s *sliceStreamer) Stream(dst [][2]float64) (int, bool) {
	n := min(len(dst), len(s.samples))
	copy(dst, s.samples[:n])
	s.samples = s.samples[n:]
	return n, len(s.samples) > 0
}
func (s *sliceStreamer) Err() error { return nil }

func TestFaderRampsWithoutOvershoot(t *testing.T) {
	f := newFaderStreamer(&sliceStreamer{samples: [][2]float64{{1, 1}, {1, 1}, {1, 1}, {1, 1}}})
	f.SetGain(0, 4)
	buf := make([][2]float64, 4)
	f.Stream(buf)
	if got := f.Gain(); got != 0 {
		t.Fatalf("target gain = %v, want 0", got)
	}
	for i := 1; i < len(buf); i++ {
		if buf[i][0] > buf[i-1][0] {
			t.Fatalf("gain increased at sample %d", i)
		}
	}
	if buf[0][0] <= 0 || buf[len(buf)-1][0] != 0 {
		t.Fatalf("unexpected ramp: %#v", buf)
	}
}

func TestEqualPowerGainsEndpoints(t *testing.T) {
	a, b := equalPowerGains(0)
	if a != 1 || b != 0 {
		t.Fatalf("left = %v,%v", a, b)
	}
	a, b = equalPowerGains(1)
	if a != 0 || b != 1 {
		t.Fatalf("right = %v,%v", a, b)
	}
}

func TestTransitionCompletes(t *testing.T) {
	now := time.Unix(0, 0)
	tr := newTransition(TransitionFade, time.Second, 0, 1, now)
	_, _, done := tr.gains(now.Add(time.Second))
	if !done {
		t.Fatal("transition did not complete")
	}
}

func TestDJControllerSyncRejectsLowConfidence(t *testing.T) {
	c := NewDJController()
	if err := c.SyncDeck(0); err == nil {
		t.Fatal("expected low-confidence sync error")
	}
}
