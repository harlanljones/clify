package player

import (
	"fmt"
	"sync"
	"time"
)

// DJEngine is the optional dual-deck control surface. It is separate from
// Engine so existing player fakes and normal single-deck playback remain
// source-compatible.
type DJEngine interface {
	DJMode() bool
	EnableDJ(bool)
	LoadDeck(deck int, path string, knownDuration time.Duration) error
	SetCrossfader(position float64, ramp time.Duration)
	Crossfader() float64
	SetDeckPitch(deck int, ratio float64)
	DeckPitch(deck int) float64
	SyncDeck(deck int) error
	SetDeckCue(deck int, position time.Duration) error
	Decks() (a, b DJDeckStatus)
}

type DJDeckStatus struct {
	Path          string
	Position, Cue time.Duration
	BPM           BPMResult
	Pitch         float64
}

// DJController is a lightweight, pipeline-independent controller. Player can
// attach real pipelines later without changing the control contract.
type DJController struct {
	mu         sync.RWMutex
	active     bool
	decks      [2]DJDeckStatus
	crossfader float64
}

func NewDJController() *DJController {
	c := &DJController{}
	c.decks[0].Pitch, c.decks[1].Pitch = 1, 1
	return c
}
func (c *DJController) DJMode() bool    { c.mu.RLock(); defer c.mu.RUnlock(); return c.active }
func (c *DJController) EnableDJ(v bool) { c.mu.Lock(); c.active = v; c.mu.Unlock() }
func (c *DJController) LoadDeck(deck int, path string, _ time.Duration) error {
	if deck < 0 || deck > 1 {
		return fmt.Errorf("invalid deck %d", deck)
	}
	c.mu.Lock()
	c.decks[deck].Path = path
	c.mu.Unlock()
	return nil
}
func (c *DJController) SetCrossfader(v float64, _ time.Duration) {
	c.mu.Lock()
	c.crossfader = max(0, min(1, v))
	c.mu.Unlock()
}
func (c *DJController) Crossfader() float64 { c.mu.RLock(); defer c.mu.RUnlock(); return c.crossfader }
func (c *DJController) SetDeckPitch(deck int, ratio float64) {
	if deck >= 0 && deck < 2 && ratio > 0 {
		c.mu.Lock()
		c.decks[deck].Pitch = ratio
		c.mu.Unlock()
	}
}
func (c *DJController) DeckPitch(deck int) float64 {
	if deck < 0 || deck > 1 {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.decks[deck].Pitch
}
func (c *DJController) SyncDeck(deck int) error {
	if deck < 0 || deck > 1 {
		return fmt.Errorf("invalid deck %d", deck)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	lead := 1 - deck
	if c.decks[deck].BPM.Confidence < .5 || c.decks[lead].BPM.Confidence < .5 || c.decks[deck].BPM.BPM <= 0 {
		return fmt.Errorf("BPM confidence too low")
	}
	c.decks[deck].Pitch = c.decks[lead].BPM.BPM / c.decks[deck].BPM.BPM
	return nil
}
func (c *DJController) SetDeckCue(deck int, position time.Duration) error {
	if deck < 0 || deck > 1 || position < 0 {
		return fmt.Errorf("invalid cue")
	}
	c.mu.Lock()
	c.decks[deck].Cue = position
	c.mu.Unlock()
	return nil
}
func (c *DJController) Decks() (a, b DJDeckStatus) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.decks[0], c.decks[1]
}
