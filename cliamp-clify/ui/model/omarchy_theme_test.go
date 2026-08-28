package model

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bjarneo/cliamp/theme"
)

func TestEnableOmarchySyncEnablesWatcher(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".config", "omarchy", "current", "theme")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	colors := `accent = "#89b4fa"
bright_foreground = "#cdd6f4"
foreground = "#6c7086"
green = "#a6e3a1"
yellow = "#f9e2af"
red = "#f38ba8"
`
	if err := os.WriteFile(filepath.Join(dir, "colors.toml"), []byte(colors), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	th, ok := theme.LoadOmarchy()
	if !ok {
		t.Fatal("LoadOmarchy() = false")
	}

	m := Model{themes: []theme.Theme{th}}
	m.EnableOmarchySync(th)

	if !m.omarchySync {
		t.Fatal("omarchySync = false, want true")
	}
	if m.themeIdx != 0 {
		t.Fatalf("themeIdx = %d, want 0", m.themeIdx)
	}
	if m.omarchyMtime.IsZero() {
		t.Fatal("omarchyMtime is zero after EnableOmarchySync")
	}
}

func TestMaybeSyncOmarchyThemeReloadsOnChange(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".config", "omarchy", "current", "theme")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "colors.toml")
	writeColors := func(green string) {
		t.Helper()
		body := `accent = "#89b4fa"
bright_foreground = "#cdd6f4"
foreground = "#6c7086"
green = "` + green + `"
yellow = "#f9e2af"
red = "#f38ba8"
`
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	writeColors("#a6e3a1")

	th, ok := theme.LoadOmarchy()
	if !ok {
		t.Fatal("LoadOmarchy() = false")
	}

	m := Model{themes: []theme.Theme{th}}
	m.EnableOmarchySync(th)
	before := m.omarchyMtime

	time.Sleep(10 * time.Millisecond)
	writeColors("#55bb66")

	m.omarchyCheck = 2 * time.Second
	m.maybeSyncOmarchyTheme(0)

	if !m.omarchyMtime.After(before) {
		t.Fatalf("omarchyMtime did not advance: before=%v after=%v", before, m.omarchyMtime)
	}
	reloaded, ok := theme.LoadOmarchy()
	if !ok || reloaded.Green != "#55bb66" {
		t.Fatalf("reloaded green = %q, want #55bb66", reloaded.Green)
	}
}

func TestSetThemeOmarchyEnablesSync(t *testing.T) {
	th := theme.Theme{
		Name:     theme.OmarchyName,
		Accent:   "#111111",
		BrightFG: "#222222",
		FG:       "#333333",
		Green:    "#44aa55",
		Yellow:   "#ddcc44",
		Red:      "#cc4455",
	}
	m := Model{themes: []theme.Theme{th}}
	if !m.SetTheme(theme.OmarchyName) {
		t.Fatal("SetTheme(omarchy) = false")
	}
	if !m.omarchySync {
		t.Fatal("omarchySync = false after SetTheme(omarchy)")
	}
}

func TestSetThemeDefaultDisablesOmarchySync(t *testing.T) {
	m := Model{omarchySync: true}
	if !m.SetTheme("default") {
		t.Fatal("SetTheme(default) = false")
	}
	if m.omarchySync {
		t.Fatal("omarchySync = true after SetTheme(default)")
	}
}
