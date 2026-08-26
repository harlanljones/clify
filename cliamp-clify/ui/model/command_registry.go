package model

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/bjarneo/cliamp/ui"
)

// commandMode identifies the UI contexts in which a command is available.
// A command may belong to more than one context.
type commandMode uint64

const (
	commandModeMain commandMode = 1 << iota
	commandModeProvider
	commandModeEQ
	commandModeSpeed
	commandModeProviderPill
	commandModeKeymap
	commandModeKeymapSearch
	commandModeFileBrowser
	commandModeFileBrowserSearch
	commandModeNavBrowser
	commandModeNavSearch
	commandModePlaylistManager
	commandModePlaylistManagerInput
	commandModePlaylistPicker
	commandModePlaylistPickerInput
	commandModeQueue
	commandModeSearch
	commandModeNetSearch
	commandModeSpotSearch
	commandModeJump
	commandModeURL
	commandModeLyrics
	commandModeThemePicker
	commandModeVisPicker
	commandModeDevicePicker
	commandModeInfo
	commandModeThemePickerFilter
	commandModeVisPickerFilter
	commandModeProviderSearch
	commandModeDJ
)

const commandModeAny = ^commandMode(0)

// commandSection groups commands in the keymap help screen. Sections are
// emitted in a fixed order so the categorized help is stable and scannable.
type commandSection string

const (
	sectionPlayback   commandSection = "Playback"
	sectionNavigation commandSection = "Navigation"
	sectionPlaylist   commandSection = "Playlist & Queue"
	sectionProviders  commandSection = "Providers & Source"
	sectionSearch     commandSection = "Search & Filter"
	sectionVisuals    commandSection = "EQ & Visuals"
	sectionGeneral    commandSection = "General"
	// sectionFork marks the clify fork-only commands (the optional DJ engine).
	sectionFork commandSection = "DJ mode (clify fork)"
)

// orderedSections is the display order for categorized keymap sections. The
// fork section is listed last so legacy cliamp bindings stay prominent.
var orderedSections = []commandSection{
	sectionPlayback,
	sectionNavigation,
	sectionPlaylist,
	sectionProviders,
	sectionSearch,
	sectionVisuals,
	sectionGeneral,
	sectionFork,
}

// commandSpec is the single source of metadata for in-app commands. Dispatch
// remains in the focused handlers, while keymap, plugin reservations, and help
// all consume this description.
type commandSpec struct {
	Mode        commandMode
	Keys        []string // Bubbletea KeyPressMsg.String values.
	KeyLabel    string   // Human-readable key label.
	Label       string
	Section     commandSection // categorized group in the keymap help; "" = context-only
	Fork        bool           // true = clify fork-only command (DJ engine)
	Enabled     func(Model) bool
	Destructive bool
	Keymap      bool
	ContextHelp bool
	Primary     bool
	Cancel      bool
	Help        bool
}

func (c commandSpec) enabled(m Model) bool {
	return c.Enabled == nil || c.Enabled(m)
}

