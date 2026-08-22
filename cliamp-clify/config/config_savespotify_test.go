package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveSpotifyKey(t *testing.T) {
	cases := []struct {
		name        string
		initial     string
		key         string
		value       string
		wantParts   []string // substrings required, in order
		wantMissing []string
	}{
		{
			name:      "creates config with section and key",
			initial:   "",
			key:       "client_id",
			value:     "abc123",
			wantParts: []string{"[spotify]", `client_id = "abc123"`},
		},
		{
			name: "replaces existing key in place",
			initial: "# top comment\n" +
				"[spotify]\n" +
				"bitrate = 320\n" +
				`client_id = "stale"` + "\n",
			key:         "client_id",
			value:       "fresh",
			wantParts:   []string{"# top comment", "[spotify]", "bitrate = 320", `client_id = "fresh"`},
			wantMissing: []string{`"stale"`},
		},
		{
			name: "appends key into existing section",
			initial: "[spotify]\n" +
				"bitrate = 160\n" +
				"\n[other]\n" +
				"key = 1\n",
			key:       "client_id",
			value:     "xyz",
			wantParts: []string{"[spotify]", "bitrate = 160", `client_id = "xyz"`, "[other]"},
		},
		{
			name:      "appends section when absent",
			initial:   "[other]\nkey = 1\n",
			key:       "client_id",
			value:     "xyz",
			wantParts: []string{"[other]", "key = 1", "[spotify]", `client_id = "xyz"`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("CLIAMP_CONFIG_DIR", dir)
			path := filepath.Join(dir, "config.toml")
			if tc.initial != "" {
				if err := os.WriteFile(path, []byte(tc.initial), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			if err := SaveSpotifyKey(tc.key, tc.value); err != nil {
				t.Fatalf("SaveSpotifyKey(%q, %q): %v", tc.key, tc.value, err)
			}

			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			got := string(data)

			pos := -1
			for _, want := range tc.wantParts {
				next := strings.Index(got[pos+1:], want)
				if next < 0 {
					t.Fatalf("config %q missing %q in order:\n%s", got, want, got)
				}
				pos += next + 1
			}
			for _, banned := range tc.wantMissing {
				if strings.Contains(got, banned) {
					t.Fatalf("config still contains %q:\n%s", banned, got)
				}
			}
		})
	}
}
