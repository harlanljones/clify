package model

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// keymapEntry is a row in the Ctrl+K help overlay. Rows with `divider = true`
// are unselectable section headers (e.g. "— Playback —" or "— plugins —").
type keymapEntry struct {
	key, action string
	divider     bool
}

// keymapMatch records which runes of a filtered keymap row matched the query,
// so the renderer can highlight them. keyIdx applies when the key matched;
// actionIdx when the action matched.
type keymapMatch struct {
	keyIdx, actionIdx []int
}

// orderedSectionEntries groups the categorized keymap entries into labeled
// sections. It shares `add` (and its dedup map) with the caller so a command
// shown in the current-context block does not repeat in the categorized grid.
func (m Model) orderedSectionEntries(add func(key, action string)) {
	collect := func(section commandSection) []commandSpec {
		var entries []commandSpec
		for _, command := range commandRegistry {
			if command.Section != section {
				continue
			}
			if !command.Keymap && !command.Fork {
				continue
			}
			if !command.enabled(m) {
				continue
			}
			entries = append(entries, command)
		}
		return entries
	}
	for _, section := range orderedSections {
		entries := collect(section)
		if len(entries) == 0 {
			continue
		}
		add("", "— "+string(section)+" —")
		for _, command := range entries {
			add(command.KeyLabel, command.Label)
		}
	}
}

// fuzzyMatchRunes reports whether needle is a subsequence of haystack (both
// lowercased by the caller) using a rune-aware scan, and returns the rune
// indices in haystack that matched. It returns ok=false when needle is not a
// subsequence.
func fuzzyMatchRunes(haystack, needle []rune) (bool, []int) {
	if len(needle) == 0 {
		return true, nil
	}
	idxs := make([]int, 0, len(needle))
	hi := 0
	for _, r := range needle {
		adv := false
		for hi < len(haystack) {
			if haystack[hi] == r {
				idxs = append(idxs, hi)
				hi++
				adv = true
				break
			}
			hi++
		}
		if !adv {
			return false, nil
		}
	}
	return true, idxs
}

// highlightMatch wraps the matched rune indices of s in keymapMatchStyle,
// grouping consecutive matched runes into a single style run.
func highlightMatch(s string, idxs []int) string {
	if len(idxs) == 0 {
		return s
	}
	set := make(map[int]bool, len(idxs))
	for _, i := range idxs {
		set[i] = true
	}
	var b, m strings.Builder
	flush := func() {
		if m.Len() > 0 {
			b.WriteString(keymapMatchStyle.Render(m.String()))
			m.Reset()
		}
	}
	for i, r := range []rune(s) {
		if set[i] {
			m.WriteRune(r)
		} else {
			flush()
			b.WriteRune(r)
		}
	}
	flush()
	return b.String()
}

// ReservedKeys returns a fresh copy of every key described by commandRegistry.
// It is handed to the Lua plugin manager at startup so plugins cannot shadow
// a core action or an active text field.
func ReservedKeys() map[string]bool {
	out := make(map[string]bool)
	for _, command := range commandRegistry {
		for _, key := range command.Keys {
			out[key] = true
		}
	}
	return out
}

// buildKeymapEntries starts with commands for the screen that opened the help,
// then lists every discoverable binding grouped into labeled sections (with the
// clify fork section last). The result is cached on open so navigation (which
// calls keymapCount many times per frame) is allocation-free.
func (m Model) buildKeymapEntries() []keymapEntry {
	out := make([]keymapEntry, 0, len(commandRegistry)+6)
	seen := make(map[string]bool)
	add := func(key, action string) {
		id := key + "\x00" + action
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		divider := key == "" && strings.HasPrefix(action, "— ")
		out = append(out, keymapEntry{key: key, action: action, divider: divider})
	}

	mode, label := m.keymapContext()
	if mode != commandModeMain {
		out = append(out, keymapEntry{action: "— current: " + label + " —", divider: true})
		for _, command := range commandRegistry {
			if command.Mode != commandModeAny && (command.Keymap || command.ContextHelp) && command.enabled(m) && command.Mode&mode != 0 {
				add(command.KeyLabel, command.Label)
			}
		}
	}
	m.orderedSectionEntries(add)
	if mode != commandModeMain || m.luaMgr == nil {
		return out
	}
	binds := m.luaMgr.KeyBindings()
	if len(binds) == 0 {
		return out
	}
	out = append(out, keymapEntry{action: "— plugins —", divider: true})
	for _, b := range binds {
		label := b.Description
		if b.Plugin != "" {
			label += "  (" + b.Plugin + ")"
		}
		add(b.Key, label)
	}
	return out
}

