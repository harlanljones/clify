package player

import "math"

// BPMResult is a best-effort beat estimate. Confidence below 0.5 should be
// treated as unknown by sync controls.
type BPMResult struct {
	BPM        float64
	Confidence float64
}

// DetectBPM estimates tempo from mono PCM samples using onset energy and
// autocorrelation. It intentionally has no decoder dependency, making it
// suitable for background analysis and deterministic tests.
func DetectBPM(samples []float64, sampleRate int) BPMResult {
	if sampleRate <= 0 || len(samples) < sampleRate/2 {
		return BPMResult{}
	}
	const minBPM, maxBPM = 70.0, 180.0
	minLag := int(float64(sampleRate) * 60 / maxBPM)
	maxLag := int(float64(sampleRate) * 60 / minBPM)
	if maxLag >= len(samples) {
		maxLag = len(samples) - 1
	}
	energy := make([]float64, len(samples))
	for i, v := range samples {
		energy[i] = v * v
	}
	bestLag, best := 0, 0.0
	for lag := minLag; lag <= maxLag; lag++ {
		var score float64
		for i := lag; i < len(energy); i++ {
			score += energy[i] * energy[i-lag]
		}
		if score > best {
			best, bestLag = score, lag
		}
	}
	if bestLag == 0 {
		return BPMResult{}
	}
	bpm := 60 * float64(sampleRate) / float64(bestLag)
	// Normalize confidence against the zero-lag energy, avoiding a false
	// positive for silence or near-silence.
	var total float64
	for _, v := range energy {
		total += v * v
	}
	confidence := 0.0
	if total > 0 {
		confidence = math.Min(1, best/total*float64(len(samples))*4)
	}
	return BPMResult{BPM: bpm, Confidence: confidence}
}
