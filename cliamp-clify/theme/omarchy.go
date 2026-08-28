package theme

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// OmarchyName is the display name for the live Omarchy desktop theme.
const OmarchyName = "omarchy"

// ColorsPath returns the active Omarchy theme colors file, or "" when HOME is unset.
func ColorsPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "omarchy", "current", "theme", "colors.toml")
}

// LoadOmarchy reads ~/.config/omarchy/current/theme/colors.toml and maps it to
// the cliamp six-color palette. Returns false when the file is missing or does
// not contain a complete palette.
func LoadOmarchy() (Theme, bool) {
	path := ColorsPath()
	if path == "" {
		return Theme{}, false
	}
	f, err := os.Open(path)
	if err != nil {
		return Theme{}, false
	}
	defer f.Close()

	t, ok := ParseOmarchy(f)
	if !ok {
		return Theme{}, false
	}
	return t, true
}

// OmarchyModTime returns the modification time of the active Omarchy colors file.
func OmarchyModTime() (time.Time, error) {
	path := ColorsPath()
	if path == "" {
		return time.Time{}, os.ErrNotExist
	}
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

// ParseOmarchy maps an Omarchy colors.toml stream onto cliamp's Theme palette.
func ParseOmarchy(r io.Reader) (Theme, bool) {
	colors := parseColorMap(r)
	t := Theme{
		Name:     OmarchyName,
		Accent:   firstColor(colors, "accent", "blue", "bright_blue"),
		BrightFG: firstColor(colors, "bright_foreground", "bright_fg", "foreground", "light_foreground"),
		FG:       firstColor(colors, "foreground", "fg", "dark_foreground", "muted"),
		Green:    firstColor(colors, "green", "bright_green", "color2"),
		Yellow:   firstColor(colors, "yellow", "bright_yellow", "color3"),
		Red:      firstColor(colors, "red", "bright_red", "color1"),
	}
	if err := t.Validate(); err != nil {
		return Theme{}, false
	}
	return t, true
}

func parseColorMap(r io.Reader) map[string]string {
	colors := make(map[string]string)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		if key != "" && val != "" {
			colors[key] = val
		}
	}
	return colors
}

func firstColor(colors map[string]string, keys ...string) string {
	for _, key := range keys {
		if val := colors[key]; val != "" {
			return val
		}
	}
	return ""
}
