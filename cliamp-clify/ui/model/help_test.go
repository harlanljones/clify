package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestKeymapBuildsCategorizedSections(t *testing.T) {
	m := Model{}
	entries := m.buildKeymapEntries()

	want := []string{
		"— Playback —",
		"— Navigation —",
		"— Playlist & Queue —",
		"— Providers & Source —",
		"— Search & Filter —",
		"— EQ & Visuals —",
		"— General —",
		"— DJ mode (clify fork) —",
	}

	var dividers []string
	for _, e := range entries {
		if e.divider {
			dividers = append(dividers, e.action)
		}
	}

	prev := -1
	for _, section := range want {
		found := -1
		for i, d := range dividers {
			if d == section {
				found = i
				break
			}
		}
		if found < 0 {
			t.Fatalf("missing section %q in dividers: %v", section, dividers)
		}
		if found <= prev {
			t.Fatalf("section %q out of order (prev index %d, got %d); dividers: %v", section, prev, found, dividers)
		}
		prev = found
	}
}

func TestKeymapSeparatesForkFromLegacy(t *testing.T) {
	m := Model{}
	entries := m.buildKeymapEntries()

	forkActions := map[string]bool{}
	forkDiv := -1
	for i, e := range entries {
		if e.divider && e.action == "— DJ mode (clify fork) —" {
			forkDiv = i
			for j := i + 1; j < len(entries) && !entries[j].divider; j++ {
				forkActions[entries[j].action] = true
			}
			break
		}
	}

	for _, act := range []string{
		"DJ mode", "Exit DJ mode", "Focus deck A / B", "Crossfader",
		"Pitch nudge", "Sync focused deck", "Center crossfader",
	} {
		if !forkActions[act] {
			t.Fatalf("fork section missing %q; got %v", act, forkActions)
		}
	}

	// Fork-only actions must never leak into a legacy section (i.e. before the
	// fork divider).
	if forkDiv < 0 {
		t.Fatalf("no '— DJ mode (clify fork) —' divider found; entries: %+v", entries)
	}
	for _, e := range entries[:forkDiv] {
		if e.action == "Crossfader" || e.action == "Pitch nudge" || e.action == "Sync focused deck" {
			t.Fatalf("fork action %q leaked outside the fork section", e.action)
		}
	}
}

func TestKeymapFilterFuzzyMatchesAndRecordsHighlight(t *testing.T) {
	m := Model{}
	m.keymap.entries = m.buildKeymapEntries()
	m.keymap.searching = true
	m.keymap.search = "dj"
	m.updateKeymapFilter()

	if len(m.keymap.filtered) == 0 {
		t.Fatal("fuzzy filter for 'dj' returned no matches")
	}
	if len(m.keymap.filtered) != len(m.keymap.filterMatch) {
		t.Fatalf("filtered (%d) and filterMatch (%d) lengths differ", len(m.keymap.filtered), len(m.keymap.filterMatch))
	}

	var actions []string
	for i, idx := range m.keymap.filtered {
		actions = append(actions, m.keymap.entries[idx].action)
		if m.keymap.filterMatch[i].actionIdx == nil && m.keymap.filterMatch[i].keyIdx == nil {
			t.Fatalf("entry %q has no recorded match highlight", m.keymap.entries[idx].action)
		}
	}
	if !strings.Contains(strings.Join(actions, "|"), "DJ mode") {
		t.Fatalf("filter 'dj' did not surface DJ actions: %v", actions)
	}
}

func TestKeymapFilterFuzzySkipsDividers(t *testing.T) {
	m := Model{}
	m.keymap.entries = m.buildKeymapEntries()
	m.keymap.search = "help"
	m.updateKeymapFilter()

	for _, idx := range m.keymap.filtered {
		if m.keymap.entries[idx].divider {
			t.Fatalf("divider %q matched filter %q", m.keymap.entries[idx].action, m.keymap.search)
		}
	}
	// 'help' is a substring (and subsequence) of the Help rows.
	if len(m.keymap.filtered) == 0 {
		t.Fatal("filter 'help' should match the Help bindings")
	}
}

func TestKeymapRenderAlignsActionColumn(t *testing.T) {
	m := Model{
		plVisible: 8,
		keymap: keymapOverlay{
			entries: []keymapEntry{
				{action: "— Playback —", divider: true},
				{key: "Space", action: "Play / Pause"},
				{key: "Left Right", action: "Seek +/-5s"},
			},
		},
	}

	out := m.renderKeymapList()
	lines := strings.Split(ansi.Strip(out), "\n")

	var starts []int
	for _, l := range lines {
		for _, act := range []string{"Play / Pause", "Seek +/-5s"} {
			if idx := strings.Index(l, act); idx >= 0 {
				starts = append(starts, idx)
			}
		}
	}
	if len(starts) != 2 {
		t.Fatalf("found %d action rows, want 2:\n%s", len(starts), ansi.Strip(out))
	}
	if starts[0] != starts[1] {
		t.Fatalf("action column not aligned: %d vs %d\n%s", starts[0], starts[1], ansi.Strip(out))
	}
}

func TestKeymapRenderHighlightMatchedAction(t *testing.T) {
	m := Model{
		plVisible: 8,
		keymap: keymapOverlay{
			searching: true,
			search:    "play",
			entries: []keymapEntry{
				{key: "Space", action: "Play / Pause"},
				{key: "z", action: "Toggle shuffle"},
			},
			filtered:    []int{0},
			filterMatch: []keymapMatch{{actionIdx: []int{0, 1, 2, 3}}},
		},
	}

	out := m.renderKeymapList()
	// Matched runes "Play" are wrapped in keymapMatchStyle; the unstyled row
	// containing a match must be present and the unmatched row omitted.
	if !strings.Contains(ansi.Strip(out), "Play / Pause") {
		t.Fatalf("matched action missing from keymap: %q", ansi.Strip(out))
	}
	plain := ansi.Strip(out)
	if strings.Contains(plain, "Toggle shuffle") {
		t.Fatalf("unmatched entry leaked into filtered keymap: %q", plain)
	}
}