func (m Model) keymapContext() (commandMode, string) {
	switch m.activeScreen() {
	case screenDevicePicker:
		return commandModeDevicePicker, "Audio Device"
	case screenPlaylistPicker:
		if m.plPicker.screen == plPickerNewName {
			return commandModePlaylistPickerInput, "Playlist Name"
		}
		return commandModePlaylistPicker, "Save to Playlist"
	case screenFileBrowser:
		if m.fileBrowser.searching {
			return commandModeFileBrowserSearch, "File Filter"
		}
		return commandModeFileBrowser, "Files"
	case screenSpotSearch:
		return commandModeSpotSearch, "Provider Search"
	case screenNavBrowser:
		if m.navBrowser.searching {
			return commandModeNavSearch, "Browser Filter"
		}
		return commandModeNavBrowser, "Browse"
	case screenThemePicker:
		if m.themePicker.filtering {
			return commandModeThemePickerFilter, "Theme Filter"
		}
		return commandModeThemePicker, "Themes"
	case screenVisPicker:
		if m.visPicker.filtering {
			return commandModeVisPickerFilter, "Visualizer Filter"
		}
		return commandModeVisPicker, "Visualizers"
	case screenPlaylistManager:
		if m.plManager.screen == plMgrScreenNewName || m.plManager.screen == plMgrScreenRename {
			return commandModePlaylistManagerInput, "Playlist Name"
		}
		return commandModePlaylistManager, "Playlists"
	case screenQueue:
		return commandModeQueue, "Queue"
	case screenInfo:
		return commandModeInfo, "Track Info"
	case screenSearch:
		return commandModeSearch, "Playlist Filter"
	case screenNetSearch:
		return commandModeNetSearch, "Online Search"
	case screenURLInput:
		return commandModeURL, "Load URL"
	case screenLyrics:
		return commandModeLyrics, "Lyrics"
	case screenJump:
		return commandModeJump, "Jump to Time"
	}

	switch m.focus {
	case focusProvider:
		if m.provSearch.active {
			return commandModeProviderSearch, "Provider Filter"
		}
		return commandModeProvider, "Provider"
	case focusEQ:
		return commandModeEQ, "Equalizer"
	case focusSpeed:
		return commandModeSpeed, "Speed"
	case focusProvPill:
		return commandModeProviderPill, "Source"
	default:
		return commandModeMain, "Playlist"
	}
}

func (m *Model) keymapCount() int {
	if m.keymap.searching || m.keymap.search != "" {
		return len(m.keymap.filtered)
	}
	return len(m.keymap.entries)
}

func (m *Model) keymapHelpLine() string {
	if m.keymap.searching {
		return m.commandHelp(commandModeKeymapSearch)
	}
	return m.commandHelp(commandModeKeymap)
}

// keymapHeaderLine renders the keymap's single-line header for the playlist
// region: the filter prompt while searching/filtered, otherwise a labeled
// separator with the match count.
func (m Model) keymapHeaderLine() string {
	if m.keymap.searching || m.keymap.search != "" {
		return m.filterCountHeader("keymap", m.keymap.search, fmt.Sprintf("%d/%d", m.keymapCount(), len(m.keymap.entries)))
	}
	return sepHeaderN("Help", m.keymap.cursor+1, len(m.keymap.entries))
}

func (m *Model) keymapVisible() int {
	return m.effectivePlaylistVisible()
}

