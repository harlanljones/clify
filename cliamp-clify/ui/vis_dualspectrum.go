package ui

import (
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

const (
	dualRiseRate = 28.0
	dualFallRate = 8.0
	dualEpsilon  = 1e-3
)

// dualSpectrumDriver renders independent L/R frequency-spectrum bars placed
// side by side horizontally: the left half of the panel shows the left channel
// and the right half shows the right channel. When the player is in mono mode
// the Tick context already supplies identical L/R samples, so both halves
// mirror each other naturally.
type dualSpectrumDriver struct {
	lBody   []float64
	rBody   []float64
	lTarget []float64
	rTarget []float64

	samples   [][2]float64
	lastTick  time.Time
	samplesAt time.Time
}

func newDualSpectrumDriver() visModeDriver {
	return &dualSpectrumDriver{samples: make([][2]float64, defaultFFTSize)}
}

func (*dualSpectrumDriver) AnalysisSpec(*Visualizer) VisAnalysisSpec {
	return spectrumAnalysisSpec(DefaultSpectrumBands)
}

// Render draws L spectrum bars in the left half of each row and R bars in the
// right half, both spanning the full panel height.
func (d *dualSpectrumDriver) Render(v *Visualizer) string {
	rows := v.Rows
	if rows <= 0 {
		return ""
	}
	nBands := DefaultSpectrumBands
	d.ensureBodies(nBands)

	if rows == 1 {
		return renderDualBandRow(d.lBody, d.rBody, "L ", "R ", 0, 1, nBands, PanelWidth)
	}

	lines := make([]string, 0, rows)
	for row := range rows {
		rowBottom := float64(rows-1-row) / float64(rows)
		rowTop := float64(rows-row) / float64(rows)
		lLabel, rLabel := "  ", "  "
		if row == 0 {
			lLabel, rLabel = "L ", "R "
		}
		lines = append(lines, renderDualBandRow(d.lBody, d.rBody, lLabel, rLabel, rowBottom, rowTop, nBands, PanelWidth))
	}

	return strings.Join(lines, "\n")
}

// renderDualBandRow draws one row split into two halves: L bands on the left
// side of the screen and R bands on the right side.
func renderDualBandRow(lBands, rBands []float64, lLabel, rLabel string, rowBottom, rowTop float64, nBands, width int) string {
	if width <= 0 {
		return ""
	}
	halfW := width / 2
	left := renderDualHalfRow(lBands, lLabel, rowBottom, rowTop, nBands, halfW)
	right := renderDualHalfRow(rBands, rLabel, rowBottom, rowTop, nBands, width-halfW)
	return left + right
}

// renderDualHalfRow draws one channel's spectrum blocks into its half of the row.
func renderDualHalfRow(bands []float64, label string, rowBottom, rowTop float64, nBands, width int) string {
	if width <= 0 {
		return ""
	}
	prefixLen := 2
	if width <= prefixLen {
		return strings.Repeat(" ", width)
	}
	bodyCols := width - prefixLen

	var line strings.Builder
	line.Grow(width * 2)
	line.WriteString(label)

	var run strings.Builder
	runTag := -1
	col := 0

	for i := range nBands {
		bw := dualBandWidth(nBands, i, bodyCols)
		if bw <= 0 || col >= bodyCols {
			continue
		}
		level := bands[i]
		block := fracBlock(level, rowBottom, rowTop)
		tag := specTag(rowBottom)

		if tag != runTag {
			flushStyleRun(&line, &run, runTag)
			runTag = tag
		}
		for range bw {
			if col >= bodyCols {
				break
			}
			run.WriteString(block)
			col++
		}
		if i < nBands-1 && col < bodyCols {
			run.WriteByte(' ')
			col++
		}
	}
	flushStyleRun(&line, &run, runTag)

	// Pad to exact visible width (ANSICodes don't count).
	result := line.String()
	for lipgloss.Width(result) < width {
		result += " "
	}
	return result
}

// dualBandWidth returns the character width for band b inside a region of cols
// columns. Mirrors visBandWidth but scoped to a sub-region instead of the
// global PanelWidth, so each half distributes its own bands evenly.
func dualBandWidth(totalBands, b, cols int) int {
	if totalBands <= 0 || b < 0 || b >= totalBands || cols <= 0 {
		return 0
	}
	gapCount := min(totalBands-1, max(0, cols-totalBands))
	bandCols := cols - gapCount
	base := bandCols / totalBands
	extra := bandCols % totalBands
	if b < extra {
		return base + 1
	}
	return base
}

// Tick samples stereo audio and advances both channels independently.
func (d *dualSpectrumDriver) Tick(v *Visualizer, ctx VisTickContext) {
	if ctx.OverlayActive {
		d.lastTick = time.Time{}
		d.samplesAt = time.Time{}
		return
	}
	if ctx.Playing {
		d.sample(v, ctx)
	} else {
		d.lTarget = nil
		d.rTarget = nil
		d.samplesAt = time.Time{}
	}
	d.advance(ctx.Now)
}

func (d *dualSpectrumDriver) TickInterval(_ *Visualizer, ctx VisTickContext) time.Duration {
	if ctx.OverlayActive {
		return TickSlow
	}
	if ctx.Playing || d.animating() {
		return TickAnim
	}
	return TickSlow
}

func (d *dualSpectrumDriver) OnEnter(*Visualizer) {
	samples := d.samples
	*d = dualSpectrumDriver{samples: samples}
}

func (*dualSpectrumDriver) OnLeave(*Visualizer) {}

func (d *dualSpectrumDriver) ensureBodies(nBands int) {
	if len(d.lBody) < nBands {
		d.lBody = make([]float64, nBands)
	}
	if len(d.rBody) < nBands {
		d.rBody = make([]float64, nBands)
	}
}

func (d *dualSpectrumDriver) sample(v *Visualizer, ctx VisTickContext) {
	if ctx.StereoSamplesInto == nil {
		return
	}
	if !d.samplesAt.IsZero() && !ctx.Now.IsZero() && ctx.Now.Sub(d.samplesAt) < TickAnalyze {
		return
	}
	n := ctx.StereoSamplesInto(d.samples)
	if n == 0 {
		return
	}

	spec := NormalizeAnalysisSpec(spectrumAnalysisSpec(DefaultSpectrumBands))
	use := min(n, spec.FFTSize)

	// Build L mono buffer and run FFT analysis.
	lBuf := v.EnsureSampleBuf(spec.FFTSize)
	for i := range use {
		lBuf[i] = d.samples[i][0]
	}
	for i := use; i < spec.FFTSize; i++ {
		lBuf[i] = 0
	}
	lResult := v.Analyze(lBuf, spec)
	d.lTarget = make([]float64, len(lResult))
	copy(d.lTarget, lResult)

	// Reset shared history so R analysis is independent of L.
	v.resetSpectrumHistory()

	// Build R mono buffer and run FFT analysis.
	rBuf := v.EnsureSampleBuf(spec.FFTSize)
	for i := range use {
		rBuf[i] = d.samples[i][1]
	}
	for i := use; i < spec.FFTSize; i++ {
		rBuf[i] = 0
	}
	rResult := v.Analyze(rBuf, spec)
	d.rTarget = make([]float64, len(rResult))
	copy(d.rTarget, rResult)

	if !ctx.Now.IsZero() {
		d.samplesAt = ctx.Now
	}
}

// advance applies exponential easing toward the current target band values.
func (d *dualSpectrumDriver) advance(now time.Time) {
	dt := TickAnim
	if !now.IsZero() && !d.lastTick.IsZero() {
		dt = now.Sub(d.lastTick)
	}
	if dt <= 0 || dt > maxSmoothDtFrames*TickAnim {
		dt = TickAnim
	}
	d.lastTick = now
	dtSec := dt.Seconds()

	nBands := DefaultSpectrumBands
	d.ensureBodies(nBands)

	for i := range nBands {
		lt := 0.0
		rt := 0.0
		if d.lTarget != nil && i < len(d.lTarget) {
			lt = d.lTarget[i]
		}
		if d.rTarget != nil && i < len(d.rTarget) {
			rt = d.rTarget[i]
		}

		rate := dualFallRate
		if lt > d.lBody[i] {
			rate = dualRiseRate
		}
		d.lBody[i] += (lt - d.lBody[i]) * (1 - math.Exp(-rate*dtSec))

		rate = dualFallRate
		if rt > d.rBody[i] {
			rate = dualRiseRate
		}
		d.rBody[i] += (rt - d.rBody[i]) * (1 - math.Exp(-rate*dtSec))
	}
}

func (d *dualSpectrumDriver) animating() bool {
	for i := range len(d.lBody) {
		if d.lBody[i] > dualEpsilon || d.rBody[i] > dualEpsilon {
			return true
		}
	}
	if d.lTarget != nil {
		for i, v := range d.lTarget {
			if i < len(d.lBody) && math.Abs(v-d.lBody[i]) > dualEpsilon {
				return true
			}
		}
	}
	if d.rTarget != nil {
		for i, v := range d.rTarget {
			if i < len(d.rBody) && math.Abs(v-d.rBody[i]) > dualEpsilon {
				return true
			}
		}
	}
	return false
}
