package player

import "time"

// TransitionStyle controls how one deck hands off to the other.
type TransitionStyle string

const (
	TransitionCut   TransitionStyle = "cut"
	TransitionFade  TransitionStyle = "fade"
	TransitionBrake TransitionStyle = "brake"
)

type transitionState struct {
	style    TransitionStyle
	started  time.Time
	duration time.Duration
	from     float64
	to       float64
}

func newTransition(style TransitionStyle, duration time.Duration, from, to float64, now time.Time) transitionState {
	if duration < 0 {
		duration = 0
	}
	return transitionState{style: style, started: now, duration: duration, from: from, to: to}
}

func (t transitionState) gains(now time.Time) (a, b float64, done bool) {
	if t.duration == 0 {
		return t.to, 1, true
	}
	x := float64(now.Sub(t.started)) / float64(t.duration)
	if x <= 0 {
		x = 0
	}
	if x >= 1 {
		x = 1
	}
	// Equal-power interpolation preserves perceived loudness in the overlap.
	pos := t.from + (t.to-t.from)*x
	a, b = equalPowerGains(pos)
	return a, b, x >= 1
}
