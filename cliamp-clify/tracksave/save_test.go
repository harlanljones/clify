package tracksave

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bjarneo/cliamp/playlist"
)

func TestSaveCopiesTemporaryDownload(t *testing.T) {
	home := setTestHome(t)
	source, err := os.CreateTemp("", "cliamp-save-*.flac")
	if err != nil {
		t.Fatal(err)
	}
	sourcePath := source.Name()
	t.Cleanup(func() { _ = os.Remove(sourcePath) })
	if _, err := source.WriteString("audio"); err != nil {
		t.Fatal(err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}

	destination, err := Save(playlist.Track{Path: sourcePath, Title: "Song", Artist: "Artist"})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "Music", "cliamp", "Artist - Song.flac")
	if destination != want {
		t.Fatalf("destination = %q, want %q", destination, want)
	}
	if data, err := os.ReadFile(destination); err != nil || string(data) != "audio" {
		t.Fatalf("saved data = %q, err=%v", data, err)
	}
}

func TestSaveRejectsUserLibraryFile(t *testing.T) {
	setTestHome(t)
	if _, err := Save(playlist.Track{Path: "/var/lib/music/song.flac"}); err == nil {
		t.Fatal("Save accepted a non-temporary library file")
	}
}

func setTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}