// commandRegistry deliberately lists every core-reserved key, including text
// editor keys that are not shown in the global keymap. Keep key labels in this
// table so the keymap cannot drift from plugin key reservations.
var commandRegistry = []commandSpec{
	{Mode: commandModeMain | commandModeEQ | commandModeSpeed, Keys: []string{"space"}, KeyLabel: "Space", Label: "Play / Pause", Section: sectionPlayback, Keymap: true, ContextHelp: true, Primary: true},
	{Mode: commandModeMain, Keys: []string{"s"}, KeyLabel: "s", Label: "Stop", Section: sectionPlayback, Keymap: true},
	{Mode: commandModeMain, Keys: []string{">", "."}, KeyLabel: "> .", Label: "Next track", Section: sectionPlayback, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"<", ","}, KeyLabel: "< ,", Label: "Previous track", Section: sectionPlayback, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"left", "right"}, KeyLabel: "Left Right", Label: "Seek +/-5s", Section: sectionPlayback, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"shift+left", "shift+right"}, KeyLabel: "Shift+Left Right", Label: "Seek +/-large step", Section: sectionPlayback, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "j"}, KeyLabel: "Nj", Label: "Seek to N x 10% of track (e.g. 7j = 70%)", Section: sectionPlayback, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"+", "=", "-"}, KeyLabel: "+ -", Label: "Volume up/down", Section: sectionPlayback, Keymap: true},
	{Mode: commandModeMain | commandModeSpeed, Keys: []string{"]", "["}, KeyLabel: "] [", Label: "Speed up/down (+/-0.25x)", Section: sectionPlayback, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"z"}, KeyLabel: "z", Label: "Toggle shuffle", Section: sectionPlaylist, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"r"}, KeyLabel: "r", Label: "Cycle repeat", Section: sectionPlaylist, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"m"}, KeyLabel: "m", Label: "Toggle mono", Section: sectionPlayback, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"e"}, KeyLabel: "e", Label: "Cycle EQ preset", Section: sectionVisuals, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"t"}, KeyLabel: "t", Label: "Choose theme", Section: sectionVisuals, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"v"}, KeyLabel: "v", Label: "Cycle visualizer", Section: sectionVisuals, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"ctrl+v"}, KeyLabel: "Ctrl+V", Label: "Choose visualizer", Section: sectionVisuals, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"V"}, KeyLabel: "V", Label: "Full-screen visualizer", Section: sectionVisuals, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"up", "down", "k", "j"}, KeyLabel: "Up Down", Label: "Playlist scroll / EQ adjust (wraps around)", Section: sectionNavigation, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"pgup", "pgdown", "ctrl+u", "ctrl+d"}, KeyLabel: "PgUp PgDn / Ctrl+U D", Label: "Scroll playlist/browser by page", Section: sectionNavigation, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"home", "end", "g", "G"}, KeyLabel: "Home End / g G", Label: "Go to top/end of playlist/browser", Section: sectionNavigation, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"shift+up", "shift+down"}, KeyLabel: "Shift+Up Down", Label: "Move track up/down", Section: sectionNavigation, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"h", "l"}, KeyLabel: "h l", Label: "EQ cursor left/right", Section: sectionVisuals, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"enter"}, KeyLabel: "Enter", Label: "Play selected track", Section: sectionPlaylist, Keymap: true, ContextHelp: true, Primary: true},
	{Mode: commandModeMain, Keys: []string{"a"}, KeyLabel: "a", Label: "Toggle queue (play next)", Section: sectionPlaylist, Keymap: true, ContextHelp: true},
	{Mode: commandModeMain, Keys: []string{"A"}, KeyLabel: "A", Label: "Queue manager", Section: sectionPlaylist, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"x"}, KeyLabel: "x", Label: "Remove selected track from playlist", Section: sectionPlaylist, Destructive: true, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"w"}, KeyLabel: "w", Label: "Write selected track/selection to playlist", Section: sectionPlaylist, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"o"}, KeyLabel: "o", Label: "Open file browser", Section: sectionProviders, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"N"}, KeyLabel: "N", Label: "Provider browser", Section: sectionProviders, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"L"}, KeyLabel: "L", Label: "Browse local playlists", Section: sectionProviders, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"R"}, KeyLabel: "R", Label: "Open radio provider", Section: sectionProviders, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"S"}, KeyLabel: "S", Label: "Open Spotify provider", Section: sectionProviders, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"P"}, KeyLabel: "P", Label: "Open Plex provider", Section: sectionProviders, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"Y"}, KeyLabel: "Y", Label: "Open YouTube provider", Section: sectionProviders, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"C"}, KeyLabel: "C", Label: "Open SoundCloud provider", Section: sectionProviders, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"M"}, KeyLabel: "M", Label: "Open NetEase provider", Section: sectionProviders, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"J"}, KeyLabel: "J", Label: "Open Jellyfin provider", Section: sectionProviders, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"E"}, KeyLabel: "E", Label: "Open Emby provider", Section: sectionProviders, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"Q"}, KeyLabel: "Q", Label: "Open Qobuz provider", Section: sectionProviders, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"ctrl+j"}, KeyLabel: "Ctrl+J", Label: "Jump to time", Section: sectionPlayback, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"p"}, KeyLabel: "p", Label: "Playlist manager", Section: sectionPlaylist, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"ctrl+h"}, KeyLabel: "Ctrl+H", Label: "Toggle album headers", Section: sectionNavigation, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"i"}, KeyLabel: "i", Label: "Track info / metadata", Section: sectionPlayback, Keymap: true, ContextHelp: true},
	{Mode: commandModeMain, Keys: []string{"ctrl+s"}, KeyLabel: "Ctrl+S", Label: "Save/download track to ~/Music/cliamp", Section: sectionPlayback, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"ctrl+x"}, KeyLabel: "Ctrl+X", Label: "Expand/collapse view", Section: sectionNavigation, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"ctrl+x"}, KeyLabel: "Ctrl+X", Label: "Expand", Section: sectionNavigation, Enabled: func(m Model) bool {
		return !m.heightExpanded && m.layout.bodyRows > m.plVisible
	}},
	{Mode: commandModeMain, Keys: []string{"/"}, KeyLabel: "/", Label: "Filter/search list", Section: sectionSearch, Keymap: true, ContextHelp: true},
	{Mode: commandModeMain, Keys: []string{"f"}, KeyLabel: "f", Label: "Toggle bookmark/favorite", Section: sectionPlaylist, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"ctrl+f"}, KeyLabel: "Ctrl+F", Label: "Search active provider or YouTube", Section: sectionSearch, Keymap: true, ContextHelp: true},
	{Mode: commandModeMain, Keys: []string{"u"}, KeyLabel: "u", Label: "Load URL (stream/playlist)", Section: sectionPlaylist, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"d"}, KeyLabel: "d", Label: "Audio device picker", Section: sectionProviders, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"D"}, KeyLabel: "D", Label: "DJ mode", Section: sectionFork, Fork: true, Keymap: true, ContextHelp: true},
	{Mode: commandModeMain, Keys: []string{"y"}, KeyLabel: "y", Label: "Show lyrics", Section: sectionPlayback, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"tab"}, KeyLabel: "Tab", Label: "Toggle focus", Section: sectionNavigation, Keymap: true},
	{Mode: commandModeMain, Keys: []string{"esc", "backspace", "b"}, KeyLabel: "Esc", Label: "Back to provider", Section: sectionNavigation, Keymap: true, ContextHelp: true, Cancel: true},
	{Mode: commandModeAny, Keys: []string{"ctrl+k"}, KeyLabel: "Ctrl+K", Label: "Help", Section: sectionGeneral, Keymap: true, ContextHelp: true, Help: true},
	{Mode: commandModeMain, Keys: []string{"?"}, KeyLabel: "?", Label: "Help", Section: sectionGeneral, Keymap: true},
	{Mode: commandModeAny, Keys: []string{"ctrl+c", "q"}, KeyLabel: "q", Label: "Quit", Section: sectionGeneral, Keymap: true},
	{Mode: commandModeAny, Keys: []string{"ctrl+z"}, KeyLabel: "Ctrl+Z", Label: "Undo latest playlist or queue mutation", Section: sectionGeneral},
	{Mode: commandModeProvider, Keys: []string{"ctrl+r"}, KeyLabel: "Ctrl+R", Label: "Refresh provider", Section: sectionProviders},

	// Shared text editing is reserved even though these are intentionally absent
	// from the global keymap, where they would be misleading outside a field.
	{Mode: commandModeKeymapSearch | commandModeFileBrowserSearch | commandModeNavSearch | commandModePlaylistManagerInput | commandModePlaylistPickerInput | commandModeSearch | commandModeNetSearch | commandModeSpotSearch | commandModeJump | commandModeURL | commandModeThemePickerFilter | commandModeVisPickerFilter | commandModeProviderSearch, Keys: []string{"left", "right", "home", "end", "ctrl+a", "ctrl+e", "backspace", "delete", "ctrl+w", "ctrl+u"}, KeyLabel: "Text editor", Label: "Move cursor and delete text"},

	{Mode: commandModeProvider, Keys: []string{"enter"}, KeyLabel: "Enter", Label: "Load", ContextHelp: true, Primary: true},
	{Mode: commandModeProvider, Keys: []string{"esc", "backspace", "b"}, KeyLabel: "Esc", Label: "Back", ContextHelp: true, Cancel: true},
	{Mode: commandModeEQ, Keys: []string{"up", "down"}, KeyLabel: "Up Down", Label: "Gain", ContextHelp: true},
	{Mode: commandModeSpeed, Keys: []string{"left", "right"}, KeyLabel: "Left Right", Label: "Speed", ContextHelp: true},
	{Mode: commandModeProviderPill, Keys: []string{"enter"}, KeyLabel: "Enter", Label: "Open", ContextHelp: true, Primary: true},
	{Mode: commandModeProviderPill, Keys: []string{"esc", "backspace"}, KeyLabel: "Esc", Label: "Back", ContextHelp: true, Cancel: true},
	{Mode: commandModeKeymap | commandModeFileBrowser | commandModeNavBrowser | commandModePlaylistManager | commandModePlaylistPicker | commandModeQueue | commandModeDevicePicker, Keys: []string{"esc"}, KeyLabel: "Esc", Label: "Back", ContextHelp: true, Cancel: true},
	{Mode: commandModeKeymapSearch | commandModeFileBrowserSearch | commandModeNavSearch | commandModePlaylistManagerInput | commandModePlaylistPickerInput | commandModeSearch | commandModeNetSearch | commandModeSpotSearch | commandModeJump | commandModeURL | commandModeProviderSearch, Keys: []string{"esc"}, KeyLabel: "Esc", Label: "Cancel", ContextHelp: true, Cancel: true},
	{Mode: commandModeKeymapSearch | commandModeFileBrowserSearch | commandModeNavSearch | commandModePlaylistManagerInput | commandModePlaylistPickerInput | commandModeSearch | commandModeNetSearch | commandModeSpotSearch | commandModeJump | commandModeURL | commandModeProviderSearch, Keys: []string{"enter"}, KeyLabel: "Enter", Label: "Confirm", ContextHelp: true, Primary: true},
	{Mode: commandModeNavBrowser | commandModePlaylistManager | commandModePlaylistPicker | commandModeQueue | commandModeDevicePicker | commandModeProviderSearch, Keys: []string{"up", "down", "k", "j"}, KeyLabel: "Up Down", Label: "Navigate", ContextHelp: true},
	{Mode: commandModeFileBrowser | commandModeNavBrowser | commandModePlaylistManager | commandModePlaylistPicker | commandModeDevicePicker, Keys: []string{"enter"}, KeyLabel: "Enter", Label: "Select", ContextHelp: true, Primary: true},
	{Mode: commandModeKeymap, Keys: []string{"/"}, KeyLabel: "/", Label: "Filter", ContextHelp: true, Primary: true},
	{Mode: commandModeThemePicker, Keys: []string{"esc"}, KeyLabel: "Esc", Label: "Cancel preview", ContextHelp: true, Cancel: true},
	{Mode: commandModeThemePicker, Keys: []string{"enter"}, KeyLabel: "Enter", Label: "Apply theme", ContextHelp: true, Primary: true},
	{Mode: commandModeThemePicker, Keys: []string{"up", "down", "k", "j"}, KeyLabel: "Up Down", Label: "Preview", ContextHelp: true},
	{Mode: commandModeThemePicker, Keys: []string{"/"}, KeyLabel: "/", Label: "Filter", ContextHelp: true},
	{Mode: commandModeVisPicker, Keys: []string{"esc"}, KeyLabel: "Esc", Label: "Cancel preview", ContextHelp: true, Cancel: true},
	{Mode: commandModeVisPicker, Keys: []string{"enter"}, KeyLabel: "Enter", Label: "Save", ContextHelp: true, Primary: true},
	{Mode: commandModeVisPicker, Keys: []string{"up", "down", "k", "j"}, KeyLabel: "Up Down", Label: "Preview", ContextHelp: true},
	{Mode: commandModeVisPicker, Keys: []string{"/"}, KeyLabel: "/", Label: "Filter", ContextHelp: true},
	{Mode: commandModeThemePickerFilter | commandModeVisPickerFilter, Keys: []string{"esc"}, KeyLabel: "Esc", Label: "Cancel filter", ContextHelp: true, Cancel: true},
	{Mode: commandModeThemePickerFilter | commandModeVisPickerFilter, Keys: []string{"enter"}, KeyLabel: "Enter", Label: "Finish filter", ContextHelp: true, Primary: true},
	{Mode: commandModeQueue, Keys: []string{"d"}, KeyLabel: "d", Label: "Remove", Destructive: true, ContextHelp: true, Primary: true, Enabled: func(m Model) bool { return m.playlist != nil && m.playlist.QueueLen() > 0 }},
	{Mode: commandModeQueue, Keys: []string{"c"}, KeyLabel: "c", Label: "Clear", Destructive: true, ContextHelp: true},
	{Mode: commandModeFileBrowser, Keys: []string{"R"}, KeyLabel: "R", Label: "Replace queue", Destructive: true, ContextHelp: true},
	{Mode: commandModeNavBrowser, Keys: []string{"R"}, KeyLabel: "R", Label: "Replace queue", Destructive: true, ContextHelp: true},
	{Mode: commandModeLyrics, Keys: []string{"r"}, KeyLabel: "r", Label: "Retry", ContextHelp: true, Primary: true, Enabled: func(m Model) bool { return !m.lyrics.loading && (m.lyrics.err != nil || len(m.lyrics.lines) == 0) }},
	{Mode: commandModeLyrics, Keys: []string{"esc"}, KeyLabel: "Esc", Label: "Close", ContextHelp: true, Cancel: true},
	{Mode: commandModeInfo, Keys: []string{"esc"}, KeyLabel: "Esc", Label: "Close", ContextHelp: true, Cancel: true},
	{Mode: commandModeDJ, Keys: []string{"esc", "D"}, KeyLabel: "Esc / D", Label: "Exit DJ mode", Section: sectionFork, Fork: true, ContextHelp: true, Cancel: true},
	{Mode: commandModeDJ, Keys: []string{"1", "2"}, KeyLabel: "1 / 2", Label: "Focus deck A / B", Section: sectionFork, Fork: true, ContextHelp: true},
	{Mode: commandModeDJ, Keys: []string{"left", "right", "\\"}, KeyLabel: "Left Right \\", Label: "Crossfader", Section: sectionFork, Fork: true, ContextHelp: true},
	{Mode: commandModeDJ, Keys: []string{"[", "]"}, KeyLabel: "[ ]", Label: "Pitch nudge", Section: sectionFork, Fork: true, ContextHelp: true},
	{Mode: commandModeDJ, Keys: []string{"s"}, KeyLabel: "s", Label: "Sync focused deck", Section: sectionFork, Fork: true, ContextHelp: true},
	{Mode: commandModeDJ, Keys: []string{"c"}, KeyLabel: "c", Label: "Center crossfader", Section: sectionFork, Fork: true, ContextHelp: true},
}

