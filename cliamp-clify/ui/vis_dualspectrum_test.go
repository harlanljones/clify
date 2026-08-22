package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestDualSpectrumLRAreIndependent(t *testing.T) {
	v := NewVisualizer(44100)
	v.Cols = 48
	v.Rows = 5
	activateMode(t, v, VisDualSpectrum)

	// Left-heavy and right-heavy samples — must fill the FFT window.
	n := defaultFFTSize
	samples := make([][2]float64, n)
	for i := range n {
		samples[i] = [2]float64{0.9, 0.1}
	}
	calls := 0
	v.Tick(VisTickContext{
		Now:     time.Unix(1, 0),
		Playing: true,
		StereoSamplesInto: func(dst [][2]float64) int {
			calls++
			return copy(dst, samples)
		},
	})

	driver, ok := v.driverFor(VisDualSpectrum).(*dualSpectrumDriver)
	if !ok {
		t.Fatal("driverFor(VisDualSpectrum) did not return *dualSpectrumDriver")
	}
	if calls != 1 {
		t.Fatalf("StereoSamplesInto() calls = %d, want 1", calls)
	}

	if driver.lTarget == nil || driver.rTarget == nil {
		t.Fatal("targets are nil — sample() did not populate them")
	}
	lSum := 0.0
	rSum := 0.0
	for i := range driver.lTarget {
		lSum += driver.lTarget[i]
		rSum += driver.rTarget[i]
	}
	if lSum <= rSum {
		t.Fatalf("lTarget sum (%v) should be greater than rTarget sum (%v) for left-heavy input", lSum, rSum)
	}
}

func TestDualSpectrumSymmetryWithMonoInput(t *testing.T) {
	v := NewVisualizer(44100)
	v.Cols = 48
	v.Rows = 5
	activateMode(t, v, VisDualSpectrum)

	// Identical L/R: simulate mono.
	const n = 64
	samples := make([][2]float64, n)
	for i := range n {
		sine := float64(i) / float64(n)
		samples[i] = [2]float64{sine, sine}
	}
	v.Tick(VisTickContext{
		Now:     time.Unix(1, 0),
		Playing: true,
		StereoSamplesInto: func(dst [][2]float64) int {
			return copy(dst, samples)
		},
	})

	driver := v.driverFor(VisDualSpectrum).(*dualSpectrumDriver)
	if driver.lTarget == nil || driver.rTarget == nil {
		t.Fatal("targets are nil")
	}
	for i := range driver.lTarget {
		if diff := driver.lTarget[i] - driver.rTarget[i]; diff > 1e-6 || diff < -1e-6 {
			t.Errorf("band[%d] L=%v R=%v, want equal for mono input", i, driver.lTarget[i], driver.rTarget[i])
		}
	}
}

func TestDualSpectrumRenderContainsLabels(t *testing.T) {
	v := NewVisualizer(44100)
	v.Cols = 48
	v.Rows = 5
	activateMode(t, v, VisDualSpectrum)

	driver := v.driverFor(VisDualSpectrum).(*dualSpectrumDriver)
	// Seed with some non-zero body values so bars are visible.
	driver.lBody = []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0}
	driver.rBody = []float64{0.9, 0.8, 0.7, 0.6, 0.5, 0.4, 0.3, 0.2, 0.1, 0.0}

	plain := ansi.Strip(v.Render())
	if !strings.Contains(plain, "L ") {
		t.Errorf("render missing 'L ' label:\n%s", plain)
	}
	if !strings.Contains(plain, "R ") {
		t.Errorf("render missing 'R ' label:\n%s", plain)
	}

	// Side-by-side layout: both labels share the first line, L on the left
	// half and R on the right half.
	lines := strings.Split(plain, "\n")
	first := lines[0]
	lIdx := strings.Index(first, "L ")
	rIdx := strings.Index(first, "R ")
	if lIdx < 0 || rIdx < 0 {
		t.Fatalf("first line missing labels:\n%s", first)
	}
	if lIdx >= rIdx {
		t.Fatalf("'L' should sit left of 'R' on the same line:\n%s", first)
	}
	half := len([]rune(first)) / 2
	if lIdx > half || rIdx < half {
		t.Fatalf("labels not anchored to their halves: L@%d R@%d width=%d", lIdx, rIdx, half*2)
	}
}

func TestDualSpectrumSingleRow(t *testing.T) {
	v := NewVisualizer(44100)
	v.Cols = 48
	v.Rows = 1
	activateMode(t, v, VisDualSpectrum)

	driver := v.driverFor(VisDualSpectrum).(*dualSpectrumDriver)
	driver.lBody = []float64{0.3, 0.5, 0.7}
	driver.rBody = []float64{0.7, 0.5, 0.3}

	result := v.Render()
	if result == "" {
		t.Fatal("single-row render returned empty")
	}
	if strings.Count(result, "\n") != 0 {
		t.Fatalf("single-row render should be one line, got %d", strings.Count(result, "\n")+1)
	}
}

func TestDualSpectrumTickInterval(t *testing.T) {
	v := NewVisualizer(44100)
	v.Cols = 48
	v.Rows = 5
	activateMode(t, v, VisDualSpectrum)

	// Playing -> TickAnim.
	if got := v.TickInterval(VisTickContext{Playing: true}); got != TickAnim {
		t.Fatalf("TickInterval(playing) = %v, want %v", got, TickAnim)
	}
	// Not playing -> TickSlow.
	if got := v.TickInterval(VisTickContext{}); got != TickSlow {
		t.Fatalf("TickInterval(idle) = %v, want %v", got, TickSlow)
	}
}

func TestDualSpectrumOverlaySilencesTick(t *testing.T) {
	v := NewVisualizer(44100)
	v.Cols = 48
	v.Rows = 5
	activateMode(t, v, VisDualSpectrum)

	// First tick: populate targets.
	v.Tick(VisTickContext{
		Now:     time.Unix(1, 0),
		Playing: true,
		StereoSamplesInto: func(dst [][2]float64) int {
			for i := range dst {
				dst[i] = [2]float64{0.5, 0.5}
			}
			return len(dst)
		},
	})

	driver := v.driverFor(VisDualSpectrum).(*dualSpectrumDriver)
	if driver.lTarget == nil {
		t.Fatal("expected targets to be populated after tick")
	}

	// Overlay tick should not overwrite, but Tick resets timestamp trackers.
	v.Tick(VisTickContext{
		Now:           time.Unix(2, 0),
		Playing:       true,
		OverlayActive: true,
	})
	// Overlay just resets lastTick/samplesAt; targets are not cleared.
}