// keymapMaybeAdjustScroll keeps the cursor visible in the current keymap window.
func (m *Model) keymapMaybeAdjustScroll(visible int) {
	clampScroll(&m.keymap.cursor, &m.keymap.scroll, m.keymapCount(), visible)
}

// openKeymap resets the keymap state and shows it. Snapshots plugin bindings
// once so the render/navigation code doesn't re-query the plugin manager.
func (m *Model) openKeymap() {
	m.keymap.searching = false
	m.keymap.search = ""
	m.keymap.filtered = nil
	m.keymap.cursor = 0
	m.keymap.scroll = 0
	m.keymap.entries = m.buildKeymapEntries()
	m.keymap.visible = true
	// The keymap now renders in the playlist region; recompute chrome so its
	// header/help are reflected in the visible-row budget, then fit the cursor.
	m.refreshChrome()
	m.applyHeightMode()
	m.keymapMaybeAdjustScroll(m.keymapVisible())
}

// closeKeymap hides the keymap, clears its filter state, and restores playlist
// sizing after the inline header and help line are dismissed.
func (m *Model) closeKeymap() {
	m.keymap.visible = false
	m.keymap.searching = false
	m.keymap.search = ""
	m.keymap.filtered = nil
	m.refreshChrome()
	m.applyHeightMode()
	m.adjustScroll()
}

func (m *Model) handleKeymapSearchKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c":
		m.keymap.visible = false
		return m.quit()
	case "esc":
		m.keymap.searching = false
		m.keymap.search = ""
		m.keymap.filtered = nil
		m.keymap.cursor = m.keymap.savedCursor
		m.keymap.scroll = m.keymap.savedScroll
		return nil
	case "enter":
		m.keymap.searching = false
		if m.keymap.search == "" {
			m.keymap.cursor = m.keymap.savedCursor
			m.keymap.scroll = m.keymap.savedScroll
		}
		return nil
	case "down":
		m.keymap.searching = false
		if m.keymapCount() > 0 {
			m.keymap.cursor = 0
			m.keymapMaybeAdjustScroll(m.keymapVisible())
		}
		return nil
	case "backspace":
		if m.keymap.search == "" {
			m.keymap.searching = false
			m.keymap.cursor = m.keymap.savedCursor
			m.keymap.scroll = m.keymap.savedScroll
			return nil
		}
	}

	if m.editText("keymap", &m.keymap.search, msg) {
		m.updateKeymapFilter()
	}
	return nil
}

// handleKeymapKey processes key presses while the keymap overlay is open.
func (m *Model) handleKeymapKey(msg tea.KeyPressMsg) tea.Cmd {
	if m.keymap.searching {
		return m.handleKeymapSearchKey(msg)
	}

	switch msg.String() {
	case "ctrl+c":
		m.keymap.visible = false
		return m.quit()

	case "esc", "ctrl+k", "?", "q":
		m.closeKeymap()

	case "/":
		m.keymap.savedCursor = m.keymap.cursor
		m.keymap.savedScroll = m.keymap.scroll
		m.keymap.searching = true
		m.keymap.search = ""
		m.updateKeymapFilter()
		return nil

	case "up", "k":
		if m.keymap.search != "" && m.keymap.cursor == 0 {
			m.keymap.searching = true
			return nil
		}
		count := m.keymapCount()
		if m.keymap.cursor > 0 {
			m.keymap.cursor--
		} else if count > 0 {
			m.keymap.cursor = count - 1
		}
		m.keymapMaybeAdjustScroll(m.keymapVisible())

	case "down", "j":
		count := m.keymapCount()
		if m.keymap.cursor < count-1 {
			m.keymap.cursor++
		} else if count > 0 {
			m.keymap.cursor = 0
		}
		m.keymapMaybeAdjustScroll(m.keymapVisible())

	case "ctrl+x":
		m.toggleExpandedView()
		m.keymapMaybeAdjustScroll(m.keymapVisible())

	case "pgup", "ctrl+u":
		if m.keymap.cursor > 0 {
			visible := m.keymapVisible()
			m.keymap.cursor -= min(m.keymap.cursor, visible)
			m.keymapMaybeAdjustScroll(visible)
		}

	case "pgdown", "ctrl+d":
		count := m.keymapCount()
		if m.keymap.cursor < count-1 {
			visible := m.keymapVisible()
			m.keymap.cursor = min(count-1, m.keymap.cursor+visible)
			m.keymapMaybeAdjustScroll(visible)
		}

	case "home", "g":
		m.keymap.cursor = 0
		m.keymapMaybeAdjustScroll(m.keymapVisible())

	case "end", "G":
		count := m.keymapCount()
		if count > 0 {
			m.keymap.cursor = count - 1
		}
		m.keymapMaybeAdjustScroll(m.keymapVisible())

	case "backspace", "h":
		if m.keymap.search != "" {
			m.keymap.search = ""
			m.updateKeymapFilter()
		} else {
			m.closeKeymap()
		}

	case "enter", "l":
		m.closeKeymap()
	}

	return nil
}

