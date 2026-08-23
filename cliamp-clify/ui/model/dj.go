package model

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bjarneo/cliamp/player"
	"github.com/bjarneo/cliamp/ui"
)

func (m *Model) openDJMode() {
	if m.dj == nil {
		m.status.Warning("DJ engine unavailable in this player", statusTTLDefault)
		return
	}
	m.dj.EnableDJ(true)
	m.djState.visible = true
	m.djState.focus = 0
	if track, _ := m.currentPlaybackTrack(); track.Path != "" {
		_ = m.dj.LoadDeck(0, track.Path, m.player.Duration())
		if m.playlist != nil {
			tracks := m.playlist.Tracks()
			if len(tracks) > 1 {
				_, index := m.currentPlaybackTrack()
				if index >= 0 && index+1 < len(tracks) {
					_ = m.dj.LoadDeck(1, tracks[index+1].Path, time.Duration(tracks[index+1].DurationSecs)*time.Second)
				}
			}
		}
	}
}

func (m *Model) closeDJMode() {
	m.djState.visible = false
	if m.dj != nil {
		m.dj.EnableDJ(false)
	}
}

func (m *Model) handleDJKey(msg tea.KeyPressMsg) tea.Cmd {
	if m.dj == nil {
		m.closeDJMode()
		return nil
	}
	switch msg.String() {
	case "q", "ctrl+c":
		return m.quit()
	case "esc", "D":
		m.closeDJMode()
	case "1":
		m.djState.focus = 0
	case "2":
		m.djState.focus = 1
	case "tab":
		m.djState.focus = 1 - m.djState.focus
	case "left", "h":
		m.dj.SetCrossfader(m.dj.Crossfader()-0.05, 50*time.Millisecond)
	case "right", "l":
		m.dj.SetCrossfader(m.dj.Crossfader()+0.05, 50*time.Millisecond)
	case "\\":
		m.dj.SetCrossfader(0.5, 100*time.Millisecond)
	case "[":
		m.dj.SetDeckPitch(m.djState.focus, max(0.5, m.dj.DeckPitch(m.djState.focus)-0.01))
	case "]":
		m.dj.SetDeckPitch(m.djState.focus, min(1.5, m.dj.DeckPitch(m.djState.focus)+0.01))
	case "s":
		if err := m.dj.SyncDeck(m.djState.focus); err != nil {
			m.status.Warning("Sync unavailable: "+err.Error(), statusTTLDefault)
		} else {
			m.status.Success("Deck synced", statusTTLDefault)
		}
	case "c":
		m.dj.SetCrossfader(0.5, 100*time.Millisecond)
	}
	return nil
}

func (m Model) renderDJMode() string {
	if m.dj == nil {
		return ui.FitRect(dimStyle.Render("DJ engine unavailable"), m.layout.panelWidth, m.layout.bodyRows)
	}
	a, b := m.dj.Decks()
	deck := func(name string, d player.DJDeckStatus, focused bool) string {
		style := dimStyle
		if focused {
			style = activeToggle
		}
		path := d.Path
		if path == "" {
			path = "Empty — load a track from the playlist"
		}
		bpm := "BPM --"
		if d.BPM.BPM > 0 {
			bpm = fmt.Sprintf("BPM %.1f  confidence %.0f%%", d.BPM.BPM, d.BPM.Confidence*100)
		}
		return strings.Join([]string{
			style.Render("┌─ " + name + " ───────────────────────────────┐"),
			"│ " + trackStyle.Render(truncate(path, max(1, m.layout.panelWidth/2-5))),
			"│ " + dimStyle.Render(fmt.Sprintf("Pitch %+.0f%%   %s", (d.Pitch-1)*100, bpm)),
			"└────────────────────────────────────────┘",
		}, "\n")
	}
	cross := int(m.dj.Crossfader() * 20)
	bar := strings.Repeat("A", cross) + "◆" + strings.Repeat("B", 20-cross)
	lines := []string{
		activeToggle.Render("DJ MODE") + dimStyle.Render("  dual-deck control surface"),
		"",
		deck("DECK A", a, m.djState.focus == 0),
		"",
		deck("DECK B", b, m.djState.focus == 1),
		"",
		labelStyle.Render("CROSSFADER ") + trackStyle.Render("["+bar+"]") +
			dimStyle.Render(fmt.Sprintf("  %.0f%% A → B", m.dj.Crossfader()*100)),
		"",
		dimStyle.Render("1/2 focus  ·  ←/→ crossfade  ·  [ ] pitch  ·  s sync  ·  \\ center  ·  Esc close"),
	}
	return ui.FitRect(lipgloss.JoinVertical(lipgloss.Left, lines...), m.layout.panelWidth, m.layout.bodyRows)
}
