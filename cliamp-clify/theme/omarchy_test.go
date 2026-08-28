package theme

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseOmarchyCatppuccinShape(t *testing.T) {
	input := `mode = "dark"
accent = "#89b4fa"
foreground = "#cdd6f4"
bright_foreground = "#cdd6f4"
green = "#a6e3a1"
yellow = "#f9e2af"
red = "#f38ba8"
`
	th, ok := ParseOmarchy(strings.NewReader(input))
	if !ok {
		t.Fatal("ParseOmarchy() = false, want true")
	}
	if th.Name != OmarchyName {
		t.Errorf("Name = %q, want %q", th.Name, OmarchyName)
	}
	if th.Accent != "#89b4fa" {
		t.Errorf("Accent = %q, want #89b4fa", th.Accent)
	}
	if th.Green != "#a6e3a1" || th.Yellow != "#f9e2af" || th.Red != "#f38ba8" {
		t.Fatalf("spectrum colors = green %q yellow %q red %q", th.Green, th.Yellow, th.Red)
	}
}

func TestParseOmarchyLegacyColorSlots(t *testing.T) {
	input := `accent = "#658594"
foreground = "#c5c9c5"
color2 = "#8a9a7b"
color3 = "#c4b28a"
color1 = "#c4746e"
`
	th, ok := ParseOmarchy(strings.NewReader(input))
	if !ok {
		t.Fatal("ParseOmarchy() = false, want true")
	}
	if th.Green != "#8a9a7b" || th.Yellow != "#c4b28a" || th.Red != "#c4746e" {
		t.Fatalf("legacy spectrum colors = green %q yellow %q red %q", th.Green, th.Yellow, th.Red)
	}
}

func TestParseOmarchyIncomplete(t *testing.T) {
	if _, ok := ParseOmarchy(strings.NewReader(`accent = "#112233"`)); ok {
		t.Fatal("ParseOmarchy() accepted incomplete palette")
	}
}

func TestLoadOmarchyFromHome(t *testing.T) {
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
	path := filepath.Join(dir, "colors.toml")
	if err := os.WriteFile(path, []byte(colors), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	th, ok := LoadOmarchy()
	if !ok {
		t.Fatal("LoadOmarchy() = false, want true")
	}
	if th.Green != "#a6e3a1" {
		t.Errorf("Green = %q, want #a6e3a1", th.Green)
	}

	mod, err := OmarchyModTime()
	if err != nil {
		t.Fatalf("OmarchyModTime: %v", err)
	}
	if mod.IsZero() {
		t.Fatal("OmarchyModTime returned zero time")
	}
}

func TestLoadAllIncludesOmarchyWhenPresent(t *testing.T) {
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

	var found bool
	for _, th := range LoadAll() {
		if th.Name == OmarchyName {
			found = true
			if th.Green != "#a6e3a1" {
				t.Errorf("omarchy Green = %q, want #a6e3a1", th.Green)
			}
		}
	}
	if !found {
		t.Fatal("LoadAll() missing omarchy theme")
	}
}

func TestLoadOmarchyMissingFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if _, ok := LoadOmarchy(); ok {
		t.Fatal("LoadOmarchy() = true with no colors.toml")
	}
}

func TestColorsPathUsesHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	want := filepath.Join(home, ".config", "omarchy", "current", "theme", "colors.toml")
	if got := ColorsPath(); got != want {
		t.Errorf("ColorsPath() = %q, want %q", got, want)
	}
}

func TestOmarchyModTimeMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if _, err := OmarchyModTime(); err == nil {
		t.Fatal("OmarchyModTime() error = nil, want missing file error")
	}
}

func TestParseOmarchyPrefersNamedSpectrumColors(t *testing.T) {
	input := `accent = "#111111"
bright_foreground = "#222222"
foreground = "#333333"
green = "#44aa55"
yellow = "#ddcc44"
red = "#cc4455"
color2 = "#000001"
color3 = "#000002"
color1 = "#000003"
`
	th, ok := ParseOmarchy(strings.NewReader(input))
	if !ok {
		t.Fatal("ParseOmarchy() = false, want true")
	}
	if th.Green != "#44aa55" || th.Yellow != "#ddcc44" || th.Red != "#cc4455" {
		t.Fatalf("named spectrum colors not preferred: green %q yellow %q red %q", th.Green, th.Yellow, th.Red)
	}
}

func TestOmarchyModTimeUpdatesAfterWrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".config", "omarchy", "current", "theme")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "colors.toml")
	if err := os.WriteFile(path, []byte(`accent = "#111111"
bright_foreground = "#222222"
foreground = "#333333"
green = "#44aa55"
yellow = "#ddcc44"
red = "#cc4455"
`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	before, err := OmarchyModTime()
	if err != nil {
		t.Fatalf("OmarchyModTime: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(path, []byte(`accent = "#111111"
bright_foreground = "#222222"
foreground = "#333333"
green = "#55bb66"
yellow = "#ddcc44"
red = "#cc4455"
`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	after, err := OmarchyModTime()
	if err != nil {
		t.Fatalf("OmarchyModTime after write: %v", err)
	}
	if !after.After(before) {
		t.Fatalf("mod time did not advance: before=%v after=%v", before, after)
	}
}