// updateKeymapFilter rebuilds the filtered indices (fuzzy subsequence match)
// and records which runes matched so the renderer can highlight them.
func (m *Model) updateKeymapFilter() {
	m.keymap.filtered = nil
	m.keymap.filterMatch = nil
	m.keymap.cursor = 0
	m.keymap.scroll = 0
	if m.keymap.search == "" {
		return
	}
	needle := []rune(strings.ToLower(m.keymap.search))
	for i, e := range m.keymap.entries {
		if e.divider {
			continue
		}
		okKey, kIdx := fuzzyMatchRunes([]rune(strings.ToLower(e.key)), needle)
		okAction, aIdx := fuzzyMatchRunes([]rune(strings.ToLower(e.action)), needle)
		if okKey || okAction {
			m.keymap.filtered = append(m.keymap.filtered, i)
			m.keymap.filterMatch = append(m.keymap.filterMatch, keymapMatch{keyIdx: kIdx, actionIdx: aIdx})
		}
	}
}

// renderKeymapList renders the help entries for the playlist region while the
// help overlay is open. The header and help line are supplied by the main
// layout (renderPlaylistHeader / renderHelp), mirroring renderVisPickerList.
// Keys are rendered as uniform-width pills so the action column aligns; when a
// filter is active the matched runes are highlighted.
func (m Model) renderKeymapList() string {
	budget := m.effectivePlaylistVisible()
	if budget <= 0 {
		return ""
	}

	entries := m.keymap.entries
	var visible []keymapEntry
	var matches []keymapMatch
	if m.keymap.search != "" {
		visible = make([]keymapEntry, 0, len(m.keymap.filtered))
		for j, idx := range m.keymap.filtered {
			visible = append(visible, entries[idx])
			matches = append(matches, m.keymap.filterMatch[j])
		}
	} else {
		visible = entries
	}

	if len(visible) == 0 {
		msg := "(empty)"
		if m.keymap.search != "" {
			msg = "No matches"
		}
		return strings.Join(fitLines([]string{dimStyle.Render("  " + msg)}, budget), "\n")
	}

	maxKeyW := 0
	for _, e := range visible {
		if e.divider {
			continue
		}
		if w := lipgloss.Width(e.key); w > maxKeyW {
			maxKeyW = w
		}
	}

	lines := make([]string, 0, budget)
	for i := m.keymap.scroll; i < len(visible) && len(lines) < budget; i++ {
		entry := visible[i]
		if entry.divider {
			lines = append(lines, dimStyle.Render("  "+entry.action))
			continue
		}
		keyText := fmt.Sprintf("%-*s", maxKeyW, entry.key)
		action := entry.action
		if m.keymap.search != "" && i < len(matches) {
			keyText = highlightMatch(keyText, matches[i].keyIdx)
			action = highlightMatch(action, matches[i].actionIdx)
		}
		keyPill := helpKeyStyle.Render(" " + keyText + " ")
		line := keyPill + " " + action
		if m.keymap.searching {
			lines = append(lines, dimStyle.Render("  "+line))
		} else {
			lines = append(lines, cursorLine(line, i == m.keymap.cursor))
		}
	}
	return strings.Join(padLines(lines, budget, len(lines)), "\n")
}