func (m Model) commandHelp(mode commandMode) string {
	var cancel, primary, help, optional []commandSpec
	for _, command := range commandRegistry {
		if !command.ContextHelp || command.Mode&mode == 0 || !command.enabled(m) {
			continue
		}
		switch {
		case command.Cancel:
			cancel = append(cancel, command)
		case command.Primary:
			primary = append(primary, command)
		case command.Help:
			help = append(help, command)
		default:
			optional = append(optional, command)
		}
	}

	// Back/cancel, the primary action, and help are the only mandatory hints.
	// Keep them first so narrow terminals drop optional navigation rather than
	// trapping the user in an unfamiliar overlay.
	var ordered []commandSpec
	if len(cancel) > 0 {
		ordered = append(ordered, cancel[0])
	}
	if len(primary) > 0 {
		ordered = append(ordered, primary[0])
	}
	if len(help) > 0 {
		ordered = append(ordered, help[0])
	}
	ordered = append(ordered, optional...)
	return renderCommandHelp(ordered, m.helpWidth())
}

func (m Model) helpWidth() int {
	if ui.PanelWidth > 0 {
		return ui.PanelWidth
	}
	if m.width > 0 {
		return m.width
	}
	return 80
}

func renderCommandHelp(commands []commandSpec, width int) string {
	if width <= 0 {
		return ""
	}
	narrow := width < 48
	var b strings.Builder
	for _, command := range commands {
		label := command.Label
		if narrow {
			switch {
			case command.Cancel:
				label = "Back"
			case command.Primary:
				label = "Go"
			case command.Help:
				label = "Help"
			}
		}
		hint := helpKey(command.KeyLabel, label)
		if b.Len() > 0 {
			hint = " " + hint
		}
		if lipgloss.Width(b.String()+hint) > width {
			break
		}
		b.WriteString(hint)
	}
	return b.String()
}
